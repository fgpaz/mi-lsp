param(
    [int]$IssueNumber,
    [string]$IssueUrl,
    [string]$WaiverReason,
    [string[]]$ExpectedScope,
    [string[]]$TraceabilityEvidence,
    [string[]]$SharedSkillName,
    [switch]$DryRun,
    [switch]$Json,
    [ValidateSet("solo", "governed")]
    [string]$CollaborationMode = "governed"
)

$ErrorActionPreference = "Stop"

function Invoke-Git {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & git @Args 2>&1
        $exit = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    return [pscustomobject]@{
        ExitCode = $exit
        Output = @($output)
    }
}

function Add-Unique {
    param([System.Collections.Generic.List[string]]$List, [string]$Value)
    if ($Value -and -not $List.Contains($Value)) {
        [void]$List.Add($Value)
    }
}

function Test-BranchIdentityStable {
    param(
        [int]$CapturedExitCode,
        [AllowEmptyString()][string]$CapturedRef,
        [int]$CurrentExitCode,
        [AllowEmptyString()][string]$CurrentRef
    )

    if ($CapturedExitCode -notin @(0, 1) -or $CurrentExitCode -notin @(0, 1)) {
        return $false
    }
    return $CapturedExitCode -eq $CurrentExitCode -and $CapturedRef -ceq $CurrentRef
}

function Resolve-SharedSkillRoot {
    param(
        [Parameter(Mandatory = $true)][string]$EnvironmentVariable,
        [AllowNull()][AllowEmptyString()][string]$DefaultRoot
    )

    $explicitRoot = [Environment]::GetEnvironmentVariable($EnvironmentVariable)
    if ([string]::IsNullOrWhiteSpace($explicitRoot)) {
        if ([string]::IsNullOrWhiteSpace($DefaultRoot)) {
            throw "No explicit or default shared-skill root is available for '$EnvironmentVariable'"
        }
        return $DefaultRoot
    }

    $resolvedRoot = [Environment]::ExpandEnvironmentVariables($explicitRoot.Trim())
    if (-not (Test-Path -LiteralPath $resolvedRoot -PathType Container)) {
        throw "Explicit shared-skill root '$EnvironmentVariable' was not found: $resolvedRoot"
    }
    return $resolvedRoot
}

function Invoke-GitRawText {
    param([string[]]$Arguments)

    $process = $null
    try {
        $gitPath = (Get-Command git -CommandType Application | Select-Object -First 1).Source
        $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
        $startInfo.FileName = $gitPath
        $startInfo.UseShellExecute = $false
        $startInfo.CreateNoWindow = $true
        $startInfo.RedirectStandardOutput = $true
        $startInfo.RedirectStandardError = $true
        $startInfo.StandardOutputEncoding = [System.Text.UTF8Encoding]::new($false)
        $startInfo.StandardErrorEncoding = [System.Text.UTF8Encoding]::new($false)
        foreach ($argument in $Arguments) {
            $startInfo.ArgumentList.Add([string]$argument)
        }

        $process = [System.Diagnostics.Process]::new()
        $process.StartInfo = $startInfo
        if (-not $process.Start()) {
            return [pscustomobject]@{ ExitCode = -1; OutputText = ""; ErrorText = "git process did not start" }
        }
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        return [pscustomobject]@{
            ExitCode = [int]$process.ExitCode
            OutputText = $stdoutTask.GetAwaiter().GetResult()
            ErrorText = $stderrTask.GetAwaiter().GetResult()
        }
    }
    catch {
        return [pscustomobject]@{ ExitCode = -1; OutputText = ""; ErrorText = "git raw invocation failed" }
    }
    finally {
        if ($process) {
            $process.Dispose()
        }
    }
}

function Split-NulTerminatedText {
    param([string]$Text)

    if ([string]::IsNullOrEmpty($Text)) {
        return @()
    }
    if ($Text[$Text.Length - 1] -ne [char]0) {
        throw "NUL-delimited git output is not terminated"
    }
    $tokens = $Text.Split([char[]]@([char]0), [System.StringSplitOptions]::None)
    if ($tokens.Length -lt 2 -or $tokens[$tokens.Length - 1] -ne "") {
        throw "NUL-delimited git output is malformed"
    }
    $records = [System.Collections.Generic.List[string]]::new()
    for ($index = 0; $index -lt $tokens.Length - 1; $index++) {
        if ($tokens[$index].Length -eq 0) {
            throw "NUL-delimited git output contains an empty field"
        }
        [void]$records.Add($tokens[$index])
    }
    return $records.ToArray()
}

function Convert-StatusPorcelainZ {
    param([string]$OutputText)

    $records = @(Split-NulTerminatedText $OutputText)
    $changes = [System.Collections.Generic.List[object]]::new()
    for ($index = 0; $index -lt $records.Count; ) {
        $record = $records[$index++]
        if ($record.Length -lt 4 -or $record[2] -ne ' ') {
            throw "porcelain status record is malformed"
        }
        $xy = $record.Substring(0, 2)
        $path = $record.Substring(3)
        if ($path.Length -eq 0) {
            throw "porcelain status path is empty"
        }
        $previousPath = $null
        if ($xy -match '[RC]') {
            if ($index -ge $records.Count) {
                throw "porcelain rename/copy record is missing its source path"
            }
            # In status -z, the destination is first and the source path follows.
            $previousPath = $records[$index++]
        }
        $kind = if ($xy -eq "??") {
            "untracked"
        } elseif ($xy[0] -ne ' ') {
            "staged"
        } elseif ($xy[1] -ne ' ') {
            "working"
        } else {
            "unknown"
        }
        [void]$changes.Add([pscustomobject]@{
            Kind = $kind
            Path = $path
            PreviousPath = $previousPath
            X = [string]$xy[0]
            Y = [string]$xy[1]
        })
    }
    return $changes.ToArray()
}

function Convert-NameStatusZ {
    param([string]$OutputText)

    $records = @(Split-NulTerminatedText $OutputText)
    $changes = [System.Collections.Generic.List[object]]::new()
    for ($index = 0; $index -lt $records.Count; ) {
        $status = $records[$index++]
        if ($status -notmatch '^(?:A|C[0-9]*|D|M|R[0-9]*|T|U|X|B)$') {
            throw "outgoing name-status record has an invalid status"
        }
        if ($index -ge $records.Count) {
            throw "outgoing name-status record is missing its path"
        }
        $firstPath = $records[$index++]
        if ($firstPath.Length -eq 0) {
            throw "outgoing name-status path is empty"
        }
        $path = $firstPath
        $previousPath = $null
        if ($status -match '^[RC]') {
            if ($index -ge $records.Count) {
                throw "outgoing rename/copy record is missing its second path"
            }
            $secondPath = $records[$index++]
            if ($secondPath.Length -eq 0) {
                throw "outgoing rename/copy path is empty"
            }
            # diff --name-status -z retains the diff order: source, then destination.
            $path = $secondPath
            $previousPath = $firstPath
        }
        [void]$changes.Add([pscustomobject]@{
            Kind = "outgoing"
            Status = $status
            Path = $path
            PreviousPath = $previousPath
            X = $status
            Y = ' '
        })
    }
    return $changes.ToArray()
}

function Get-InventoryPaths {
    param([Parameter(Mandatory = $true)][object]$Change)

    $paths = [System.Collections.Generic.List[string]]::new()
    if (-not [string]::IsNullOrEmpty([string]$Change.Path)) {
        [void]$paths.Add([string]$Change.Path)
    }
    if (-not [string]::IsNullOrEmpty([string]$Change.PreviousPath)) {
        [void]$paths.Add([string]$Change.PreviousPath)
    }
    return $paths.ToArray()
}

function Get-Surface {
    param([string]$Path)
    $p = $Path.Replace('\', '/')
    switch -Regex ($p) {
        '^infra/' { return 'git-tooling' }
        '^\.docs/raw/' { return 'raw-docs' }
        '^\.docs/planificacion/' { return 'evidence-docs' }
        '^\.docs/wiki/' { return 'canon-docs' }
        '^README\.md$' { return 'canon-docs' }
        '^internal/|^cmd/|^worker-dotnet/|^go\.mod$|^go\.sum$' { return 'backend' }
        '^scripts/' { return 'git-tooling' }
        '^skills/' { return 'shared-skill' }
        default { return 'unknown' }
    }
}

function Get-SecretScan {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Range,
        [switch]$SnapshotUnavailable
    )

    $patterns = [ordered]@{
        pem_private_key_header = '-----BEGIN(?: [A-Z0-9]+)* PRIVATE KEY-----'
        github_classic_token = '\bgh(?:p|o|u|s|r)_[A-Za-z0-9]{36}\b'
        github_fine_grained_token = '\bgithub_pat_[A-Za-z0-9_]{20,}\b'
        aws_access_key_id = '\b(?:AKIA|ASIA)[0-9A-Z]{16}\b'
        slack_token = '\bxox(?:[abcdprs]-|e\.xox[abcdprs]-)[A-Za-z0-9-]{10,}\b'
        stripe_live_secret = '\bsk_live_[A-Za-z0-9]{20,}\b'
        openai_project_token = '\bsk-proj-[A-Za-z0-9_-]{20,}\b'
    }
    $ruleCounts = [ordered]@{}
    foreach ($ruleId in $patterns.Keys) {
        $ruleCounts[$ruleId] = 0
    }

    $scanError = [bool]$SnapshotUnavailable
    $scannedAddedLines = 0
    try {
        if ($scanError) {
            throw "captured commit snapshot is unavailable"
        }
        $numstat = Invoke-Git diff --no-ext-diff --no-textconv --numstat $Range '--'
        if ($numstat.ExitCode -ne 0) {
            $scanError = $true
        }
        if (-not $scanError) {
            foreach ($rawLine in @($numstat.Output)) {
                $line = [string]$rawLine
                if ($line -match '^(?:git\s*:\s*)?(fatal|error|warning):') {
                    $scanError = $true
                    break
                }
                $columns = $line -split "`t", 3
                if (($columns.Count -ge 2 -and $columns[0] -eq '-' -and $columns[1] -eq '-') -or
                    $line -match '^-[ \t]+-[ \t]+') {
                    $scanError = $true
                    break
                }
            }
        }

        if (-not $scanError) {
            $diff = Invoke-Git diff --no-ext-diff --no-textconv --no-color --unified=0 $Range '--'
            if ($diff.ExitCode -ne 0) {
                $scanError = $true
            }
            if (-not $scanError) {
                foreach ($rawLine in @($diff.Output)) {
                    $line = [string]$rawLine
                    if ($line -match '^(?:git\s*:\s*)?(fatal|error|warning):' -or
                        $line -match '^Binary files .* differ$' -or
                        $line -eq 'GIT binary patch' -or
                        $line -match '^(literal|delta) [0-9]+$') {
                        $scanError = $true
                        break
                    }
                    if ($line.StartsWith('+') -and $line -notmatch '^\+\+\+ ') {
                        $scannedAddedLines++
                        $addedText = $line.Substring(1)
                        foreach ($ruleId in $patterns.Keys) {
                            $ruleMatches = [regex]::Matches(
                                $addedText,
                                [string]$patterns[$ruleId],
                                [System.Text.RegularExpressions.RegexOptions]::IgnoreCase
                            )
                            $ruleCounts[$ruleId] += $ruleMatches.Count
                        }
                    }
                }
            }
        }
    }
    catch {
        $scanError = $true
    }

    if ($scanError) {
        foreach ($ruleId in $patterns.Keys) {
            $ruleCounts[$ruleId] = 0
        }
        return [pscustomobject]@{
            status = 'error'
            range = $Range
            scanned_added_lines = 0
            match_count = 0
            rule_counts = $ruleCounts
        }
    }

    $matchCount = 0
    foreach ($ruleId in $patterns.Keys) {
        $matchCount += $ruleCounts[$ruleId]
    }
    return [pscustomobject]@{
        status = if ($matchCount -gt 0) { 'blocked' } else { 'passed' }
        range = $Range
        scanned_added_lines = $scannedAddedLines
        match_count = $matchCount
        rule_counts = $ruleCounts
    }
}

$blockers = [System.Collections.Generic.List[string]]::new()
$warnings = [System.Collections.Generic.List[string]]::new()
$sharedSkillMirrorChecks = @()

$rootResult = Invoke-Git rev-parse --show-toplevel
if ($rootResult.ExitCode -ne 0) {
    throw "Not a git repository"
}
$repoRoot = ($rootResult.Output | Select-Object -First 1).ToString().Trim()
Set-Location $repoRoot

$fetch = Invoke-Git fetch origin main
if ($fetch.ExitCode -ne 0) {
    Add-Unique $blockers "git fetch origin main failed"
}

# Capture branch identity and commit refs together after fetch.
$branchIdentityCapture = Invoke-Git symbolic-ref --quiet HEAD
$branchIdentityExitCode = $branchIdentityCapture.ExitCode
$branchRef = if ($branchIdentityExitCode -eq 0) {
    ([string]($branchIdentityCapture.Output | Select-Object -First 1)).Trim()
} else {
    ""
}
$branch = if ($branchRef.StartsWith("refs/heads/", [System.StringComparison]::Ordinal)) {
    $branchRef.Substring("refs/heads/".Length)
} else {
    ""
}
$branchIdentityAvailable = $branchIdentityExitCode -in @(0, 1)
if (-not $branchIdentityAvailable) {
    Add-Unique $blockers "could not capture symbolic branch identity after fetch"
}
if ($CollaborationMode -eq "governed" -and $branchIdentityExitCode -eq 1) {
    Add-Unique $blockers "governed mode requires an attached symbolic branch; detached HEAD cannot be approved"
}
if ($branchIdentityExitCode -eq 0 -and -not $branchRef.StartsWith("refs/heads/", [System.StringComparison]::Ordinal)) {
    $branchIdentityAvailable = $false
    Add-Unique $blockers "HEAD symbolic ref is not a local branch; cannot verify branch identity"
}

$baseCapture = Invoke-Git rev-parse --verify 'refs/remotes/origin/main^{commit}'
$headCapture = Invoke-Git rev-parse --verify 'HEAD^{commit}'
$baseSha = ([string]($baseCapture.Output | Select-Object -First 1)).Trim().ToLowerInvariant()
$headSha = ([string]($headCapture.Output | Select-Object -First 1)).Trim().ToLowerInvariant()
$fullShaPattern = '^(?:[0-9a-f]{40}|[0-9a-f]{64})$'
if ($baseCapture.ExitCode -ne 0 -or $baseSha -notmatch $fullShaPattern) {
    $baseSha = ""
    Add-Unique $blockers "could not capture a full origin/main commit after fetch"
}
if ($headCapture.ExitCode -ne 0 -or $headSha -notmatch $fullShaPattern) {
    $headSha = ""
    Add-Unique $blockers "could not capture a full HEAD commit after fetch"
}
$snapshotAvailable = -not [string]::IsNullOrEmpty($baseSha) -and -not [string]::IsNullOrEmpty($headSha)
$snapshotRange = if ($snapshotAvailable) { "$baseSha..$headSha" } else { "" }

$fastForwardSafe = $false
$ahead = @()
$behind = @()
if ($snapshotAvailable) {
    $ff = Invoke-Git merge-base --is-ancestor $baseSha $headSha
    $fastForwardSafe = $ff.ExitCode -eq 0
    if (-not $fastForwardSafe) {
        Add-Unique $blockers "captured base SHA is not an ancestor of captured head SHA; reconcile before push"
    }
    $ahead = @((Invoke-Git log --oneline ($baseSha + '..' + $headSha) -n 50).Output)
    $behind = @((Invoke-Git log --oneline ($headSha + '..' + $baseSha) -n 50).Output)
}

$secretsScan = Get-SecretScan -Range $snapshotRange -SnapshotUnavailable:(-not $snapshotAvailable)
if ($secretsScan.status -eq "blocked") {
    Add-Unique $blockers "High-confidence secret detected in added content; see secrets_scan rule counts"
} elseif ($secretsScan.status -eq "error") {
    Add-Unique $blockers "Secrets scan failed closed for captured base/head snapshot"
}

$hasIssue = $false
$hasWaiver = $false
$evidence = @()
$changes = @()
$surfaces = [System.Collections.Generic.HashSet[string]]::new()

if ($CollaborationMode -eq "governed") {
    # Keep historical direct-main, input, surface, and shared-skill checks governed-only.
if ($branch -eq "main") {
    Add-Unique $blockers "direct push from local main is not allowed by repo policy"
}

$hasIssue = $IssueNumber -gt 0 -or -not [string]::IsNullOrWhiteSpace($IssueUrl)
$hasWaiver = -not [string]::IsNullOrWhiteSpace($WaiverReason)
if (-not $hasIssue -and -not $hasWaiver) {
    Add-Unique $blockers "IssueNumber, IssueUrl, or WaiverReason is required"
}
if ($hasWaiver) {
    Add-Unique $warnings "board/card verification waived: $WaiverReason"
}

if (-not $ExpectedScope -or $ExpectedScope.Count -eq 0) {
    Add-Unique $blockers "ExpectedScope is required"
}

$evidence = @()
if ($TraceabilityEvidence) {
    $evidence += $TraceabilityEvidence
}
if ($env:PREPUSH_GUARD_TRACEABILITY_EVIDENCE) {
    $evidence += ($env:PREPUSH_GUARD_TRACEABILITY_EVIDENCE -split ';')
}
if ($evidence.Count -eq 0) {
    Add-Unique $blockers "TraceabilityEvidence is required"
}
foreach ($path in $evidence) {
    if ([string]::IsNullOrWhiteSpace($path)) {
        continue
    }
    if (-not (Test-Path -LiteralPath $path)) {
        Add-Unique $blockers "Traceability evidence not found: $path"
        continue
    }
    $normalized = $path.Replace('\', '/')
    if ($normalized -like ".docs/raw/*") {
        Add-Unique $blockers "Traceability evidence cannot live only under .docs/raw: $path"
    }
    if ($normalized -notmatch '(auditoria|audit|traceability|trazabilidad|verdict|planificacion)') {
        Add-Unique $warnings "Traceability evidence path does not look like closure evidence: $path"
    }
}

$pendingChanges = @()
$pendingStatus = Invoke-GitRawText @("status", "--porcelain=v1", "-z", "--untracked-files=all")
if ($pendingStatus.ExitCode -ne 0) {
    Add-Unique $blockers "pending changed-path inventory failed"
} else {
    try {
        $pendingChanges = @(Convert-StatusPorcelainZ $pendingStatus.OutputText)
    } catch {
        Add-Unique $blockers "pending changed-path inventory was malformed"
    }
}

$pendingInventory = [ordered]@{
    staged = @($pendingChanges | Where-Object { $_.X -ne ' ' -and $_.X -ne '?' })
    unstaged = @($pendingChanges | Where-Object { $_.Y -ne ' ' -and $_.Y -ne '?' })
    untracked = @($pendingChanges | Where-Object { $_.Kind -eq 'untracked' })
}

$outgoingChanges = @()
if (-not $snapshotAvailable) {
    Add-Unique $blockers "outgoing changed-path inventory unavailable without captured base/head SHAs"
} else {
    $outgoingDiff = Invoke-GitRawText @("diff", "--name-status", "--no-ext-diff", "-M", "-C", "--find-copies-harder", "-z", $snapshotRange, "--")
    if ($outgoingDiff.ExitCode -ne 0) {
        Add-Unique $blockers "outgoing changed-path inventory failed"
    } else {
        try {
            $outgoingChanges = @(Convert-NameStatusZ $outgoingDiff.OutputText)
        } catch {
            Add-Unique $blockers "outgoing changed-path inventory was malformed"
        }
    }
}
$changes = @($outgoingChanges) + @($pendingChanges)

$rawDirty = @()
$surfaces = [System.Collections.Generic.HashSet[string]]::new()
foreach ($change in $changes) {
    foreach ($path in @(Get-InventoryPaths $change)) {
        $surface = Get-Surface $path
        [void]$surfaces.Add($surface)
        $copySourceOnly = $change.X -match '^C' -and $path -eq $change.PreviousPath
        if (-not $copySourceOnly -and $path.Replace('\', '/') -like ".docs/raw/*" -and
            $change.X -ne 'D' -and $change.Y -ne 'D') {
            $rawDirty += $path
        }
    }
}
foreach ($path in $rawDirty) {
    Add-Unique $blockers "Added or modified .docs/raw path is blocked: $path"
}

$expected = [System.Collections.Generic.HashSet[string]]::new()
foreach ($scopeItem in $ExpectedScope) {
    [void]$expected.Add([string]$scopeItem)
}
foreach ($change in $changes) {
    foreach ($path in @(Get-InventoryPaths $change)) {
        $surface = Get-Surface $path
        if ($surface -eq "unknown") {
            Add-Unique $blockers "Changed path has unknown surface; cannot verify ExpectedScope: $path"
            continue
        }
        if (-not $expected.Contains($surface) -and $surface -ne "raw-docs") {
            Add-Unique $blockers "Changed surface '$surface' is not declared in ExpectedScope"
        }
    }
}

if ($expected.Contains("shared-skill")) {
    $skillNames = @()
    if ($SharedSkillName) {
        $skillNames += $SharedSkillName
    }
    if (-not [string]::IsNullOrWhiteSpace($env:PREPUSH_GUARD_SHARED_SKILL)) {
        $skillNames += ($env:PREPUSH_GUARD_SHARED_SKILL -split ',')
    }
    $skillNames = @($skillNames | ForEach-Object { $_.Trim() } | Where-Object { $_ } | Select-Object -Unique)
    if ($skillNames.Count -eq 0) {
        Add-Unique $blockers "SharedSkillName is required when ExpectedScope includes shared-skill"
    }
    $defaultInstalledRoot = $null
    if ([string]::IsNullOrWhiteSpace($env:AE_SKILL_SOURCE_ROOT) -or
        [string]::IsNullOrWhiteSpace($env:AE_SKILL_INSTALLED_ROOT)) {
        $userProfileRoot = [Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)
        if ([string]::IsNullOrWhiteSpace($userProfileRoot)) {
            $userProfileRoot = if (-not [string]::IsNullOrWhiteSpace($env:USERPROFILE)) { $env:USERPROFILE } else { $env:HOME }
        }
        if ([string]::IsNullOrWhiteSpace($userProfileRoot)) {
            throw "Unable to resolve the default user profile for shared skills"
        }
        $defaultInstalledRoot = Join-Path $userProfileRoot ".agents\skills"
    }
    $sourceRoot = Resolve-SharedSkillRoot -EnvironmentVariable "AE_SKILL_SOURCE_ROOT" -DefaultRoot $defaultInstalledRoot
    $mirrorRoot = Resolve-SharedSkillRoot -EnvironmentVariable "AE_SKILL_MIRROR_ROOT" -DefaultRoot "C:\repos\buho\assets\skills"
    $installedRoot = Resolve-SharedSkillRoot -EnvironmentVariable "AE_SKILL_INSTALLED_ROOT" -DefaultRoot $defaultInstalledRoot

    foreach ($skillName in $skillNames) {
        $skillRoots = @(
            [pscustomobject]@{ Name = "source"; Root = $sourceRoot }
            [pscustomobject]@{ Name = "installed"; Root = $installedRoot }
            [pscustomobject]@{ Name = "mirror"; Root = $mirrorRoot }
        )
        for ($index = 0; $index -lt $skillRoots.Count - 1; $index++) {
            $leftRoot = $skillRoots[$index]
            $rightRoot = $skillRoots[$index + 1]
            if ([IO.Path]::GetFullPath($leftRoot.Root).TrimEnd([char[]]"\/") -eq [IO.Path]::GetFullPath($rightRoot.Root).TrimEnd([char[]]"\/")) {
                continue
            }

            $leftSkill = Join-Path $leftRoot.Root "$skillName\SKILL.md"
            $rightSkill = Join-Path $rightRoot.Root "$skillName\SKILL.md"
            if (-not (Test-Path -LiteralPath $leftSkill)) {
                Add-Unique $blockers "Shared skill $($leftRoot.Name) not found: $leftSkill"
                continue
            }
            if (-not (Test-Path -LiteralPath $rightSkill)) {
                Add-Unique $blockers "Shared skill $($rightRoot.Name) not found: $rightSkill"
                continue
            }
            $leftHash = (Get-FileHash -LiteralPath $leftSkill -Algorithm SHA256).Hash
            $rightHash = (Get-FileHash -LiteralPath $rightSkill -Algorithm SHA256).Hash
            $inSync = $leftHash -eq $rightHash
            $sharedSkillMirrorChecks += [pscustomobject]@{
                skill = $skillName
                source = $leftSkill
                mirror = $rightSkill
                in_sync = $inSync
                sha256 = if ($inSync) { $leftHash } else { "" }
            }
            if (-not $inSync) {
                Add-Unique $blockers "Shared skill source and mirror differ: $skillName ($($leftRoot.Name) vs $($rightRoot.Name))"
            }
        }
    }
}

$dangerousNames = @("test-results", "playwright-report", ".next", "coverage", "dist", "build", "node_modules")
$dangerous = @()
if (Test-Path -LiteralPath "src") {
    $dangerous = @(Get-ChildItem -Recurse -Force -Directory -LiteralPath "src" -ErrorAction SilentlyContinue |
        Where-Object { $dangerousNames -contains $_.Name } |
        ForEach-Object { $_.FullName })
}
foreach ($path in $dangerous) {
    Add-Unique $blockers "Dangerous untracked/build artifact under src: $path"
}
}

$branchIdentityStable = $false
$headAtApprovalResult = Invoke-Git rev-parse --verify 'HEAD^{commit}'
$headAtApproval = ([string]($headAtApprovalResult.Output | Select-Object -First 1)).Trim().ToLowerInvariant()
$branchIdentityApproval = Invoke-Git symbolic-ref --quiet HEAD
$currentBranchRef = if ($branchIdentityApproval.ExitCode -eq 0) {
    ([string]($branchIdentityApproval.Output | Select-Object -First 1)).Trim()
} else {
    ""
}
$branchIdentityStable = $branchIdentityAvailable -and
    (Test-BranchIdentityStable $branchIdentityExitCode $branchRef $branchIdentityApproval.ExitCode $currentBranchRef)
if (-not $branchIdentityStable) {
    Add-Unique $blockers "symbolic branch identity changed or could not be revalidated during guard"
}
$snapshotStable = $snapshotAvailable -and $headAtApprovalResult.ExitCode -eq 0 -and
    $headAtApproval -eq $headSha -and $branchIdentityStable
if ($snapshotAvailable -and ($headAtApprovalResult.ExitCode -ne 0 -or $headAtApproval -ne $headSha)) {
    Add-Unique $blockers "HEAD changed during guard; captured snapshot cannot be approved"
}

$verdict = "Approved"
if ($blockers.Count -gt 0) {
    $verdict = "Blocked"
} elseif ($CollaborationMode -eq "governed" -and $hasWaiver) {
    $verdict = "Approved with waiver"
}

$report = [pscustomobject]@{
    verdict = $verdict
    collaboration_mode = $CollaborationMode
    branch = $branch
    branch_ref = $branchRef
    branch_identity_stable = $branchIdentityStable
    head = $headSha
    base_sha = $baseSha
    head_sha = $headSha
    snapshot_stable = $snapshotStable
    fast_forward_safe = $fastForwardSafe
    secrets_scan = $secretsScan
    ahead_count = $ahead.Count
    behind_count = $behind.Count
    expected_scope = @($ExpectedScope)
    detected_surfaces = @($surfaces)
    traceability_evidence = @($evidence)
    shared_skill_mirror_checks = @($sharedSkillMirrorChecks)
    waiver_reason = $WaiverReason
    changed_paths = @($changes)
    pending_changes = $pendingInventory
    blockers = @($blockers)
    warnings = @($warnings)
    dry_run = [bool]$DryRun
}

if ($Json) {
    $report | ConvertTo-Json -Depth 8
} else {
    "PrePushGuard verdict: $verdict"
    "collaboration_mode: $CollaborationMode"
    "branch: $branch"
    "branch_ref: $branchRef"
    "branch_identity_stable: $branchIdentityStable"
    "snapshot_stable: $snapshotStable"
    "head_sha: $headSha"
    "base_sha: $baseSha"
    "fast_forward_safe: $fastForwardSafe"
    "secrets_scan: status=$($secretsScan.status), match_count=$($secretsScan.match_count)"
    "expected_scope: $($ExpectedScope -join ',')"
    "traceability_evidence: $($evidence -join ',')"
    if ($warnings.Count -gt 0) {
        "warnings:"
        $warnings | ForEach-Object { " - $_" }
    }
    if ($sharedSkillMirrorChecks.Count -gt 0) {
        "shared_skill_mirror_checks:"
        $sharedSkillMirrorChecks | ForEach-Object { " - $($_.skill): in_sync=$($_.in_sync)" }
    }
    if ($blockers.Count -gt 0) {
        "blockers:"
        $blockers | ForEach-Object { " - $_" }
    }
}

if ($blockers.Count -gt 0) {
    exit 1
}
exit 0

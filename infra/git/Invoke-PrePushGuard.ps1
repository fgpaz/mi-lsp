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

function Convert-StatusLine {
    param([string]$Line)
    if ($Line.Length -lt 4) {
        return $null
    }
    $xy = $Line.Substring(0, 2)
    $path = $Line.Substring(3)
    if ($xy -eq "??") {
        return [pscustomobject]@{ Kind = "untracked"; Path = $path; X = "?"; Y = "?" }
    }
    return [pscustomobject]@{
        Kind = if ($xy[0] -ne ' ') { "staged" } elseif ($xy[1] -ne ' ') { "working" } else { "unknown" }
        Path = $path
        X = [string]$xy[0]
        Y = [string]$xy[1]
    }
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
    param([Parameter(Mandatory = $true)][string]$Range)

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

    $scanError = $false
    $scannedAddedLines = 0
    try {
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

$branch = ((Invoke-Git branch --show-current).Output | Select-Object -First 1).ToString().Trim()
$head = ((Invoke-Git rev-parse HEAD).Output | Select-Object -First 1).ToString().Trim()

$fetch = Invoke-Git fetch origin main
if ($fetch.ExitCode -ne 0) {
    Add-Unique $blockers "git fetch origin main failed"
}

$ff = Invoke-Git merge-base --is-ancestor origin/main HEAD
$fastForwardSafe = $ff.ExitCode -eq 0
if (-not $fastForwardSafe) {
    Add-Unique $blockers "origin/main is not an ancestor of HEAD; reconcile before push"
}

$ahead = @((Invoke-Git log --oneline origin/main..HEAD -n 50).Output)
$behind = @((Invoke-Git log --oneline HEAD..origin/main -n 50).Output)

$secretsScan = Get-SecretScan -Range "origin/main..HEAD"
if ($secretsScan.status -eq "blocked") {
    Add-Unique $blockers "High-confidence secret detected in added content; see secrets_scan rule counts"
} elseif ($secretsScan.status -eq "error") {
    Add-Unique $blockers "Secrets scan failed closed for origin/main..HEAD"
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

$statusLines = @((Invoke-Git status --porcelain=v1).Output)
$changes = @()
foreach ($line in $statusLines) {
    $entry = Convert-StatusLine $line
    if ($entry) {
        $changes += $entry
    }
}

$rawDirty = @()
$surfaces = [System.Collections.Generic.HashSet[string]]::new()
foreach ($change in $changes) {
    $surface = Get-Surface $change.Path
    [void]$surfaces.Add($surface)
    if ($change.Path.Replace('\', '/') -like ".docs/raw/*") {
        if ($change.X -ne 'D' -and $change.Y -ne 'D') {
            $rawDirty += $change.Path
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
foreach ($surface in $surfaces) {
    if ($surface -eq "unknown") {
        Add-Unique $warnings "Changed path has unknown surface; review scope manually"
        continue
    }
    if (-not $expected.Contains($surface) -and $surface -ne "raw-docs") {
        Add-Unique $blockers "Changed surface '$surface' is not declared in ExpectedScope"
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
    head = $head
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

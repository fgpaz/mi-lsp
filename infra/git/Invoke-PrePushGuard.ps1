param(
    [int]$IssueNumber,
    [string]$IssueUrl,
    [string]$WaiverReason,
    [string[]]$ExpectedScope,
    [string[]]$TraceabilityEvidence,
    [string[]]$SharedSkillName,
    [switch]$DryRun,
    [switch]$Json
)

$ErrorActionPreference = "Stop"

function Invoke-Git {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    $output = & git @Args 2>&1
    $exit = $LASTEXITCODE
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

$expected = [System.Collections.Generic.HashSet[string]]::new([string[]]$ExpectedScope)
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
    foreach ($skillName in $skillNames) {
        $globalSkill = Join-Path $env:USERPROFILE ".agents\skills\$skillName\SKILL.md"
        $mirrorSkill = "C:\repos\buho\assets\skills\$skillName\SKILL.md"
        if (-not (Test-Path -LiteralPath $globalSkill)) {
            Add-Unique $blockers "Shared skill source not found: $globalSkill"
            continue
        }
        if (-not (Test-Path -LiteralPath $mirrorSkill)) {
            Add-Unique $blockers "Shared skill mirror not found: $mirrorSkill"
            continue
        }
        $globalHash = (Get-FileHash -LiteralPath $globalSkill -Algorithm SHA256).Hash
        $mirrorHash = (Get-FileHash -LiteralPath $mirrorSkill -Algorithm SHA256).Hash
        $inSync = $globalHash -eq $mirrorHash
        $sharedSkillMirrorChecks += [pscustomobject]@{
            skill = $skillName
            source = $globalSkill
            mirror = $mirrorSkill
            in_sync = $inSync
            sha256 = if ($inSync) { $globalHash } else { "" }
        }
        if (-not $inSync) {
            Add-Unique $blockers "Shared skill source and mirror differ: $skillName"
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

$verdict = "Approved"
if ($blockers.Count -gt 0) {
    $verdict = "Blocked"
} elseif ($hasWaiver) {
    $verdict = "Approved with waiver"
}

$report = [pscustomobject]@{
    verdict = $verdict
    branch = $branch
    head = $head
    fast_forward_safe = $fastForwardSafe
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
    "branch: $branch"
    "fast_forward_safe: $fastForwardSafe"
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

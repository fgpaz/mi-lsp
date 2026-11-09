[CmdletBinding()]
param(
    [Alias('issue-key')][string]$IssueKey,
    [Alias('linear-url')][string]$LinearUrl,
    [Alias('waiver-reason')][string]$WaiverReason,
    [Alias('expected-scope')][string[]]$ExpectedScope = @(),
    [Alias('session-contract')][string]$SessionContract,
    [Alias('traceability-evidence')][string[]]$TraceabilityEvidence = @(),
    [Alias('closure-packet','single-closure-packet')][string]$SingleClosurePacket,
    [Alias('session-authorizations')][string]$SessionAuthorizations,
    [Alias('partial-publication-packet')][string]$PartialPublicationPacket,
    [Alias('base-ref')][string]$BaseRef = '',
    [string]$Remote = 'origin',
    [switch]$FirstPush,
    [Alias('shared-skill-name')][string[]]$SharedSkillName = @(),
    [switch]$NoFetch,
    [ValidateSet('governed','solo')][string]$CollaborationMode = 'governed',
    [ValidateRange(1,3600)][int]$GitTimeoutSeconds = 30,
    [ValidateRange(1,3600)][int]$ScannerTimeoutSeconds = 30,
    [Alias('h')][switch]$Help,
    [switch]$Json,
    [switch]$DryRun
)

# This file intentionally uses only Windows PowerShell 5.1 syntax.  It is also
# consumed by the native git hook, so it must not depend on external modules.
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$baseRefExplicit = $PSBoundParameters.ContainsKey('BaseRef')
$telemetry = @{ walks = 0; git_processes = 0; hash_calls = 0; started_at = [System.Diagnostics.Stopwatch]::GetTimestamp() }
$script:scannerDurationMs = 0
$script:GuardRepositoryRoot = ''
try {
    $installAnchor = Join-Path $PSScriptRoot '..\..'
    if (Test-Path -LiteralPath $installAnchor) {
        $resolvedAnchor = (Resolve-Path -LiteralPath $installAnchor).Path
        $global:LASTEXITCODE = 0
        $probe = @(& git -C $resolvedAnchor 'rev-parse' '--show-toplevel' 2>&1)
        if ([int]$global:LASTEXITCODE -eq 0 -and @($probe).Count -gt 0) {
            $script:GuardRepositoryRoot = ([string]$probe[-1]).Trim()
        }
    }
} catch { $script:GuardRepositoryRoot = '' }
$hashCache = @{}

function ConvertTo-ContentScanArgument {
    param([string]$Text)
    if ($null -eq $Text) { return '""' }
    if ($Text -match '[\s"]') { return '"' + ($Text -replace '"', '\"') + '"' }
    return $Text
}

function Invoke-ContentScanProcess {
    param([string]$FileName, [string[]]$Arguments, [string]$StdinText, [int]$TimeoutSeconds, [string]$WorkingDirectory, [switch]$ScannerClock)
    $process = $null
    $clockStart = 0
    if ($ScannerClock) { $clockStart = [System.Diagnostics.Stopwatch]::GetTimestamp() }
    try {
        $start = New-Object System.Diagnostics.ProcessStartInfo
        $start.FileName = $FileName
        $start.Arguments = [string]::Join(' ', @($Arguments | ForEach-Object { ConvertTo-ContentScanArgument ([string]$_) }))
        $start.UseShellExecute = $false
        $start.RedirectStandardOutput = $true
        $start.RedirectStandardError = $true
        $start.RedirectStandardInput = $true
        $start.CreateNoWindow = $true
        if ($WorkingDirectory) { $start.WorkingDirectory = $WorkingDirectory }
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $start
        if (-not $process.Start()) { return [pscustomobject]@{ ExitCode = 127; TimedOut = $false; OutputText = '' } }
        if ($null -ne $StdinText) { $process.StandardInput.Write($StdinText) }
        $process.StandardInput.Close()
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $timeoutMs = 1000
        if ($TimeoutSeconds -gt 0) { $timeoutMs = [int][Math]::Min([int64]$TimeoutSeconds * 1000, [int]::MaxValue) }
        if (-not $process.WaitForExit($timeoutMs)) {
            try { $process.Kill() } catch { }
            return [pscustomobject]@{ ExitCode = 124; TimedOut = $true; OutputText = '' }
        }
        [void][System.Threading.Tasks.Task]::WaitAll(@($stdoutTask, $stderrTask), 5000)
        return [pscustomobject]@{ ExitCode = [int]$process.ExitCode; TimedOut = $false; OutputText = [string]$stdoutTask.Result }
    } catch {
        if ($process) { try { $process.Kill() } catch { } }
        return [pscustomobject]@{ ExitCode = 127; TimedOut = $false; OutputText = '' }
    } finally {
        if ($ScannerClock) {
            $scanTicks = [System.Diagnostics.Stopwatch]::GetTimestamp() - [int64]$clockStart
            $script:scannerDurationMs += [int][Math]::Max(0, [Math]::Round(($scanTicks * 1000.0) / [System.Diagnostics.Stopwatch]::Frequency))
        }
        if ($process) { $process.Dispose() }
    }
}

function Add-ContentScanVerdict {
    param($Result, [System.Collections.ArrayList]$Blockers, [System.Collections.ArrayList]$Infra, [string]$Label)
    if ($Result.TimedOut -or $Result.ExitCode -eq 124) { [void]$Infra.Add('content scan timed out: ' + $Label); return }
    if ($Result.ExitCode -eq 1) { [void]$Blockers.Add('content scan findings: ' + $Label); return }
    if ($Result.ExitCode -ne 0) { [void]$Infra.Add('content scan failed: ' + $Label + ' exit ' + $Result.ExitCode) }
}

# Reuses the Gitleaks CLI contract. It does not embed rules, install the binary,
# synthesize .gitleaks.toml, or treat a path-name match as content coverage.
# Missing tool/config/range is a block, never a pass. A hidden but present config
# is readable; metadata or access failure is a typed configuration error, never
# an uncaught abort. Ignored paths are named as omitted, not scanned.
function Invoke-OutgoingContentScan {
    param(
        [string]$Repo,
        [string]$BaseSha,
        [string]$HeadSha,
        [string]$RangeLabel,
        [bool]$FirstPush,
        [bool]$PendingInventoryOk,
        [string[]]$UntrackedPaths,
        [System.Collections.ArrayList]$Blockers,
        [System.Collections.ArrayList]$Infra,
        [System.Collections.ArrayList]$ConfigErrors,
        [System.Collections.ArrayList]$Warnings
    )
    [void]$Warnings.Add('content scan coverage is the outgoing commit range plus non-ignored pending paths; ignored paths are not coverage')
    $scanner = Get-Command gitleaks -ErrorAction SilentlyContinue
    if (-not $scanner -or [string]::IsNullOrWhiteSpace([string]$scanner.Source)) {
        [void]$Infra.Add('content scanner missing: gitleaks; outgoing content was not scanned')
        return
    }
    $scannerPath = [string]$scanner.Source
    $configPath = Join-Path $Repo '.gitleaks.toml'
    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        [void]$ConfigErrors.Add('content scanner config missing: .gitleaks.toml; outgoing content was not scanned')
        return
    }
    # Linux pwsh hides dotfiles from Get-Item unless -Force; Windows Hidden
    # attribute has the same trap. Test-Path already saw the leaf. Contain the
    # metadata read so access failure cannot escape as empty stdout.
    try {
        $configInfo = Get-Item -LiteralPath $configPath -Force -ErrorAction Stop
        if ($null -eq $configInfo -or $configInfo.PSIsContainer) { throw 'config metadata is not a file' }
        $configLength = [int64]$configInfo.Length
    } catch {
        [void]$ConfigErrors.Add('content scanner config unreadable: .gitleaks.toml metadata failed; outgoing content was not scanned')
        return
    }
    if ($configLength -lt 1) {
        [void]$ConfigErrors.Add('content scanner config invalid: .gitleaks.toml is empty; outgoing content was not scanned')
        return
    }
    $gitTimeout = $GitTimeoutSeconds
    if ($gitTimeout -lt 1) { $gitTimeout = 1 }
    $scanTimeout = $ScannerTimeoutSeconds
    if ($scanTimeout -lt 1) { $scanTimeout = 1 }
    $historyLabel = if ($RangeLabel) { $RangeLabel + '..HEAD' } else { 'outgoing range' }
    if ($FirstPush) {
        if ($HeadSha -notmatch '^[0-9a-fA-F]{40}$') {
            [void]$Infra.Add('outgoing content range incomplete: HEAD unresolved; scan not run')
        } else {
            $history = Invoke-ContentScanProcess -FileName $scannerPath -Arguments @('detect','--source',$Repo,'--config',$configPath,'--redact',('--log-opts=' + $HeadSha)) -StdinText $null -TimeoutSeconds $scanTimeout -WorkingDirectory $Repo -ScannerClock
            Add-ContentScanVerdict $history $Blockers $Infra ('first-push history ' + $HeadSha)
        }
    } elseif ($BaseSha -notmatch '^[0-9a-fA-F]{40}$' -or $HeadSha -notmatch '^[0-9a-fA-F]{40}$') {
        [void]$Infra.Add('outgoing content range incomplete: BaseRef or HEAD unresolved; scan not run')
    } else {
        $count = Invoke-ContentScanProcess -FileName 'git' -Arguments @('-C',$Repo,'rev-list','--count',($BaseSha + '..' + $HeadSha)) -StdinText $null -TimeoutSeconds $gitTimeout -WorkingDirectory $Repo
        if ($count.TimedOut -or $count.ExitCode -ne 0) {
            [void]$Infra.Add('outgoing content range incomplete: rev-list failed; scan not run')
        } else {
            $countText = ([string]$count.OutputText).Trim()
            if ($countText -notmatch '^[0-9]+$') {
                [void]$Infra.Add('outgoing content range incomplete: rev-list output unreadable; scan not run')
            } elseif ([int]$countText -gt 0) {
                $history = Invoke-ContentScanProcess -FileName $scannerPath -Arguments @('detect','--source',$Repo,'--config',$configPath,'--redact',('--log-opts=' + $BaseSha + '..' + $HeadSha)) -StdinText $null -TimeoutSeconds $scanTimeout -WorkingDirectory $Repo -ScannerClock
                Add-ContentScanVerdict $history $Blockers $Infra $historyLabel
            }
        }
    }
    if (-not $PendingInventoryOk) {
        [void]$Infra.Add('pending content inventory failed; staged, unstaged, and untracked content was not scanned')
        return
    }
    foreach ($diffArgs in @(@('diff','-U0'), @('diff','--cached','-U0'))) {
        $diff = Invoke-ContentScanProcess -FileName 'git' -Arguments (@('-C',$Repo) + $diffArgs) -StdinText $null -TimeoutSeconds $gitTimeout -WorkingDirectory $Repo
        $diffLabel = 'pending ' + ([string]::Join(' ', $diffArgs))
        if ($diff.TimedOut -or $diff.ExitCode -ne 0) {
            [void]$Infra.Add('pending diff failed; content scan not skipped: ' + $diffLabel)
            continue
        }
        $stdinScan = Invoke-ContentScanProcess -FileName $scannerPath -Arguments @('stdin','--config',$configPath,'--redact') -StdinText ([string]$diff.OutputText) -TimeoutSeconds $scanTimeout -WorkingDirectory $Repo -ScannerClock
        Add-ContentScanVerdict $stdinScan $Blockers $Infra $diffLabel
    }
    foreach ($relative in @($UntrackedPaths)) {
        if ([string]::IsNullOrWhiteSpace([string]$relative)) { continue }
        $full = if ([System.IO.Path]::IsPathRooted([string]$relative)) { [string]$relative } else { Join-Path $Repo ([string]$relative) }
        if (-not (Test-Path -LiteralPath $full -PathType Leaf)) {
            [void]$Infra.Add('untracked path missing during content scan; not treated as clean: ' + $relative)
            continue
        }
        $fileScan = Invoke-ContentScanProcess -FileName $scannerPath -Arguments @('detect','--no-git','--source',$full,'--config',$configPath,'--redact') -StdinText $null -TimeoutSeconds $scanTimeout -WorkingDirectory $Repo -ScannerClock
        Add-ContentScanVerdict $fileScan $Blockers $Infra ('untracked ' + $relative)
    }
}

# Solo mode is an intentionally small, local-only escape hatch for this kernel
# bootstrap. It must not load ae-pre-push, fetch, inspect contracts, or require
# traceability evidence; the critical checks are bounded and fail closed.
if ($CollaborationMode -eq 'solo') {
    $soloStart = [System.Diagnostics.Stopwatch]::GetTimestamp()
    $soloBlockers = New-Object System.Collections.ArrayList
    $soloWarnings = New-Object System.Collections.ArrayList
    $soloInfra = New-Object System.Collections.ArrayList
    function Invoke-SoloGit {
        param([string[]]$Arguments)
        $global:LASTEXITCODE = 0
        try {
            $gitArgs = @($Arguments)
            if (-not [string]::IsNullOrWhiteSpace($script:GuardRepositoryRoot)) {
                $gitArgs = @('-C', $script:GuardRepositoryRoot) + $gitArgs
            }
            $result = @(& git @gitArgs 2>&1)
            $telemetry.git_processes++
            return [pscustomobject]@{ ExitCode=[int]$global:LASTEXITCODE; Output=$result }
        } catch {
            return [pscustomobject]@{ ExitCode=127; Output=@($_.Exception.Message) }
        }
    }
    function Add-Solo { param([System.Collections.ArrayList]$List,[string]$Text) [void]$List.Add($Text) }
    if (-not [string]::IsNullOrWhiteSpace($PartialPublicationPacket)) { Add-Solo $soloBlockers 'explicit_incomplete: partial publication boundary is non-authorizing and cannot use solo mode' }
    $root = Invoke-SoloGit @('rev-parse','--show-toplevel')
    if ($root.ExitCode -eq 0) { $repo = ([string]$root.Output[0]).Trim() }
    elseif (-not [string]::IsNullOrWhiteSpace($script:GuardRepositoryRoot)) { $repo = $script:GuardRepositoryRoot }
    else { Add-Solo $soloInfra 'not inside a git repository'; $repo = (Get-Location).Path }
    $branch = ''
    $branchResult = Invoke-SoloGit @('branch','--show-current')
    if ($branchResult.ExitCode -eq 0) { $branch = ([string]$branchResult.Output[0]).Trim() } else { Add-Solo $soloInfra 'branch is not resolvable' }
    $head = ''
    $headResult = Invoke-SoloGit @('rev-parse','HEAD')
    if ($headResult.ExitCode -eq 0) { $head = ([string]$headResult.Output[0]).Trim() } else { Add-Solo $soloInfra 'HEAD is not resolvable' }
    $status = Invoke-SoloGit @('status','--porcelain=v1','--untracked-files=all')
    if ($status.ExitCode -ne 0) { Add-Solo $soloInfra 'working-tree inventory failed' }
    $paths = @($status.Output | ForEach-Object {
        $line = [string]$_
        if ($line.Length -ge 4) { $line.Substring(3).Trim().Trim('"') }
    } | Where-Object { $_ })
    $soloBase = ''
    if (@('main','master') -contains $branch) {
        if ($baseRefExplicit) { $soloBase = $BaseRef }
        else {
            $defaultRef = Invoke-SoloGit @('symbolic-ref','--quiet','--short',('refs/remotes/' + $Remote + '/HEAD'))
            if ($defaultRef.ExitCode -eq 0 -and @($defaultRef.Output).Count -gt 0) { $soloBase = ([string]$defaultRef.Output[0]).Trim() }
            else { $soloBase = $Remote + '/' + $branch }
        }
        $resolvedBase = if ($soloBase) { Invoke-SoloGit @('rev-parse','--verify',($soloBase + '^{commit}')) } else { $null }
        if (-not $resolvedBase -or $resolvedBase.ExitCode -ne 0 -or @($resolvedBase.Output).Count -eq 0 -or ([string]$resolvedBase.Output[0]).Trim() -notmatch '^[0-9a-fA-F]{40}$') {
            Add-Solo $soloInfra ('base ref is invalid or unavailable locally: ' + $soloBase)
        } else {
            $baseSha = ([string]$resolvedBase.Output[0]).Trim()
            $ancestor = Invoke-SoloGit @('merge-base','--is-ancestor',$baseSha,'HEAD')
            if ($ancestor.ExitCode -ne 0) { Add-Solo $soloBlockers ($soloBase + ' is not an ancestor of ' + $branch + ' HEAD') }
            $divergence = Invoke-SoloGit @('rev-list','--count',($baseSha + '..HEAD'))
            if ($divergence.ExitCode -ne 0) { Add-Solo $soloInfra ('bounded ' + $branch + ' divergence check failed') }
            elseif ([int]$divergence.Output[0] -gt 50) { Add-Solo $soloBlockers ($branch + ' divergence exceeds bounded solo limit (50 commits)') }
        }
    } elseif ($baseRefExplicit) {
        $resolvedBase = Invoke-SoloGit @('rev-parse','--verify',($BaseRef + '^{commit}'))
        if ($resolvedBase.ExitCode -ne 0 -or @($resolvedBase.Output).Count -eq 0) { Add-Solo $soloInfra ('base ref is invalid or unavailable locally: ' + $BaseRef) }
        else {
            $soloBase = $BaseRef
            $ancestor = Invoke-SoloGit @('merge-base','--is-ancestor',([string]$resolvedBase.Output[0]).Trim(),'HEAD')
            if ($ancestor.ExitCode -ne 0) { Add-Solo $soloBlockers ($soloBase + ' is not an ancestor of ' + $branch + ' HEAD') }
        }
    }
    $sensitive = '(^|/)(\.env($|\.)|id_rsa|.*\.(pfx|pem|key)$|secrets?)(/|$)'
    foreach ($path in $paths) { if ($path -match $sensitive) { Add-Solo $soloBlockers ('sensitive path is not push-safe: ' + $path) } }
    $soloBaseSha = ''
    if ($soloBase) {
        $soloBaseResolved = Invoke-SoloGit @('rev-parse','--verify',($soloBase + '^{commit}'))
        if ($soloBaseResolved.ExitCode -eq 0 -and @($soloBaseResolved.Output).Count -gt 0) { $soloBaseSha = ([string]$soloBaseResolved.Output[0]).Trim() }
    }
    $soloUntracked = @()
    if ($status.ExitCode -eq 0) {
        $soloUntracked = @($status.Output | ForEach-Object {
            $line = [string]$_
            if ($line.Length -ge 4 -and $line.Substring(0,2) -eq '??') { $line.Substring(3).Trim().Trim('"') }
        } | Where-Object { $_ })
    }
    Invoke-OutgoingContentScan -Repo $repo -BaseSha $soloBaseSha -HeadSha $head -RangeLabel $soloBase -FirstPush $false -PendingInventoryOk ($status.ExitCode -eq 0) -UntrackedPaths $soloUntracked -Blockers $soloBlockers -Infra $soloInfra -ConfigErrors $soloBlockers -Warnings $soloWarnings
    $elapsedTicks = [System.Diagnostics.Stopwatch]::GetTimestamp() - [int64]$soloStart
    $duration = [int][Math]::Max(0,[Math]::Round(($elapsedTicks * 1000.0) / [System.Diagnostics.Stopwatch]::Frequency))
    $scannerMs = [int]$script:scannerDurationMs
    $infraMs = [int][Math]::Max(0, $duration - $scannerMs)
    if ($infraMs -ge 5000) { Add-Solo $soloInfra 'solo guard exceeded hard 5 second budget' }
    $code = if ($soloInfra.Count -gt 0) { 2 } elseif ($soloBlockers.Count -gt 0) { 1 } else { 0 }
    $report = [ordered]@{ schema='ae-pre-push-report/v2'; mode='solo'; collaboration_mode='solo'; verdict=if ($code -eq 0) {'Approved'} else {'Blocked'}; exit_code=$code; code_name=if ($code -eq 0) {'success'} elseif ($code -eq 1) {'validation_failure'} else {'infrastructure_error'}; branch=$branch; base=$soloBase; head=$head; fetched=$false; blockers=@($soloBlockers); configuration_errors=@(); infrastructure_errors=@($soloInfra); warnings=@($soloWarnings); changed_paths=@($paths); session_contract_bypassed=$true; traceability_bypassed=$true; telemetry=[ordered]@{git_processes=[int]$telemetry.git_processes; duration_ms=$duration; infrastructure_duration_ms=$infraMs; scanner_duration_ms=$scannerMs; infrastructure_budget_ms=5000; git_timeout_seconds=[int]$GitTimeoutSeconds; scanner_timeout_seconds=[int]$ScannerTimeoutSeconds} }
    if ($Json) { $report | ConvertTo-Json -Depth 8 } else { Write-Output ('Solo pre-push guard: ' + $report.verdict + ' (' + $report.code_name + ')') }
    exit $code
}

if (-not $baseRefExplicit) { $BaseRef = 'origin/main' }

# Prefer the shared implementation when present and intact; all hard gates below
# remain local and uncached so a missing/corrupt module cannot create a false PASS.
$guardV2 = if ($env:AE_GUARDV2_PATH) { $env:AE_GUARDV2_PATH } else { Join-Path $HOME '.agents\skills\ae-pre-push\scripts\GuardV2.psm1' }
$guardV2TrustedSha256 = @(
    '307c3037525678c16239d9c0ed1c08cb86bf4bdd095e572727b28752a3d507e8',
    '5ef0fb0aa811ee01684774294044317990bd716058b07a407b54575300670a9a'
)
$guardV2Loaded = $false
$guardV2Mode = 'fallback'
$guardV2Reason = 'missing_or_corrupt'
$guardV2Commands = @()
$guardV2Fingerprint = ''
$guardV2IndependentSha256 = ''
if (Test-Path -LiteralPath $guardV2 -PathType Leaf) {
    try {
        $telemetry.hash_calls++
        $guardV2IndependentSha256 = (Get-FileHash -LiteralPath $guardV2 -Algorithm SHA256 -ErrorAction Stop).Hash.ToLowerInvariant()
        if ($guardV2TrustedSha256 -notcontains $guardV2IndependentSha256) { throw 'GuardV2 module hash is not trusted' }
        $module = Import-Module -Name $guardV2 -Force -PassThru -ErrorAction Stop
        $guardV2Commands = @(Get-Command -Module $module.Name | Select-Object -ExpandProperty Name | Sort-Object -Unique)
        if ($guardV2Commands -contains 'Get-AeGuardFingerprint' -and $guardV2Commands -contains 'New-AeGuardFinding') {
            $guardV2Loaded = $true
            $guardV2Mode = 'GuardV2'
            $guardV2Reason = 'imported_exported_commands'
        } else { $guardV2Reason = 'missing_required_exported_commands' }
    } catch { $guardV2Loaded = $false; $guardV2Reason = 'import_failed' }
}

function Add-Item {
    param([System.Collections.ArrayList]$List, [string]$Message)
    [void]$List.Add($Message)
}

function ConvertTo-ProcessArgument {
    param([string]$Value)
    if ($null -eq $Value -or $Value.Length -eq 0) { return '""' }
    if ($Value -notmatch '[\s"]') { return $Value }
    $escaped = [regex]::Replace($Value, '(\\*)"', '$1$1\"')
    $trailing = [regex]::Match($escaped, '(\\+)$').Value
    return '"' + $escaped + $trailing + '"'
}

function Stop-ProcessTree {
    param([System.Diagnostics.Process]$Process)
    if (-not $Process -or $Process.HasExited) { return }
    try {
        if ($env:OS -eq 'Windows_NT') {
            $killer = New-Object System.Diagnostics.ProcessStartInfo
            $killer.FileName = 'taskkill.exe'
            $killer.Arguments = '/PID ' + $Process.Id + ' /T /F'
            $killer.UseShellExecute = $false
            $killer.CreateNoWindow = $true
            $terminator = [System.Diagnostics.Process]::Start($killer)
            if ($terminator) { [void]$terminator.WaitForExit(5000); $terminator.Dispose() }
        }
    } catch { }
    try { if (-not $Process.HasExited) { $Process.Kill() } } catch { }
    try { [void]$Process.WaitForExit(5000) } catch { }
}

function Invoke-Git {
    param([string[]]$Arguments)
    $telemetry.git_processes++
    $process = $null
    $operation = if (@($Arguments).Count -gt 0) { 'git ' + ([string]$Arguments[0]) } else { 'git' }
    $callSite = @(Get-PSCallStack | Select-Object -Skip 1 -First 1)
    $callerLine = if ($callSite.Count -gt 0) { [int]$callSite[0].ScriptLineNumber } else { 0 }
    $failure = $operation + ' failed (caller line ' + $callerLine + ')'
    try {
        $start = New-Object System.Diagnostics.ProcessStartInfo
        $start.FileName = 'git'
        $start.Arguments = [string]::Join(' ', @($Arguments | ForEach-Object { ConvertTo-ProcessArgument ([string]$_) }))
        $start.UseShellExecute = $false
        $start.RedirectStandardOutput = $true
        $start.RedirectStandardError = $true
        $start.CreateNoWindow = $true
        if (-not [string]::IsNullOrWhiteSpace($script:GuardRepositoryRoot)) {
            $start.WorkingDirectory = $script:GuardRepositoryRoot
        }
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $start
        if (-not $process.Start()) { throw 'git process did not start' }
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $timeoutMs = [int][Math]::Min([int64]$GitTimeoutSeconds * 1000, [int]::MaxValue)
        if (-not $process.WaitForExit($timeoutMs)) {
            Stop-ProcessTree $process
            return [pscustomobject]@{ ExitCode = 124; Output = @($operation + ' timed out (caller line ' + $callerLine + ')'); TimedOut = $true; Operation = $operation; CallerLine = $callerLine }
        }
        [void][System.Threading.Tasks.Task]::WaitAll(@($stdoutTask, $stderrTask), 5000)
        $lines = New-Object System.Collections.ArrayList
        foreach ($text in @($stdoutTask.Result, $stderrTask.Result)) {
            foreach ($line in @(([string]$text) -split "`r?`n")) { if ($line.Length -gt 0) { [void]$lines.Add($line) } }
        }
        if ($process.ExitCode -ne 0) {
            return [pscustomobject]@{ ExitCode = [int]$process.ExitCode; Output = @($failure); TimedOut = $false; Operation = $operation; CallerLine = $callerLine }
        }
        return [pscustomobject]@{ ExitCode = [int]$process.ExitCode; Output = @($lines); TimedOut = $false; Operation = $operation; CallerLine = $callerLine }
    } catch {
        if ($process) { Stop-ProcessTree $process }
        return [pscustomobject]@{ ExitCode = 127; Output = @($failure); TimedOut = $false; Operation = $operation; CallerLine = $callerLine }
    } finally {
        if ($process) { $process.Dispose() }
    }
}

function Join-Output {
    param([object[]]$Output)
    return [string]::Join(' ', @($Output | ForEach-Object { [string]$_ }))
}

function Get-SortedUniquePaths {
    # Dedup + orden determinista para inventarios de paths que pueden tener decenas de
    # miles de entradas. `Sort-Object -Unique` es culture-aware y compara por objeto:
    # sobre 105k paths tarda segundos por llamada. El orden ordinal ademas es estable
    # entre maquinas y locales, que es lo que un gate de seguridad necesita.
    param([object[]]$Items)
    $set = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::Ordinal)
    foreach ($item in @($Items)) {
        if ($null -eq $item) { continue }
        $text = [string]$item
        if ($text) { [void]$set.Add($text) }
    }
    $array = [string[]]::new($set.Count)
    $set.CopyTo($array)
    [Array]::Sort($array, [System.StringComparer]::Ordinal)
    return @($array)
}

function Get-GuardHash {
    param([string]$Path)
    $key = [System.IO.Path]::GetFullPath($Path)
    if ($hashCache.ContainsKey($key)) { return [string]$hashCache[$key] }
    $telemetry.hash_calls++
    $value = if ($guardV2Loaded -and ($guardV2Commands -contains 'Get-AeGuardSha256')) {
        (Get-AeGuardSha256 ([System.IO.File]::ReadAllBytes($Path))).ToLowerInvariant()
    } else {
        (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    $hashCache[$key] = $value
    return $value
}

function Get-GitBlobHashes {
    param([string[]]$RelativePaths)
    $normalized = @($RelativePaths | ForEach-Object {
        $value = ([string]$_).Replace('\','/').TrimStart('/')
        if ($value -match '^skills/') { $value = $value.Substring(7) }
        'skills/' + $value
    })
    $result = @{}
    if ($normalized.Count -eq 0) { return $result }
    $batch = Invoke-Git (@('ls-tree', '-r', '--full-tree', '--format=%(objectname)%x09%(path)', 'HEAD', '--') + $normalized)
    if ($batch.ExitCode -ne 0) { return $result }
    foreach ($raw in @($batch.Output)) {
        $parts = ([string]$raw) -split "`t", 2
        if ($parts.Count -eq 2 -and $parts[0] -match '^[0-9a-fA-F]{40}$') { $result[$parts[1]] = $parts[0].ToLowerInvariant() }
    }
    return $result
}

function Get-GitFileHashes {
    param([string[]]$Paths)
    $normalized = @($Paths | ForEach-Object { ([string]$_).Replace('\','/') })
    $result = @{}
    if ($normalized.Count -eq 0) { return $result }
    $batch = Invoke-Git (@('hash-object', '--') + $normalized)
    if ($batch.ExitCode -ne 0) { return $result }
    for ($index = 0; $index -lt $normalized.Count; $index++) {
        $hash = if ($index -lt @($batch.Output).Count) { ([string]$batch.Output[$index]).Trim().ToLowerInvariant() } else { '' }
        if ($hash -match '^[0-9a-f]{40}$') { $result[$normalized[$index]] = $hash }
    }
    return $result
}

function Get-GitFileHashForRepositoryPath {
    param([string]$Path, [string]$RepositoryPath)
    $normalizedRepositoryPath = ([string]$RepositoryPath).Replace('\','/').TrimStart('/')
    if (-not $normalizedRepositoryPath -or -not (Test-Path -LiteralPath $Path -PathType Leaf)) { return '' }
    $batch = Invoke-Git @('hash-object', ('--path=' + $normalizedRepositoryPath), '--', $Path)
    if ($batch.ExitCode -ne 0 -or @($batch.Output).Count -ne 1) { return '' }
    $hash = ([string]$batch.Output[0]).Trim().ToLowerInvariant()
    if ($hash -match '^[0-9a-f]{40}$') { return $hash }
    return ''
}

function Get-CanonicalDeliveryRemote {
    param([string]$Product)
    switch ($Product) {
        'ae-kernel' { return 'github.com/fgpaz/ae-kernel' }
        'claude-skills' { return 'github.com/fgpaz/claude-skills' }
        default { return '' }
    }
}

function Normalize-DeliveryRemoteIdentity {
    param([string]$Url)
    $value = ([string]$Url).Trim()
    if (-not $value) { return '' }
    $value = $value -replace '^[A-Za-z][A-Za-z0-9+.-]*://', ''
    $value = $value -replace '^[^/]+@', ''
    $value = $value -replace '[?#].*$', ''
    $value = $value -replace '^([^/]+):', '$1/'
    $value = $value -replace '/+$', ''
    $value = $value -replace '(?i)\.git$', ''
    return $value.ToLowerInvariant()
}

function Sanitize-DeliveryRemoteIdentity {
    param([string]$Url)
    $normalized = Normalize-DeliveryRemoteIdentity $Url
    if ($normalized) { return $normalized }
    return '<unresolved>'
}

function Test-AeDeliveryPathHasReparsePoint {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return $true }
    try {
        $fullPath = [System.IO.Path]::GetFullPath($Path)
        $root = [System.IO.Path]::GetPathRoot($fullPath)
        if ([string]::IsNullOrWhiteSpace($root)) { return $true }
        $current = $root
        $remainder = $fullPath.Substring($root.Length)
        foreach ($component in @($remainder -split '[\\/]+')) {
            if ([string]::IsNullOrWhiteSpace($component)) { continue }
            $current = Join-Path $current $component
            $item = Get-Item -LiteralPath $current -Force -ErrorAction SilentlyContinue
            if ($null -eq $item) {
                if ([System.IO.File]::Exists($current) -or [System.IO.Directory]::Exists($current)) { return $true }
                break
            }
            if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { return $true }
        }
        return $false
    } catch { return $true }
}

function Test-DeliveryRemoteIdentity {
    param([string]$RemoteName, [string]$Product, [System.Collections.ArrayList]$Errors)
    $expected = Get-CanonicalDeliveryRemote $Product
    if (-not $expected) { Add-Item $Errors 'delivery remote identity is unavailable for this product'; return '' }
    $expectedNormalized = Normalize-DeliveryRemoteIdentity $expected
    $fetch = Invoke-Git @('remote', 'get-url', '--all', $RemoteName)
    $push = Invoke-Git @('remote', 'get-url', '--all', '--push', $RemoteName)
    $fetchUrls = @($fetch.Output | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
    $pushUrls = @($push.Output | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
    $fetchIdentityValid = ($fetch.ExitCode -eq 0 -and $fetchUrls.Count -gt 0 -and @($fetchUrls | Where-Object { (Normalize-DeliveryRemoteIdentity $_) -ne $expectedNormalized }).Count -eq 0)
    $pushIdentityValid = ($push.ExitCode -eq 0 -and $pushUrls.Count -gt 0 -and @($pushUrls | Where-Object { (Normalize-DeliveryRemoteIdentity $_) -ne $expectedNormalized }).Count -eq 0)
    if ($fetch.ExitCode -eq 0 -and -not $fetchIdentityValid -and @($fetchUrls | Where-Object { (Normalize-DeliveryRemoteIdentity $_) -ne $expectedNormalized }).Count -gt 0) {
        Add-Item $Errors ('delivery remote fetch URL does not match canonical repository identity for ' + $Product)
    }
    if ($fetch.ExitCode -ne 0 -or $fetchUrls.Count -ne 1) { Add-Item $Errors 'delivery remote fetch URL could not be resolved unambiguously' }
    if (-not $pushIdentityValid -and @($pushUrls | Where-Object { (Normalize-DeliveryRemoteIdentity $_) -ne $expectedNormalized }).Count -gt 0) {
        Add-Item $Errors ('delivery remote push URL does not match canonical repository identity for ' + $Product)
    }
    if ($push.ExitCode -ne 0 -or $pushUrls.Count -ne 1) { Add-Item $Errors 'delivery remote push URL could not be resolved unambiguously' }
    if ($fetchIdentityValid -and $fetchUrls.Count -eq 1 -and $pushIdentityValid -and $pushUrls.Count -eq 1) { return Sanitize-DeliveryRemoteIdentity ([string]$fetchUrls[0]) }
    return '<unresolved>'
}

function Unquote {
    param([string]$Value)
    $result = ([string]$Value).Trim()
    if ($result.Length -ge 2) {
        if (($result[0] -eq [char]39 -and $result[$result.Length - 1] -eq [char]39) -or
            ($result[0] -eq [char]34 -and $result[$result.Length - 1] -eq [char]34)) {
            return $result.Substring(1, $result.Length - 2)
        }
    }
    return $result
}

function Get-Indent {
    param([string]$Line)
    $match = [regex]::Match($Line, '^(\s*)')
    return $match.Groups[1].Value.Length
}

function Get-YamlScalar {
    param([string]$Text, [string]$Key, [int]$MinimumIndent = -1)
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^\s*' + [regex]::Escape($Key) + ':\s*(.*?)\s*$')
        if ($match.Success -and ($MinimumIndent -lt 0 -or (Get-Indent $line) -ge $MinimumIndent)) {
            $value = $match.Groups[1].Value
            if ($value -and $value -notmatch '^#') { return Unquote $value }
        }
    }
    return ''
}

# Canon (AE-POLICY-PROJECTION.md) allows worker_decision: none for coupled direct work as long
# as why_no_worker carries a real justification. A justification may be an inline scalar or a
# block scalar, so presence of the key alone is not enough: placeholders such as null/n-a and
# empty block bodies must not satisfy the gate.
function Test-YamlKeyHasContent {
    param([string]$Text, [string]$Key)
    $placeholder = '^(?i)(null|~|none|n/a|na|not_applicable|tbd|pending)$'
    $lines = @($Text -split "`r?`n")
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = [string]$lines[$i]
        if ($line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^(\s*)' + [regex]::Escape($Key) + ':\s*(.*?)\s*$')
        if (-not $match.Success) { continue }
        $indent = $match.Groups[1].Value.Length
        $value = $match.Groups[2].Value
        if ($value -match '^[>|][-+0-9]*$') {
            for ($j = $i + 1; $j -lt $lines.Count; $j++) {
                $child = [string]$lines[$j]
                if (-not $child.Trim()) { continue }
                if ((Get-Indent $child) -le $indent) { break }
                if ((Unquote $child.Trim()) -notmatch $placeholder) { return $true }
                break
            }
            continue
        }
        if ($value -and $value -notmatch '^#' -and (Unquote $value) -notmatch $placeholder) { return $true }
    }
    return $false
}

function Get-YamlScalarAtPath {
    param([string]$Text, [string]$Path)
    $stack = New-Object System.Collections.ArrayList
    foreach ($line in ($Text -split "`r?`n")) {
        if (-not $line.Trim() -or $line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):(?:\s*(.*?)\s*)?$')
        if (-not $match.Success) { continue }
        $indent = $match.Groups[1].Value.Length
        while ($stack.Count -gt 0 -and $stack[$stack.Count - 1].Indent -ge $indent) { $stack.RemoveAt($stack.Count - 1) }
        $key = $match.Groups[2].Value
        $prefix = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') + '.' } else { '' }
        $current = $prefix + $key
        if ($current -eq $Path) {
            $value = $match.Groups[3].Value
            if ($value -and $value -notmatch '^#') { return Unquote $value }
        }
        [void]$stack.Add([pscustomobject]@{ Indent=$indent; Key=$key })
    }
    return ''
}

function Get-YamlScalarAtAnyPath {
    param([string]$Text, [string[]]$Paths)
    foreach ($path in @($Paths)) {
        $value = Get-YamlScalarAtPath $Text $path
        if ($value) { return $value }
    }
    return ''
}

function Test-YamlPathDeclared {
    param([string]$Text, [string]$Path)
    $stack = New-Object System.Collections.ArrayList
    foreach ($line in ($Text -split "`r?`n")) {
        if (-not $line.Trim() -or $line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):')
        if (-not $match.Success) { continue }
        $indent = $match.Groups[1].Value.Length
        while ($stack.Count -gt 0 -and $stack[$stack.Count - 1].Indent -ge $indent) { $stack.RemoveAt($stack.Count - 1) }
        $key = $match.Groups[2].Value
        $prefix = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') + '.' } else { '' }
        if (($prefix + $key) -eq $Path) { return $true }
        [void]$stack.Add([pscustomobject]@{ Indent=$indent; Key=$key })
    }
    return $false
}

function Convert-SimpleYamlList {
    param([string]$Value)
    $text = Unquote ([string]$Value).Trim()
    if (-not $text -or $text -match '^(?i)(null|~|none)$') { return @() }
    if ($text.StartsWith('[') -and $text.EndsWith(']')) {
        $text = $text.Substring(1, $text.Length - 2)
        if (-not $text.Trim()) { return @() }
        return @($text -split ',' | ForEach-Object { Unquote $_.Trim() } | Where-Object { $_ })
    }
    return @($text)
}

function Get-YamlListAtPath {
    param([string]$Text, [string]$Path)
    $lines = @($Text -split "`r?`n")
    $items = New-Object System.Collections.ArrayList
    $stack = New-Object System.Collections.ArrayList
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = [string]$lines[$i]
        if (-not $line.Trim() -or $line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):(?:\s*(.*?)\s*)?$')
        if (-not $match.Success) { continue }
        $indent = $match.Groups[1].Value.Length
        while ($stack.Count -gt 0 -and $stack[$stack.Count - 1].Indent -ge $indent) { $stack.RemoveAt($stack.Count - 1) }
        $key = $match.Groups[2].Value
        $prefix = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') + '.' } else { '' }
        $current = $prefix + $key
        $value = $match.Groups[3].Value
        if ($current -eq $Path) {
            if ($value) { foreach ($item in @(Convert-SimpleYamlList $value)) { [void]$items.Add($item) } }
            else {
                for ($j = $i + 1; $j -lt $lines.Count; $j++) {
                    $child = [string]$lines[$j]
                    if (-not $child.Trim() -or $child -match '^\s*#') { continue }
                    $childIndent = Get-Indent $child
                    if ($childIndent -le $indent) { break }
                    $itemMatch = [regex]::Match($child, '^\s*-\s*(.*?)\s*$')
                    if ($itemMatch.Success) { foreach ($item in @(Convert-SimpleYamlList $itemMatch.Groups[1].Value)) { [void]$items.Add($item) } }
                }
            }
            return @(Get-SortedUniquePaths $items)
        }
        [void]$stack.Add([pscustomobject]@{ Indent=$indent; Key=$key })
    }
    return @()
}

function Get-LaneContextModel {
    param([string]$Text, [string]$Kind)
    $laneDeclared = Test-YamlPathDeclared $Text 'lane_context'
    $claims = @(Get-YamlListAtPath $Text 'relevance.claim_ids')
    if ($claims.Count -eq 0) { $claims = @(Get-YamlListAtPath $Text 'lane_context.relevance.claim_ids') }
    $scope = @(Get-YamlListAtPath $Text 'lane_context.evidence_scope')
    if ($scope.Count -eq 0) { $scope = @(Convert-SimpleYamlList (Get-YamlScalarAtPath $Text 'lane_context.evidence_scope')) }
    if ($scope.Count -eq 0) { $scope = @(Get-YamlListAtPath $Text 'evidence_scope') }
    if ($scope.Count -eq 0) { $scope = @(Convert-SimpleYamlList (Get-YamlScalarAtPath $Text 'evidence_scope')) }
    $acceptance = Get-YamlScalarAtAnyPath $Text @('relevance.acceptance_anchor', 'lane_context.relevance.acceptance_anchor')
    $notRelevant = Get-YamlScalarAtAnyPath $Text @('relevance.status', 'relevance.not_relevant', 'not_relevant', 'status')
    $exclusive = Get-YamlScalarAtAnyPath $Text @('lane_context.evidence_root_exclusive', 'evidence_root_exclusive')
    return [pscustomobject]@{
        Kind = $Kind
        Declared = [bool]$laneDeclared
        SessionId = Get-YamlScalarAtPath $Text 'lane_context.session_id'
        GoalId = Get-YamlScalarAtPath $Text 'lane_context.goal_id'
        LaneId = Get-YamlScalarAtPath $Text 'lane_context.lane_id'
        AttemptId = Get-YamlScalarAtPath $Text 'lane_context.attempt_id'
        PreparationRegistered = Get-YamlScalarAtPath $Text 'lane_context.preparation_registered'
        PreparationManifestSha256 = Get-YamlScalarAtPath $Text 'lane_context.preparation_manifest_sha256'
        EvidenceRoot = Get-YamlScalarAtPath $Text 'lane_context.evidence_root'
        EvidenceRootExclusive = $exclusive
        Sanitized = Get-YamlScalarAtPath $Text 'lane_context.sanitized'
        EvidenceScope = @($scope)
        AcceptanceAnchor = $acceptance
        ClaimIds = @($claims)
        NotRelevant = ($notRelevant -match '^(?i)(not_relevant|true)$')
        RelevanceDeclared = (Test-YamlPathDeclared $Text 'relevance')
    }
}

function Get-LaneContextCompletenessErrors {
    param([object]$Context, [string]$Owner)
    $errors = New-Object System.Collections.ArrayList
    if (-not $Context.Declared) { [void]$errors.Add($Owner + ' is missing lane_context') ; return @($errors) }
    foreach ($field in @('SessionId','GoalId','LaneId','AttemptId','PreparationManifestSha256','EvidenceRoot','Sanitized')) {
        if ([string]::IsNullOrWhiteSpace([string]$Context.$field)) { [void]$errors.Add($Owner + ' lane_context.' + $field + ' is missing or unknown') }
    }
    if ([string]$Context.PreparationRegistered -ine 'true') { [void]$errors.Add($Owner + ' lane_context.preparation_registered must be true') }
    if ([string]$Context.Sanitized -ine 'true') { [void]$errors.Add($Owner + ' lane_context.sanitized must be true') }
    if ($Context.PreparationManifestSha256 -notmatch '^[0-9a-fA-F]{64}$') { [void]$errors.Add($Owner + ' lane_context.preparation_manifest_sha256 must be a 64-hex digest') }
    if (-not (Test-RepoRelativePath $Context.EvidenceRoot) -or $Context.EvidenceRoot -match '(?i)(^|/)(\.docs$|\.docs/auditoria$|evidence$|worker-joins$|shared$)(/|$)|[*?]') { [void]$errors.Add($Owner + ' lane_context.evidence_root must be an exclusive repo-relative lane root') }
    if ($Context.EvidenceRootExclusive -and [string]$Context.EvidenceRootExclusive -ine 'true') { [void]$errors.Add($Owner + ' lane_context.evidence_root_exclusive must be true') }
    if (@($Context.EvidenceScope).Count -eq 0) { [void]$errors.Add($Owner + ' lane_context.evidence_scope is missing or unknown') }
    if ([string]::IsNullOrWhiteSpace([string]$Context.AcceptanceAnchor)) { [void]$errors.Add($Owner + ' relevance.acceptance_anchor is missing or unknown') }
    if (@($Context.ClaimIds).Count -eq 0) { [void]$errors.Add($Owner + ' relevance.claim_ids is missing or unknown') }
    return @($errors)
}

function Test-LaneLineage {
    param([object]$Context, [object]$ContractContext, [bool]$ContractActive, [string]$Owner)
    $errors = New-Object System.Collections.ArrayList
    $info = New-Object System.Collections.ArrayList
    $dropped = $false
    $eligible = $true
    $contractClaims = if ($ContractContext) { @($ContractContext.ClaimIds) } else { @() }
    $intersection = @($Context.ClaimIds | Where-Object { $contractClaims -contains $_ })
    if ($Context.NotRelevant) {
        $dropped = $true; $eligible = $false
        [void]$info.Add(('INFO_DROPPED: ' + $Owner + ' explicitly declares not_relevant'))
    } elseif (@($Context.ClaimIds).Count -gt 0 -and @($contractClaims).Count -gt 0 -and @($intersection).Count -eq 0) {
        $dropped = $true; $eligible = $false
        [void]$info.Add(('INFO_DROPPED: ' + $Owner + ' claim_ids have no intersection with the active lane'))
    }
    if ($dropped) { return [pscustomobject]@{ Errors=@(); Info=@($info); Dropped=$true; Eligible=$false; Intersection=@($intersection) } }
    if ($ContractActive -and -not $Context.Declared) { [void]$errors.Add($Owner + ' must declare complete lane_context because the session contract activates the lane schema'); return [pscustomobject]@{ Errors=@($errors); Info=@(); Dropped=$false; Eligible=$false; Intersection=@($intersection) } }
    if ($Context.Declared) {
        foreach ($error in @(Get-LaneContextCompletenessErrors $Context $Owner)) { [void]$errors.Add($error) }
        if (-not $ContractActive -or -not $ContractContext -or -not $ContractContext.Declared) { [void]$errors.Add($Owner + ' declares lane_context but the session contract does not activate lane_context') }
        elseif ($Context.SessionId -ne $ContractContext.SessionId) { [void]$errors.Add($Owner + ' lane_context.session_id does not match the session contract') }
        elseif ($Context.GoalId -ne $ContractContext.GoalId) { [void]$errors.Add($Owner + ' lane_context.goal_id does not match the session contract') }
        elseif ($Context.LaneId -ne $ContractContext.LaneId) { [void]$errors.Add($Owner + ' lane_context.lane_id does not match the session contract') }
        elseif ($Context.AttemptId -ne $ContractContext.AttemptId) { [void]$errors.Add($Owner + ' lane_context.attempt_id does not match the session contract') }
        elseif ($Context.PreparationManifestSha256 -ne $ContractContext.PreparationManifestSha256) { [void]$errors.Add($Owner + ' lane_context.preparation_manifest_sha256 does not match the session contract') }
        elseif ($Context.EvidenceRoot -ne $ContractContext.EvidenceRoot) { [void]$errors.Add($Owner + ' lane_context.evidence_root does not match the session contract') }
        elseif (([string]::Join('|', @($Context.EvidenceScope))) -ne ([string]::Join('|', @($ContractContext.EvidenceScope)))) { [void]$errors.Add($Owner + ' lane_context.evidence_scope does not match the session contract') }
        elseif ($Context.AcceptanceAnchor -ne $ContractContext.AcceptanceAnchor) { [void]$errors.Add($Owner + ' relevance.acceptance_anchor does not match the session contract') }
    }
    if (@($Context.ClaimIds).Count -gt 0 -and @($contractClaims).Count -gt 0 -and @($intersection).Count -eq 0) { [void]$errors.Add($Owner + ' relevance.claim_ids do not intersect the session contract') }
    if ($errors.Count -gt 0) { $eligible = $false }
    return [pscustomobject]@{ Errors=@($errors); Info=@($info); Dropped=$dropped; Eligible=$eligible; Intersection=@($intersection) }
}

function New-IssueKeyRule {
    # Un repo con un unico ticket de agente declara el key exacto. Un repo donde cada
    # sesion abre su propio ticket declara un patron: el gate sigue probando algo real
    # (que el push cuelgue de un ticket del proyecto correcto) sin fijar el numero, que
    # es justamente lo que hacia que el canonico no fuera adoptable sin editarlo.
    param([string]$Exact, [string]$Pattern)
    if ($Pattern) { return [pscustomobject]@{ Kind = 'pattern'; Value = $Pattern; Describe = ('match ' + $Pattern) } }
    return [pscustomobject]@{ Kind = 'exact'; Value = $Exact; Describe = $Exact }
}

function Test-IssueKeyRule {
    param([object]$Rule, [string]$Value)
    if (-not $Rule) { return $true }
    $text = [string]$Value
    if ($Rule.Kind -eq 'pattern') {
        # -match con un patron de politica: se ancla tal cual lo declaro el repo.
        try { return ($text -match $Rule.Value) } catch { return $false }
    }
    return ($text -eq $Rule.Value)
}

function Get-ExpectedIssueKeys {
    # Ticket topology is required only for externally tracked repositories. Local-only
    # kernels deliberately have no synthetic TES/Linear identity to invent.
    param([string]$Repo)
    $parent = 'TES-141'
    $agent = 'TES-142'
    $parentPattern = ''
    $agentPattern = ''
    $external = $true
    $policyPath = Join-Path $Repo '.docs/ae/repo-policy.yaml'
    if (Test-Path -LiteralPath $policyPath -PathType Leaf) {
        $policyText = Get-Content -LiteralPath $policyPath -Raw
        $provider = Get-YamlScalarAtAnyPath $policyText @('tracker.provider', 'tracking.provider')
        $mode = Get-YamlScalarAtAnyPath $policyText @('tracker.mode', 'tracking.mode', 'tracker.none.mode')
        if ($provider -match '^(?i)(none|not_applicable|null)$' -or $mode -match '^(?i)(none|local-only|local_only|not_applicable)$') { $external = $false }
        $declaredParent = Get-YamlScalarAtAnyPath $policyText @('prepush_guard.issues.parent', 'prepush_guard.parent_issue')
        $declaredAgent = Get-YamlScalarAtAnyPath $policyText @('prepush_guard.issues.agent', 'prepush_guard.agent_issue')
        $parentPattern = Get-YamlScalarAtAnyPath $policyText @('prepush_guard.issues.parent_pattern')
        $agentPattern = Get-YamlScalarAtAnyPath $policyText @('prepush_guard.issues.agent_pattern')
        if ($declaredParent) { $parent = $declaredParent }
        if ($declaredAgent) { $agent = $declaredAgent }
    }
    $parentRule = New-IssueKeyRule $parent $parentPattern
    $agentRule = New-IssueKeyRule $agent $agentPattern
    return [pscustomobject]@{
        Parent = $parent
        Agent = $agent
        ParentRule = $parentRule
        AgentRule = $agentRule
        External = $external
    }
}

function Test-RepoRelativePath {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return $false }
    if ([System.IO.Path]::IsPathRooted($Path)) { return $false }
    $normalized = $Path.Replace('\','/')
    return ($normalized -notmatch '(^|/)\.\.?(/|$)' -and $normalized -notmatch '^/' -and $normalized -notmatch '^[A-Za-z]:')
}

function Get-StructuredMetadata {
    param([string]$Path, [string]$Text, [string]$Kind, [string[]]$ExpectedHeads, [string]$ExpectedBase, [int]$FreshnessWindowHours = 24, [bool]$AllowEmptyBase = $false)
    $errors = New-Object System.Collections.ArrayList
    if ($Text -match "`t") { [void]$errors.Add('tabs are forbidden in structured YAML') }
    $schema = Get-YamlScalarAtPath $Text 'schema'
    $type = Get-YamlScalarAtPath $Text 'type'
    $generated = Get-YamlScalarAtPath $Text 'generated_at'
    $sha = Get-YamlScalarAtPath $Text 'sha256'
    $baseSha = Get-YamlScalarAtPath $Text 'base_sha'
    $headSha = Get-YamlScalarAtPath $Text 'head_sha'
    $verdict = Get-YamlScalarAtPath $Text 'verdict'
    $isTraceabilityV2 = $schema -match '^ae-traceability-(closure|audit)/v2$'
    if ($isTraceabilityV2) {
        if (-not $type) { $type = if ($schema -match 'closure') { 'traceability_closure' } else { 'traceability_audit' } }
        $beforeUpstream = Get-YamlScalarAtPath $Text 'git_before.upstream'
        $beforeHead = Get-YamlScalarAtPath $Text 'git_before.head'
        $afterHead = Get-YamlScalarAtPath $Text 'git_after.head'
        $implementationHead = Get-YamlScalarAtPath $Text 'changed_tree.implementation_head'
        if (-not $baseSha -and @('origin/main','origin/master') -contains $beforeUpstream) { $baseSha = $beforeHead }
        if (-not $headSha) { $headSha = if ($afterHead) { $afterHead } else { $implementationHead } }
    }
    if (-not $schema) { [void]$errors.Add('missing schema') }
    if (-not $type) { [void]$errors.Add('missing type') }
    $parsedDate = [datetime]::MinValue
    if (-not $generated) { [void]$errors.Add('missing generated_at') }
    elseif (-not [datetime]::TryParse($generated, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::AssumeUniversal, [ref]$parsedDate)) { [void]$errors.Add('generated_at is not an ISO timestamp') }
    elseif (([datetime]::UtcNow - $parsedDate.ToUniversalTime()).TotalHours -gt $FreshnessWindowHours -or $parsedDate.ToUniversalTime() -gt [datetime]::UtcNow.AddMinutes(5)) { [void]$errors.Add(('generated_at is outside freshness window of ' + $FreshnessWindowHours + ' hours')) }
    if (-not $isTraceabilityV2 -and $sha -notmatch '^[0-9a-fA-F]{64}$') { [void]$errors.Add('sha256 must be a 64-hex digest') }
    $emptyBaseDeclared = (Get-YamlScalarAtPath $Text 'base_semantics') -eq 'empty_remote_no_base'
    if ($AllowEmptyBase) {
        if (-not $emptyBaseDeclared) { [void]$errors.Add('base_semantics must be empty_remote_no_base for FirstPush evidence') }
        if ($baseSha) { [void]$errors.Add('base_sha must be absent for empty_remote_no_base evidence') }
    } elseif ($baseSha -notmatch '^[0-9a-fA-F]{40}$' -or ($ExpectedBase -and $baseSha -ne $ExpectedBase)) { [void]$errors.Add('base_sha is missing, invalid, or stale') }
    $headAccepted = $false
    foreach ($candidateHead in @($ExpectedHeads)) { if ($candidateHead -and $headSha -eq $candidateHead) { $headAccepted = $true; break } }
    if ($headSha -notmatch '^[0-9a-fA-F]{40}$' -or (@($ExpectedHeads).Count -gt 0 -and -not $headAccepted)) { [void]$errors.Add('head_sha is missing, invalid, or stale') }
    if ($verdict -notmatch '^(?i)(PASS|Approved|handoff_ready)$') { [void]$errors.Add('verdict is not PASS/Approved') }
    $finalClosureVerdict = Get-YamlScalarAtPath $Text 'final_closure_verdict'
    $finalAuditVerdict = Get-YamlScalarAtPath $Text 'final_verdict'
    $isClosurePacket = $schema -eq 'ae-closure-packet/v1'
    $workerJoinStatus = Get-YamlScalarAtPath $Text 'worker_join_status'
    $workerJoinCount = Get-YamlScalarAtPath $Text 'worker_join_count'
    if ($isTraceabilityV2 -and $schema -match 'closure' -and $finalClosureVerdict -notmatch '^(?i)PASS$') { [void]$errors.Add('traceability closure final_closure_verdict is not PASS') }
    if ($isTraceabilityV2 -and $schema -match 'audit' -and $finalAuditVerdict -notmatch '^(?i)(Approved|PASS)$') { [void]$errors.Add('traceability audit final_verdict is not Approved/PASS') }
    if ($Kind -eq 'join' -and @('ae-worker-join/v1', 'ae-closure-packet/v1') -notcontains $schema) { [void]$errors.Add('join schema must be ae-worker-join/v1 or aggregate ae-closure-packet/v1') }
    if ($Kind -eq 'evidence' -and $schema -notmatch '^ae-(verdict|traceability-closure|traceability-audit)/v[0-9]+$' -and $schema -ne 'ae-closure-packet/v1') { [void]$errors.Add('evidence schema is unsupported') }
    if ($Kind -eq 'join' -and $isClosurePacket) {
        if ($workerJoinStatus -ine 'complete') { [void]$errors.Add('aggregate closure packet worker_join_status must be complete') }
        if ($workerJoinCount -notmatch '^[1-9][0-9]*$') { [void]$errors.Add('aggregate closure packet worker_join_count must be a positive integer') }
        if ($verdict -notmatch '^(?i)PASS$') { [void]$errors.Add('aggregate closure packet verdict must be PASS') }
    }
    $exportStatus = (Get-YamlScalarAtPath $Text 'export_status').ToLowerInvariant()
    $installStatus = (Get-YamlScalarAtPath $Text 'install_status').ToLowerInvariant()
    $reexport = (Get-YamlScalarAtPath $Text 'reexport').ToLowerInvariant()
    $installReal = (Get-YamlScalarAtPath $Text 'install_real').ToLowerInvariant()
    $parityStatus = (Get-YamlScalarAtPath $Text 'parity_status').ToLowerInvariant()
    $parityVerified = (Get-YamlScalarAtPath $Text 'parity_verified').ToLowerInvariant()
    if ($exportStatus -eq 'pass' -and $reexport -ne 'true') { [void]$errors.Add('export_status PASS requires reexport: true') }
    if ($installStatus -eq 'pass' -and $installReal -ne 'true') { [void]$errors.Add('install_status PASS requires install_real: true') }
    if ($parityStatus -eq 'pass' -and $parityVerified -ne 'true') { [void]$errors.Add('parity PASS requires parity_verified: true') }
    return [pscustomobject]@{ Errors=@($errors); Schema=$schema; Type=$type; GeneratedAt=$generated; Sha256=$sha; BaseSha=$baseSha; HeadSha=$headSha }
}

function Get-YamlList {
    param([string]$Text, [string]$Key)
    $lines = @($Text -split "`r?`n")
    $items = New-Object System.Collections.ArrayList
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = [string]$lines[$i]
        if ($line -match '^\s*#') { continue }
        $match = [regex]::Match($line, '^\s*' + [regex]::Escape($Key) + ':\s*$')
        if (-not $match.Success) { continue }
        $sectionIndent = Get-Indent $line
        for ($j = $i + 1; $j -lt $lines.Count; $j++) {
            $child = [string]$lines[$j]
            if (-not $child.Trim()) { continue }
            $childIndent = Get-Indent $child
            if ($childIndent -le $sectionIndent) { break }
            $item = [regex]::Match($child, '^\s*-\s*(.*?)\s*$')
            if ($item.Success) { [void]$items.Add((Unquote $item.Groups[1].Value)) }
        }
    }
    return @($items | Sort-Object -Unique)
}

# An in-memory compatibility view, never a schema rewrite or readiness claim.
# Resolve only declared equivalent paths. A second declaration must agree;
# unioning alternative allowlists would silently widen publication scope.
function Resolve-ContractField {
    param([string]$Text, [string[]]$Paths, [bool]$IsList, [System.Collections.ArrayList]$Errors)
    $selected = @()
    $selectedPath = ''
    foreach ($path in $Paths) {
        if (-not (Test-YamlPathDeclared $Text $path)) { continue }
        $value = if ($IsList) { @(Get-YamlListAtPath $Text $path | Sort-Object -Unique) } else { @(Get-YamlScalarAtPath $Text $path) }
        if ($selectedPath) {
            if ([string]::Join("`n", [string[]]$selected) -cne [string]::Join("`n", [string[]]$value)) {
                [void]$Errors.Add('conflicting equivalent contract fields: ' + $selectedPath + ' / ' + $path)
            }
        } else { $selected = @($value); $selectedPath = $path }
    }
    return $selected
}

function Get-ContractModel {
    param([string]$Text)
    $schema = Get-YamlScalarAtPath $Text 'schema'
    $compatibilityErrors = New-Object System.Collections.ArrayList
    $branchPaths = @('repositorio.branch_actual', 'repository.branch')
    $basePaths = @('repositorio.base_remota', 'repositorio.base_ref', 'repository.base_ref')
    $baseShaPaths = @('repositorio.base_sha', 'repository.base_sha')
    $anchorPaths = @('anchors', 'anchors.existing')
    $workerPaths = @('worker_decision', 'ae_budget_gate.worker_decision')
    if ($schema -ceq 'ae-session-contract/v1') {
        $branchPaths += 'intended_branch'
        $basePaths += 'base_ref'
        $baseShaPaths += 'base_sha'
        $anchorPaths += 'sdd_anchors'
        $workerPaths += 'worker.worker_decision'
    }
    $branch = @(Resolve-ContractField $Text $branchPaths $false $compatibilityErrors) -join ''
    $contractBase = @(Resolve-ContractField $Text $basePaths $false $compatibilityErrors) -join ''
    $baseSha = @(Resolve-ContractField $Text $baseShaPaths $false $compatibilityErrors) -join ''
    $workerDecision = @(Resolve-ContractField $Text $workerPaths $false $compatibilityErrors) -join ''
    $allowed = @(Resolve-ContractField $Text @('allowed_paths', 'permitido', 'scope.allowed_paths', 'scope.permitido', 'scope.expected_scope_exactly') $true $compatibilityErrors)
    $forbidden = @(Resolve-ContractField $Text @('forbidden', 'forbidden_paths', 'scope.forbidden', 'scope.forbidden_paths') $true $compatibilityErrors)
    # anchors.existing is a structured alternative, not an anchors scalar list.
    if (Test-YamlPathDeclared $Text 'anchors.existing') { $anchorPaths = @($anchorPaths | Where-Object { $_ -ne 'anchors' }) }
    $anchors = @(Resolve-ContractField $Text $anchorPaths $true $compatibilityErrors)
    $joinPaths = @(Resolve-ContractField $Text @('join_packets', 'durable_join_paths', 'evidence.join_packets', 'orquestacion.join_packets') $true $compatibilityErrors)
    $evidencePaths = @(Resolve-ContractField $Text @('evidence_paths', 'evidence.evidence_paths', 'scope.evidence_paths') $true $compatibilityErrors)
    $requiredEvidence = @(Resolve-ContractField $Text @('required_evidence', 'evidence.required_evidence') $true $compatibilityErrors)
    $keyCounts = @{}
    $sequenceCounts = @{}
    $stack = New-Object System.Collections.ArrayList
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match '^\s*#' -or -not $line.Trim()) { continue }
        $itemMatch = [regex]::Match($line, '^(\s*)-\s+')
        if ($itemMatch.Success) {
            $itemIndent = $itemMatch.Groups[1].Value.Length
            while ($stack.Count -gt 0 -and $stack[$stack.Count - 1].Indent -ge $itemIndent) { $stack.RemoveAt($stack.Count - 1) }
            $itemPrefix = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') } else { '' }
            $counterKey = $itemPrefix + '@' + $itemIndent
            if (-not $sequenceCounts.ContainsKey($counterKey)) { $sequenceCounts[$counterKey] = 0 }
            $sequenceCounts[$counterKey] = [int]$sequenceCounts[$counterKey] + 1
            [void]$stack.Add([pscustomobject]@{ Indent=$itemIndent; Key=('item' + $sequenceCounts[$counterKey]) })
        }
        $keyMatch = [regex]::Match($line, '^(\s*)([A-Za-z_][A-Za-z0-9_-]*):(?:\s|$)')
        if ($keyMatch.Success) {
            $indent = $keyMatch.Groups[1].Value.Length
            while ($stack.Count -gt 0 -and $stack[$stack.Count - 1].Indent -ge $indent) { $stack.RemoveAt($stack.Count - 1) }
            $key = $keyMatch.Groups[2].Value
            $prefix = if ($stack.Count -gt 0) { (($stack | ForEach-Object { $_.Key }) -join '.') + '.' } else { '' }
            $pathKey = $prefix + $key
            if (-not $keyCounts.ContainsKey($pathKey)) { $keyCounts[$pathKey] = 0 }
            $keyCounts[$pathKey] = [int]$keyCounts[$pathKey] + 1
            [void]$stack.Add([pscustomobject]@{ Indent=$indent; Key=$key })
        }
    }
    $duplicates = @($keyCounts.Keys | Where-Object { $keyCounts[$_] -gt 1 } | Sort-Object)
    return [pscustomobject]@{
        Schema = $schema
        CompatibilityErrors = @($compatibilityErrors)
        Branch = $branch
        Base = $contractBase
        ScopeBase = $contractBase
        BaseSha = $baseSha
        ParentIssue = Get-YamlScalarAtPath $Text 'issues.parent'
        AgentIssue = Get-YamlScalarAtPath $Text 'issues.agent'
        TrackerWaiver = Get-YamlScalar $Text 'tracker_waiver'
        WorkerDecision = $workerDecision
        TicketFrontier = Get-YamlScalar $Text 'ticket_frontier'
        Allowed = @($allowed | Sort-Object -Unique)
        Forbidden = @($forbidden | Sort-Object -Unique)
        Anchors = @($anchors | Sort-Object -Unique)
        JoinPaths = @($joinPaths | Sort-Object -Unique)
        EvidencePaths = @($evidencePaths | Sort-Object -Unique)
        RequiredEvidence = @($requiredEvidence | Sort-Object -Unique)
        FirstPushEnabled = Get-YamlScalarAtPath $Text 'first_push.enabled'
        FirstPushRemote = Get-YamlScalarAtPath $Text 'first_push.remote'
        FirstPushTargetBranch = Get-YamlScalarAtPath $Text 'first_push.target_branch'
        FirstPushRemoteEmptyRequired = Get-YamlScalarAtPath $Text 'first_push.remote_empty_required'
        FirstPushBaseSemantics = Get-YamlScalarAtPath $Text 'first_push.base_semantics'
        FirstPushAuthorized = Get-YamlScalarAtPath $Text 'first_push.authorization.authorized'
        FirstPushAuthorizedBy = Get-YamlScalarAtPath $Text 'first_push.authorization.authorized_by'
        FirstPushAuthorizedAt = Get-YamlScalarAtPath $Text 'first_push.authorization.authorized_at'
        FirstPushHandoffOwner = Get-YamlScalarAtPath $Text 'first_push.post_push_verification_handoff.owner'
        FirstPushHandoffCommand = Get-YamlScalarAtPath $Text 'first_push.post_push_verification_handoff.command'
        FirstPushExpectedRef = Get-YamlScalarAtPath $Text 'first_push.post_push_verification_handoff.expected_ref'
        FirstPushExpectedSha = Get-YamlScalarAtPath $Text 'first_push.post_push_verification_handoff.expected_sha'
        FirstPushUnexpectedRefs = Get-YamlScalarAtPath $Text 'first_push.post_push_verification_handoff.unexpected_refs'
        FirstPushRaceOwner = Get-YamlScalarAtPath $Text 'first_push.post_push_verification_handoff.toctou_race_owner'
        LaneContext = Get-LaneContextModel $Text 'contract'
        DuplicateKeys = $duplicates
        HasScope = [bool]((Test-YamlPathDeclared $Text 'scope') -or ($schema -ceq 'ae-session-contract/v1' -and (Test-YamlPathDeclared $Text 'allowed_paths') -and (Test-YamlPathDeclared $Text 'forbidden_paths')))
        HasBudget = [bool]([regex]::IsMatch($Text, '(?m)^\s*ae_budget_gate:\s*(?:#.*)?$'))
    }
}

function Get-PartialPublicationSourceV0 {
    $embedded = '{"schema":"ae-partial-publication-source/v0","publication_status":"explicit_incomplete","grants_push_approval":false,"waives_traceability_closure":true,"combined_pass_required":false}'
    $path = Join-Path $PSScriptRoot 'ae-partial-publication-source.v0.json'
    $text = if (Test-Path -LiteralPath $path -PathType Leaf) { Get-Content -LiteralPath $path -Raw } else { $embedded }
    try {
        $doc = ConvertFrom-Json -InputObject $text -ErrorAction Stop
        if ($null -eq $doc -or [string]$doc.schema -cne 'ae-partial-publication-source/v0') { return $null }
        return $doc
    } catch { return $null }
}

# v0 records the goal boundary and permits an explicit_incomplete result without
# full closure PASS. Neither SOURCE nor authority_refs proves the owner's authority
# to integrate source; SessionAuthorizations path/existence is not that proof.
function Get-PartialPublicationBoundary {
    param(
        [string]$Text,
        $SourceV0 = $null
    )
    $missing = New-Object System.Collections.ArrayList
    $schema = ''
    $goal = ''
    $incomplete = @()
    $authority = @()
    try {
        $packet = ConvertFrom-Json -InputObject $Text -ErrorAction Stop
        if ($null -eq $packet -or $packet -is [array]) { throw 'packet must be an object' }
        if ($packet.PSObject.Properties['schema']) { $schema = [string]$packet.schema }
        if ($schema -cne 'ae-partial-publication/v0') { [void]$missing.Add('unsupported_schema') }
        if ($packet.PSObject.Properties['goal_id']) { $goal = [string]$packet.goal_id }
        if ([string]::IsNullOrWhiteSpace($goal)) { [void]$missing.Add('goal_id') }
        if ($packet.PSObject.Properties['incomplete']) { $incomplete = @($packet.incomplete) }
        if ($incomplete.Count -eq 0) { [void]$missing.Add('incomplete') }
        foreach ($item in $incomplete) {
            foreach ($key in @('item', 'owner', 'deadline')) {
                if ($null -eq $item -or -not $item.PSObject.Properties[$key] -or [string]::IsNullOrWhiteSpace([string]$item.$key)) { [void]$missing.Add('incomplete.' + $key) }
            }
        }
        if ($packet.PSObject.Properties['authority_refs']) { $authority = @($packet.authority_refs) }
        if ($authority.Count -eq 0) { [void]$missing.Add('authority_refs') }
        foreach ($item in $authority) {
            foreach ($key in @('goal_id', 'reference')) {
                if ($null -eq $item -or -not $item.PSObject.Properties[$key] -or [string]::IsNullOrWhiteSpace([string]$item.$key)) { [void]$missing.Add('authority_refs.' + $key) }
            }
            if ($null -ne $item -and $item.PSObject.Properties['goal_id'] -and [string]$item.goal_id -cne $goal) { [void]$missing.Add('authority_goal_mismatch') }
        }
    } catch { [void]$missing.Add('invalid_json_object') }
    $uniqueMissing = @($missing | Sort-Object -Unique)
    $authorityStatus = 'missing_material_contract'
    $evidenceStatus = 'missing_material_contract'
    $enforcement = 'none'
    if ($uniqueMissing.Count -eq 0 -and $null -ne $SourceV0) {
        $authorityStatus = 'source_v0_explicit_incomplete'
        $evidenceStatus = 'scoped_incomplete_items'
        $enforcement = 'observed_explicit_incomplete'
    }
    # Do not forward arbitrary nested payloads from a request into the report.
    return [ordered]@{
        schema=$schema; goal_id=$goal; provenance='observed'; enforcement=$enforcement
        incomplete_count=$incomplete.Count; authority_reference_count=$authority.Count
        missing=$uniqueMissing
        authority_status=$authorityStatus; evidence_status=$evidenceStatus
        source_contract_schema= if ($SourceV0) { [string]$SourceV0.schema } else { '' }
        publication_status='explicit_incomplete'; full_completion=$false
    }
}

function Test-GlobPattern {
    param([string]$Pattern)
    if ([string]::IsNullOrWhiteSpace($Pattern)) { return $false }
    $open = 0
    for ($i = 0; $i -lt $Pattern.Length; $i++) {
        if ($Pattern[$i] -eq '[' -and ($i -eq 0 -or $Pattern[$i - 1] -ne '\')) { $open++ }
        elseif ($Pattern[$i] -eq ']' -and ($i -eq 0 -or $Pattern[$i - 1] -ne '\')) { if ($open -eq 0) { return $false }; $open-- }
    }
    return ($open -eq 0)
}

function Convert-GlobToRegex {
    param([string]$Pattern)
    $value = $Pattern.Replace('\','/')
    $builder = New-Object System.Text.StringBuilder
    [void]$builder.Append('^')
    for ($i = 0; $i -lt $value.Length; $i++) {
        $char = $value[$i]
        if ($char -eq '*') {
            if ($i + 1 -lt $value.Length -and $value[$i + 1] -eq '*') { [void]$builder.Append('.*'); $i++ }
            else { [void]$builder.Append('[^/]*') }
        } elseif ($char -eq '?') { [void]$builder.Append('[^/]')
        } elseif ($char -eq '[') {
            $close = $value.IndexOf(']', $i + 1)
            if ($close -gt $i + 1) { [void]$builder.Append($value.Substring($i, $close - $i + 1)); $i = $close }
            else { [void]$builder.Append('\\[') }
        } else { [void]$builder.Append([regex]::Escape([string]$char)) }
    }
    [void]$builder.Append('$')
    return $builder.ToString()
}

function Test-PathPattern {
    param([string]$Path, [string]$Pattern)
    if (-not (Test-GlobPattern $Pattern)) { return $false }
    return [regex]::IsMatch($Path.Replace('\','/'), (Convert-GlobToRegex $Pattern), [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
}

function Test-GovernedRawPlan {
    param([string]$Repo, [string]$RelativePath, [string[]]$AllowedPatterns)
    $normalized = $RelativePath.Replace('\','/')
    if ($normalized -notmatch '^\.docs/raw/plans/') { return $true }
    $authorized = $false
    foreach ($pattern in @($AllowedPatterns)) {
        $patternText = ([string]$pattern).Replace('\','/')
        if ($patternText -match '^\.docs/raw/plans/' -and ($normalized -eq $patternText -or (Test-PathPattern $normalized $patternText))) { $authorized = $true; break }
    }
    if (-not $authorized) { return $false }
    $absolute = Join-Path $Repo ($normalized.Replace('/', [IO.Path]::DirectorySeparatorChar))
    if (-not (Test-Path -LiteralPath $absolute -PathType Leaf)) { return $false }
    $text = [IO.File]::ReadAllText($absolute).TrimStart([char]0xFEFF)
    $opening = $text.IndexOf('---')
    $closing = $text.IndexOf('---', $opening + 3)
    if ($opening -lt 0 -or $closing -le $opening) { return $false }
    $frontmatter = $text.Substring($opening, $closing - $opening)
    return ($frontmatter.Contains('harness_protocol: SDD-HARNESS-v1') -and
        $frontmatter.Contains('schema: ae-governed-raw-input/v1') -and
        $frontmatter.Contains('governed_raw_input: true'))
}

function Get-SourceSkillFiles {
    param([string]$SourceRoot, [string[]]$RelativeRoots)
    $result = New-Object System.Collections.ArrayList
    if (-not (Test-Path -LiteralPath $SourceRoot -PathType Container) -or @($RelativeRoots).Count -eq 0) { return @($result) }
    $telemetry.walks++
    $fullRoot = [System.IO.Path]::GetFullPath($SourceRoot).TrimEnd([char[]]@('\', '/'))
    foreach ($file in [System.IO.Directory]::EnumerateFiles($fullRoot, '*', [System.IO.SearchOption]::AllDirectories)) {
        $relative = $file.Substring($fullRoot.Length).TrimStart('\', '/').Replace('\', '/')
        foreach ($root in @($RelativeRoots)) {
            $normalizedRoot = ([string]$root).Trim('/').Replace('\', '/')
            if ($relative -eq $normalizedRoot -or $relative.StartsWith($normalizedRoot + '/')) {
                [void]$result.Add([pscustomobject]@{ Relative = $relative; Full = $file })
                break
            }
        }
    }
    return @($result)
}

function Get-ScopeForPath {
    param([string]$Path)
    $normalized = $Path.Replace('\','/')
    if ($normalized -like 'canon/*') { return 'canon-docs' }
    if ($normalized -like 'skills/*') { return 'shared-skill' }
    if ($normalized -like 'infra/*' -or $normalized -like '.githooks/*' -or $normalized -like '.github/*' -or $normalized -like 'scripts/*' -or $normalized -like 'tests/*') { return 'git-tooling' }
    if ($normalized -like '.docs/*' -or $normalized -like 'worker-joins/*' -or $normalized -like '*session-contract.yaml' -or $normalized -like '*traceability*' -or $normalized -like '*closure*') { return 'evidence-docs' }
    return 'other'
}

function Test-TextCitesCommit {
    param([string]$Text, [string[]]$AcceptedCommits)
    foreach ($commit in @($AcceptedCommits)) {
        if ($commit -and $Text -match [regex]::Escape($commit)) { return $true }
    }
    return $false
}

function Test-TextHasTerminalFailure {
    param([string]$Text)
    return $Text -match '(?im)^\s*(?:verdict|final_verdict|final_closure_verdict|status|state)\s*:\s*[''\"]?(?:PENDING|stale|preliminary|BLOCKED)(?:[_A-Za-z-]*)?[''\"]?\s*(?:#.*)?$'
}

function Get-NameStatusPaths {
    param([object[]]$Output)
    $paths = New-Object System.Collections.ArrayList
    foreach ($raw in @($Output)) {
        $line = ([string]$raw).Trim()
        if (-not $line) { continue }
        $parts = $line -split "`t"
        if ($parts.Count -lt 2) { $parts = $line -split '\s+', 3 }
        if ($parts.Count -ge 2) {
            [void]$paths.Add($parts[1].Trim('"'))
            if ($parts[0] -match '^R|^C' -and $parts.Count -ge 3) { [void]$paths.Add($parts[2].Trim('"')) }
        }
    }
    return @(Get-SortedUniquePaths $paths)
}

function Get-DeletedNameStatusPaths {
    param([object[]]$Output)
    $paths = New-Object System.Collections.ArrayList
    foreach ($raw in @($Output)) {
        $line = ([string]$raw).Trim()
        if (-not $line) { continue }
        $parts = $line -split "`t"
        if ($parts.Count -lt 2) { $parts = $line -split '\s+', 3 }
        if ($parts.Count -ge 2 -and $parts[0] -match '^D(?:$|[0-9])') {
            [void]$paths.Add($parts[1].Trim('"').Replace('\\','/'))
        }
    }
    return @(Get-SortedUniquePaths $paths)
}

function Get-IgnoredPaths {
    param([object[]]$Output)
    $paths = New-Object System.Collections.ArrayList
    foreach ($raw in @($Output)) {
        $line = [string]$raw
        if ($line.Length -ge 3 -and $line.Substring(0,2) -eq '!!') {
            $path = $line.Substring(3).Trim().Trim('"')
            if ($path) { [void]$paths.Add($path) }
        }
    }
    return @(Get-SortedUniquePaths $paths)
}

function Expand-IgnoredPaths {
    param([string]$Repo, [string[]]$Paths)
    # La expansion alimenta un solo gate: detectar rutas sensibles (.env, id_rsa,
    # *.pfx/pem/key, secrets) entre los archivos ignorados. Recorrer cada directorio
    # ignorado con Get-ChildItem -Recurse costaba minutos de CPU en repos con
    # node_modules (medido: 40.869 archivos ignorados => un core al 100% sin salida),
    # porque construye un FileInfo por archivo y ordena el resultado en PowerShell.
    # git enumera lo mismo en ~1,6 s, asi que se le delega la enumeracion y aca solo
    # se filtra. Los directorios ignorados quedan colapsados en el inventario: el
    # reporte deja de arrastrar decenas de miles de paths irrelevantes.
    $expanded = New-Object System.Collections.ArrayList
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)
    $sensitivePattern = '(^|/)(\.env($|\.)|id_rsa|.*\.(pfx|pem|key)$|secrets?)(/|$)'

    $addPath = {
        param([string]$Candidate)
        if (-not $Candidate) { return }
        $normalized = $Candidate.Replace('\','/').TrimStart('/')
        if (-not $normalized) { return }
        if ($normalized -eq '.git' -or $normalized.StartsWith('.git/')) { return }
        if ($seen.Add($normalized)) { [void]$expanded.Add($normalized) }
    }

    foreach ($path in @($Paths)) { & $addPath ([string]$path) }

    # Archivos ignorados individuales: solo los que el gate de seguridad puede marcar.
    $ignoredFiles = Invoke-Git @('-C', $Repo, 'ls-files', '--others', '--ignored', '--exclude-standard')
    if ($ignoredFiles.ExitCode -eq 0) {
        foreach ($raw in @($ignoredFiles.Output)) {
            $candidate = ([string]$raw).Trim().Trim('"')
            if (-not $candidate) { continue }
            if ($candidate -match $sensitivePattern) { & $addPath $candidate }
        }
    }

    return @($expanded)
}

function Get-StatusPaths {
    param([object[]]$Output)
    $paths = New-Object System.Collections.ArrayList
    foreach ($raw in @($Output)) {
        $line = [string]$raw
        if ($line.Length -lt 3 -or $line.Substring(0,2) -eq '!!') { continue }
        $pathText = $line.Substring(3).Trim()
        if ($pathText -match ' -> ') {
            $parts = $pathText -split ' -> ', 2
            [void]$paths.Add($parts[0].Trim('"'))
            [void]$paths.Add($parts[1].Trim('"'))
        } elseif ($pathText) { [void]$paths.Add($pathText.Trim('"')) }
    }
    return @(Get-SortedUniquePaths $paths)
}

function Get-WorktreeClassification {
    param([object[]]$Output)
    $records = New-Object System.Collections.ArrayList
    $current = [ordered]@{}
    foreach ($raw in @($Output)) {
        $line = ([string]$raw).Trim()
        if (-not $line) { continue }
        if ($line -match '^worktree (.+)$') {
            if ($current.Count -gt 0) { [void]$records.Add([pscustomobject]$current) }
            $current = [ordered]@{ path=$Matches[1]; branch=''; head=''; classification='unknown' }
        } elseif ($line -match '^HEAD (.+)$') { $current.head = $Matches[1] }
        elseif ($line -match '^branch (.+)$') { $current.branch = $Matches[1] }
        elseif ($line -eq 'bare') { $current.classification = 'bare' }
        elseif ($line -eq 'detached') { $current.classification = 'detached' }
        elseif ($line -match '^prunable') { $current.classification = 'prunable' }
        elseif ($line -match '^locked') { $current.classification = 'locked' }
    }
    if ($current.Count -gt 0) { [void]$records.Add([pscustomobject]$current) }
    foreach ($record in @($records)) {
        if ($record.classification -eq 'unknown') { $record.classification = if ($record.branch) { 'attached' } else { 'unclassified' } }
    }
    return @($records)
}

function Get-BranchClassification {
    param([object[]]$Output, [string]$CurrentBranch, [object[]]$Worktrees, [object]$MergedBranches)
    $records = New-Object System.Collections.ArrayList
    $attachedBranches = @($Worktrees | ForEach-Object { [string]$_.branch } | Where-Object { $_ })
    foreach ($raw in @($Output)) {
        $line = ([string]$raw).Trim()
        if (-not $line) { continue }
        $parts = $line -split '\|', 3
        $name = [string]$parts[0]
        if (-not $name) { continue }
        $classification = if ($name -eq $CurrentBranch) { 'current' } elseif ($attachedBranches -contains ('refs/heads/' + $name)) { 'attached-worktree' } else { 'local' }
        if ($MergedBranches.Contains($name) -and $classification -ne 'current') { $classification = 'merged-into-base' }
        elseif ($classification -eq 'local') { $classification = 'local-unmerged' }
        [void]$records.Add([pscustomobject]@{ name=$name; marker=([string]$parts[1]); upstream=([string]$parts[2]); classification=$classification })
    }
    return @($records)
}

function Get-ExitCodeName {
    param([int]$Code)
    switch ($Code) {
        0 { return 'success' }
        1 { return 'validation_failure' }
        2 { return 'infrastructure_error' }
        3 { return 'configuration_error' }
        4 { return 'explicit_incomplete' }
        default { return 'guard_error' }
    }
}

function Test-SingleClosurePacketDigest {
    param([string]$EvidenceText, [string[]]$DiffOutput)
    if ($EvidenceText -notmatch '(?m)^\s*combined_pass:\s*true' -and $EvidenceText -notmatch 'trace_audit:[\s\S]*combined_pass:\s*true') {
        return @('single closure packet missing trace_audit.combined_pass: true')
    }
    $digest = $null
    if ($EvidenceText -match '(?m)diff_digest:\s*([0-9a-f]{64})') { $digest = $Matches[1].ToLowerInvariant() }
    if (-not $digest) { return @('single closure packet missing verification_basis.diff_digest') }
    $diffText = ([string](@($DiffOutput) -join "`n"))
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes($diffText)
        $computed = ([System.BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant()
    } finally { $sha.Dispose() }
    if ($computed -ne $digest) { return @('single closure packet diff_digest stale vs current git diff') }
    return @()
}

function Show-Help {
    @'
ae-pre-push: deterministic push safety gate (PowerShell 5.1 compatible)
Usage:
  Invoke-PrePushGuard.ps1 -IssueKey TES-123 -ExpectedScope 'infra/**' \
    -SessionContract .docs/auditoria/session-contract.yaml \
    -TraceabilityEvidence .docs/auditoria/verification.yaml [-Json]
  Invoke-PrePushGuard.ps1 -IssueKey waiver -SingleClosurePacket .docs/auditoria/<session>/closure-packet.yaml [-Json]
  Invoke-PrePushGuard.ps1 -PartialPublicationPacket .docs/auditoria/<session>/partial-publication.json [-SessionContract PATH] [-Json]

Switches:
  -IssueKey, -LinearUrl, -WaiverReason   ticket identity or explicit waiver
  -ExpectedScope PATTERN                 one or more scope names; comma-separated accepted
  -SessionContract PATH                  governed contract with allowed/forbidden paths
  -TraceabilityEvidence PATH             PASS/audit/closure evidence files
  -SingleClosurePacket, -ClosurePacket    combined trace+audit closure packet (v4 FAST)
  -SessionAuthorizations PATH             optional session-state authorization export JSON
  -PartialPublicationPacket PATH         observed-only v0 partial boundary; never grants publication
  -BaseRef REF -Remote NAME              ancestry target and fetch remote (default origin/main)
                                         content scan is BaseRef..HEAD plus non-ignored pending paths
  -GitTimeoutSeconds N                    per-git-process timeout, including scan rev-list/diff (default 30; timeout fails closed)
  -ScannerTimeoutSeconds N                per-gitleaks-process timeout (default 30; timeout fails closed). Excluded from the solo 5s infrastructure budget; does not raise that cap
  -FirstPush                              governed empty-origin mode; validates only and never pushes
  -SharedSkillName PATH                  changed skill path(s), checked against both mirrors
  -NoFetch                               diagnostic label only; it never suppresses mandatory fetch
  -DryRun                                diagnostic label only; mandatory fetch and hard gates still run
  -Json                                  emit a machine-readable report
  -Help, -h                              show this help

Exit codes: 0 success, 1 validation failure, 2 infrastructure error, 3 configuration error.
'@ | Write-Output
}

if ($Help) { Show-Help; exit 0 }

$blockers = New-Object System.Collections.ArrayList
$warnings = New-Object System.Collections.ArrayList
$configErrors = New-Object System.Collections.ArrayList
$infraErrors = New-Object System.Collections.ArrayList
$infoDropped = New-Object System.Collections.ArrayList
$joinEligibility = New-Object System.Collections.ArrayList
$ownershipFindings = New-Object System.Collections.ArrayList
$laneContextActive = $false
$contractLaneContext = $null
$contract = $null
$deliveryContract = $null
$deliveryContractActive = $false
$deliveryContractPath = ''
$repo = if (-not [string]::IsNullOrWhiteSpace($script:GuardRepositoryRoot)) { $script:GuardRepositoryRoot } else { (Get-Location).Path }
$actualBranch = ''
$head = ''
$base = ''
$baseIsAncestor = $null
$fetchResult = $null
$remoteIdentity = ''
$remoteRefInventory = @()
$remoteZeroRefs = $false
$remoteObservedAt = ''
$firstPushObservationCommand = 'git ls-remote --refs origin'
$firstPushContractValid = $false

$rootResult = Invoke-Git @('rev-parse', '--show-toplevel')
if ($rootResult.ExitCode -eq 0) { $repo = ([string]$rootResult.Output[0]).Trim() }
elseif (-not [string]::IsNullOrWhiteSpace($script:GuardRepositoryRoot)) { $repo = $script:GuardRepositoryRoot }
else { Add-Item $infraErrors 'not inside a git repository' }

# .ae.yaml is the executable delivery authority when present. Legacy repositories
# without this file retain the historical guard path; a present contract is never
# advisory and is validated before branch/remote policy is evaluated.
$deliveryContractPath = Join-Path $repo '.ae.yaml'
$deliveryModule = Join-Path $PSScriptRoot 'KernelDeliveryContract.psm1'
if (Test-Path -LiteralPath $deliveryContractPath -PathType Leaf) {
    try {
        if (-not (Test-Path -LiteralPath $deliveryModule -PathType Leaf)) { throw 'KernelDeliveryContract.psm1 is missing' }
        Import-Module -Name $deliveryModule -Force -ErrorAction Stop
        $deliveryContract = Read-AeDeliveryContract -Path $deliveryContractPath -RepositoryRoot $repo
        if (-not $deliveryContract.Valid) {
            foreach ($deliveryError in @($deliveryContract.Errors)) { Add-Item $configErrors $deliveryError }
        } else {
            $deliveryContractActive = $true
            if ($deliveryContract.Product -ne 'ae-kernel') { Add-Item $configErrors 'Kernel delivery contract product must be exactly ae-kernel' }
            if ($deliveryContract.Repository -ne 'ae-kernel') { Add-Item $configErrors 'Kernel delivery contract repository must be exactly ae-kernel' }
            if ($deliveryContract.SourceRoot -ne 'skills') { Add-Item $configErrors 'Kernel delivery contract source_root must be exactly skills/' }
        }
    } catch { Add-Item $configErrors ('delivery contract could not be loaded: ' + $_.Exception.Message) }
}
if ($deliveryContractActive) {
    $remoteIdentity = Test-DeliveryRemoteIdentity -RemoteName $Remote -Product $deliveryContract.Product -Errors $configErrors
    foreach ($mirror in @($deliveryContract.DistributionMirrors)) {
        $mirrorName = if ($mirror.PSObject.Properties.Name -contains 'name') { [string]$mirror.name } else { '<unnamed>' }
        Add-Item $blockers ('publication distribution mirror identity_unverified: ' + $mirrorName)
    }
} else {
    $legacyFetch = Invoke-Git @('remote', 'get-url', '--all', $Remote)
    $legacyPush = Invoke-Git @('remote', 'get-url', '--all', '--push', $Remote)
    $legacyUrls = @($legacyFetch.Output + $legacyPush.Output | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
    foreach ($knownProduct in @('ae-kernel','claude-skills')) {
        if (@($legacyUrls | Where-Object { (Normalize-DeliveryRemoteIdentity $_) -eq (Get-CanonicalDeliveryRemote $knownProduct) }).Count -gt 0) {
            Add-Item $configErrors 'canonical delivery repository requires .ae.yaml; removing the contract cannot select the legacy path'
            break
        }
    }
}

$branchResult = Invoke-Git @('branch', '--show-current')
if ($branchResult.ExitCode -eq 0) { $actualBranch = ([string]$branchResult.Output[0]).Trim() }
$headResult = Invoke-Git @('rev-parse', 'HEAD')
if ($headResult.ExitCode -eq 0) { $head = ([string]$headResult.Output[0]).Trim() }
else { Add-Item $infraErrors 'HEAD is not resolvable' }

if ($FirstPush) {
    if ($deliveryContractActive) { Add-Item $configErrors 'FirstPush cannot override an active .ae.yaml delivery contract' }
    if ($actualBranch -ne 'main') { Add-Item $configErrors 'FirstPush requires current branch exactly main' }
    if ($Remote -ne 'origin') { Add-Item $configErrors 'FirstPush requires Remote exactly origin' }
    if ($BaseRef -ne 'origin/main') { Add-Item $configErrors 'FirstPush retains BaseRef origin/main as the target identity' }
    if ($NoFetch -or $DryRun) { Add-Item $warnings 'NoFetch/DryRun does not suppress FirstPush remote observation or hard gates' }
} else {
    if ($deliveryContractActive) {
        if ($deliveryContract.Remote -ne $Remote) { Add-Item $configErrors ('delivery contract remote mismatch: ' + $deliveryContract.Remote + ' != ' + $Remote) }
        $expectedContractBase = $deliveryContract.Remote + '/' + $deliveryContract.Base
        if ($BaseRef -ne $expectedContractBase) { Add-Item $configErrors ('delivery contract requires BaseRef exactly ' + $expectedContractBase) }
        if ($deliveryContract.Mode -eq 'mainline' -and $actualBranch -ne 'main') { Add-Item $configErrors 'mainline delivery requires current branch exactly main' }
        if ($deliveryContract.Mode -eq 'pull_request' -and [string]::IsNullOrWhiteSpace($actualBranch)) { Add-Item $configErrors 'pull_request delivery requires a checked-out branch; detached HEAD is forbidden' }
        if ($deliveryContract.Mode -eq 'pull_request' -and @('main','master') -contains $actualBranch) { Add-Item $configErrors 'pull_request delivery forbids direct publication from protected main/master' }
    } else {
        if (@('main','master') -notcontains $actualBranch) { Add-Item $configErrors 'ordinary mode requires current branch exactly main or master' }
        if ($Remote -ne 'origin') { Add-Item $configErrors 'ordinary mode requires Remote exactly origin' }
        $expectedProtectedBase = if (@('main','master') -contains $actualBranch) { 'origin/' + $actualBranch } else { '' }
        if (-not $expectedProtectedBase -or $BaseRef -ne $expectedProtectedBase) { Add-Item $configErrors ('ordinary mode requires BaseRef exactly origin/' + $actualBranch) }
    }
    $fetchRef = $BaseRef
    if ($BaseRef -match '^([^/]+)/(.+)$' -and $Matches[1] -eq $Remote) { $fetchRef = $Matches[2] }
    $fetchResult = Invoke-Git @('fetch', '--no-tags', $Remote, $fetchRef)
    if ($fetchResult.ExitCode -ne 0) { Add-Item $infraErrors (Join-Output $fetchResult.Output) }
    if ($NoFetch -or $DryRun) { Add-Item $warnings 'NoFetch/DryRun is diagnostic-only; mandatory fetch still executed' }
    $baseResult = Invoke-Git @('rev-parse', $BaseRef)
    if ($baseResult.ExitCode -eq 0) {
        $base = ([string]$baseResult.Output[0]).Trim()
        if ($base -notmatch '^[0-9a-fA-F]{40}$') { Add-Item $infraErrors ('base_sha is not a valid full commit SHA: ' + $base) }
    } else { Add-Item $infraErrors ('base ref not found: ' + $BaseRef) }
}
if ($head -and $head -notmatch '^[0-9a-fA-F]{40}$') { Add-Item $infraErrors ('HEAD is not a valid full commit SHA: ' + $head) }
if (-not $FirstPush -and $base -and $head) {
    $ancestor = Invoke-Git @('merge-base', '--is-ancestor', $base, $head)
    $baseIsAncestor = ($ancestor.ExitCode -eq 0)
    if ($ancestor.ExitCode -ne 0) { Add-Item $blockers ($BaseRef + ' is not an ancestor of HEAD; rebase or merge the current base before pushing') }
}

$changed = @()
$diffOutput = @()
$deletedFromHead = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)
if ($FirstPush -and $head) {
    $tree = Invoke-Git @('ls-tree', '-r', '--name-only', 'HEAD')
    if ($tree.ExitCode -eq 0) {
        $changed = @(Get-SortedUniquePaths $tree.Output)
        $diffOutput = @($changed | ForEach-Object { 'A' + "`t" + $_ })
    } else { Add-Item $infraErrors 'git tree inventory failed for empty-remote base semantics' }
} elseif ($base -and $head) {
    $diff = Invoke-Git @('diff', '--name-status', '-M', ($base + '...' + $head))
    if ($diff.ExitCode -eq 0) {
        $diffOutput = @($diff.Output | ForEach-Object { [string]$_ })
        $changed = @(Get-NameStatusPaths $diff.Output)
        foreach ($deletedPath in @(Get-DeletedNameStatusPaths $diff.Output)) { [void]$deletedFromHead.Add([string]$deletedPath) }
    }
    else { Add-Item $infraErrors 'git diff inventory failed' }
}
$porcelain = Invoke-Git @('status', '--porcelain=v1', '--untracked-files=all', '--ignored=matching')
# Get-IgnoredPaths se calcula UNA vez. Evaluarlo dentro del Where-Object lo re-ejecutaba
# por cada path del working tree: con un directorio grande sin trackear (medido: 105.419
# archivos) eso son 105.419 reconstrucciones del inventario y el guard no termina nunca.
$ignoredDeclared = if ($porcelain.ExitCode -eq 0) { @(Get-IgnoredPaths $porcelain.Output) } else { @() }
$ignoredLookup = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::Ordinal)
foreach ($ignoredPath in $ignoredDeclared) { [void]$ignoredLookup.Add([string]$ignoredPath) }
$working = if ($porcelain.ExitCode -eq 0) { @(Get-StatusPaths $porcelain.Output | Where-Object { -not $ignoredLookup.Contains([string]$_) }) } else { Add-Item $infraErrors 'working-tree inventory failed'; @() }
$ignored = if ($porcelain.ExitCode -eq 0) { @(Expand-IgnoredPaths $repo $ignoredDeclared) } else { @() }
$untracked = if ($porcelain.ExitCode -eq 0) {
    @(Get-SortedUniquePaths (@($porcelain.Output | ForEach-Object {
        $statusLine = [string]$_
        if ($statusLine.Length -ge 3 -and $statusLine.Substring(0,2) -eq '??') { $statusLine.Substring(3).Trim().Trim('"') }
    })))
} else { @() }
# Ignored files remain a separately audited security inventory. They are not
# changes and must never widen scope, create fake paths, or alter the evidence gate.
$allChanged = @(Get-SortedUniquePaths (@($changed) + @($working)))
# Un path ignorado no puede entrar en el push: git no lo incluye. Bloquear por su sola
# existencia hacia inusable el guard en cualquier repo con un infra/.env legitimo, que es
# justamente la practica correcta. Queda en el inventario auditado como warning.
# Lo que si bloquea es un path sensible SIN TRACKEAR y NO ignorado: ese entra con un
# `git add -A` y es el riesgo real de push.
$sensitivePathPattern = '(^|/)(\.env($|\.)|id_rsa|.*\.(pfx|pem|key)$|secrets?)(/|$)'
foreach ($path in $ignored) {
    if ($path -match $sensitivePathPattern) { Add-Item $warnings ('ignored sensitive path stays out of the push: ' + $path) }
}
foreach ($path in $untracked) {
    if ($path -match $sensitivePathPattern) { Add-Item $blockers ('untracked sensitive path is not push-safe: ' + $path) }
}
Invoke-OutgoingContentScan -Repo $repo -BaseSha $base -HeadSha $head -RangeLabel $BaseRef -FirstPush ([bool]$FirstPush) -PendingInventoryOk ($porcelain.ExitCode -eq 0) -UntrackedPaths $untracked -Blockers $blockers -Infra $infraErrors -ConfigErrors $configErrors -Warnings $warnings
$acceptedEvidenceCommits = @($head | Where-Object { $_ })
if ($head) {
    $parentResult = Invoke-Git @('rev-parse', 'HEAD^')
    if ($parentResult.ExitCode -eq 0) {
        $parentHead = ([string]$parentResult.Output[0]).Trim()
        $commitDiff = Invoke-Git @('diff', '--name-status', '-M', $parentHead, $head)
        if ($commitDiff.ExitCode -eq 0) {
            $commitPaths = @(Get-NameStatusPaths $commitDiff.Output)
            $evidenceOnlyCommit = $commitPaths.Count -gt 0
            foreach ($commitPath in $commitPaths) {
                if ((Get-ScopeForPath $commitPath) -ne 'evidence-docs') { $evidenceOnlyCommit = $false; break }
            }
            if ($evidenceOnlyCommit) { $acceptedEvidenceCommits += $parentHead }
        }
    }
}
$acceptedEvidenceCommits = @($acceptedEvidenceCommits | Where-Object { $_ } | Sort-Object -Unique)
if ($guardV2Loaded -and ($guardV2Commands -contains 'Get-AeGuardFingerprint')) {
    try {
        $fingerprintInputs = @($SessionContract) + @($TraceabilityEvidence)
        $guardV2Fingerprint = Get-AeGuardFingerprint -PolicyVersion 'ae-pre-push/v2' -GateId 'hard-gates' -Command 'Invoke-PrePushGuard.ps1' -Arguments @('-BaseRef', $BaseRef, '-Remote', $Remote) -InputPath $fingerprintInputs -BasePath $repo
    } catch { $guardV2Reason = 'fingerprint_failed'; $guardV2Loaded = $false; $guardV2Mode = 'fallback' }
}
$worktreeInventory = Invoke-Git @('worktree', 'list', '--porcelain')
$branchInventory = Invoke-Git @('for-each-ref', '--format=%(refname:short)|%(HEAD)|%(upstream:short)', 'refs/heads')
$mergedBranchInventory = if ($FirstPush) { [pscustomobject]@{ ExitCode=0; Output=@() } } else { Invoke-Git @('for-each-ref', ('--merged=' + $BaseRef), '--format=%(refname:short)', 'refs/heads') }
$mergedBranches = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::Ordinal)
foreach ($mergedBranch in @($mergedBranchInventory.Output)) {
    $name = ([string]$mergedBranch).Trim()
    if ($name) { [void]$mergedBranches.Add($name) }
}
$worktreeClassification = Get-WorktreeClassification $worktreeInventory.Output
$branchClassification = Get-BranchClassification $branchInventory.Output $actualBranch $worktreeClassification $mergedBranches
if ($worktreeInventory.ExitCode -ne 0) { Add-Item $infraErrors 'worktree inventory failed' }
if ($branchInventory.ExitCode -ne 0) { Add-Item $infraErrors 'branch inventory failed' }
if ($mergedBranchInventory.ExitCode -ne 0) { Add-Item $infraErrors 'merged branch inventory failed' }
foreach ($worktree in @($worktreeClassification)) {
    $worktreePath = [System.IO.Path]::GetFullPath([string]$worktree.path).TrimEnd([char[]]@('\', '/'))
    $repoPath = [System.IO.Path]::GetFullPath($repo).TrimEnd([char[]]@('\', '/'))
    $isCurrentWorktree = [string]::Equals($worktreePath, $repoPath, [System.StringComparison]::OrdinalIgnoreCase)
    if ($isCurrentWorktree -and [string]$worktree.classification -ne 'attached') {
        Add-Item $blockers ('current worktree classification is not push-safe: ' + $worktree.path + ' [' + $worktree.classification + ']')
    } elseif (-not $isCurrentWorktree -and [string]$worktree.classification -ne 'attached') {
        Add-Item $warnings ('non-current worktree requires separate disposition and was not mutated: ' + $worktree.path + ' [' + $worktree.classification + ']')
    }
}

$contractText = ''
$contractExists = $false
if (-not [string]::IsNullOrWhiteSpace($SessionContract)) {
    $contractPath = if ([System.IO.Path]::IsPathRooted($SessionContract)) { $SessionContract } else { Join-Path $repo $SessionContract }
    if (Test-Path -LiteralPath $contractPath -PathType Leaf) {
        $contractExists = $true
        $contractText = Get-Content -LiteralPath $contractPath -Raw
        $contract = Get-ContractModel $contractText
    }
}

$scopes = @($ExpectedScope | ForEach-Object { ([string]$_).Split(',') } | ForEach-Object { $_.Trim() } | Where-Object { $_ } | Sort-Object -Unique)
foreach ($scopePattern in $ExpectedScope) {
    if ([string]$scopePattern -match '[\[\]?*]') { continue }
}
if ($allChanged.Count -gt 0 -and $scopes.Count -eq 0) { Add-Item $blockers 'ExpectedScope is required when changes are present; pass one or more scope names' }
foreach ($path in $allChanged) {
    $scope = Get-ScopeForPath $path
    $scopeMatch = $false
    foreach ($pattern in $scopes) {
        if ($pattern -eq $scope -or (Test-PathPattern $path $pattern)) { $scopeMatch = $true; break }
    }
    if (-not $scopeMatch) { Add-Item $blockers ('changed path is outside ExpectedScope (' + $scope + '): ' + $path) }
    if ($path -match '^\.docs/raw/plans/') {
        $allowedPatterns = if ($contract) { @($contract.Allowed) } else { @() }
        if (-not (Test-GovernedRawPlan $repo $path $allowedPatterns)) { Add-Item $blockers ('raw plan is not governed and explicitly authorized by session contract: ' + $path) }
    }
    if ($path -match '(^|/)(\.env($|\.)|id_rsa|.*\.(pfx|pem|key)$|secrets?)(/|$)') { Add-Item $blockers ('secret-like path is not pushable: ' + $path) }
}
$dangerous = @($porcelain.Output | ForEach-Object {
    $line = [string]$_
    if ($line.Length -ge 3 -and $line.Substring(0,2) -eq '??') { $line.Substring(3).Trim() }
} | Where-Object { $_ -and $_ -match '^(src/|src\\)' -and $_ -match '(test-results|playwright-report|\.next|coverage|dist|build|node_modules)(/|\\|$)' })
foreach ($path in $dangerous) { Add-Item $blockers ('dangerous untracked artifact under src: ' + $path) }

$significant = $allChanged.Count -gt 0
$partialBoundary = $null
$partialPublicationAuthorizedExplicitIncomplete = $false
$partialSourceV0 = Get-PartialPublicationSourceV0
if (-not [string]::IsNullOrWhiteSpace($PartialPublicationPacket)) {
    if (-not (Test-RepoRelativePath $PartialPublicationPacket) -or $PartialPublicationPacket -match '(?i)^\.docs/raw/') {
        Add-Item $configErrors 'partial publication packet must be repo-relative and outside .docs/raw'
    } else {
        $partialPath = Join-Path $repo $PartialPublicationPacket
        if (-not (Test-Path -LiteralPath $partialPath -PathType Leaf)) { Add-Item $blockers ('partial publication packet missing: ' + $PartialPublicationPacket) }
        else {
            $partialBoundary = Get-PartialPublicationBoundary (Get-Content -LiteralPath $partialPath -Raw) $partialSourceV0
            foreach ($missingField in @($partialBoundary.missing)) { Add-Item $configErrors ('partial publication boundary missing/ambiguous: ' + $missingField) }
            if (@($partialBoundary.missing).Count -eq 0 -and $null -ne $partialSourceV0) { $partialPublicationAuthorizedExplicitIncomplete = $true }
        }
    }
}
$hasExplicitWaiver = -not [string]::IsNullOrWhiteSpace($WaiverReason)
$expectedIssues = Get-ExpectedIssueKeys $repo
$trackerExternal = [bool]$expectedIssues.External
if ($trackerExternal -and $significant -and [string]::IsNullOrWhiteSpace($IssueKey) -and [string]::IsNullOrWhiteSpace($LinearUrl) -and -not $hasExplicitWaiver) { Add-Item $blockers 'IssueKey, LinearUrl, or WaiverReason is required for externally tracked changed work' }
if ($trackerExternal -and $significant -and $IssueKey -and -not (Test-IssueKeyRule $expectedIssues.AgentRule $IssueKey)) { Add-Item $blockers ('IssueKey must be the agent issue ' + $expectedIssues.AgentRule.Describe) }

if (-not [string]::IsNullOrWhiteSpace($SessionContract) -and -not $contractExists) {
    Add-Item $blockers ('session contract not found: ' + $SessionContract + '; provide a tracked session-contract.yaml')
} elseif ([string]::IsNullOrWhiteSpace($SessionContract) -and $significant -and -not $partialPublicationAuthorizedExplicitIncomplete) {
    Add-Item $blockers 'SessionContract is required for changed work'
}

if ($contractExists) {
    if ($contractText -match "`t") { Add-Item $configErrors 'session contract contains tabs; use spaces only' }
    $contract = Get-ContractModel $contractText
    $contractLaneContext = $contract.LaneContext
    $laneContextActive = [bool]$contractLaneContext.Declared
    if (@('ae-session-contract/v1', 'ae-session-contract/v2') -cnotcontains $contract.Schema) { Add-Item $configErrors 'session contract schema must be exactly ae-session-contract/v1 or ae-session-contract/v2; unknown versions are not adapted' }
    foreach ($compatibilityError in @($contract.CompatibilityErrors)) { Add-Item $configErrors $compatibilityError }
    if ($laneContextActive) {
        foreach ($lineageError in @(Get-LaneContextCompletenessErrors $contractLaneContext 'session contract')) { Add-Item $configErrors $lineageError }
    }
    if (@($contract.DuplicateKeys).Count -gt 0) { Add-Item $configErrors ('session contract has duplicate/ambiguous keys: ' + ([string]::Join(',', [string[]]$contract.DuplicateKeys))) }
    if (-not $contract.HasScope) { Add-Item $configErrors 'session contract missing scope section' }
    if (-not $contract.HasBudget) { Add-Item $configErrors 'session contract missing ae_budget_gate section' }
    if (@($contract.Anchors).Count -eq 0) { Add-Item $configErrors 'session contract missing anchors list' }
    if (@($contract.Allowed).Count -eq 0) { Add-Item $configErrors 'session contract missing scope.allowed_paths/permitido list' }
    if (@($contract.Forbidden).Count -eq 0) { Add-Item $configErrors 'session contract missing forbidden list' }
    if ($deliveryContractActive -and $deliveryContract.Mode -eq 'pull_request' -and [string]::IsNullOrWhiteSpace($contract.Branch)) { Add-Item $configErrors 'pull_request delivery requires an explicit session contract branch' }
    if ($contract.Branch -and $contract.Branch -ne $actualBranch) { Add-Item $blockers ('session contract branch mismatch: ' + $contract.Branch + ' != ' + $actualBranch) }
    $contractBase = if ($contract.Base) { $contract.Base } else { $contract.ScopeBase }
    if ($contractBase -and $contractBase -ne $BaseRef) { Add-Item $blockers ('session contract base mismatch: ' + $contractBase + ' != ' + $BaseRef) }
    if ($contract.BaseSha -and $contract.BaseSha -notmatch '^[0-9a-fA-F]{40}$') { Add-Item $configErrors 'session contract base_sha must be a full 40-hex commit SHA' }
    elseif ($contract.BaseSha -and $base -and $contract.BaseSha -ne $base) { Add-Item $blockers 'session contract base_sha does not match fetched base ref' }
    if ($FirstPush) {
        if ($contract.FirstPushEnabled -ine 'true') { Add-Item $configErrors 'FirstPush requires first_push.enabled: true' }
        if ($contract.FirstPushRemote -ne 'origin') { Add-Item $configErrors 'FirstPush contract remote must be exactly origin' }
        if ($contract.FirstPushTargetBranch -ne 'main') { Add-Item $configErrors 'FirstPush contract target_branch must be exactly main' }
        if ($contract.FirstPushRemoteEmptyRequired -ine 'true') { Add-Item $configErrors 'FirstPush requires remote_empty_required: true' }
        if ($contract.FirstPushBaseSemantics -ne 'empty_remote_no_base') { Add-Item $configErrors 'FirstPush base_semantics must be empty_remote_no_base' }
        if ($contract.BaseSha) { Add-Item $configErrors 'FirstPush empty_remote_no_base contract must not declare repository.base_sha' }
        if ($contract.FirstPushAuthorized -ine 'true') { Add-Item $configErrors 'FirstPush requires explicit authorization.authorized: true' }
        if ([string]::IsNullOrWhiteSpace([string]$contract.FirstPushAuthorizedBy)) { Add-Item $configErrors 'FirstPush authorization.authorized_by is required' }
        $authorizationDate = [datetime]::MinValue
        if (-not [datetime]::TryParse($contract.FirstPushAuthorizedAt, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::AssumeUniversal, [ref]$authorizationDate)) { Add-Item $configErrors 'FirstPush authorization.authorized_at must be an ISO timestamp' }
        elseif (([datetime]::UtcNow - $authorizationDate.ToUniversalTime()).TotalHours -gt 24 -or $authorizationDate.ToUniversalTime() -gt [datetime]::UtcNow.AddMinutes(5)) { Add-Item $blockers 'FirstPush authorization is outside freshness window of 24 hours' }
        if ([string]::IsNullOrWhiteSpace([string]$contract.FirstPushHandoffOwner)) { Add-Item $configErrors 'FirstPush post_push_verification_handoff.owner is required' }
        if ($contract.FirstPushHandoffCommand -ne 'git ls-remote --refs origin') { Add-Item $configErrors 'FirstPush post-push handoff command must be exactly git ls-remote --refs origin' }
        if ($contract.FirstPushExpectedRef -ne 'refs/heads/main') { Add-Item $configErrors 'FirstPush post-push expected_ref must be refs/heads/main' }
        if ($contract.FirstPushExpectedSha -ne 'approved_head') { Add-Item $configErrors 'FirstPush post-push expected_sha must be approved_head' }
        if ($contract.FirstPushUnexpectedRefs -ne 'forbidden') { Add-Item $configErrors 'FirstPush post-push unexpected_refs must be forbidden' }
        if ([string]::IsNullOrWhiteSpace([string]$contract.FirstPushRaceOwner)) { Add-Item $configErrors 'FirstPush post-push TOCTOU/race owner is required' }
        $firstPushContractValid = ($configErrors.Count -eq 0)
    }
    $waiverTopology = @('not_applicable', 'none', 'n/a')
    $contractParentIssue = ([string]$contract.ParentIssue).ToLowerInvariant()
    $contractAgentIssue = ([string]$contract.AgentIssue).ToLowerInvariant()
    if ($trackerExternal) {
        if ($hasExplicitWaiver) {
            if (-not $contract.TrackerWaiver) { Add-Item $configErrors 'explicit waiver requires session contract tracker_waiver' }
            if ($waiverTopology -notcontains $contractParentIssue -or $waiverTopology -notcontains $contractAgentIssue) { Add-Item $configErrors 'waived session contract must declare issues.parent/agent as not_applicable' }
        } elseif (-not (Test-IssueKeyRule $expectedIssues.ParentRule $contract.ParentIssue) -or -not (Test-IssueKeyRule $expectedIssues.AgentRule $contract.AgentIssue)) {
            Add-Item $configErrors ('session contract must declare issues.parent=' + $expectedIssues.ParentRule.Describe + ' and issues.agent=' + $expectedIssues.AgentRule.Describe + ' unless an explicit waiver is supplied')
        }
        if (-not $contract.TicketFrontier -and -not $hasExplicitWaiver -and $contractText -notmatch '(?i)(ticket_frontier|frontera_ticket)') { Add-Item $configErrors 'session contract must declare ticket_frontier/frontera_ticket unless an explicit waiver is supplied' }
    }
    if ($significant -and @($contract.EvidencePaths).Count -eq 0 -and @($contract.RequiredEvidence).Count -eq 0) { Add-Item $configErrors 'session contract must declare evidence_paths or required_evidence' }
    foreach ($evidencePath in @($contract.EvidencePaths)) {
        if (-not (Test-RepoRelativePath $evidencePath)) { Add-Item $configErrors ('structured evidence path must be repo-relative: ' + $evidencePath); continue }
        $contractEvidencePath = Join-Path $repo $evidencePath
        if (-not (Test-Path -LiteralPath $contractEvidencePath -PathType Leaf)) { Add-Item $blockers ('structured contract evidence missing: ' + $evidencePath) }
        else {
            $contractEvidenceText = Get-Content -LiteralPath $contractEvidencePath -Raw
            $evidenceContext = Get-LaneContextModel $contractEvidenceText 'evidence'
            $lineage = Test-LaneLineage $evidenceContext $contractLaneContext $laneContextActive ('structured contract evidence ' + $evidencePath)
            foreach ($info in @($lineage.Info)) { [void]$infoDropped.Add($info) }
            foreach ($lineageError in @($lineage.Errors)) { Add-Item $blockers $lineageError }
            if (-not $lineage.Dropped) {
                $metadata = Get-StructuredMetadata $evidencePath $contractEvidenceText 'evidence' $acceptedEvidenceCommits $base 24 ([bool]$FirstPush)
                foreach ($metadataError in @($metadata.Errors)) { Add-Item $blockers ('structured contract evidence invalid (' + $evidencePath + '): ' + $metadataError) }
                if ((Test-TextHasTerminalFailure $contractEvidenceText) -or ($head -and -not (Test-TextCitesCommit $contractEvidenceText $acceptedEvidenceCommits))) { Add-Item $blockers ('structured contract evidence is not fresh for current implementation HEAD: ' + $evidencePath) }
            }
        }
    }
    $workerNoneJustified = ($contract.WorkerDecision -eq 'none') -and (Test-YamlKeyHasContent $contractText 'why_no_worker')
    if ($significant -and $contract.WorkerDecision -ne 'spawned' -and -not $workerNoneJustified) { Add-Item $blockers 'worker_decision must be spawned, or none with a documented why_no_worker' }
    if ($contract.WorkerDecision -eq 'spawned') {
        if (@($contract.JoinPaths).Count -eq 0 -and $contractText -notmatch '(?i)worker-joins') { Add-Item $configErrors 'spawned worker evidence must declare structured join_packets/durable_join_paths; the aggregate closure packet is the sole join output' }
        foreach ($join in @($contract.JoinPaths)) {
            if (-not (Test-RepoRelativePath $join)) { Add-Item $configErrors ('durable join or aggregate closure path must be repo-relative: ' + $join); continue }
            $joinPath = Join-Path $repo $join
            if (-not (Test-Path -LiteralPath $joinPath -PathType Leaf)) { Add-Item $blockers ('durable worker join or aggregate closure packet missing: ' + $join) }
            else {
                $joinText = Get-Content -LiteralPath $joinPath -Raw
                $joinContext = Get-LaneContextModel $joinText 'join'
                $lineage = Test-LaneLineage $joinContext $contractLaneContext $laneContextActive ('durable worker join or aggregate closure ' + $join)
                foreach ($info in @($lineage.Info)) { [void]$infoDropped.Add($info) }
                foreach ($lineageError in @($lineage.Errors)) { Add-Item $blockers $lineageError }
                [void]$joinEligibility.Add([ordered]@{ path=$join; eligible=[bool]$lineage.Eligible; dropped=[bool]$lineage.Dropped; reason=if ($lineage.Dropped) { 'INFO_DROPPED' } elseif (@($lineage.Errors).Count -gt 0) { 'lineage_blocked' } else { 'eligible' } })
                if (-not $lineage.Dropped) {
                    $metadata = Get-StructuredMetadata $join $joinText 'join' $acceptedEvidenceCommits $base 24 ([bool]$FirstPush)
                    foreach ($metadataError in @($metadata.Errors)) { Add-Item $blockers ('durable worker join or aggregate closure invalid (' + $join + '): ' + $metadataError) }
                    if ((Test-TextHasTerminalFailure $joinText) -or -not (Test-TextCitesCommit $joinText $acceptedEvidenceCommits)) { Add-Item $blockers ('durable worker join or aggregate closure is not fresh for accepted implementation HEAD: ' + $join) }
                }
            }
        }
    }
    foreach ($pattern in @($contract.Allowed + $contract.Forbidden)) {
        if (-not (Test-GlobPattern $pattern)) { Add-Item $configErrors ('invalid contract glob pattern: ' + $pattern) }
    }
    foreach ($path in $allChanged) {
        $allowedMatch = $false
        foreach ($pattern in $contract.Allowed) { if (Test-PathPattern $path $pattern) { $allowedMatch = $true; break } }
        if (-not $allowedMatch) { Add-Item $blockers ('changed path is outside session contract scope: ' + $path) }
        foreach ($pattern in $contract.Forbidden) { if (Test-PathPattern $path $pattern) { Add-Item $blockers ('changed path is forbidden by session contract: ' + $path) } }
    }
}

# Dirty paths owned by another lane remain a hard HOLD. Ownership is reported
# separately, but unresolved dirty state can never produce an Approved verdict.
if ($contract) {
    foreach ($dirtyPath in @($working | Where-Object { $working -contains $_ -and -not ($changed -contains $_) })) {
        $ownedHere = $false
        foreach ($pattern in @($contract.Allowed)) { if (Test-PathPattern $dirtyPath $pattern) { $ownedHere = $true; break } }
        if (-not $ownedHere -and $dirtyPath -notmatch '(?i)(^|/)(\.env($|\.)|id_rsa|.*\.(pfx|pem|key)$|secrets?)(/|$)') {
            [void]$ownershipFindings.Add([ordered]@{ classification='ownership'; status='HOLD'; path=$dirtyPath; reason='dirty work is outside the active lane ownership' })
            Add-Item $blockers ('dirty work remains unresolved outside active lane ownership: ' + $dirtyPath)
        }
    }
}

if (-not [string]::IsNullOrWhiteSpace($SingleClosurePacket)) {
    $TraceabilityEvidence = @($SingleClosurePacket) + @($TraceabilityEvidence | Where-Object { $_ -and $_ -ne $SingleClosurePacket })
}
if (-not [string]::IsNullOrWhiteSpace($SessionAuthorizations)) {
    if (-not (Test-RepoRelativePath $SessionAuthorizations)) { Add-Item $configErrors ('session authorizations path must be repo-relative: ' + $SessionAuthorizations) }
    elseif (-not (Test-Path -LiteralPath (Join-Path $repo $SessionAuthorizations) -PathType Leaf)) { Add-Item $blockers ('session authorizations not found: ' + $SessionAuthorizations) }
}
if (-not [string]::IsNullOrWhiteSpace($PartialPublicationPacket) -and -not $partialPublicationAuthorizedExplicitIncomplete) {
    Add-Item $blockers 'explicit_incomplete: partial publication packet is invalid or SOURCE ae-partial-publication-source/v0 is unavailable'
} elseif ($contractExists -and (Get-YamlScalarAtPath $contractText 'publication.mode') -ceq 'partial' -and -not $partialPublicationAuthorizedExplicitIncomplete) {
    Add-Item $blockers 'explicit_incomplete: partial publication requires a valid boundary packet; full closure cannot be substituted'
}
$evidence = @($TraceabilityEvidence | Where-Object { $_ } | Sort-Object -Unique)
if ($significant -and $evidence.Count -eq 0 -and -not $partialPublicationAuthorizedExplicitIncomplete) { Add-Item $blockers 'TraceabilityEvidence is required for changed work' }
$closureDiff = if ($FirstPush) { @() } else { @((Invoke-Git @('diff', $BaseRef)).Output) }
foreach ($item in $evidence) {
    if (-not (Test-RepoRelativePath $item)) { Add-Item $configErrors ('traceability evidence path must be repo-relative: ' + $item); continue }
    $path = Join-Path $repo $item
    $leaf = Split-Path -Leaf $path
    if ($item -match '(?i)^\.docs/raw/') { Add-Item $blockers ('traceability evidence cannot be under .docs/raw: ' + $item); continue }
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { Add-Item $blockers ('traceability evidence not found: ' + $item) }
    else {
        $evidenceText = Get-Content -LiteralPath $path -Raw
        $evidenceContext = Get-LaneContextModel $evidenceText 'evidence'
        $lineage = Test-LaneLineage $evidenceContext $contractLaneContext $laneContextActive ('traceability evidence ' + $item)
        foreach ($info in @($lineage.Info)) { [void]$infoDropped.Add($info) }
        foreach ($lineageError in @($lineage.Errors)) { Add-Item $blockers $lineageError }
        if (-not $lineage.Dropped -and $leaf -notmatch '(?i)(verification|traceability|trazabilidad|audit|auditoria|verdict|closure|cierre|handoff)') { Add-Item $blockers ('traceability evidence filename is not closure-like: ' + $item) }
        if (-not $lineage.Dropped) {
            $metadata = Get-StructuredMetadata $item $evidenceText 'evidence' $acceptedEvidenceCommits $base 24 ([bool]$FirstPush)
            foreach ($metadataError in @($metadata.Errors)) { Add-Item $blockers ('traceability evidence invalid (' + $item + '): ' + $metadataError) }
            if ((Test-TextHasTerminalFailure $evidenceText) -or ($head -and -not (Test-TextCitesCommit $evidenceText $acceptedEvidenceCommits))) { Add-Item $blockers ('traceability evidence is not fresh for accepted implementation HEAD: ' + $item) }
            if (-not [string]::IsNullOrWhiteSpace($SingleClosurePacket) -and $item -eq $SingleClosurePacket) {
                foreach ($digestError in @(Test-SingleClosurePacketDigest $evidenceText $closureDiff)) { Add-Item $blockers ($digestError + ' (' + $item + ')') }
            }
        }
    }
}

# Every changed source file is applicable by the declared source layout. The new
# delivery contract deliberately does not use SharedSkillName or ExpectedScope as
# an opt-in switch, and publication never evaluates installation targets.
$skillPaths = @()
$sourceRoot = Join-Path $repo 'skills'
if ($deliveryContractActive) {
    $sourceRoot = $deliveryContract.SourcePath
    $sourcePrefix = $deliveryContract.SourceRoot.TrimEnd('/', '\') + '/'
    $skillPaths = @($allChanged | Where-Object { ([string]$_).Replace('\','/').StartsWith($sourcePrefix, [System.StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { ([string]$_).Replace('\','/').Substring($sourcePrefix.Length) } | Where-Object { $_ } | Sort-Object -Unique)
    if ($skillPaths.Count -gt 0) {
        $sourceCommitHashes = Get-GitBlobHashes $skillPaths
        foreach ($relative in $skillPaths) {
            $source = Join-Path $sourceRoot $relative
            $sourceCommitKey = $deliveryContract.SourceRoot.TrimEnd('/', '\') + '/' + $relative
            $sourceDeletedAtHead = $deletedFromHead.Contains($sourceCommitKey)
            $sourceCommitHash = if ($sourceCommitHashes.ContainsKey($sourceCommitKey)) { [string]$sourceCommitHashes[$sourceCommitKey] } else { '' }
            if (-not $sourceCommitHash) {
                if (-not $sourceDeletedAtHead) {
                    Add-Item $blockers ('delivery source file is not committed at HEAD: ' + $sourceCommitKey)
                    continue
                }
                if (Test-AeDeliveryPathHasReparsePoint $source) {
                    Add-Item $blockers ('delivery source path contains a reparse point; file was not read or hashed: ' + $source)
                    continue
                }
                if (Test-Path -LiteralPath $source -PathType Leaf) { Add-Item $blockers ('delivery source deletion is not reflected in the worktree: ' + $source) }
                foreach ($mirror in @($deliveryContract.DistributionMirrors)) {
                    $mirrorRoot = Resolve-AeDeliveryPath -RepositoryRoot $repo -Path ([string]$mirror.root)
                    $mirrorPath = Join-Path $mirrorRoot $relative
                    if (Test-AeDeliveryPathHasReparsePoint $mirrorPath) {
                        Add-Item $blockers ('distribution mirror path contains a reparse point; file was not read or hashed: ' + $mirrorPath)
                        continue
                    }
                    if (Test-Path -LiteralPath $mirrorPath -PathType Leaf) { Add-Item $blockers ('distribution mirror must be deleted with the declared source deletion: ' + $mirrorPath) }
                }
                continue
            }
            if (Test-AeDeliveryPathHasReparsePoint $source) {
                Add-Item $blockers ('delivery source path contains a reparse point; file was not read or hashed: ' + $source)
                continue
            }
            if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { Add-Item $blockers ('delivery source file missing: ' + $source); continue }
            $sourceWorktreeHash = Get-GitFileHashForRepositoryPath -Path $source -RepositoryPath $sourceCommitKey
            if (-not $sourceWorktreeHash) { Add-Item $blockers ('delivery source worktree hash unavailable: ' + $source); continue }
            if ($sourceWorktreeHash -ne $sourceCommitHash) { Add-Item $blockers ('delivery source worktree/HEAD hash mismatch: ' + $relative + ' (' + $source + ')') }
            foreach ($mirror in @($deliveryContract.DistributionMirrors)) {
                $mirrorRoot = Resolve-AeDeliveryPath -RepositoryRoot $repo -Path ([string]$mirror.root)
                $mirrorPath = Join-Path $mirrorRoot $relative
                if (Test-AeDeliveryPathHasReparsePoint $mirrorPath) {
                    Add-Item $blockers ('distribution mirror path contains a reparse point; file was not read or hashed: ' + $mirrorPath)
                    continue
                }
                if (-not (Test-Path -LiteralPath $mirrorPath -PathType Leaf)) { Add-Item $blockers ('distribution mirror required by delivery contract is missing: ' + $mirrorPath); continue }
                $mirrorHash = Get-GitFileHashForRepositoryPath -Path $mirrorPath -RepositoryPath $sourceCommitKey
                if (-not $mirrorHash -or $sourceCommitHash -ne $mirrorHash) { Add-Item $blockers ('distribution mirror hash mismatch: ' + $relative + ' (' + $mirrorPath + ')') }
            }
        }
    }
    $skillPaths = @($skillPaths | Sort-Object -Unique)
} else {
    # Legacy repositories retain the previous two-target parity contract until they
    # adopt .ae.yaml. This compatibility branch is intentionally not used by the
    # Kernel contract and therefore cannot turn installation into publication.
    $skillPaths = @($allChanged | Where-Object { $_ -match '^skills/' } | ForEach-Object { $_.Substring(7).Replace('\','/') } | Sort-Object -Unique)
    $namedSkills = @($SharedSkillName | ForEach-Object { ([string]$_).Replace('\','/').Trim('/') } | Where-Object { $_ })
    $namedRelativeRoots = @($namedSkills | ForEach-Object { if ($_ -match '^skills/') { $_.Substring(7) } else { $_ } } | Where-Object { $_ } | Sort-Object -Unique)
    $namedSourceFiles = @(Get-SourceSkillFiles $sourceRoot $namedRelativeRoots)
    foreach ($relative in $namedRelativeRoots) {
        $candidate = Join-Path $sourceRoot $relative
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { $skillPaths += $relative }
        elseif (Test-Path -LiteralPath $candidate -PathType Container) {
            $skillPaths += @($namedSourceFiles | Where-Object { $_.Relative -eq $relative -or $_.Relative.StartsWith($relative + '/') } | ForEach-Object { $_.Relative })
        } else { $skillPaths += $relative }
    }
    $skillPaths = @($skillPaths | Where-Object { $_ } | Sort-Object -Unique)
    if ($skillPaths.Count -gt 0 -or $scopes -contains 'shared-skill') {
        if ($skillPaths.Count -eq 0) { Add-Item $blockers 'SharedSkillName or a changed skills/<name> path is required for shared-skill scope' }
        $mirrorRoots = @(@($env:AE_SKILL_MIRROR_ROOT, $env:AE_SKILL_INSTALLED_ROOT) | Where-Object { $_ })
        if ($mirrorRoots.Count -lt 2) { Add-Item $blockers 'shared-skill parity requires AE_SKILL_MIRROR_ROOT and AE_SKILL_INSTALLED_ROOT' }
        else {
            $mirrorRoot = $mirrorRoots[0]
            $installedRoot = $mirrorRoots[1]
            $gitBlobHashes = Get-GitBlobHashes $skillPaths
            foreach ($relative in $skillPaths) {
                $source = Join-Path $sourceRoot $relative
                if (Test-AeDeliveryPathHasReparsePoint $source) {
                    Add-Item $blockers ('shared skill source path contains a reparse point; file was not read or hashed: ' + $source)
                    continue
                }
                if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { Add-Item $blockers ('shared skill source missing: ' + $source); continue }
                $sourceWorktreeHash = Get-GuardHash $source
                $sourceCommitKey = 'skills/' + $relative
                $sourceCommitHash = if ($gitBlobHashes.ContainsKey($sourceCommitKey)) { [string]$gitBlobHashes[$sourceCommitKey] } else { '' }
                if (-not $sourceCommitHash) { Add-Item $blockers ('shared skill source commit blob missing: skills/' + $relative); continue }
                $mirror = Join-Path $mirrorRoot $relative
                $installed = Join-Path $installedRoot $relative
                if (Test-AeDeliveryPathHasReparsePoint $mirror) {
                    Add-Item $blockers ('shared skill parity path contains a reparse point; file was not read or hashed: ' + $mirror)
                } elseif (-not (Test-Path -LiteralPath $mirror -PathType Leaf)) { Add-Item $blockers ('shared skill parity target missing: ' + $mirror) }
                else {
                    $mirrorHash = Get-GitFileHashForRepositoryPath -Path $mirror -RepositoryPath $sourceCommitKey
                    if (-not $mirrorHash -or $sourceCommitHash -ne $mirrorHash) { Add-Item $blockers ('shared skill source commit/mirror hash mismatch: ' + $relative + ' (' + $mirror + ')') }
                }
                if (Test-AeDeliveryPathHasReparsePoint $installed) {
                    Add-Item $blockers ('shared skill installation path contains a reparse point; file was not read or hashed: ' + $installed)
                } elseif (-not (Test-Path -LiteralPath $installed -PathType Leaf)) { Add-Item $blockers ('shared skill parity target missing: ' + $installed) }
                else {
                    $installedHash = Get-GuardHash $installed
                    if ($sourceWorktreeHash -ne $installedHash) { Add-Item $blockers ('shared skill worktree/installed hash mismatch: ' + $relative + ' (' + $installed + ')') }
                }
            }
        }
    }
}

function ConvertTo-DeliveryDestinationReport {
    param([object]$Destination)
    return [ordered]@{
        name = if ($Destination.PSObject.Properties.Name -contains 'name') { [string]$Destination.name } else { '' }
        product = if ($Destination.PSObject.Properties.Name -contains 'product') { [string]$Destination.product } else { '' }
        root = if ($Destination.PSObject.Properties.Name -contains 'root') { [string]$Destination.root } else { '' }
    }
}

# The remote observation is the final approval-side operation. The guard never pushes.
if ($FirstPush -and $contractExists -and $firstPushContractValid) {
    $remoteUrlResult = Invoke-Git @('remote', 'get-url', '--all', 'origin')
    if ($remoteUrlResult.ExitCode -ne 0 -or @($remoteUrlResult.Output | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }).Count -ne 1 -or [string]::IsNullOrWhiteSpace([string]$remoteUrlResult.Output[0])) {
        Add-Item $infraErrors 'FirstPush remote identity could not be resolved unambiguously'
    } else { $remoteIdentity = ([string]$remoteUrlResult.Output[0]).Trim() }
    $remoteObservedAt = [datetime]::UtcNow.ToString('o', [Globalization.CultureInfo]::InvariantCulture)
    $lsRemote = Invoke-Git @('ls-remote', '--refs', 'origin')
    if ($lsRemote.ExitCode -ne 0) { Add-Item $infraErrors (Join-Output $lsRemote.Output) }
    else {
        $remoteRefInventory = @($lsRemote.Output | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
        $ambiguous = @($remoteRefInventory | Where-Object { $_ -notmatch '^[0-9a-fA-F]{40}	refs/.+$' })
        if ($ambiguous.Count -gt 0) { Add-Item $infraErrors ('FirstPush remote ref inventory is ambiguous: ' + ([string]::Join(' | ', [string[]]$ambiguous))) }
        elseif ($remoteRefInventory.Count -ne 0) { Add-Item $blockers ('FirstPush requires exactly zero remote refs; observed ' + $remoteRefInventory.Count) }
        else { $remoteZeroRefs = $true }
    }
}
foreach ($item in @($infraErrors)) { Add-Item $blockers $item }
$code = 0
$verdict = 'Approved'
if ($configErrors.Count -gt 0) { $code = 3; $verdict = 'Blocked' }
elseif ($infraErrors.Count -gt 0) { $code = 2; $verdict = 'Blocked' }
elseif ($partialPublicationAuthorizedExplicitIncomplete -and $blockers.Count -eq 0) {
    $code = 4
    $verdict = 'explicit_incomplete'
    [void]$warnings.Add('source safety checks satisfied; caller must apply existing durable authority and integration HOLDs; this guard grants no protected-main push approval; product acceptance remains explicit_incomplete under SOURCE ae-partial-publication-source/v0')
}
elseif ($blockers.Count -gt 0) { $code = 1; $verdict = 'Blocked' }
elseif ($WaiverReason) { $verdict = 'Approved with waiver' }
# Like ordinary Approved readiness above, source safety does not identify the
# owner or grant authority. The caller applies its existing durable mandate and
# integration HOLDs; SOURCE, exit 4 and file presence cannot supply that mandate.
# Not evaluated here is neither permission nor a global denial of integration.
$sourceIntegration = $null
if (-not [string]::IsNullOrWhiteSpace($PartialPublicationPacket) -or ($contractExists -and (Get-YamlScalarAtPath $contractText 'publication.mode') -ceq 'partial')) {
    $sourceIntegration = [ordered]@{
        safety_status = if ($code -eq 4) { 'satisfied' } else { 'blocked' }
        authority_status = 'not_evaluated_by_guard'
        integration_authorized = $null
        final_pass = $false
        explicit_incomplete = [bool]($code -eq 4)
        grants_push_approval = $false
        release_authorized = $false
        execution_authorized = $false
    }
}
$elapsedTicks = [System.Diagnostics.Stopwatch]::GetTimestamp() - [int64]$telemetry.started_at
$governedDurationMs = [int][Math]::Max(0, [Math]::Round(($elapsedTicks * 1000.0) / [System.Diagnostics.Stopwatch]::Frequency))
$governedScannerMs = [int]$script:scannerDurationMs
$telemetrySnapshot = [ordered]@{
    walks = [int]$telemetry.walks
    git_processes = [int]$telemetry.git_processes
    hash_calls = [int]$telemetry.hash_calls
    duration_ms = $governedDurationMs
    scanner_duration_ms = $governedScannerMs
    infrastructure_duration_ms = [int][Math]::Max(0, ($governedDurationMs - $governedScannerMs))
    git_timeout_seconds = [int]$GitTimeoutSeconds
    scanner_timeout_seconds = [int]$ScannerTimeoutSeconds
}
$report = [ordered]@{
    schema = 'ae-pre-push-report/v2'
    verdict = $verdict
    exit_code = $code
    code_name = Get-ExitCodeName $code
    branch = $actualBranch
    base = $BaseRef
    head = $head
    fetched = [bool]($fetchResult -and $fetchResult.ExitCode -eq 0)
    first_push = [bool]$FirstPush
    base_semantics = if ($FirstPush) { 'empty_remote_no_base' } else { 'fetched_remote_base' }
    remote_identity = $remoteIdentity
    remote_ref_inventory = @($remoteRefInventory)
    remote_zero_refs = [bool]$remoteZeroRefs
    remote_observation_command = if ($FirstPush) { $firstPushObservationCommand } else { '' }
    observed_at = $remoteObservedAt
    first_push_authorization = if ($FirstPush -and $contract) { [ordered]@{ authorized=$contract.FirstPushAuthorized; authorized_by=$contract.FirstPushAuthorizedBy; authorized_at=$contract.FirstPushAuthorizedAt } } else { $null }
    post_push_verification_handoff = if ($FirstPush -and $contract) { [ordered]@{ owner=$contract.FirstPushHandoffOwner; command=$contract.FirstPushHandoffCommand; expected_ref=$contract.FirstPushExpectedRef; expected_sha=$contract.FirstPushExpectedSha; approved_head=$head; unexpected_refs=$contract.FirstPushUnexpectedRefs; toctou_race_owner=$contract.FirstPushRaceOwner; rule='refs/heads/main must equal approved_head and no unexpected refs may exist' } } else { $null }
    delivery_contract = if ($deliveryContract) { [ordered]@{ path=$deliveryContract.Path; schema=$deliveryContract.Schema; product=$deliveryContract.Product; repository=$deliveryContract.Repository; mode=$deliveryContract.Mode; remote=$deliveryContract.Remote; base=$deliveryContract.Base; branch=$deliveryContract.Branch; source_root=$deliveryContract.SourceRoot + '/'; source_product=$deliveryContract.SourceProduct; same_product_only=$deliveryContract.SameProductOnly; publication_stage=$deliveryContract.PublicationStage; mirror_policy=$deliveryContract.MirrorPolicy; distribution_mirrors=@($deliveryContract.DistributionMirrors | ForEach-Object { ConvertTo-DeliveryDestinationReport $_ }); installation_stage=$deliveryContract.InstallationStage; target_policy=$deliveryContract.TargetPolicy; installation_targets=@($deliveryContract.InstallationTargets | ForEach-Object { ConvertTo-DeliveryDestinationReport $_ }) } } else { $null }
    publication = if ($deliveryContractActive) { [ordered]@{ stage='publication'; source_root=$deliveryContract.SourceRoot + '/'; source_paths=@($skillPaths); distribution_mirrors_checked=[int]@($deliveryContract.DistributionMirrors).Count; installation_evaluated=$false; installation_status='not_evaluated' } } else { $null }
    installation = if ($deliveryContractActive) { [ordered]@{ stage='installation'; status='not_evaluated'; control='infra/git/Test-KernelInstallation.ps1'; reason='installation is a separate control and is never claimed by publication' } } else { $null }
    origin_main_ancestor_of_head = $baseIsAncestor
    fast_forward = $baseIsAncestor
    guard_v2 = [ordered]@{ mode=$guardV2Mode; loaded=$guardV2Loaded; reason=$guardV2Reason; module_path=$guardV2; independent_sha256=$guardV2IndependentSha256; trusted_sha256=$guardV2TrustedSha256; exported_commands=$guardV2Commands; fingerprint=$guardV2Fingerprint; hard_gates_cached=$false }
    changed_paths = $allChanged
    expected_scope = $scopes
    inventories = [ordered]@{
        changed_from_base = @($changed | ForEach-Object { [string]$_ })
        changed_name_status = @($diffOutput | ForEach-Object { [string]$_ })
        working_tree = @($working | ForEach-Object { [string]$_ })
        ignored = @($ignored | ForEach-Object { [string]$_ })
        worktrees = @($worktreeInventory.Output | ForEach-Object { [string]$_ })
        branches = @($branchInventory.Output | ForEach-Object { [string]$_ })
        branch_classification = @($branchClassification | ForEach-Object { [ordered]@{ name=[string]$_.name; marker=[string]$_.marker; upstream=[string]$_.upstream; classification=[string]$_.classification } })
        worktree_classification = @($worktreeClassification | ForEach-Object { [ordered]@{ path=[string]$_.path; branch=[string]$_.branch; head=[string]$_.head; classification=[string]$_.classification } })
    }
    session_contract = $SessionContract
    session_contract_compatibility = if ($contract) { [ordered]@{ source_schema=$contract.Schema; representation='in_memory_only'; errors=@($contract.CompatibilityErrors); grants_authority=$false; credits_verification=$false } } else { $null }
    partial_publication_boundary = $partialBoundary
    source_integration = $sourceIntegration
    contract = if ($contract) { [ordered]@{ schema=$contract.Schema; branch=$contract.Branch; base_sha=$contract.BaseSha; ticket_frontier=$contract.TicketFrontier; tracker_waiver=$contract.TrackerWaiver; allowed_paths=$contract.Allowed; forbidden=$contract.Forbidden; anchors=$contract.Anchors; worker_decision=$contract.WorkerDecision; required_evidence=$contract.RequiredEvidence; lane_context=if ($contractLaneContext) { [ordered]@{ declared=[bool]$contractLaneContext.Declared; session_id=$contractLaneContext.SessionId; goal_id=$contractLaneContext.GoalId; lane_id=$contractLaneContext.LaneId; attempt_id=$contractLaneContext.AttemptId; evidence_root=$contractLaneContext.EvidenceRoot; evidence_scope=$contractLaneContext.EvidenceScope; acceptance_anchor=$contractLaneContext.AcceptanceAnchor; claim_ids=$contractLaneContext.ClaimIds } } else { $null } } } else { $null }
    lane_context_active = [bool]$laneContextActive
    info_dropped = @($infoDropped)
    join_eligibility = @($joinEligibility)
    ownership_findings = @($ownershipFindings)
    traceability_evidence = $evidence
    shared_skill_paths = $skillPaths
    blockers = @($blockers)
    configuration_errors = @($configErrors)
    infrastructure_errors = @($infraErrors)
    warnings = @($warnings)
    dry_run = [bool]$DryRun
    telemetry = $telemetrySnapshot
}
if ($Json) { $report | ConvertTo-Json -Depth 12 }
else {
    Write-Output ('Pre-push guard: ' + $verdict + ' (' + (Get-ExitCodeName $code) + ')')
    foreach ($item in $blockers) { Write-Output ('BLOCKED: ' + $item) }
    foreach ($item in $configErrors) { Write-Output ('CONFIGURATION: ' + $item) }
    foreach ($item in $warnings) { Write-Output ('WARNING: ' + $item) }
    if ($WaiverReason -and $verdict -eq 'Approved with waiver') { Write-Output ('WAIVER: ' + $WaiverReason) }
}
exit $code

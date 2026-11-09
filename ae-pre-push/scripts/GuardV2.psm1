Set-StrictMode -Version 2.0

function Get-AeGuardSha256 {
    param([byte[]]$Bytes)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try { return ([System.BitConverter]::ToString($sha.ComputeHash($Bytes))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Get-AeGuardFingerprint {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$PolicyVersion,
        [Parameter(Mandatory=$true)][string]$GateId,
        [Parameter(Mandatory=$true)][string]$Command,
        [string[]]$Arguments = @(),
        [string[]]$InputPath = @(),
        [string]$BasePath = (Get-Location).Path
    )
    $base = [System.IO.Path]::GetFullPath($BasePath)
    $entries = @()
    foreach ($candidate in @($InputPath)) {
        if ([string]::IsNullOrWhiteSpace($candidate)) { continue }
        $candidatePath = if ([System.IO.Path]::IsPathRooted($candidate)) { $candidate } else { Join-Path $base $candidate }
        $full = [System.IO.Path]::GetFullPath($candidatePath)
        $baseUri = New-Object System.Uri (($base.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar))
        $relative = $baseUri.MakeRelativeUri((New-Object System.Uri($full))).ToString().Replace('\','/')
        if (Test-Path -LiteralPath $full -PathType Leaf) {
            $hash = Get-AeGuardSha256 ([System.IO.File]::ReadAllBytes($full))
            $entries += ($relative + '|sha256:' + $hash)
        } else {
            $entries += ($relative + '|missing')
        }
    }
    $canonical = @(
        'policy_version=' + $PolicyVersion
        'gate_id=' + $GateId
        'command=' + $Command
        'args_json=' + ([System.Management.Automation.PSSerializer]::Serialize(@($Arguments)))
        'inputs=' + (($entries | Sort-Object) -join "`n")
    ) -join "`n"
    return Get-AeGuardSha256 ([System.Text.Encoding]::UTF8.GetBytes($canonical))
}

function Invoke-AeGuardCachedCommand {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$Fingerprint,
        [Parameter(Mandatory=$true)][string]$StampPath,
        [string]$GateId = '',
        [string]$Command,
        [object[]]$Arguments = @(),
        [scriptblock]$ScriptBlock,
        [int]$MaxAgeMinutes = 0
    )
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $cacheHit = $false
    $cacheStatus = 'MISS'
    try {
        if (Test-Path -LiteralPath $StampPath -PathType Leaf) {
            $stamp = Get-Content -LiteralPath $StampPath -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
            $maxAgeExpiry = $false
            if ($MaxAgeMinutes -gt 0 -and $stamp.PSObject.Properties.Name -contains 'written_utc') {
                $written = [datetime]::Parse($stamp.written_utc)
                if ((New-TimeSpan -Start $written -End (Get-Date)) -gt (New-TimeSpan -Minutes $MaxAgeMinutes)) {
                    $maxAgeExpiry = $true
                }
            }
            if (-not $maxAgeExpiry -and $stamp.status -eq 'PASS' -and $stamp.fingerprint -eq $Fingerprint) {
                $cacheHit = $true
                $cacheStatus = 'HIT'
                return [pscustomobject]@{ Status='PASS'; CacheHit=$true; CacheStatus=$cacheStatus; DurationMs=$sw.ElapsedMilliseconds; ExitCode=0; Fingerprint=$Fingerprint; GateId=$GateId; Output=$stamp.output }
            }
        }
    } catch { $cacheHit = $false }

    $output = @()
    $exitCode = 0
    try {
        # Reset the process-wide exit code before either invocation so a prior
        # native command cannot contaminate this guard result.
        $global:LASTEXITCODE = 0
        if ($null -ne $ScriptBlock) { $output = @(& $ScriptBlock @Arguments 2>&1) }
        elseif (-not [string]::IsNullOrWhiteSpace($Command)) { $output = @(& $Command @Arguments 2>&1) }
        else { throw 'A Command or ScriptBlock is required.' }
        $exitCode = [int]$global:LASTEXITCODE
    } catch {
        $output += $_.Exception.Message
        $exitCode = 1
    }
    $status = if ($exitCode -eq 0) { 'PASS' } else { 'FAIL' }
    if ($status -eq 'PASS') {
        try {
            $parent = Split-Path -Parent $StampPath
            if ($parent -and -not (Test-Path -LiteralPath $parent)) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }
            [pscustomobject]@{ schema='ae-guard-cache/v2'; status='PASS'; fingerprint=$Fingerprint; output=([string]::Join("`n", [string[]]$output)); written_utc=(Get-Date).ToUniversalTime().ToString('o'); gate_id=$GateId } |
                ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $StampPath -Encoding UTF8 -ErrorAction Stop
        } catch { }
    }
    return [pscustomobject]@{ Status=$status; CacheHit=$false; CacheStatus=$cacheStatus; DurationMs=$sw.ElapsedMilliseconds; ExitCode=$exitCode; Fingerprint=$Fingerprint; GateId=$GateId; Output=([string]::Join("`n", [string[]]$output)) }
}

function New-AeGuardFinding {
    [CmdletBinding()]
    param([Parameter(Mandatory=$true)][string]$Kind, [Parameter(Mandatory=$true)][string]$Message, [string]$Path='', [string]$GateId='')
    $stable = Get-AeGuardSha256 ([System.Text.Encoding]::UTF8.GetBytes(($Kind+'|'+$GateId+'|'+$Path+'|'+$Message)))
    return [pscustomobject]@{ id=$stable; kind=$Kind; gate_id=$GateId; path=$Path; message=$Message }
}

function Update-AeGuardWarningLedger {
    [CmdletBinding()]
    param([Parameter(Mandatory=$true)][string]$LedgerPath, [object[]]$Finding=@(), [datetime]$Now=(Get-Date).ToUniversalTime())
    $records = @{}
    try {
        if (Test-Path -LiteralPath $LedgerPath) {
            $old = Get-Content -LiteralPath $LedgerPath -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
            foreach ($item in @($old.findings)) { $records[[string]$item.id] = $item }
        }
    } catch { $records = @{} }
    foreach ($item in @($Finding)) {
        $id = [string]$item.id
        if (-not $records.ContainsKey($id)) {
            $records[$id] = [pscustomobject]@{ id=$id; kind=$item.kind; gate_id=$item.gate_id; path=$item.path; message=$item.message; first_seen=$Now.ToString('o'); last_seen=$Now.ToString('o'); occurrences=1; escalated=$false }
        } else {
            $r = $records[$id]; $r.last_seen=$Now.ToString('o'); $r.occurrences=[int]$r.occurrences + 1
        }
    }
    foreach ($r in $records.Values) {
        $age = ($Now - ([datetime]$r.first_seen)).TotalDays
        $r.escalated = ($age -ge 7 -or [int]$r.occurrences -ge 3)
    }
    $result = @($records.Values | Sort-Object id)
    try {
        $parent=Split-Path -Parent $LedgerPath; if ($parent -and -not (Test-Path $parent)) { New-Item -ItemType Directory $parent -Force | Out-Null }
        [pscustomobject]@{ schema='ae-guard-warning-ledger/v2'; findings=$result } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $LedgerPath -Encoding UTF8 -ErrorAction Stop
    } catch { }
    return $result
}

function Get-AeGuardTier {
    [CmdletBinding()]
    param(
        [string]$FindingId = '',
        [string]$DefaultTier = 'CONDITIONAL',
        [string]$ManifestGate = '',
        [switch]$Runtime,
        [switch]$Deployable
    )
    $riskMap = @{
        'HARD'     = 'HARD'
        'CONDITIONAL' = 'CONDITIONAL'
        'RUNTIME'  = 'RUNTIME'
    }
    if ($Deployable -or $Runtime) { return 'RUNTIME' }
    if (-not [string]::IsNullOrWhiteSpace($ManifestGate)) {
        $mg = $ManifestGate.ToUpperInvariant()
        if ($mg -eq 'HARD') { return 'HARD' }
        if ($mg -eq 'RUNTIME') { return 'RUNTIME' }
        return 'CONDITIONAL'
    }
    if (-not [string]::IsNullOrWhiteSpace($FindingId)) {
        $known = @{
            'HARD'     = 'HARD'
            'CONDITIONAL' = 'CONDITIONAL'
            'RUNTIME'  = 'RUNTIME'
        }
        if ($known.ContainsKey($FindingId)) { return $known[$FindingId] }
    }
    if (-not [string]::IsNullOrWhiteSpace($DefaultTier)) {
        if ($riskMap.ContainsKey($DefaultTier)) { return $DefaultTier }
    }
    return 'CONDITIONAL'
}

Export-ModuleMember -Function Get-AeGuardFingerprint,Invoke-AeGuardCachedCommand,New-AeGuardFinding,Update-AeGuardWarningLedger,Get-AeGuardTier

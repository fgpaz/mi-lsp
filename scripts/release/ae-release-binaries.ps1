[CmdletBinding()]
param(
    [string[]]$Rids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64'),
    [string]$OutDir = (Join-Path $PSScriptRoot '..\..\dist'),
    [string]$InstallDir = (Join-Path $HOME 'bin'),
    [string]$WslUser = '',
    [string[]]$WslInstallPaths = @(),
    [string]$MirrorRoot = '',
    [string]$Tag = '',
    [string]$Remote = 'origin',
    [switch]$Clean,
    [switch]$SkipBuild,
    [switch]$SkipLocalInstall,
    [switch]$SkipWslInstall,
    [switch]$SkipMirror,
    [switch]$SkipWorkerStatus,
    [switch]$Publish
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$buildScript = Join-Path $PSScriptRoot 'build-dist.ps1'
$installScript = Join-Path $PSScriptRoot 'install-local.ps1'
$supportedRids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64')

function Test-IsWindows {
    return [System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT
}

function Get-HostArchitecture {
    if (Test-IsWindows) {
        $windowsHostHints = @(
            $env:PROCESSOR_ARCHITEW6432,
            $env:PROCESSOR_ARCHITECTURE,
            $env:PROCESSOR_IDENTIFIER
        ) -join ' '
        if ($windowsHostHints -match '(?i)\bARM64\b|ARMv8|Qualcomm|Snapdragon') {
            return 'arm64'
        }
    }

    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    switch ($arch) {
        'arm64' { return 'arm64' }
        'x64' { return 'x64' }
        'amd64' { return 'x64' }
        default { throw "Unsupported process architecture '$arch'." }
    }
}

function Get-HostRid {
    $arch = Get-HostArchitecture
    if (Test-IsWindows) {
        return "win-$arch"
    }
    return "linux-$arch"
}

function Get-WslCommandPath {
    $command = Get-Command wsl -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace($env:WINDIR)) {
        $candidates += (Join-Path $env:WINDIR 'System32\wsl.exe')
    }
    $candidates += 'C:\Windows\System32\wsl.exe'
    foreach ($candidate in $candidates) {
        if (Test-Path $candidate) {
            return $candidate
        }
    }
    return $null
}

function Get-CliName {
    param([Parameter(Mandatory = $true)][string]$Rid)
    if ($Rid -like 'win-*') {
        return 'mi-lsp.exe'
    }
    return 'mi-lsp'
}

function Get-DistCliPath {
    param([Parameter(Mandatory = $true)][string]$Rid)
    return (Join-Path (Join-Path $OutDir $Rid) (Get-CliName -Rid $Rid))
}

function Get-FileSha256 {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path $Path)) {
        return $null
    }
    return (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToLowerInvariant()
}

function ConvertTo-WslPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    $full = (Resolve-Path $Path).Path
    $drive = $full.Substring(0, 1).ToLowerInvariant()
    $rest = $full.Substring(2).Replace('\', '/')
    return "/mnt/$drive$rest"
}

function Quote-Sh {
    param([Parameter(Mandatory = $true)][string]$Text)
    $sq = [string][char]39
    return $sq + $Text.Replace($sq, $sq + '\' + $sq + $sq) + $sq
}

function Invoke-NativeChecked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$FailureMessage
    )
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw $FailureMessage
    }
}

function Get-WslRid {
    $wsl = Get-WslCommandPath
    if ([string]::IsNullOrWhiteSpace($wsl)) {
        return $null
    }
    $archOutput = & $wsl sh -lc 'uname -m' 2>$null
    $exitCode = $LASTEXITCODE
    $arch = $archOutput | Select-Object -First 1
    if ($exitCode -ne 0 -or [string]::IsNullOrWhiteSpace($arch)) {
        return $null
    }
    switch ($arch.Trim()) {
        'aarch64' { return 'linux-arm64' }
        'arm64' { return 'linux-arm64' }
        'x86_64' { return 'linux-x64' }
        'amd64' { return 'linux-x64' }
        default { throw "Unsupported WSL architecture '$($arch.Trim())'." }
    }
}

function Get-WslUser {
    $wsl = Get-WslCommandPath
    if ([string]::IsNullOrWhiteSpace($wsl)) {
        return $null
    }
    $userOutput = & $wsl sh -lc 'whoami' 2>$null
    $exitCode = $LASTEXITCODE
    $user = $userOutput | Select-Object -First 1
    if ($exitCode -ne 0 -or [string]::IsNullOrWhiteSpace($user)) {
        return $null
    }
    return $user.Trim()
}

function Get-WslHome {
    $wsl = Get-WslCommandPath
    if ([string]::IsNullOrWhiteSpace($wsl)) {
        return $null
    }
    $homeOutput = & $wsl sh -lc 'printf "%s" "$HOME"' 2>$null
    $exitCode = $LASTEXITCODE
    $homeLine = $homeOutput | Select-Object -First 1
    if ($exitCode -ne 0 -or [string]::IsNullOrWhiteSpace($homeLine)) {
        return $null
    }
    return $homeLine.Trim()
}

foreach ($rid in $Rids) {
    if ($supportedRids -notcontains $rid) {
        throw "Unsupported RID '$rid'. Supported values: $($supportedRids -join ', ')."
    }
}

$resolvedOutDir = Resolve-Path $OutDir -ErrorAction SilentlyContinue
$outDirForReport = if ($resolvedOutDir) { $resolvedOutDir.Path } else { $OutDir }

$result = [ordered]@{
    repo = $repoRoot
    rids = $Rids
    out_dir = $outDirForReport
    built = @()
    checksums = @()
    local_install = $null
    wsl_install = $null
    mirror = $null
    publish = $null
    warnings = @()
}

if (-not $SkipBuild) {
    $buildArgs = @{
        Rids = $Rids
        OutDir = $OutDir
    }
    if ($Clean) {
        $buildArgs.Clean = $true
    }
    & $buildScript @buildArgs | Out-Null
}

foreach ($rid in $Rids) {
    $cliPath = Get-DistCliPath -Rid $rid
    if (-not (Test-Path $cliPath)) {
        throw "Missing CLI artifact for RID '$rid' at '$cliPath'."
    }
    $workerDir = Join-Path (Join-Path $OutDir $rid) (Join-Path 'workers' $rid)
    if (-not (Test-Path $workerDir)) {
        throw "Missing worker bundle for RID '$rid' at '$workerDir'."
    }
    $result.built += [pscustomobject]@{
        rid = $rid
        cli = (Resolve-Path $cliPath).Path
        worker_dir = (Resolve-Path $workerDir).Path
    }
    $result.checksums += [pscustomobject]@{
        rid = $rid
        cli = (Resolve-Path $cliPath).Path
        sha256 = Get-FileSha256 -Path $cliPath
    }
}

$hostRid = Get-HostRid
if ($SkipLocalInstall) {
    $result.local_install = [pscustomobject]@{ skipped = $true; reason = 'SkipLocalInstall' }
}
elseif ($Rids -contains $hostRid) {
    $installArgs = @{
        Rid = $hostRid
        InstallDir = $InstallDir
        OutDir = $OutDir
        SkipBuild = $true
    }
    if ($SkipWorkerStatus) {
        $installArgs.SkipWorkerRefresh = $true
    }
    $installResult = & $installScript @installArgs
    $installedCli = Join-Path $InstallDir (Get-CliName -Rid $hostRid)
    if (-not $SkipWorkerStatus) {
        Invoke-NativeChecked -FilePath $installedCli -Arguments @('version', '--format', 'compact') -FailureMessage "Installed CLI version check failed for '$installedCli'."
        Invoke-NativeChecked -FilePath $installedCli -Arguments @('worker', 'status', '--format', 'compact') -FailureMessage "Installed CLI worker status failed for '$installedCli'."
    }
    $result.local_install = [pscustomobject]@{
        skipped = $false
        rid = $hostRid
        cli = (Resolve-Path $installedCli).Path
        sha256 = Get-FileSha256 -Path $installedCli
        detail = $installResult
    }
}
else {
    $result.local_install = [pscustomobject]@{ skipped = $true; reason = "Host RID '$hostRid' was not in requested RIDs." }
}

if ($SkipWslInstall -or -not (Test-IsWindows)) {
    $reason = if ($SkipWslInstall) { 'SkipWslInstall' } else { 'not_windows_host' }
    $result.wsl_install = [pscustomobject]@{ skipped = $true; reason = $reason }
}
else {
    $wslRid = Get-WslRid
    if ([string]::IsNullOrWhiteSpace($wslRid)) {
        $result.warnings += 'WSL was not available; Linux install was skipped.'
        $result.wsl_install = [pscustomobject]@{ skipped = $true; reason = 'wsl_unavailable' }
    }
    elseif ($Rids -notcontains $wslRid) {
        $result.wsl_install = [pscustomobject]@{ skipped = $true; reason = "WSL RID '$wslRid' was not in requested RIDs." }
    }
    else {
        $effectiveWslUser = $WslUser
        if ([string]::IsNullOrWhiteSpace($effectiveWslUser)) {
            $effectiveWslUser = Get-WslUser
        }
        $effectiveWslInstallPaths = $WslInstallPaths
        if ($effectiveWslInstallPaths.Count -eq 0) {
            $wslHome = Get-WslHome
            if ([string]::IsNullOrWhiteSpace($wslHome)) {
                throw 'WSL home could not be detected; pass -WslInstallPaths or use -SkipWslInstall.'
            }
            $effectiveWslInstallPaths = @("$wslHome/.local/bin/mi-lsp", "$wslHome/bin/mi-lsp")
        }
        $linuxCli = Get-DistCliPath -Rid $wslRid
        $sourceWsl = ConvertTo-WslPath -Path $linuxCli
        $commands = New-Object System.Collections.Generic.List[string]
        $commands.Add('set -eu')
        foreach ($target in $effectiveWslInstallPaths) {
            $commands.Add("install -D -m 0755 $(Quote-Sh $sourceWsl) $(Quote-Sh $target)")
            $commands.Add("$(Quote-Sh $target) version --format compact >/dev/null")
            if (-not $SkipWorkerStatus) {
                $commands.Add("$(Quote-Sh $target) worker install --rid $(Quote-Sh $wslRid) --format compact >/dev/null")
                $commands.Add("$(Quote-Sh $target) worker status --format compact >/dev/null")
            }
        }
        $script = ($commands -join "`n")
        $wslCommand = Get-WslCommandPath
        if ([string]::IsNullOrWhiteSpace($wslCommand)) {
            throw 'WSL command was not found during install.'
        }
        Invoke-NativeChecked -FilePath $wslCommand -Arguments @('sh', '-lc', $script) -FailureMessage 'WSL install or verification failed.'
        $result.wsl_install = [pscustomobject]@{
            skipped = $false
            rid = $wslRid
            paths = $effectiveWslInstallPaths
            source = $sourceWsl
            user = $effectiveWslUser
        }
    }
}

if ([string]::IsNullOrWhiteSpace($MirrorRoot) -or $SkipMirror) {
    $reason = if ($SkipMirror) { 'SkipMirror' } else { 'MirrorRoot not provided' }
    $result.mirror = [pscustomobject]@{ skipped = $true; reason = $reason }
}
else {
    $mirrorBin = Join-Path $MirrorRoot 'bin'
    New-Item -ItemType Directory -Force -Path $mirrorBin | Out-Null
    $mirrorFiles = @(
        @{ Rid = 'win-x64'; Name = 'mi-lsp-win-x64.exe' },
        @{ Rid = 'linux-x64'; Name = 'mi-lsp-linux-x64' }
    )
    $copied = @()
    foreach ($file in $mirrorFiles) {
        if ($Rids -notcontains $file.Rid) {
            continue
        }
        $source = Get-DistCliPath -Rid $file.Rid
        $target = Join-Path $mirrorBin $file.Name
        Copy-Item -Force $source $target
        $copied += [pscustomobject]@{
            rid = $file.Rid
            path = (Resolve-Path $target).Path
            sha256 = Get-FileSha256 -Path $target
        }
    }
    $result.mirror = [pscustomobject]@{
        skipped = $false
        root = (Resolve-Path $MirrorRoot).Path
        copied = $copied
    }
}

if ($Publish) {
    if ([string]::IsNullOrWhiteSpace($Tag)) {
        throw 'Publish requires -Tag <vX.Y.Z>.'
    }
    Push-Location $repoRoot
    try {
        $status = & git status --porcelain
        if (-not [string]::IsNullOrWhiteSpace(($status -join "`n"))) {
            throw 'Publish requires a clean worktree so released provenance matches source. Commit or stash changes first.'
        }
        $head = (& git rev-parse HEAD).Trim()
        $tagSha = (& git rev-list -n 1 $Tag).Trim()
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($tagSha)) {
            throw "Tag '$Tag' was not found."
        }
        if ($head -ne $tagSha) {
            throw "Tag '$Tag' does not point at HEAD ($head)."
        }
        Invoke-NativeChecked -FilePath 'git' -Arguments @('push', $Remote, $Tag) -FailureMessage "Failed to push tag '$Tag' to '$Remote'."
        $result.publish = [pscustomobject]@{
            skipped = $false
            tag = $Tag
            remote = $Remote
            trigger = '.github/workflows/release.yml'
        }
    }
    finally {
        Pop-Location
    }
}
else {
    $result.publish = [pscustomobject]@{ skipped = $true; reason = 'Publish switch not set' }
}

[pscustomobject]$result

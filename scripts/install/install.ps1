[CmdletBinding()]
param(
    [string]$Repo = 'fgpaz/mi-lsp',
    [string]$Rid = '',
    [string]$InstallDir = (Join-Path $HOME 'bin'),
    [string]$GitHubToken = $env:GITHUB_TOKEN,
    [switch]$DryRun,
    [switch]$NoPathUpdate,
    [switch]$SkipWorkerInstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# SEC-08: warn if GITHUB_TOKEN is in environment (token should not be embedded in scripts)
if (-not [string]::IsNullOrWhiteSpace($env:GITHUB_TOKEN)) {
    Write-Warning "GITHUB_TOKEN environment variable is set. It will be used for authentication during download. Ensure you trust this environment and do not commit credentials in history."
}

function Get-HostRid {
    if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
        throw 'install.ps1 supports Windows only. Use install.sh on Linux or macOS.'
    }

    $hints = @(
        $env:PROCESSOR_ARCHITEW6432,
        $env:PROCESSOR_ARCHITECTURE,
        $env:PROCESSOR_IDENTIFIER,
        [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    ) -join ' '

    if ($hints -match '(?i)arm64|armv8|snapdragon|qualcomm') {
        return 'win-arm64'
    }
    if ($hints -match '(?i)x64|amd64') {
        return 'win-x64'
    }
    throw "Unsupported Windows architecture: $hints"
}

function Assert-SupportedRid {
    param([Parameter(Mandatory = $true)][string]$Value)
    $supported = @('win-x64', 'win-arm64')
    if ($supported -notcontains $Value) {
        throw "Unsupported RID '$Value' for install.ps1. Supported values: $($supported -join ', ')."
    }
}

function Invoke-Download {
    param(
        [Parameter(Mandatory = $true)]$Asset,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [Parameter(Mandatory = $true)][string]$Tag
    )
    $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
    if ($curl) {
        $curlArgs = @('-fL', '-H', 'User-Agent: mi-lsp-installer')
        if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
            $curlArgs += @('-H', "Authorization: Bearer $GitHubToken")
        }
        $curlArgs += @($Asset.browser_download_url, '-o', $OutFile)
        & $curl.Source @curlArgs
        if ($LASTEXITCODE -eq 0 -and (Test-Path -LiteralPath $OutFile) -and (Get-Item -LiteralPath $OutFile).Length -gt 0) {
            return
        }
    }

    $headers = Get-GitHubHeaders
    try {
        Invoke-WebRequest -Headers $headers -Uri $Asset.browser_download_url -OutFile $OutFile -UseBasicParsing
        if ((Test-Path -LiteralPath $OutFile) -and (Get-Item -LiteralPath $OutFile).Length -gt 0) {
            return
        }
    }
    catch {
        $gh = Get-Command gh -ErrorAction SilentlyContinue
        if ($gh) {
            $downloadDir = Split-Path -Parent $OutFile
            & $gh.Source release download $Tag --repo $Repo --pattern $Asset.name --dir $downloadDir --clobber
            if ($LASTEXITCODE -eq 0) {
                $downloaded = Join-Path $downloadDir $Asset.name
                if ((Test-Path -LiteralPath $downloaded) -and $downloaded -ne $OutFile) {
                    Move-Item -LiteralPath $downloaded -Destination $OutFile -Force
                }
                if (Test-Path -LiteralPath $OutFile) {
                    return
                }
            }
        }
        if (-not ($Asset.PSObject.Properties.Name -contains 'url') -or [string]::IsNullOrWhiteSpace($Asset.url)) {
            throw
        }
    }

    $apiHeaders = Get-GitHubHeaders -OctetStream
    Invoke-WebRequest -Headers $apiHeaders -Uri $Asset.url -OutFile $OutFile -UseBasicParsing
}

function Get-GitHubHeaders {
    param([switch]$OctetStream)
    $headers = @{ 'User-Agent' = 'mi-lsp-installer' }
    if ($OctetStream) {
        $headers.Accept = 'application/octet-stream'
    }
    if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
        $headers.Authorization = "Bearer $GitHubToken"
        if (-not $OctetStream) {
            $headers.Accept = 'application/vnd.github+json'
        }
    }
    return $headers
}

function Get-Release {
    param([Parameter(Mandatory = $true)][string]$Repo)
    $headers = Get-GitHubHeaders
    Invoke-RestMethod -Headers $headers -Uri "https://api.github.com/repos/$Repo/releases/latest"
}

function Find-Asset {
    param(
        [Parameter(Mandatory = $true)]$Release,
        [Parameter(Mandatory = $true)][string]$Name
    )
    $asset = @($Release.assets | Where-Object { $_.name -eq $Name }) | Select-Object -First 1
    if (-not $asset) {
        throw "Release '$($Release.tag_name)' does not contain asset '$Name'."
    }
    return $asset
}

function Get-ChecksumForAsset {
    param(
        [Parameter(Mandatory = $true)][string]$ChecksumFile,
        [Parameter(Mandatory = $true)][string]$AssetName
    )
    foreach ($line in Get-Content -LiteralPath $ChecksumFile) {
        if ($line -match "^\s*([a-fA-F0-9]{64})\s+\*?$([regex]::Escape($AssetName))\s*$") {
            return $matches[1].ToLowerInvariant()
        }
        if ($line -like "*$AssetName*") {
            $parts = $line.Trim() -split '\s+'
            if ($parts.Count -ge 1 -and $parts[0] -match '^[a-fA-F0-9]{64}$') {
                return $parts[0].ToLowerInvariant()
            }
        }
    }
    throw "Checksum for '$AssetName' was not found in '$ChecksumFile'."
}

function Ensure-Path {
    param([Parameter(Mandatory = $true)][string]$Directory)
    $resolved = (Resolve-Path -LiteralPath $Directory).Path
    $segments = @($env:PATH -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if ($segments -notcontains $resolved) {
        $env:PATH = "$resolved;$env:PATH"
    }

    if ($NoPathUpdate) {
        return
    }

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $userSegments = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if ($userSegments -notcontains $resolved) {
        $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $resolved } else { "$userPath;$resolved" }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "Added $resolved to the user PATH. Open a new terminal to inherit it."
    }
}

function Stop-ExistingDaemon {
    param([Parameter(Mandatory = $true)][string]$CliPath)
    if (-not (Test-Path -LiteralPath $CliPath)) {
        return
    }
    try {
        & $CliPath daemon stop --format compact | Out-Null
        Start-Sleep -Milliseconds 500
    }
    catch {
        Write-Warning "Could not stop existing mi-lsp daemon before install: $($_.Exception.Message)"
    }
}

function Invoke-WithRetry {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Action,
        [Parameter(Mandatory = $true)][string]$Description
    )
    for ($attempt = 1; $attempt -le 5; $attempt++) {
        try {
            & $Action
            return
        }
        catch {
            if ($attempt -eq 5) {
                throw
            }
            Write-Warning "$Description failed on attempt $attempt; retrying after file-lock delay."
            Start-Sleep -Milliseconds (250 * $attempt)
        }
    }
}

if ([string]::IsNullOrWhiteSpace($Rid)) {
    $Rid = Get-HostRid
}
Assert-SupportedRid -Value $Rid

$release = Get-Release -Repo $Repo
$version = $release.tag_name.TrimStart('v')
$archiveName = "mi-lsp_${version}_${Rid}.zip"
$checksumName = "mi-lsp_${version}_checksums.txt"
$archiveAsset = Find-Asset -Release $release -Name $archiveName
$checksumAsset = Find-Asset -Release $release -Name $checksumName

$plan = [pscustomobject]@{
    Repo = $Repo
    Version = $release.tag_name
    Rid = $Rid
    Archive = $archiveName
    Checksums = $checksumName
    InstallDir = $InstallDir
    Skill = 'not_installed_by_this_script'
}

if ($DryRun) {
    $plan | Format-List
    return
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("mi-lsp-install-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
    $archivePath = Join-Path $tmp $archiveName
    $checksumPath = Join-Path $tmp $checksumName
    Invoke-Download -Asset $archiveAsset -OutFile $archivePath -Tag $release.tag_name
    Invoke-Download -Asset $checksumAsset -OutFile $checksumPath -Tag $release.tag_name

    $expected = Get-ChecksumForAsset -ChecksumFile $checksumPath -AssetName $archiveName
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        throw "Checksum mismatch for '$archiveName'. Expected $expected, got $actual."
    }

    $extractDir = Join-Path $tmp 'extract'
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force
    $sourceCli = Get-ChildItem -LiteralPath $extractDir -Recurse -File -Filter 'mi-lsp.exe' | Select-Object -First 1
    if (-not $sourceCli) {
        throw "Extracted archive did not contain mi-lsp.exe."
    }
    $sourceWorkerDir = Get-ChildItem -LiteralPath $extractDir -Recurse -Directory |
        Where-Object { $_.FullName -replace '/', '\' -like "*\workers\$Rid" } |
        Select-Object -First 1
    if (-not $sourceWorkerDir) {
        throw "Extracted archive did not contain workers/$Rid."
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $targetCli = Join-Path $InstallDir 'mi-lsp.exe'
    $targetWorkersRoot = Join-Path $InstallDir 'workers'
    $targetWorkerDir = Join-Path $targetWorkersRoot $Rid
    New-Item -ItemType Directory -Force -Path $targetWorkersRoot | Out-Null
    $resolvedWorkersRoot = (Resolve-Path -LiteralPath $targetWorkersRoot).Path.TrimEnd('\')
    $resolvedWorkerDir = [System.IO.Path]::GetFullPath($targetWorkerDir)
    if (-not $resolvedWorkerDir.StartsWith($resolvedWorkersRoot + '\', [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to replace worker directory outside install workers root: $targetWorkerDir"
    }

    Stop-ExistingDaemon -CliPath $targetCli
    if (Test-Path -LiteralPath $targetWorkerDir) {
        Invoke-WithRetry -Description "Removing existing worker bundle" -Action {
            Remove-Item -LiteralPath $targetWorkerDir -Recurse -Force
        }
    }
    Invoke-WithRetry -Description "Copying mi-lsp executable" -Action {
        Copy-Item -LiteralPath $sourceCli.FullName -Destination $targetCli -Force
    }
    Invoke-WithRetry -Description "Copying worker bundle" -Action {
        Copy-Item -LiteralPath $sourceWorkerDir.FullName -Destination $targetWorkersRoot -Recurse -Force
    }

    Ensure-Path -Directory $InstallDir
    Stop-ExistingDaemon -CliPath $targetCli

    if (-not $SkipWorkerInstall) {
        & $targetCli worker install --rid $Rid --format compact | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "mi-lsp worker install failed for RID '$Rid'."
        }
    }

    & $targetCli version --format toon | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw 'mi-lsp version verification failed.'
    }
    & $targetCli worker status --format compact | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw 'mi-lsp worker status verification failed.'
    }

    Write-Host "mi-lsp $($release.tag_name) installed at $targetCli"
}
finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

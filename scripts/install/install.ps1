[CmdletBinding()]
param(
    [string]$Repo = 'fgpaz/mi-lsp',
    [string]$Rid = '',
    [string]$InstallDir = (Join-Path $HOME 'bin'),
    [string]$GitHubToken = $env:GITHUB_TOKEN,
    [switch]$DryRun,
    [switch]$NoPathUpdate,
    [switch]$SkipWorkerInstall,
    [string]$ValidateArchive = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
trap { Write-Error $_; exit 1 }

# SEC-08: warn if GITHUB_TOKEN is in environment (token should not be embedded in scripts)
if (-not [string]::IsNullOrWhiteSpace($env:GITHUB_TOKEN)) {
    Write-Warning "GITHUB_TOKEN environment variable is set. It will be used for authentication during download. Ensure you trust this environment and do not commit credentials in history."
}

function Ensure-ZipFileSupport {
    $zipFileType = 'System.IO.Compression.ZipFile' -as [type]
    if ($null -eq $zipFileType) {
        foreach ($assemblyName in @('System.IO.Compression', 'System.IO.Compression.FileSystem')) {
            try {
                Add-Type -AssemblyName $assemblyName -ErrorAction Stop
            }
            catch {
                if ($assemblyName -eq 'System.IO.Compression.FileSystem') {
                    throw "Could not load $assemblyName required for ZIP archive validation: $($_.Exception.Message)"
                }
            }
            $zipFileType = 'System.IO.Compression.ZipFile' -as [type]
            if ($null -ne $zipFileType) { break }
        }
    }
    if ($null -eq $zipFileType) {
        throw 'System.IO.Compression.ZipFile is unavailable; refusing to process the archive.'
    }
}

function Get-NormalizedFullPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    $full = [System.IO.Path]::GetFullPath($Path)
    $root = [System.IO.Path]::GetPathRoot($full)
    if ($full.Length -gt $root.Length) {
        return $full.TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
    }
    return $full
}

function Assert-PathLexicallyUnder {
    param(
        [Parameter(Mandatory = $true)][string]$Parent,
        [Parameter(Mandatory = $true)][string]$Child
    )
    $parentFull = Get-NormalizedFullPath -Path $Parent
    $childFull = Get-NormalizedFullPath -Path $Child
    $separator = [string][System.IO.Path]::DirectorySeparatorChar
    $comparison = if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) { [System.StringComparison]::OrdinalIgnoreCase } else { [System.StringComparison]::Ordinal }
    if ($childFull.Equals($parentFull, $comparison)) { return }
    if (-not $childFull.StartsWith($parentFull + $separator, $comparison)) {
        throw "Path '$Child' escaped lexical root '$Parent'."
    }
}

function Assert-DirectoryRootSafe {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [string]$ParentRoot = ''
    )
    $item = Get-Item -LiteralPath $Root -Force
    if (-not $item.PSIsContainer) { throw "Extraction root is not a directory: $Root" }
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Extraction root is a symlink or reparse point: $Root"
    }
    if (-not [string]::IsNullOrWhiteSpace($ParentRoot)) {
        Assert-PathLexicallyUnder -Parent $ParentRoot -Child $item.FullName
    }
}

function Assert-ZipArchiveSafe {
    param(
        [Parameter(Mandatory = $true)][string]$ArchivePath,
        [string]$DestinationRoot = '',
        [string]$ParentRoot = ''
    )
    if (-not (Test-Path -LiteralPath $ArchivePath -PathType Leaf)) {
        throw "Archive not found: $ArchivePath"
    }
    $destination = if ([string]::IsNullOrWhiteSpace($DestinationRoot)) { Split-Path -Parent $ArchivePath } else { $DestinationRoot }
    $destination = (New-Item -ItemType Directory -Force -Path $destination).FullName
    Assert-DirectoryRootSafe -Root $destination -ParentRoot $ParentRoot
    $destination = Get-NormalizedFullPath -Path $destination
    Ensure-ZipFileSupport
    $zip = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $ArchivePath).Path)
    try {
        foreach ($entry in $zip.Entries) {
            $name = [string]$entry.FullName
            if ([string]::IsNullOrWhiteSpace($name) -or $name.Contains('\') -or $name -match '^(?:/|//|[A-Za-z]:)' -or $name -match '(^|/)\.\.(?:/|$)') {
                throw "Archive contains an unsafe path member: $name"
            }
            $mode = ([int64]$entry.ExternalAttributes -shr 16) -band 0xF000
            if ($mode -ne 0 -and $mode -ne 0x4000 -and $mode -ne 0x8000) {
                throw "Archive contains a symlink, hardlink, or special member: $name"
            }
            $candidate = [System.IO.Path]::GetFullPath([System.IO.Path]::Combine($destination, $name.Replace('/', [System.IO.Path]::DirectorySeparatorChar)))
            if ($candidate -eq $destination -or -not $candidate.StartsWith($destination + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
                throw "Archive member destination escaped its staging root: $name"
            }
        }
    }
    finally {
        $zip.Dispose()
    }
}

function Expand-ZipArchiveSafely {
    param(
        [Parameter(Mandatory = $true)][string]$ArchivePath,
        [Parameter(Mandatory = $true)][string]$DestinationRoot,
        [string]$ParentRoot = ''
    )
    Assert-DirectoryRootSafe -Root $DestinationRoot -ParentRoot $ParentRoot
    $destination = Get-NormalizedFullPath -Path $DestinationRoot
    Ensure-ZipFileSupport
    $zip = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $ArchivePath).Path)
    try {
        foreach ($entry in $zip.Entries) {
            $name = [string]$entry.FullName
            if ([string]::IsNullOrWhiteSpace($name) -or $name.Contains('\') -or $name -match '^(?:/|//|[A-Za-z]:)' -or $name -match '(^|/)\.\.(?:/|$)') {
                throw "Archive contains an unsafe path member: $name"
            }
            $mode = ([int64]$entry.ExternalAttributes -shr 16) -band 0xF000
            if ($mode -ne 0 -and $mode -ne 0x4000 -and $mode -ne 0x8000) {
                throw "Archive contains a symlink, hardlink, or special member: $name"
            }
            $candidate = [System.IO.Path]::GetFullPath([System.IO.Path]::Combine($destination, $name.Replace('/', [System.IO.Path]::DirectorySeparatorChar)))
            if ($candidate -eq $destination -or -not $candidate.StartsWith($destination + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
                throw "Archive member destination escaped its staging root: $name"
            }
            if ($name.EndsWith('/')) {
                New-Item -ItemType Directory -Force -Path $candidate | Out-Null
                Assert-DirectoryRootSafe -Root $candidate -ParentRoot $destination
                continue
            }
            $parent = Split-Path -Parent $candidate
            New-Item -ItemType Directory -Force -Path $parent | Out-Null
            Assert-DirectoryRootSafe -Root $parent -ParentRoot $destination
            if (Test-Path -LiteralPath $candidate) {
                $existing = Get-Item -LiteralPath $candidate -Force
                if (($existing.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Archive extraction encountered a reparse point: $candidate" }
                throw "Archive contains duplicate destination path: $name"
            }
            $inputStream = $null
            $outputStream = $null
            try {
                $inputStream = $entry.Open()
                $outputStream = [System.IO.File]::Open($candidate, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
                $buffer = New-Object byte[] 81920
                while (($count = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                    $outputStream.Write($buffer, 0, $count)
                }
            }
            finally {
                if ($null -ne $outputStream) { $outputStream.Dispose() }
                if ($null -ne $inputStream) { $inputStream.Dispose() }
            }
            $created = Get-Item -LiteralPath $candidate -Force
            if (($created.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Archive extraction produced a reparse point: $candidate" }
        }
    }
    finally {
        $zip.Dispose()
    }
}

function Assert-ConfinedExtraction {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [string]$ParentRoot = ''
    )
    Assert-DirectoryRootSafe -Root $Root -ParentRoot $ParentRoot
    $resolvedRoot = Get-NormalizedFullPath -Path (Resolve-Path -LiteralPath $Root).Path
    foreach ($item in @(Get-ChildItem -LiteralPath $Root -Force -Recurse)) {
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Extraction produced a link or reparse point: $($item.FullName)"
        }
        $full = [System.IO.Path]::GetFullPath($item.FullName)
        if (-not $full.StartsWith($resolvedRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "Extraction escaped confined root: $($item.FullName)"
        }
        $real = (Get-Item -LiteralPath $item.FullName -Force).FullName
        if (-not $real.StartsWith($resolvedRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "Extraction resolved outside confined root: $($item.FullName)"
        }
    }
}

if (-not [string]::IsNullOrWhiteSpace($ValidateArchive)) {
    Assert-ZipArchiveSafe -ArchivePath $ValidateArchive
    Write-Output 'PASS: zip archive members are confined and link-free'
    return
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

function Invoke-ConfinedActivation {
    param([Parameter(Mandatory=$true)][string]$InstallRoot,[Parameter(Mandatory=$true)][string]$SourceCli,[Parameter(Mandatory=$true)][string]$SourceWorker,[Parameter(Mandatory=$true)][string]$Rid)
    $root=(New-Item -ItemType Directory -Force -Path $InstallRoot).FullName.TrimEnd('\'); $workers=(New-Item -ItemType Directory -Force -Path (Join-Path $root 'workers')).FullName.TrimEnd('\')
    $cli=Join-Path $root 'mi-lsp.exe'; $worker=Join-Path $workers $Rid
    Assert-PathLexicallyUnder -Parent $root -Child $cli; Assert-PathLexicallyUnder -Parent $workers -Child $worker
    $backup=Join-Path $root ('.mi-lsp-backup-'+[guid]::NewGuid().ToString('N')); New-Item -ItemType Directory -Path $backup | Out-Null
    $oldCli=$false; $oldWorker=$false; $committed=$false
    $testMode = $env:MI_LSP_INSTALL_TEST_MODE -eq 'activation'
    try {
        if (-not $testMode) { Stop-ExistingDaemon -CliPath $cli }
        if (Test-Path -LiteralPath $cli) { Move-Item $cli (Join-Path $backup 'mi-lsp.exe'); $oldCli=$true }
        if (Test-Path -LiteralPath $worker) { Move-Item $worker (Join-Path $backup 'worker'); $oldWorker=$true }
        Copy-Item $SourceCli $cli -Force
        $testCli = if ($testMode) { $testPath = $cli + '.ps1'; Copy-Item $cli $testPath -Force; $testPath } else { $cli }
        if ($env:MI_LSP_INSTALL_FAIL_PHASE -eq 'cli-activation') { throw 'Injected failure after CLI activation.' }
        New-Item -ItemType Directory -Force -Path $worker | Out-Null
        Copy-Item (Join-Path $SourceWorker '*') $worker -Recurse -Force
        if ($env:MI_LSP_INSTALL_FAIL_PHASE -eq 'worker-activation') { throw 'Injected failure after worker activation.' }
        $invokeCli = { param([string[]]$Arguments) if ($testMode) { & (Get-Command pwsh -ErrorAction Stop).Source -NoProfile -File $testCli @Arguments } else { & $cli @Arguments } }
        if (-not $testMode -and -not $SkipWorkerInstall) { & $cli worker install --rid $Rid --format compact | Out-Host; if ($LASTEXITCODE -ne 0) { throw "mi-lsp worker install failed for RID '$Rid'." } }
        & $invokeCli @('version','--format','toon') | Out-Host; if ($LASTEXITCODE -ne 0) { throw 'mi-lsp version verification failed.' }
        & $invokeCli @('worker','status','--format','compact') | Out-Host; if ($LASTEXITCODE -ne 0) { throw 'mi-lsp worker status verification failed.' }
        $committed=$true
    } finally {
        if (-not $committed) {
            if (Test-Path -LiteralPath $cli) { Remove-Item $cli -Force }; if (Test-Path -LiteralPath ($cli + '.ps1')) { Remove-Item ($cli + '.ps1') -Force }; if (Test-Path -LiteralPath $worker) { Remove-Item $worker -Recurse -Force }
            if ($oldCli) { Move-Item (Join-Path $backup 'mi-lsp.exe') $cli }; if ($oldWorker) { Move-Item (Join-Path $backup 'worker') $worker }
        }
        if (Test-Path -LiteralPath $backup) { Remove-Item $backup -Recurse -Force }
    }
}

if ($env:MI_LSP_INSTALL_TEST_MODE -eq 'activation') {
    if ([string]::IsNullOrWhiteSpace($env:MI_LSP_TEST_INSTALL_ROOT) -or [string]::IsNullOrWhiteSpace($env:MI_LSP_TEST_SOURCE_CLI) -or [string]::IsNullOrWhiteSpace($env:MI_LSP_TEST_SOURCE_WORKER) -or [string]::IsNullOrWhiteSpace($env:MI_LSP_TEST_RID)) { throw 'Activation test mode requires test paths and RID.' }
    Invoke-ConfinedActivation -InstallRoot $env:MI_LSP_TEST_INSTALL_ROOT -SourceCli $env:MI_LSP_TEST_SOURCE_CLI -SourceWorker $env:MI_LSP_TEST_SOURCE_WORKER -Rid $env:MI_LSP_TEST_RID
    Write-Output 'PASS: activation test mode'; return
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

if ($DryRun) {
    [pscustomobject]@{ Repo=$Repo; Version='<network lookup skipped>'; Rid=$Rid; Archive='<not resolved>'; InstallDir=$InstallDir; DryRun=$true } | Format-List
    return
}
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
    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
    Assert-DirectoryRootSafe -Root $tmp
    Assert-PathLexicallyUnder -Parent $tmp -Child $extractDir
    Assert-ZipArchiveSafe -ArchivePath $archivePath -DestinationRoot $extractDir -ParentRoot $tmp
    Expand-ZipArchiveSafely -ArchivePath $archivePath -DestinationRoot $extractDir -ParentRoot $tmp
    Assert-ConfinedExtraction -Root $extractDir -ParentRoot $tmp
    $sourceCli = Get-ChildItem -LiteralPath $extractDir -Force -Recurse -File -Filter 'mi-lsp.exe' | Select-Object -First 1
    if (-not $sourceCli) {
        throw "Extracted archive did not contain mi-lsp.exe."
    }
    $sourceWorkerDir = Get-ChildItem -LiteralPath $extractDir -Recurse -Directory |
        Where-Object { $_.FullName -replace '/', '\' -like "*\workers\$Rid" } |
        Select-Object -First 1
    if (-not $sourceWorkerDir) {
        throw "Extracted archive did not contain workers/$Rid."
    }
    $workerManifestPath = Join-Path $sourceWorkerDir.FullName 'worker-manifest.json'
    if (-not (Test-Path -LiteralPath $workerManifestPath -PathType Leaf)) {
        throw "Extracted archive did not contain workers/$Rid/worker-manifest.json."
    }
    $workerManifest = Get-Content -LiteralPath $workerManifestPath -Raw | ConvertFrom-Json
    if ($workerManifest.schema -ne 'mi-lsp-worker-manifest/v1' -or $workerManifest.rid -ne $Rid -or $workerManifest.protocol -ne 'mi-lsp-v1.1') {
        throw "Worker manifest metadata does not match RID '$Rid'."
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Invoke-ConfinedActivation -InstallRoot $InstallDir -SourceCli $sourceCli.FullName -SourceWorker $sourceWorkerDir.FullName -Rid $Rid
    $targetCli = Join-Path $InstallDir 'mi-lsp.exe'
    Ensure-Path -Directory $InstallDir

    Write-Host "mi-lsp $($release.tag_name) installed at $targetCli"
}
finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$root = Join-Path ([System.IO.Path]::GetTempPath()) ('mi-lsp-archive-safety-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $root | Out-Null
$installer = Join-Path $PSScriptRoot '..\install\install.ps1'
$verifier = Join-Path $PSScriptRoot '..\release\verify-worker-bundles.ps1'
$manifestGenerator = Join-Path $PSScriptRoot '..\release\worker-manifest.ps1'
$isWindowsHost = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)
$isMacHost = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::OSX)

function Get-PowerShellHost {
    $candidates = @()
    if ($PSVersionTable.PSEdition -eq 'Desktop') {
        $candidates += Join-Path $PSHOME 'powershell.exe'
        $command = Get-Command powershell.exe -ErrorAction SilentlyContinue
    }
    else {
        $candidates += Join-Path $PSHOME 'pwsh.exe'
        $command = Get-Command pwsh.exe -ErrorAction SilentlyContinue
    }
    if ($command) { $candidates += $command.Source }
    foreach ($candidate in $candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    throw 'Could not resolve the current PowerShell host executable.'
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
                    throw "Could not load $assemblyName required for ZIP fixture creation: $($_.Exception.Message)"
                }
            }
            $zipFileType = 'System.IO.Compression.ZipFile' -as [type]
            if ($null -ne $zipFileType) { break }
        }
    }
    if ($null -eq $zipFileType) {
        throw 'System.IO.Compression.ZipFile is unavailable; refusing to create fixtures.'
    }
}

function New-ZipFixture {
    param([string]$Path, [string]$Name, [int]$ExternalAttributes = 0)
    Ensure-ZipFileSupport
    $archive = [System.IO.Compression.ZipFile]::Open($Path, [System.IO.Compression.ZipArchiveMode]::Create)
    try {
        $entry = $archive.CreateEntry($Name)
        $entry.ExternalAttributes = $ExternalAttributes
        $stream = $entry.Open()
        $stream.Dispose()
    }
    finally { $archive.Dispose() }
}

function Assert-RejectedArchive {
    param([string]$Path)
    $hostExecutable = Get-PowerShellHost
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $installer -ValidateArchive $Path
    if ($LASTEXITCODE -eq 0) {
        throw "Archive unexpectedly accepted: $Path"
    }
}

function Assert-RejectedBundle {
    param(
        [string]$Workers,
        [string]$Expected,
        [string]$Rid = 'win-x64'
    )
    $hostExecutable = Get-PowerShellHost
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $verifier -WorkersRoot $Workers -ExpectedManifestRoot $Expected -Rids $Rid -AllowPartialRoot | Out-Null
    if ($LASTEXITCODE -eq 0) {
        throw "Worker bundle unexpectedly accepted for adversarial fixture: $Workers"
    }
}

function New-HardLinkIfSupported {
    param([string]$Path, [string]$Target)
    try {
        if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) {
            New-Item -ItemType HardLink -Path $Path -Target $Target | Out-Null
        }
        else {
            $ln = Get-Command ln -ErrorAction Stop
            & $ln.Source $Target $Path
            if ($LASTEXITCODE -ne 0) { throw 'ln failed' }
        }
        return $true
    }
    catch {
        Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
        return $false
    }
}

function New-ReparseIfSupported {
    param([string]$Path, [string]$Target)
    try {
        if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) {
            New-Item -ItemType SymbolicLink -Path $Path -Target $Target | Out-Null
        }
        else {
            $ln = Get-Command ln -ErrorAction Stop
            & $ln.Source '-s' $Target $Path
            if ($LASTEXITCODE -ne 0) { throw 'ln failed' }
        }
        return $true
    }
    catch {
        Remove-Item -LiteralPath $Path -Force -Recurse -ErrorAction SilentlyContinue
        return $false
    }
}

try {
    Ensure-ZipFileSupport
    $hostExecutable = Get-PowerShellHost
    . $manifestGenerator
    $cases = @(
        @{ Name = 'traversal.zip'; Entry = '../escape.txt'; Attr = 0 },
        @{ Name = 'absolute.zip'; Entry = '/escape.txt'; Attr = 0 },
        @{ Name = 'drive.zip'; Entry = 'C:/escape.txt'; Attr = 0 },
        @{ Name = 'symlink.zip'; Entry = 'link'; Attr = ([int]0xA000 -shl 16) }
    )
    foreach ($case in $cases) {
        $path = Join-Path $root $case.Name
        New-ZipFixture -Path $path -Name $case.Entry -ExternalAttributes $case.Attr
        Assert-RejectedArchive -Path $path
    }

    $fixture = Join-Path $root 'bundle'
    $workers = Join-Path $fixture 'workers'
    $expected = Join-Path $fixture 'expected'
    $bundle = Join-Path $workers 'win-x64'
    New-Item -ItemType Directory -Force -Path $bundle, $expected | Out-Null
    $workerFile = Join-Path $bundle 'MiLsp.Worker.exe'
    $nestedWorkerFile = Join-Path $bundle 'config\worker.json'
    $hiddenWorkerFile = Join-Path $bundle '.hidden-worker'
    Set-Content -LiteralPath $workerFile -Value 'worker' -NoNewline
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $nestedWorkerFile) | Out-Null
    Set-Content -LiteralPath $nestedWorkerFile -Value '{}' -NoNewline
    Set-Content -LiteralPath $hiddenWorkerFile -Value 'hidden' -NoNewline
    if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) {
        $hiddenItem = Get-Item -LiteralPath $hiddenWorkerFile -Force
        $hiddenItem.Attributes = $hiddenItem.Attributes -bor [System.IO.FileAttributes]::Hidden
    }
    $manifestPath = Join-Path $bundle 'worker-manifest.json'
    $manifest = New-WorkerManifest -Rid 'win-x64' -WorkerDir $bundle -ManifestPath $manifestPath
    if (-not (@($manifest.files | ForEach-Object { $_.path }) -contains '.hidden-worker')) {
        throw 'Generated worker manifest omitted a hidden worker file.'
    }
    $outsideManifestRejected = $false
    try { New-WorkerManifest -Rid 'win-x64' -WorkerDir $bundle -ManifestPath (Join-Path $fixture 'outside.json') | Out-Null }
    catch { $outsideManifestRejected = $true }
    if (-not $outsideManifestRejected) { throw 'Worker manifest generator accepted a manifest path outside the lexical worker root.' }
    Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $expected 'win-x64.json')
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $verifier -WorkersRoot $workers -ExpectedManifestRoot $expected -Rids win-x64 -AllowPartialRoot | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw 'Generated worker manifest fixture was rejected by the verifier.'
    }

    $completeWorkers = Join-Path $fixture 'complete-workers'
    $completeExpected = Join-Path $fixture 'complete-expected'
    $completeRids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64', 'osx-arm64', 'osx-x64')
    New-Item -ItemType Directory -Force -Path $completeWorkers, $completeExpected | Out-Null
    foreach ($completeRid in $completeRids) {
        $completeBundle = Join-Path $completeWorkers $completeRid
        New-Item -ItemType Directory -Force -Path $completeBundle | Out-Null
        Set-Content -LiteralPath (Join-Path $completeBundle 'payload.bin') -Value $completeRid -NoNewline
        $completeManifestPath = Join-Path $completeBundle 'worker-manifest.json'
        New-WorkerManifest -Rid $completeRid -WorkerDir $completeBundle -ManifestPath $completeManifestPath | Out-Null
        Copy-Item -LiteralPath $completeManifestPath -Destination (Join-Path $completeExpected "$completeRid.json")
    }
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $verifier -WorkersRoot $completeWorkers -ExpectedManifestRoot $completeExpected | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Exactly-six-RID worker root was rejected.' }
    $hiddenRid = Join-Path $completeWorkers '.unexpected-rid'
    New-Item -ItemType Directory -Force -Path $hiddenRid | Out-Null
    Set-Content -LiteralPath (Join-Path $hiddenRid 'payload.bin') -Value 'unexpected' -NoNewline
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $verifier -WorkersRoot $completeWorkers -ExpectedManifestRoot $completeExpected | Out-Null
    if ($LASTEXITCODE -eq 0) { throw 'Worker verifier accepted an unexpected hidden RID directory.' }
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $verifier -WorkersRoot $completeWorkers -ExpectedManifestRoot $completeExpected -Rids plan9-x64 | Out-Null
    if ($LASTEXITCODE -eq 0) { throw 'Worker verifier accepted a RID outside the exact allowlist.' }
    & $hostExecutable -NoProfile -ExecutionPolicy Bypass -File $verifier -WorkersRoot $completeWorkers -ExpectedManifestRoot $completeExpected -Rids '' | Out-Null
    if ($LASTEXITCODE -eq 0) { throw 'Worker verifier accepted an empty RID.' }

    $nestedManifest = Join-Path $bundle 'config\worker-manifest.json'
    Set-Content -LiteralPath $nestedManifest -Value 'unexpected nested manifest' -NoNewline
    Assert-RejectedBundle -Workers $workers -Expected $expected
    Remove-Item -LiteralPath $nestedManifest -Force

    $hiddenExtra = Join-Path $bundle '.unexpected-hidden'
    Set-Content -LiteralPath $hiddenExtra -Value 'extra' -NoNewline
    Assert-RejectedBundle -Workers $workers -Expected $expected
    Remove-Item -LiteralPath $hiddenExtra -Force

    $hardlink = Join-Path $bundle 'hardlink-extra'
    if (New-HardLinkIfSupported -Path $hardlink -Target $workerFile) {
        $generatorRejected = $false
        try { New-WorkerManifest -Rid 'win-x64' -WorkerDir $bundle -ManifestPath $manifestPath | Out-Null }
        catch { $generatorRejected = $true }
        if (-not $generatorRejected) { throw 'Worker manifest generator accepted a hardlinked file.' }
        Assert-RejectedBundle -Workers $workers -Expected $expected
        Remove-Item -LiteralPath $hardlink -Force
        Write-Output 'PASS: worker manifest generator and verifier rejected hardlink fixtures'
    }
    else {
        if ($isMacHost -or -not $isWindowsHost) { throw 'Host refused hardlink fixture creation outside Windows; refusing to omit the test.' }
        Write-Output 'SKIP: Windows host refused hardlink fixture creation without link privilege'
    }

    $reparse = Join-Path $bundle 'reparse-extra'
    if (New-ReparseIfSupported -Path $reparse -Target $workerFile) {
        $generatorRejected = $false
        try { New-WorkerManifest -Rid 'win-x64' -WorkerDir $bundle -ManifestPath $manifestPath | Out-Null }
        catch { $generatorRejected = $true }
        if (-not $generatorRejected) { throw 'Worker manifest generator accepted a reparse/symlink fixture.' }
        Assert-RejectedBundle -Workers $workers -Expected $expected
        Remove-Item -LiteralPath $reparse -Force -Recurse
        Write-Output 'PASS: worker manifest generator and verifier rejected reparse/symlink fixtures'
    }
    else {
        if ($isMacHost -or -not $isWindowsHost) { throw 'Host refused reparse/symlink fixture creation outside Windows; refusing to omit the test.' }
        Write-Output 'SKIP: Windows host refused reparse/symlink fixture creation without link privilege'
    }

    $swapped = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $swapped.rid = 'win-arm64'
    Set-Content -LiteralPath $manifestPath -Value ($swapped | ConvertTo-Json -Compress -Depth 6)
    Assert-RejectedBundle -Workers $workers -Expected $expected

    Write-Output 'PASS: generated worker manifest verifies hidden files and rejects hidden extras and RID-swapped manifests'
    Write-Output 'PASS: PowerShell rejects traversal, absolute, drive, and symlink archive members'
}
finally {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}

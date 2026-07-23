[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$WorkersRoot,
    [Parameter(Mandatory = $true)][string]$ExpectedManifestRoot,
    [string[]]$Rids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64', 'osx-arm64', 'osx-x64'),
    [switch]$ProbeHost,
    [switch]$AllowPartialRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
trap { Write-Error $_; exit 1 }

$expectedProtocol = 'mi-lsp-v1.1'
$allowedRids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64', 'osx-arm64', 'osx-x64')
. (Join-Path $PSScriptRoot 'worker-manifest.ps1')

function Assert-SafeManifestPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path) -or $Path.Contains('\') -or $Path -match '^(?:/|[A-Za-z]:|//)' -or $Path -match '(^|/)\.\.(/|$)') {
        throw "Unsafe worker manifest path '$Path'."
    }
}

function Write-UInt32BigEndian {
    param(
        [Parameter(Mandatory = $true)][byte[]]$Buffer,
        [Parameter(Mandatory = $true)][uint32]$Value
    )
    if ($Buffer.Length -lt 4) { throw 'Big-endian buffer must contain at least four bytes.' }
    $Buffer[0] = [byte](($Value -shr 24) -band 0xFF)
    $Buffer[1] = [byte](($Value -shr 16) -band 0xFF)
    $Buffer[2] = [byte](($Value -shr 8) -band 0xFF)
    $Buffer[3] = [byte]($Value -band 0xFF)
}

function Read-UInt32BigEndian {
    param([Parameter(Mandatory = $true)][byte[]]$Buffer)
    if ($Buffer.Length -lt 4) { throw 'Big-endian buffer must contain at least four bytes.' }
    return [uint32]((([uint32]$Buffer[0] -shl 24) -bor ([uint32]$Buffer[1] -shl 16) -bor ([uint32]$Buffer[2] -shl 8) -bor [uint32]$Buffer[3]))
}

function Stop-ProbeProcess {
    param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process)
    if (-not $Process.HasExited) {
        try { $Process.Kill($true) }
        catch { $Process.Kill() }
    }
}

function Assert-WorkersRootShape {
    param([Parameter(Mandatory = $true)][System.IO.DirectoryInfo]$Root)

    $children = @(Get-ChildItem -LiteralPath $Root.FullName -Force)
    foreach ($child in $children) {
        if (($child.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Workers root contains a reparse point or symlink: '$($child.FullName)'."
        }
        if (-not $child.PSIsContainer) {
            throw "Workers root contains an unexpected direct file: '$($child.FullName)'."
        }
    }
    $directories = @($children | Where-Object { $_.PSIsContainer })
    $names = @($directories | ForEach-Object { $_.Name })
    if ($directories.Count -ne $allowedRids.Count -or (Compare-Object -ReferenceObject ($allowedRids | Sort-Object) -DifferenceObject ($names | Sort-Object))) {
        throw "Workers root must contain exactly the six RID directories: $($allowedRids -join ', ')."
    }
}

function Get-ExpectedManifestPath {
    param([Parameter(Mandatory = $true)][string]$Rid)
    $flat = Join-Path $ExpectedManifestRoot "$Rid.json"
    if (Test-Path -LiteralPath $flat -PathType Leaf) { return $flat }
    $nested = Join-Path (Join-Path $ExpectedManifestRoot $Rid) 'worker-manifest.json'
    if (Test-Path -LiteralPath $nested -PathType Leaf) { return $nested }
    throw "Expected worker manifest for RID '$Rid' was not found."
}

function Get-ManifestText {
    param([Parameter(Mandatory = $true)][string]$Path)
    $bytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path).Path)
    $offset = if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) { 3 } else { 0 }
    return [System.Text.Encoding]::UTF8.GetString($bytes, $offset, $bytes.Length - $offset)
}

function Get-HostRid {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    $archPart = switch ($arch) {
        'x64' { 'x64' }
        'amd64' { 'x64' }
        'arm64' { 'arm64' }
        default { return $null }
    }
    if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) { return "win-$archPart" }
    if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::OSX)) { return "osx-$archPart" }
    if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Linux)) { return "linux-$archPart" }
    return $null
}

function Read-ProcessBytesWithDeadline {
    param(
        [Parameter(Mandatory = $true)][System.IO.Stream]$Stream,
        [Parameter(Mandatory = $true)][byte[]]$Buffer,
        [Parameter(Mandatory = $true)][int]$Offset,
        [Parameter(Mandatory = $true)][int]$Count,
        [Parameter(Mandatory = $true)][datetime]$Deadline
    )
    $read = 0
    while ($read -lt $Count) {
        $remaining = [int][Math]::Ceiling(($Deadline - [datetime]::UtcNow).TotalMilliseconds)
        if ($remaining -le 0) { throw 'Worker protocol probe timed out while reading a response.' }
        $async = $null
        try {
            $async = $Stream.BeginRead($Buffer, $Offset + $read, $Count - $read, $null, $null)
            if (-not $async.AsyncWaitHandle.WaitOne($remaining)) { throw 'Worker protocol probe timed out while reading a response.' }
            $countRead = $Stream.EndRead($async)
            if ($countRead -eq 0) { throw 'Worker probe returned a truncated response.' }
            $read += $countRead
        }
        finally {
            if ($null -ne $async) { $async.AsyncWaitHandle.Close() }
        }
    }
}

function Invoke-WorkerProtocolProbe {
    param([Parameter(Mandatory = $true)][string]$WorkerPath)

    $request = [ordered]@{
        protocol_version = $expectedProtocol
        method = 'status'
        workspace = ''
        workspace_name = 'release-probe'
        backend_type = 'roslyn'
        repo_id = 'release-probe'
        repo_name = 'release-probe'
        repo_root = ''
        entrypoint_id = 'release-probe'
        entrypoint_path = ''
        entrypoint_type = 'project'
        payload = @{}
    }
    $payload = [System.Text.Encoding]::UTF8.GetBytes(($request | ConvertTo-Json -Compress -Depth 4))
    $header = [byte[]]::new(4)
    Write-UInt32BigEndian -Buffer $header -Value ([uint32]$payload.Length)
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = (Resolve-Path -LiteralPath $WorkerPath).Path
    $psi.WorkingDirectory = (Split-Path -Parent $psi.FileName)
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $psi
    if (-not $process.Start()) { throw "Could not start worker probe '$WorkerPath'." }
    $deadline = [datetime]::UtcNow.AddSeconds(30)
    try {
        $process.StandardInput.BaseStream.Write($header, 0, $header.Length)
        $process.StandardInput.BaseStream.Write($payload, 0, $payload.Length)
        $process.StandardInput.BaseStream.Flush()
        $process.StandardInput.Close()
        $responseHeader = [byte[]]::new(4)
        Read-ProcessBytesWithDeadline -Stream $process.StandardOutput.BaseStream -Buffer $responseHeader -Offset 0 -Count $responseHeader.Length -Deadline $deadline
        $length = Read-UInt32BigEndian -Buffer $responseHeader
        if ($length -eq 0 -or $length -gt 1048576) { throw "Worker probe returned invalid response length $length." }
        $responseBytes = [byte[]]::new([int]$length)
        Read-ProcessBytesWithDeadline -Stream $process.StandardOutput.BaseStream -Buffer $responseBytes -Offset 0 -Count $responseBytes.Length -Deadline $deadline
        $response = [System.Text.Encoding]::UTF8.GetString($responseBytes) | ConvertFrom-Json
        if (-not $response.ok -or $null -eq $response.items -or $response.items.Count -ne 1 -or $response.items[0].protocol_version -ne $expectedProtocol) {
            throw 'Worker protocol probe returned an invalid status response.'
        }
        $remaining = [int][Math]::Ceiling(($deadline - [datetime]::UtcNow).TotalMilliseconds)
        if ($remaining -le 0 -or -not $process.WaitForExit($remaining)) { throw 'Worker protocol probe timed out.' }
    }
    finally {
        Stop-ProbeProcess -Process $process
        $process.Dispose()
    }
}

if ($null -eq $Rids -or @($Rids).Count -eq 0) { throw 'Rids must contain at least one RID.' }
if (@($Rids | Where-Object { $allowedRids -notcontains $_ }).Count -gt 0) { throw "Rids must use only the exact allowlist: $($allowedRids -join ', ')." }
if (@($Rids).Count -ne @($Rids | Sort-Object -Unique).Count) { throw 'Rids must not contain duplicates.' }
$workersRootItem = Get-Item -LiteralPath $WorkersRoot -Force
if (-not $workersRootItem.PSIsContainer) { throw "Workers root is not a directory: '$WorkersRoot'." }
if (($workersRootItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Workers root is a reparse point or symlink: '$WorkersRoot'." }
if (-not $AllowPartialRoot) { Assert-WorkersRootShape -Root $workersRootItem }
$expectedRootItem = Get-Item -LiteralPath $ExpectedManifestRoot -Force
if (-not $expectedRootItem.PSIsContainer) { throw "Expected manifest root is not a directory: '$ExpectedManifestRoot'." }
if (($expectedRootItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Expected manifest root is a reparse point or symlink: '$ExpectedManifestRoot'." }
$workersRootResolved = $workersRootItem.FullName
$workersRootLexical = [System.IO.Path]::GetFullPath($WorkersRoot).TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
$expectedRootLexical = [System.IO.Path]::GetFullPath($ExpectedManifestRoot).TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
$hostRid = if ($ProbeHost) { Get-HostRid } else { $null }
$results = @()
foreach ($rid in $Rids) {
    $bundle = Join-Path $workersRootResolved $rid
    $bundleLexical = [System.IO.Path]::GetFullPath((Join-Path $WorkersRoot $rid))
    $separator = [string][System.IO.Path]::DirectorySeparatorChar
    $comparison = if (Test-WorkerManifestWindows) { [System.StringComparison]::OrdinalIgnoreCase } else { [System.StringComparison]::Ordinal }
    if (-not $bundleLexical.StartsWith($workersRootLexical + $separator, $comparison)) { throw "Worker bundle path escaped WorkersRoot for RID '$rid'." }
    if (-not (Test-Path -LiteralPath $bundle -PathType Container)) { throw "Worker bundle for RID '$rid' is missing." }
    Assert-WorkerBundleSafe -Root $bundle | Out-Null
    $manifestPath = Join-Path $bundle 'worker-manifest.json'
    $manifestItem = Get-Item -LiteralPath $manifestPath -Force -ErrorAction SilentlyContinue
    if ($null -eq $manifestItem -or $manifestItem.PSIsContainer) { throw "Worker manifest for RID '$rid' is missing." }
    $expectedPath = Get-ExpectedManifestPath -Rid $rid
    $expectedPathLexical = [System.IO.Path]::GetFullPath($expectedPath)
    if (-not $expectedPathLexical.StartsWith($expectedRootLexical + $separator, $comparison)) { throw "Expected manifest path escaped ExpectedManifestRoot for RID '$rid'." }
    if ((Get-ManifestText -Path $manifestPath) -cne (Get-ManifestText -Path $expectedPath)) { throw "Worker manifest mismatch for RID '$rid'." }

    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($manifest.schema -ne 'mi-lsp-worker-manifest/v1' -or $manifest.rid -ne $rid -or $manifest.protocol -ne $expectedProtocol) { throw "Worker manifest metadata mismatch for RID '$rid'." }
    $entries = @($manifest.files)
    if ($entries.Count -eq 0 -or [int]$manifest.file_count -ne $entries.Count) { throw "Worker manifest file list is invalid for RID '$rid'." }
    $manifestFullPath = [System.IO.Path]::GetFullPath($manifestItem.FullName)
    $actualFiles = @(Get-ChildItem -LiteralPath $bundle -Force -File -Recurse | Where-Object { [System.IO.Path]::GetFullPath($_.FullName) -ne $manifestFullPath })
    if ($actualFiles.Count -ne $entries.Count) { throw "Worker file count mismatch for RID '$rid'." }
    $entryPaths = @{}
    foreach ($entry in $entries) {
        Assert-SafeManifestPath -Path ([string]$entry.path)
        if ($entryPaths.ContainsKey($entry.path)) { throw "Duplicate worker manifest path '$($entry.path)'." }
        $entryPaths[$entry.path] = $true
        $path = Join-Path $bundle ($entry.path.Replace('/', [System.IO.Path]::DirectorySeparatorChar))
        $item = Get-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
        if ($null -eq $item -or $item.PSIsContainer) { throw "Manifest file '$($entry.path)' is missing for RID '$rid'." }
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
        if ($hash -ne ([string]$entry.sha256).ToLowerInvariant() -or $item.Length -ne [int64]$entry.size) { throw "Worker file hash/size mismatch for RID '$rid', file '$($entry.path)'." }
    }
    foreach ($file in $actualFiles) {
        $relative = Get-WorkerManifestRelativePath -Root $bundle -Path $file.FullName
        if (-not $entryPaths.ContainsKey($relative)) { throw "Unexpected worker file '$relative' for RID '$rid'." }
    }

    $mode = 'metadata'
    if ($rid -eq $hostRid) {
        $workerName = if ($rid -like 'win-*') { 'MiLsp.Worker.exe' } else { 'MiLsp.Worker' }
        $workerPath = Join-Path $bundle $workerName
        if (-not (Test-Path -LiteralPath $workerPath -PathType Leaf)) { throw "Host-compatible worker executable '$workerName' is missing for RID '$rid'." }
        Invoke-WorkerProtocolProbe -WorkerPath $workerPath
        $mode = 'probe'
    }
    $results += [pscustomobject]@{ rid = $rid; verification = $mode; file_count = $entries.Count; protocol = $manifest.protocol }
}
$results

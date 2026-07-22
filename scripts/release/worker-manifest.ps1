[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Test-WorkerManifestWindows {
    return [System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT
}

function Ensure-WorkerFileIdentityNativeType {
    if (-not (Test-WorkerManifestWindows)) { return }
    if ($null -eq ('MiLspNativeFileIdentity' -as [type])) {
        Add-Type @'
using System;
using System.Runtime.InteropServices;

public static class MiLspNativeFileIdentity
{
    [StructLayout(LayoutKind.Sequential)]
    public struct ByHandleFileInformation
    {
        public uint FileAttributes;
        public uint CreationTimeLow;
        public uint CreationTimeHigh;
        public uint LastAccessTimeLow;
        public uint LastAccessTimeHigh;
        public uint LastWriteTimeLow;
        public uint LastWriteTimeHigh;
        public uint VolumeSerialNumber;
        public uint FileSizeHigh;
        public uint FileSizeLow;
        public uint NumberOfLinks;
        public uint FileIndexHigh;
        public uint FileIndexLow;
    }

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    public static extern IntPtr CreateFile(
        string fileName,
        uint desiredAccess,
        uint shareMode,
        IntPtr securityAttributes,
        uint creationDisposition,
        uint flagsAndAttributes,
        IntPtr templateFile);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    public static extern bool GetFileInformationByHandle(
        IntPtr file,
        out ByHandleFileInformation information);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    public static extern bool CloseHandle(IntPtr handle);
}
'@
    }
}

function Get-WorkerManifestRelativePath {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$Path
    )

    $rootFull = [System.IO.Path]::GetFullPath((Get-Item -LiteralPath $Root -Force).FullName)
    $pathFull = [System.IO.Path]::GetFullPath((Get-Item -LiteralPath $Path -Force).FullName)
    $separator = [string][System.IO.Path]::DirectorySeparatorChar
    $alternateSeparator = [string][System.IO.Path]::AltDirectorySeparatorChar
    $rootNormalized = $rootFull.Replace($alternateSeparator, $separator)
    $pathNormalized = $pathFull.Replace($alternateSeparator, $separator)
    $rootName = $rootNormalized.TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
    $comparison = if (Test-WorkerManifestWindows) {
        [System.StringComparison]::OrdinalIgnoreCase
    }
    else {
        [System.StringComparison]::Ordinal
    }

    if ($pathNormalized.Equals($rootName, $comparison)) {
        return ''
    }
    $prefix = $rootName + $separator
    if (-not $pathNormalized.StartsWith($prefix, $comparison)) {
        throw "Worker manifest path '$Path' is outside worker root '$Root'."
    }
    return $pathNormalized.Substring($prefix.Length).Replace($separator, '/')
}

function Get-WorkerFileIdentity {
    param([Parameter(Mandatory = $true)][string]$Path)

    $item = Get-Item -LiteralPath $Path -Force
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Worker bundle contains a reparse point or symlink: $Path"
    }

    if (Test-WorkerManifestWindows) {
        Ensure-WorkerFileIdentityNativeType
        $handle = [MiLspNativeFileIdentity]::CreateFile(
            $item.FullName,
            0,
            7,
            [IntPtr]::Zero,
            3,
            0,
            [IntPtr]::Zero)
        if ($handle -eq [IntPtr]::new(-1)) {
            throw "Could not open worker file for identity verification: $Path"
        }
        try {
            $information = New-Object MiLspNativeFileIdentity+ByHandleFileInformation
            if (-not [MiLspNativeFileIdentity]::GetFileInformationByHandle($handle, [ref]$information)) {
                throw "Could not read worker file identity: $Path"
            }
            return [pscustomobject]@{
                Key = "win:$($information.VolumeSerialNumber):$($information.FileIndexHigh):$($information.FileIndexLow)"
                LinkCount = [int64]$information.NumberOfLinks
            }
        }
        finally {
            [void][MiLspNativeFileIdentity]::CloseHandle($handle)
        }
    }

    $stat = Get-Command stat -ErrorAction SilentlyContinue
    if (-not $stat) {
        throw 'stat is required to reject hardlinked worker files on this host.'
    }
    $output = & $stat.Source '-c' '%d:%i:%h' $item.FullName 2>$null
    if ($LASTEXITCODE -ne 0 -or ($output -join '').Trim() -notmatch '^\d+:\d+:\d+$') {
        $output = & $stat.Source '-f' '%d:%i:%l' $item.FullName 2>$null
    }
    $identity = ($output -join '').Trim()
    if ($LASTEXITCODE -ne 0 -or $identity -notmatch '^(\d+):(\d+):(\d+)$') {
        throw "Could not obtain a portable file identity for '$Path'; refusing the worker bundle."
    }
    return [pscustomobject]@{
        Key = "unix:$($matches[1]):$($matches[2])"
        LinkCount = [int64]$matches[3]
    }
}

function Assert-WorkerBundleSafe {
    param([Parameter(Mandatory = $true)][string]$Root)

    $rootItem = Get-Item -LiteralPath $Root -Force
    if (-not $rootItem.PSIsContainer) {
        throw "Worker bundle root is not a directory: $Root"
    }
    if (($rootItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Worker bundle root is a reparse point or symlink: $Root"
    }

    $rootFull = [System.IO.Path]::GetFullPath($rootItem.FullName).TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
    $separator = [string][System.IO.Path]::DirectorySeparatorChar
    $comparison = if (Test-WorkerManifestWindows) { [System.StringComparison]::OrdinalIgnoreCase } else { [System.StringComparison]::Ordinal }
    $seen = @{}
    foreach ($item in @(Get-ChildItem -LiteralPath $Root -Force -Recurse)) {
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Worker bundle contains a reparse point or symlink: $($item.FullName)"
        }
        $full = [System.IO.Path]::GetFullPath($item.FullName)
        if (-not $full.StartsWith($rootFull + $separator, $comparison)) {
            throw "Worker bundle path escaped its root: $($item.FullName)"
        }
        if (-not $item.PSIsContainer) {
            $identity = Get-WorkerFileIdentity -Path $item.FullName
            if ($identity.LinkCount -gt 1) {
                throw "Worker bundle contains a hardlinked file: $($item.FullName)"
            }
            if ($seen.ContainsKey($identity.Key)) {
                throw "Worker bundle contains hardlinked files: '$($seen[$identity.Key])' and '$($item.FullName)'."
            }
            $seen[$identity.Key] = $item.FullName
        }
    }
}

function New-WorkerManifest {
    param(
        [Parameter(Mandatory = $true)][string]$Rid,
        [Parameter(Mandatory = $true)][string]$WorkerDir,
        [Parameter(Mandatory = $true)][string]$ManifestPath,
        [string]$Protocol = 'mi-lsp-v1.1'
    )

    $workerDirLexical = [System.IO.Path]::GetFullPath($WorkerDir).TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
    $manifestFullPath = [System.IO.Path]::GetFullPath($ManifestPath)
    $separator = [string][System.IO.Path]::DirectorySeparatorChar
    $comparison = if (Test-WorkerManifestWindows) { [System.StringComparison]::OrdinalIgnoreCase } else { [System.StringComparison]::Ordinal }
    if (-not $manifestFullPath.StartsWith($workerDirLexical + $separator, $comparison)) {
        throw "Worker manifest path '$ManifestPath' is outside worker root '$WorkerDir'."
    }
    $workerDirItem = Get-Item -LiteralPath $WorkerDir -Force
    if (-not $workerDirItem.PSIsContainer) {
        throw "Worker bundle root is not a directory: $WorkerDir"
    }
    if (($workerDirItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Worker bundle root is a reparse point or symlink: $WorkerDir"
    }
    $root = $workerDirItem.FullName
    Assert-WorkerBundleSafe -Root $root | Out-Null
    $rootFull = [System.IO.Path]::GetFullPath($root).TrimEnd([char][System.IO.Path]::DirectorySeparatorChar, [char][System.IO.Path]::AltDirectorySeparatorChar)
    if (-not $manifestFullPath.StartsWith($rootFull + $separator, $comparison)) {
        throw "Worker manifest path '$ManifestPath' is outside resolved worker root '$root'."
    }

    $files = @(
        Get-ChildItem -LiteralPath $root -Force -File -Recurse |
            Where-Object { [System.IO.Path]::GetFullPath($_.FullName) -ne $manifestFullPath } |
            Sort-Object { Get-WorkerManifestRelativePath -Root $root -Path $_.FullName } |
            ForEach-Object {
                [ordered]@{
                    path = Get-WorkerManifestRelativePath -Root $root -Path $_.FullName
                    size = [int64]$_.Length
                    sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
                }
            }
    )

    if ($files.Count -eq 0) {
        throw "Worker bundle '$Rid' is empty."
    }

    $manifest = [ordered]@{
        schema = 'mi-lsp-worker-manifest/v1'
        rid = $Rid
        protocol = $Protocol
        file_count = $files.Count
        files = $files
    }
    $json = $manifest | ConvertTo-Json -Depth 8 -Compress
    $encoding = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($manifestFullPath, $json + [Environment]::NewLine, $encoding)
    return $manifest
}

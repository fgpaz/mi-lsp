[CmdletBinding()]
param(
    [string[]]$Rids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64'),
    [string]$OutDir = (Join-Path $PSScriptRoot '..\..\.goreleaser\workers'),
    [switch]$Clean
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$workerProject = Join-Path $repoRoot 'worker-dotnet\MiLsp.Worker\MiLsp.Worker.csproj'

if ($Clean -and (Test-Path $OutDir)) {
    Remove-Item -Recurse -Force $OutDir
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

foreach ($rid in $Rids) {
    $workerOut = Join-Path $OutDir $rid
    if (Test-Path $workerOut) {
        Remove-Item -Recurse -Force $workerOut
    }
    New-Item -ItemType Directory -Force -Path $workerOut | Out-Null

    Push-Location $repoRoot
    try {
        & dotnet publish $workerProject -c Release -r $rid --self-contained true -o $workerOut
        if ($LASTEXITCODE -ne 0) {
            throw "dotnet publish failed for RID '$rid'"
        }
    }
    finally {
        Pop-Location
    }
}

Get-ChildItem -Directory $OutDir | Select-Object Name, FullName

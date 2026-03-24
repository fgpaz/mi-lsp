[CmdletBinding()]
param(
    [string[]]$Rids = @('win-arm64', 'win-x64', 'linux-arm64', 'linux-x64'),
    [string]$OutDir = (Join-Path $PSScriptRoot '..\..\dist'),
    [switch]$Clean
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$workerProject = Join-Path $repoRoot 'worker-dotnet\MiLsp.Worker\MiLsp.Worker.csproj'

function Get-RidSpec {
    param([Parameter(Mandatory = $true)][string]$Rid)

    switch ($Rid) {
        'win-arm64'   { return @{ GOOS = 'windows'; GOARCH = 'arm64'; CliName = 'mi-lsp.exe' } }
        'win-x64'     { return @{ GOOS = 'windows'; GOARCH = 'amd64'; CliName = 'mi-lsp.exe' } }
        'linux-arm64' { return @{ GOOS = 'linux'; GOARCH = 'arm64'; CliName = 'mi-lsp' } }
        'linux-x64'   { return @{ GOOS = 'linux'; GOARCH = 'amd64'; CliName = 'mi-lsp' } }
        default       { throw "Unsupported RID '$Rid'. Supported values: win-arm64, win-x64, linux-arm64, linux-x64." }
    }
}

$results = @()
foreach ($rid in $Rids) {
    $spec = Get-RidSpec -Rid $rid
    $distRoot = Join-Path $OutDir $rid
    $workerOut = Join-Path $distRoot (Join-Path 'workers' $rid)
    if ($Clean -and (Test-Path $distRoot)) {
        Remove-Item -Recurse -Force $distRoot
    }
    New-Item -ItemType Directory -Force -Path $distRoot | Out-Null
    New-Item -ItemType Directory -Force -Path $workerOut | Out-Null

    Push-Location $repoRoot
    try {
        $env:GOOS = $spec.GOOS
        $env:GOARCH = $spec.GOARCH
        $cliOut = Join-Path $distRoot $spec.CliName
        & go build '-ldflags=-s -w' -o $cliOut ./cmd/mi-lsp
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for RID '$rid'"
        }

        & dotnet publish $workerProject -c Release -r $rid --self-contained true -o $workerOut
        if ($LASTEXITCODE -ne 0) {
            throw "dotnet publish failed for RID '$rid'"
        }
    }
    finally {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
        Pop-Location
    }

    $results += [pscustomobject]@{
        Rid = $rid
        Cli = Join-Path $distRoot $spec.CliName
        WorkerDir = $workerOut
    }
}

$results

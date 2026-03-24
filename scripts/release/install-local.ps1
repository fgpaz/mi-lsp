[CmdletBinding()]
param(
    [string]$Rid = 'win-arm64',
    [string]$InstallDir = (Join-Path $HOME 'bin'),
    [string]$OutDir = (Join-Path $PSScriptRoot '..\..\dist'),
    [switch]$SkipBuild,
    [switch]$SkipWorkerRefresh
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$buildScript = Join-Path $PSScriptRoot 'build-dist.ps1'

function Get-CliName {
    param([Parameter(Mandatory = $true)][string]$Rid)
    if ($Rid -like 'win-*') {
        return 'mi-lsp.exe'
    }
    return 'mi-lsp'
}

if (-not $SkipBuild) {
    & $buildScript -Rids @($Rid) -OutDir $OutDir -Clean
    if ($LASTEXITCODE -ne 0) {
        throw "Distribution build failed for RID '$Rid'"
    }
}

$cliName = Get-CliName -Rid $Rid
$distRoot = Join-Path $OutDir $Rid
$sourceCli = Join-Path $distRoot $cliName
$sourceWorkerDir = Join-Path $distRoot (Join-Path 'workers' $Rid)
if (-not (Test-Path $sourceCli)) {
    throw "Built CLI was not found at '$sourceCli'"
}
if (-not (Test-Path $sourceWorkerDir)) {
    throw "Built worker directory was not found at '$sourceWorkerDir'"
}

$targetCli = Join-Path $InstallDir $cliName
$targetWorkersRoot = Join-Path $InstallDir 'workers'
$targetWorkerDir = Join-Path $targetWorkersRoot $Rid

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $targetWorkersRoot | Out-Null
if (Test-Path $targetWorkerDir) {
    Remove-Item -Recurse -Force $targetWorkerDir
}
New-Item -ItemType Directory -Force -Path $targetWorkerDir | Out-Null

Copy-Item -Force $sourceCli $targetCli
Copy-Item -Recurse -Force (Join-Path $sourceWorkerDir '*') $targetWorkerDir

if (-not $SkipWorkerRefresh) {
    & $targetCli worker install --rid $Rid --format compact
    if ($LASTEXITCODE -ne 0) {
        throw "Installed CLI could not refresh the global worker for RID '$Rid'"
    }
}

[pscustomobject]@{
    Rid = $Rid
    InstallDir = $InstallDir
    Cli = $targetCli
    WorkerDir = $targetWorkerDir
}

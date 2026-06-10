[CmdletBinding()]
param(
    [string]$Repo = 'fgpaz/mi-lsp',
    [string]$Rid = '',
    [string]$InstallDir = (Join-Path $HOME 'bin'),
    [string]$GitHubToken = $env:GITHUB_TOKEN,
    [string]$SkillRepo = 'fgpaz/mi-lsp',
    [string]$Skill = 'mi-lsp',
    [string[]]$Agent = @('codex', 'claude-code'),
    [switch]$DryRun,
    [switch]$NoPathUpdate
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# SEC-08: warn if GITHUB_TOKEN is in environment (token should not be embedded in scripts)
if (-not [string]::IsNullOrWhiteSpace($env:GITHUB_TOKEN)) {
    Write-Warning "GITHUB_TOKEN environment variable is set. It will be used for authentication during download. Ensure you trust this environment and do not commit credentials in history."
}

function Get-InstallScript {
    if (-not [string]::IsNullOrWhiteSpace($PSScriptRoot)) {
        $local = Join-Path $PSScriptRoot 'install.ps1'
        if (Test-Path -LiteralPath $local) {
            return $local
        }
    }

    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("mi-lsp-install-script-" + [guid]::NewGuid().ToString('N') + '.ps1')
    Invoke-WebRequest -UseBasicParsing -Uri 'https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.ps1' -OutFile $tmp
    return $tmp
}

$installScript = Get-InstallScript
$installArgs = @{
    Repo = $Repo
    InstallDir = $InstallDir
}
if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
    $installArgs.GitHubToken = $GitHubToken
}
if (-not [string]::IsNullOrWhiteSpace($Rid)) {
    $installArgs.Rid = $Rid
}
if ($DryRun) {
    $installArgs.DryRun = $true
}
if ($NoPathUpdate) {
    $installArgs.NoPathUpdate = $true
}

& $installScript @installArgs

$npxArgs = @('skills', 'add', $SkillRepo, '--skill', $Skill, '-g')
foreach ($targetAgent in $Agent) {
    if (-not [string]::IsNullOrWhiteSpace($targetAgent)) {
        $npxArgs += @('-a', $targetAgent)
    }
}
$npxArgs += '-y'

Write-Host ('npx ' + ($npxArgs -join ' '))
if ($DryRun) {
    return
}

$npx = Get-Command npx -ErrorAction SilentlyContinue
if (-not $npx) {
    throw 'install-agent requires npx. Install Node.js/npm, then rerun this script. No direct skill-copy fallback is used.'
}

& $npx.Source @npxArgs
if ($LASTEXITCODE -ne 0) {
    throw 'Skill install failed through npx skills.'
}

Write-Host "Installed skill '$Skill' for agents: $($Agent -join ', ')"

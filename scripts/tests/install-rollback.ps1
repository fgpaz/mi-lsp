$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('mi-lsp-rollback-' + [guid]::NewGuid())
$src = Join-Path $root 'src'; $active = Join-Path $root 'active'
New-Item -ItemType Directory -Force -Path (Join-Path $src 'worker') | Out-Null
Set-Content (Join-Path $src 'worker/file') 'new-worker'
@'
param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
if ($env:MI_LSP_INSTALL_FAIL_PHASE -eq 'status') { exit 1 }
'@ | Set-Content (Join-Path $src 'cli.ps1')
$env:MI_LSP_INSTALL_TEST_MODE='activation'; $env:MI_LSP_TEST_INSTALL_ROOT=$active; $env:MI_LSP_TEST_SOURCE_CLI=(Join-Path $src 'cli.ps1'); $env:MI_LSP_TEST_SOURCE_WORKER=(Join-Path $src 'worker'); $env:MI_LSP_TEST_RID='win-x64'
try {
  & powershell -NoProfile -File scripts/install/install.ps1
  if (-not (Test-Path (Join-Path $active 'workers/win-x64/file'))) { throw 'worker was not activated at RID path' }
  $env:MI_LSP_INSTALL_FAIL_PHASE='status'; if ((& powershell -NoProfile -File scripts/install/install.ps1 2>$null) -eq $null) { }
  if (-not (Test-Path (Join-Path $active 'workers/win-x64/file'))) { throw 'rollback removed prior worker' }
  Write-Output 'PASS: PowerShell executable activation, rollback, and RID confinement'
} finally { Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue; Remove-Item Env:MI_LSP_INSTALL_TEST_MODE,Env:MI_LSP_TEST_INSTALL_ROOT,Env:MI_LSP_TEST_SOURCE_CLI,Env:MI_LSP_TEST_SOURCE_WORKER,Env:MI_LSP_TEST_RID,Env:MI_LSP_INSTALL_FAIL_PHASE -ErrorAction SilentlyContinue }

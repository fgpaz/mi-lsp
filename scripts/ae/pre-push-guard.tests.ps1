param(
    [Parameter(Mandatory = $true)]
    [string]$SessionContract
)

$ErrorActionPreference = "Stop"
$guard = Join-Path $PSScriptRoot "pre-push-guard.ps1"
$pwsh = (Get-Command pwsh).Source

function Invoke-Guard([string]$ContractPath) {
    $null = & $pwsh -NoProfile -File $guard -SessionContract $ContractPath -AllowDirty 2>$null
    return [int]$LASTEXITCODE
}

$positive = Invoke-Guard $SessionContract
if ($positive -ne 0) {
    throw "positive guard case failed with exit code $positive"
}

$contractText = Get-Content -Raw -LiteralPath $SessionContract
$badScope = [IO.Path]::GetTempFileName()
$badForbidden = [IO.Path]::GetTempFileName()
try {
    $scopeText = $contractText -replace '(?m)^  - scripts/ae/pre-push-guard\.ps1\s*$', '  - README.md'
    Set-Content -LiteralPath $badScope -Value $scopeText -NoNewline
    $scopeExit = Invoke-Guard $badScope
    if ($scopeExit -eq 0) {
        throw "negative allowed_paths case unexpectedly passed"
    }

    $forbiddenText = $contractText -replace '(?m)^forbidden_paths:\s*$', "forbidden_paths:`r`n  - scripts/ae/**"
    Set-Content -LiteralPath $badForbidden -Value $forbiddenText -NoNewline
    $forbiddenExit = Invoke-Guard $badForbidden
    if ($forbiddenExit -eq 0) {
        throw "negative forbidden_paths case unexpectedly passed"
    }

    [pscustomobject]@{
        positive = "PASS"
        disallowed_path = "PASS"
        contractual_forbidden = "PASS"
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -LiteralPath $badScope, $badForbidden -Force -ErrorAction SilentlyContinue
}

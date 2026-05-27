param(
    [Parameter(Mandatory = $true)]
    [string]$SessionContract,

    [switch]$AllowDirty,
    [switch]$AllowMain
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
    Write-Error $Message
    exit 1
}

if (-not (Test-Path -LiteralPath $SessionContract)) {
    Fail "Session contract not found: $SessionContract"
}

$branch = (git rev-parse --abbrev-ref HEAD).Trim()
if (-not $AllowMain -and $branch -eq "main") {
    Fail "Refusing push guard on main without -AllowMain; use an isolated task branch."
}

$contractText = Get-Content -Raw -LiteralPath $SessionContract
foreach ($required in @("task_slug:", "allowed_paths:", "forbidden_paths:", "required_evidence:", "ae_contract:", "cleanup_policy:")) {
    if ($contractText -notmatch [regex]::Escape($required)) {
        Fail "Session contract is missing required field marker: $required"
    }
}

$status = @(git status --porcelain=v1)
if (-not $AllowDirty -and $status.Count -gt 0) {
    Fail "Working tree is dirty; rerun after committing/stashing or pass -AllowDirty for pre-commit validation."
}

$rawTouched = @($status | Where-Object { $_ -match " \.docs/raw/" })
if ($rawTouched.Count -gt 0) {
    Fail "Ungoverned .docs/raw changes detected in working tree: $($rawTouched -join '; ')"
}

$forbiddenHits = @($status | Where-Object {
    $_ -match " \.mi-lsp/" -or
    $_ -match " \.docs/wiki/_mi-lsp/read-model\.toml" -or
    $_ -match " \.env" -or
    $_ -match " dist/"
})
if ($forbiddenHits.Count -gt 0) {
    Fail "Forbidden-path changes detected: $($forbiddenHits -join '; ')"
}

[pscustomobject]@{
    ok = $true
    branch = $branch
    session_contract = $SessionContract
    dirty_allowed = [bool]$AllowDirty
    dirty_count = $status.Count
} | ConvertTo-Json -Compress

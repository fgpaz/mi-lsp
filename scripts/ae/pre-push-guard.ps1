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

function Get-ScalarValue($Text, $FieldName) {
    $pattern = "(?m)^\s*" + [regex]::Escape($FieldName) + ":\s*(?<value>.+?)\s*$"
    $match = [regex]::Match($Text, $pattern)
    if (-not $match.Success) {
        return $null
    }
    return $match.Groups["value"].Value.Trim().Trim('"').Trim("'")
}

function Assert-ScalarValue($Text, $FieldName) {
    $value = Get-ScalarValue $Text $FieldName
    if ([string]::IsNullOrWhiteSpace($value)) {
        Fail "Session contract field '$FieldName' must have a non-empty value."
    }
    return $value
}

function Assert-ListHasItems($Text, $FieldName) {
    $pattern = "(?ms)^\s*" + [regex]::Escape($FieldName) + ":\s*\r?\n(?<items>(?:\s+-\s+\S.*\r?\n?)+)"
    if (-not [regex]::IsMatch($Text, $pattern)) {
        Fail "Session contract list '$FieldName' must include at least one item."
    }
}

function Get-AECanonStatus($Text) {
    $nestedPattern = "(?ms)^\s*ae_canon:\s*\r?\n(?<body>(?:\s{2,}.+\r?\n?)*)"
    $nested = [regex]::Match($Text, $nestedPattern)
    if ($nested.Success) {
        $body = $nested.Groups["body"].Value
        $status = Get-ScalarValue $body "status"
        if (-not [string]::IsNullOrWhiteSpace($status)) {
            return $status
        }
    }
    $flat = Get-ScalarValue $Text "ae_canon_status"
    if (-not [string]::IsNullOrWhiteSpace($flat)) {
        return $flat
    }
    return $null
}

function Get-TouchedPaths($BaseSha) {
    $paths = @()
    if (-not [string]::IsNullOrWhiteSpace($BaseSha)) {
        try {
            $paths += @(git diff --name-only "$BaseSha..HEAD")
        } catch {
            Fail "Unable to inspect branch diff from base_sha '$BaseSha': $($_.Exception.Message)"
        }
    }
    $paths += @(git status --porcelain=v1 | ForEach-Object {
        if ($_ -match "^.{2}\s+(?<path>.+)$") {
            $Matches["path"] -replace "^""|""$", ""
        }
    })
    return @($paths | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Sort-Object -Unique)
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
foreach ($requiredScalar in @("task_slug", "mode", "decision_lock", "harness_adapter", "orchestration_depth", "closure_profile", "cleanup_policy")) {
    [void](Assert-ScalarValue $contractText $requiredScalar)
}
foreach ($requiredList in @("allowed_paths", "forbidden_paths", "required_evidence", "stop_conditions")) {
    Assert-ListHasItems $contractText $requiredList
}
if ($contractText -notmatch "(?m)^\s*mi_lsp_preflight:\s*$") {
    Fail "Session contract is missing mi_lsp_preflight for governed AE work."
}
foreach ($preflightField in @("alias", "root", "client_name", "session_id", "governance_blocked", "docs_ready", "doc_count")) {
    [void](Assert-ScalarValue $contractText $preflightField)
}
$clientName = Get-ScalarValue $contractText "client_name"
if ($clientName -eq "manual-cli") {
    Fail "mi_lsp_preflight.client_name must identify the harness; manual-cli is not valid for governed T2+ work."
}
if ((Get-ScalarValue $contractText "governance_blocked") -eq "true") {
    Fail "mi_lsp_preflight.governance_blocked=true; only diagnosis/repair is allowed."
}
if ((Get-ScalarValue $contractText "docs_ready") -eq "false") {
    Fail "mi_lsp_preflight.docs_ready=false; governed docs-first/AE work cannot close as warning-only."
}
$aeCanonStatus = Get-AECanonStatus $contractText
if ([string]::IsNullOrWhiteSpace($aeCanonStatus)) {
    Fail "mi_lsp_preflight must record ae_canon.status."
}
if ($aeCanonStatus -in @("missing", "mismatch", "not_implemented")) {
    Fail "mi_lsp_preflight.ae_canon.status=$aeCanonStatus blocks guarded closure."
}

$status = @(git status --porcelain=v1)
if (-not $AllowDirty -and $status.Count -gt 0) {
    Fail "Working tree is dirty; rerun after committing/stashing or pass -AllowDirty for pre-commit validation."
}

$baseSha = Get-ScalarValue $contractText "base_sha"
$touchedPaths = Get-TouchedPaths $baseSha
$rawTouched = @($touchedPaths | Where-Object { $_ -match "^\.docs/raw/" })
if ($rawTouched.Count -gt 0) {
    Fail "Ungoverned .docs/raw changes detected in working tree: $($rawTouched -join '; ')"
}

$forbiddenHits = @($touchedPaths | Where-Object {
    $_ -match "^\.mi-lsp/" -or
    $_ -match "^\.docs/wiki/_mi-lsp/read-model\.toml$" -or
    $_ -match "(^|/)\.env(\.|$)" -or
    $_ -match "(^|/)dist/"
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
    diff_count = $touchedPaths.Count
    ae_canon_status = $aeCanonStatus
} | ConvertTo-Json -Compress

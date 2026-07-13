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

function Test-ContractListContainsExactPath($Text, $FieldName, $Path) {
    $inList = $false
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match "^\s*" + [regex]::Escape($FieldName) + ":\s*$") {
            $inList = $true
            continue
        }
        if ($inList -and $line -match "^[A-Za-z0-9_][A-Za-z0-9_-]*:\s*") {
            break
        }
        if ($inList -and $line -match '^\s+-\s+"?(?<path>[^"].*?)"?\s*$' -and $Matches["path"] -eq $Path) {
            return $true
        }
    }
    return $false
}

function Get-ContractListValues($Text, $FieldName) {
    $values = @()
    $inList = $false
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match "^\s*" + [regex]::Escape($FieldName) + ":\s*$") {
            $inList = $true
            continue
        }
        if (-not $inList) {
            continue
        }
        if ($line -match "^[A-Za-z0-9_][A-Za-z0-9_-]*:\s*") {
            break
        }
        if ($line -match '^\s+-\s+"?(?<value>[^"].*?)"?\s*$') {
            $value = $Matches["value"].Trim()
            if ($value -ne "") {
                $values += $value
            }
        }
    }
    return @($values)
}

function Normalize-ContractPath($Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) {
        return ""
    }
    $normalized = $Path.Trim().Trim('"').Trim("'") -replace "\\", "/"
    while ($normalized.StartsWith("./")) {
        $normalized = $normalized.Substring(2)
    }
    return $normalized.TrimStart('/')
}

function Test-ContractPathPattern($Path, $Pattern) {
    $pathValue = Normalize-ContractPath $Path
    $patternValue = Normalize-ContractPath $Pattern
    if ($pathValue -eq "" -or $patternValue -eq "") {
        return $false
    }
    if ($pathValue.Equals($patternValue, [StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }
    if (-not ($patternValue.Contains('*') -or $patternValue.Contains('?'))) {
        return $false
    }
    $marker = '__AE_DOUBLESTAR__'
    $escaped = [regex]::Escape($patternValue.Replace('**', $marker))
    $escaped = $escaped.Replace([regex]::Escape($marker), '.*')
    $escaped = $escaped.Replace('\*', '[^/]*')
    $escaped = $escaped.Replace('\?', '[^/]')
    return [regex]::IsMatch($pathValue, "^(?i:" + $escaped + ")$")
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
    $paths += @(git status --porcelain=v1 --untracked-files=all | ForEach-Object { Get-StatusPaths $_ })
    return @($paths | ForEach-Object { Normalize-ContractPath $_ } | Where-Object { $_ -ne "" } | Sort-Object -Unique)
}

function Get-StatusPaths($StatusLine) {
    if ($StatusLine -notmatch "^.{2}\s+(?<path>.+)$") {
        return @()
    }
    $raw = ($Matches["path"] -replace "^""|""$", "")
    if ($raw -match "^(?<old>.+?)\s+->\s+(?<new>.+)$") {
        return @($Matches["old"], $Matches["new"])
    }
    return @($raw)
}

function Get-StatusPath($StatusLine) {
    return @(Get-StatusPaths $StatusLine) | Select-Object -First 1
}

function Get-WaivedDirtyPaths($Text) {
    $paths = @()
    $inWaivers = $false
    $inScope = $false
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match "^\s*waivers:\s*$") {
            $inWaivers = $true
            $inScope = $false
            continue
        }
        if ($inWaivers -and $line -match "^[A-Za-z0-9_][A-Za-z0-9_-]*:\s*") {
            break
        }
        if (-not $inWaivers) {
            continue
        }
        if ($line -match "^\s+scope:\s*$") {
            $inScope = $true
            continue
        }
        if ($inScope -and $line -match "^\s+-\s+""?(?<path>[^""]+?)""?\s*$") {
            $paths += $Matches["path"]
            continue
        }
        if ($inScope -and $line -match "^\s+[A-Za-z0-9_][A-Za-z0-9_-]*:\s*") {
            $inScope = $false
        }
    }
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
    # Migration note: older session contracts may not have mi_lsp_preflight block yet.
    # This is a deprecation window — if you have old contracts without it, add the block with
    # mandatory fields: alias, root, client_name, session_id, governance_blocked, docs_ready, doc_count.
    # See AE-SESSION-CONTRACT.md for the required schema. Gates do NOT relax; only documentation changes.
    Fail "Session contract is missing mi_lsp_preflight for governed AE work (see migration note in pre-push-guard.ps1)."
}
foreach ($preflightField in @("alias", "root", "client_name", "session_id", "governance_blocked", "docs_ready", "doc_count")) {
    [void](Assert-ScalarValue $contractText $preflightField)
}
$clientName = Get-ScalarValue $contractText "client_name"
if ($clientName -eq "manual-cli") {
    Fail "mi_lsp_preflight.client_name must identify the harness; manual-cli is not valid for governed T2+ work."
}
$sessionID = Get-ScalarValue $contractText "session_id"
if ($sessionID -match "^cli-\d+$") {
    Fail "mi_lsp_preflight.session_id must identify the governed session; default cli-<pid> is not valid for governed T2+ work."
}
if ((Get-ScalarValue $contractText "governance_blocked") -eq "true") {
    Fail "mi_lsp_preflight.governance_blocked=true; only diagnosis/repair is allowed."
}
if ((Get-ScalarValue $contractText "docs_ready") -eq "false") {
    Fail "mi_lsp_preflight.docs_ready=false; governed docs-first/AE work cannot close as warning-only."
}
$docCountText = Get-ScalarValue $contractText "doc_count"
$docCount = 0
if (-not [int]::TryParse($docCountText, [ref]$docCount)) {
    Fail "mi_lsp_preflight.doc_count must be numeric."
}
if ($docCount -le 0) {
    Fail "mi_lsp_preflight.doc_count=$docCount; governed docs-first/AE work cannot close with an empty document index."
}
$aeCanonStatus = Get-AECanonStatus $contractText
if ([string]::IsNullOrWhiteSpace($aeCanonStatus)) {
    Fail "mi_lsp_preflight must record ae_canon.status."
}
if ($aeCanonStatus -in @("missing", "mismatch", "projection_only", "not_implemented")) {
    Fail "mi_lsp_preflight.ae_canon.status=$aeCanonStatus blocks guarded closure."
}

$status = @(git status --porcelain=v1)
$waivedDirtyPaths = @(Get-WaivedDirtyPaths $contractText)
$blockingStatus = @($status | Where-Object {
    $path = Get-StatusPath $_
    [string]::IsNullOrWhiteSpace($path) -or ($waivedDirtyPaths -notcontains $path)
})
if (-not $AllowDirty -and $blockingStatus.Count -gt 0) {
    $blockingPaths = @($blockingStatus | ForEach-Object { Get-StatusPath $_ } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    Fail "Working tree has unwaived dirty paths: $($blockingPaths -join '; '). Commit/stash them or pass -AllowDirty for pre-commit validation."
}

$baseSha = Get-ScalarValue $contractText "base_sha"
$touchedPaths = Get-TouchedPaths $baseSha
$allowedPatterns = @(Get-ContractListValues $contractText "allowed_paths")
$forbiddenPatterns = @(Get-ContractListValues $contractText "forbidden_paths")

# Hard blocks are independent of the contract and cannot be waived by allowed_paths.
$hardForbiddenPatterns = @(
    ".docs/raw/**",
    ".mi-lsp/**",
    ".env",
    ".env.*",
    "*.env",
    "*.env.*",
    "**/.env",
    "**/.env.*",
    "dist/**",
    "**/dist/**",
    "secrets/**",
    "**/secrets/**"
)
$hardForbiddenHits = @($touchedPaths | Where-Object {
    $path = $_
    @($hardForbiddenPatterns | Where-Object { Test-ContractPathPattern $path $_ }).Count -gt 0
})
if ($hardForbiddenHits.Count -gt 0) {
    Fail "Hard-forbidden path changes detected: $($hardForbiddenHits -join '; ')"
}

$contractForbiddenHits = @($touchedPaths | Where-Object {
    $path = $_
    @($forbiddenPatterns | Where-Object { Test-ContractPathPattern $path $_ }).Count -gt 0
})
if ($contractForbiddenHits.Count -gt 0) {
    Fail "Contract-forbidden path changes detected: $($contractForbiddenHits -join '; ')"
}

$unallowedPaths = @($touchedPaths | Where-Object {
    $path = $_
    @($allowedPatterns | Where-Object { Test-ContractPathPattern $path $_ }).Count -eq 0
})
if ($unallowedPaths.Count -gt 0) {
    Fail "Changed paths outside session contract allowed_paths: $($unallowedPaths -join '; ')"
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

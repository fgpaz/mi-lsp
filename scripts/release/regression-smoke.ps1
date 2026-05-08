[CmdletBinding()]
param(
    [string]$Cli = "mi-lsp",
    [string]$Query = "RS RF TP CT TECH DB",
    [string]$OutDir = "artifacts/release-regression",
    [int]$Top = 5,
    [switch]$AllowStatusAutoSync
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-MiLspJson {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [string]$Workspace = "",
        [string]$Root = ""
    )

    $operation = ($Arguments -join " ")
    $dbPath = if ($Root) { Join-Path $Root ".mi-lsp/index.db" } else { "" }
    try {
        $output = & $Cli @Arguments --format json 2>&1
        $exitCode = $LASTEXITCODE
        $text = ($output | Out-String).Trim()
        if ($exitCode -ne 0) {
            return [pscustomobject]@{
                ok = $false
                operation = $operation
                workspace = $Workspace
                root = $Root
                db_path = $dbPath
                error = $text
            }
        }
        $parsed = if ($text) { $text | ConvertFrom-Json -Depth 20 } else { $null }
        return [pscustomobject]@{
            ok = $true
            operation = $operation
            workspace = $Workspace
            root = $Root
            db_path = $dbPath
            result = $parsed
        }
    } catch {
        return [pscustomobject]@{
            ok = $false
            operation = $operation
            workspace = $Workspace
            root = $Root
            db_path = $dbPath
            error = $_.Exception.Message
        }
    }
}

function New-SkippedOperationResult {
    param(
        [string]$Operation,
        [string]$Workspace,
        [string]$Root,
        [string]$Reason
    )

    $dbPath = if ($Root) { Join-Path $Root ".mi-lsp/index.db" } else { "" }
    return [pscustomobject]@{
        ok = $false
        operation = $Operation
        workspace = $Workspace
        root = $Root
        db_path = $dbPath
        skipped = $true
        error = $Reason
    }
}

function Test-GovernanceBlocked {
    param($StatusResult)

    if ($null -eq $StatusResult -or -not $StatusResult.ok) {
        return $true
    }
    $resultProperty = $StatusResult.PSObject.Properties["result"]
    if ($null -eq $resultProperty -or $null -eq $resultProperty.Value) {
        return $true
    }
    $itemsProperty = $resultProperty.Value.PSObject.Properties["items"]
    if ($null -eq $itemsProperty -or $null -eq $itemsProperty.Value) {
        return $true
    }
    $items = @($itemsProperty.Value)
    if ($items.Count -eq 0) {
        return $true
    }
    $blockedProperty = $items[0].PSObject.Properties["governance_blocked"]
    if ($null -eq $blockedProperty) {
        return $true
    }
    return [bool]$blockedProperty.Value
}

function Get-FirstTraceableDocId {
    param($SearchResult)

    if ($null -eq $SearchResult) {
        return ""
    }
    $resultProperty = $SearchResult.PSObject.Properties["result"]
    if ($null -eq $resultProperty -or $null -eq $resultProperty.Value) {
        return ""
    }
    $itemsProperty = $resultProperty.Value.PSObject.Properties["items"]
    if ($null -eq $itemsProperty -or $null -eq $itemsProperty.Value) {
        return ""
    }
    foreach ($item in @($itemsProperty.Value)) {
        $docIDProperty = $item.PSObject.Properties["doc_id"]
        if ($null -eq $docIDProperty) {
            continue
        }
        $docID = [string]$docIDProperty.Value
        if ($docID -match "^(RS|RF|TP)-") {
            return $docID
        }
    }
    return ""
}

function Test-OperationSkipped {
    param($Result)

    if ($null -eq $Result) {
        return $false
    }
    $property = $Result.PSObject.Properties["skipped"]
    return ($null -ne $property -and [bool]$property.Value)
}

function Test-OperationFailed {
    param($Result)

    if ($null -eq $Result) {
        return $false
    }
    $okProperty = $Result.PSObject.Properties["ok"]
    $ok = ($null -ne $okProperty -and [bool]$okProperty.Value)
    return (-not $ok -and -not (Test-OperationSkipped -Result $Result))
}

function Get-WorkspaceSmokeStatus {
    param($WorkspaceReport)

    if ((Test-OperationFailed -Result $WorkspaceReport.status) -or
        (Test-OperationFailed -Result $WorkspaceReport.wiki_search) -or
        (Test-OperationFailed -Result $WorkspaceReport.wiki_pack) -or
        (Test-OperationFailed -Result $WorkspaceReport.wiki_trace)) {
        return "failed"
    }
    if ((Test-OperationSkipped -Result $WorkspaceReport.status) -or
        (Test-OperationSkipped -Result $WorkspaceReport.wiki_search) -or
        (Test-OperationSkipped -Result $WorkspaceReport.wiki_pack) -or
        (Test-OperationSkipped -Result $WorkspaceReport.wiki_trace)) {
        return "skipped"
    }
    return "passed"
}

function Get-GoVersionMetadata {
    param(
        [string]$Name,
        [string]$Path
    )

    if (-not $Path -or -not (Test-Path -LiteralPath $Path)) {
        return [pscustomobject]@{ name = $Name; path = $Path; ok = $false; output = "missing" }
    }
    $output = & go version -m $Path 2>&1
    return [pscustomobject]@{
        name = $Name
        path = $Path
        ok = ($LASTEXITCODE -eq 0)
        output = ($output | Out-String).Trim()
    }
}

function Get-WslGoVersionMetadata {
    $output = & wsl sh -lc 'p="\$(command -v mi-lsp 2>/dev/null || true)"; if [ -z "\$p" ] && [ -x "\$HOME/.local/bin/mi-lsp" ]; then p="\$HOME/.local/bin/mi-lsp"; fi; if [ -z "\$p" ]; then echo missing; exit 3; fi; go version -m "\$p"' 2>&1
    return [pscustomobject]@{
        name = "wsl-global"
        path = "wsl:mi-lsp"
        ok = ($LASTEXITCODE -eq 0)
        output = ($output | Out-String).Trim()
    }
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$targetDir = Join-Path $OutDir $timestamp
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

$workspaceList = Invoke-MiLspJson -Arguments @("workspace", "list")
if (-not $workspaceList.ok) {
    throw "workspace list failed: $($workspaceList.error)"
}

$pathStatusArgs = @("workspace", "status", ".")
if (-not $AllowStatusAutoSync) {
    $pathStatusArgs += "--no-auto-sync"
}
$pathStatus = Invoke-MiLspJson -Arguments $pathStatusArgs -Workspace "." -Root (Get-Location).Path

$workspaceReports = New-Object System.Collections.Generic.List[object]
foreach ($workspace in @($workspaceList.result.items)) {
    $alias = [string]$workspace.name
    $root = [string]$workspace.root
    $kind = [string]$workspace.kind
    $family = if ($root) { Split-Path -Leaf $root } else { "" }
    if (-not $alias) {
        continue
    }

    $statusArgs = @("workspace", "status", $alias)
    if (-not $AllowStatusAutoSync) {
        $statusArgs += "--no-auto-sync"
    }
    $status = Invoke-MiLspJson -Arguments $statusArgs -Workspace $alias -Root $root

    $governanceBlocked = Test-GovernanceBlocked -StatusResult $status
    if ($governanceBlocked) {
        $reason = "skipped because workspace governance is blocked or status failed"
        $search = New-SkippedOperationResult -Operation "nav wiki search" -Workspace $alias -Root $root -Reason $reason
        $pack = New-SkippedOperationResult -Operation "nav wiki pack" -Workspace $alias -Root $root -Reason $reason
    } else {
        $search = Invoke-MiLspJson -Arguments @("nav", "wiki", "search", $Query, "--workspace", $alias, "--layer", "RS,RF,FL,TP,CT,TECH,DB", "--top", [string]$Top) -Workspace $alias -Root $root
        $pack = Invoke-MiLspJson -Arguments @("nav", "wiki", "pack", $Query, "--workspace", $alias) -Workspace $alias -Root $root
    }
    $traceID = Get-FirstTraceableDocId -SearchResult $search
    $trace = $null
    if ($traceID) {
        $trace = Invoke-MiLspJson -Arguments @("nav", "wiki", "trace", $traceID, "--workspace", $alias) -Workspace $alias -Root $root
    }

    $workspaceReports.Add([pscustomobject]@{
        workspace = $alias
        root = $root
        kind = $kind
        family = $family
        status = $status
        wiki_search = $search
        wiki_pack = $pack
        trace_id = $traceID
        wiki_trace = $trace
    })
}

$localBuild = Join-Path (Get-Location) "mi-lsp.exe"
$globalWindows = Join-Path $HOME "bin\mi-lsp.exe"
$metadata = @(
    Get-GoVersionMetadata -Name "windows-global" -Path $globalWindows
    Get-GoVersionMetadata -Name "local-build" -Path $localBuild
)
try {
    $metadata += Get-WslGoVersionMetadata
} catch {
    $metadata += [pscustomobject]@{ name = "wsl-global"; path = "wsl:mi-lsp"; ok = $false; output = $_.Exception.Message }
}

$rootGroups = @($workspaceReports | Group-Object -Property root)
$aliasesPerRoot = @{}
foreach ($group in $rootGroups) {
    $aliasesPerRoot[$group.Name] = @($group.Group | ForEach-Object { $_.workspace })
}
$pathStatusFailed = Test-OperationFailed -Result $pathStatus

$report = [pscustomobject]@{
    generated_at = (Get-Date).ToString("o")
    cli = $Cli
    query = $Query
    allow_status_auto_sync = [bool]$AllowStatusAutoSync
    workspace_count = $workspaceReports.Count
    unique_root_count = $rootGroups.Count
    duplicate_root_count = @($rootGroups | Where-Object { $_.Count -gt 1 }).Count
    path_status = $pathStatus
    path_status_ok = (-not $pathStatusFailed)
    aliases_per_root = $aliasesPerRoot
    workspaces = $workspaceReports.ToArray()
    go_version_m = $metadata
}

$jsonPath = Join-Path $targetDir "report.json"
$mdPath = Join-Path $targetDir "report.md"
$report | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $jsonPath -Encoding utf8

$failed = @($workspaceReports | Where-Object { (Get-WorkspaceSmokeStatus -WorkspaceReport $_) -eq "failed" })
$skipped = @($workspaceReports | Where-Object { (Get-WorkspaceSmokeStatus -WorkspaceReport $_) -eq "skipped" })

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Release Regression Smoke")
$lines.Add("")
$lines.Add("- generated_at: $($report.generated_at)")
$lines.Add("- cli: ``$Cli``")
$lines.Add("- workspaces: $($workspaceReports.Count)")
$lines.Add("- unique_roots: $($report.unique_root_count)")
$lines.Add("- duplicate_roots: $($report.duplicate_root_count)")
$lines.Add("- skipped_workspaces: $($skipped.Count)")
$lines.Add("- failed_workspaces: $($failed.Count)")
$lines.Add("- path_status_dot_ok: $($report.path_status_ok)")
$lines.Add("")
$lines.Add("## Binary Metadata")
foreach ($item in $metadata) {
    $lines.Add("- $($item.name): ok=$($item.ok) path=$($item.path)")
}
$lines.Add("")
$lines.Add("## Workspace Results By Status")
$statusGroups = @($workspaceReports | Group-Object -Property @{ Expression = {
    Get-WorkspaceSmokeStatus -WorkspaceReport $_
} })
foreach ($statusGroup in $statusGroups | Sort-Object Name) {
    $lines.Add("")
    $lines.Add("### $($statusGroup.Name)")
    foreach ($rootGroup in @($statusGroup.Group | Group-Object -Property root | Sort-Object Name)) {
        $lines.Add("- root: $($rootGroup.Name)")
        foreach ($item in @($rootGroup.Group | Sort-Object workspace)) {
            $trace = if ($item.trace_id) { $item.trace_id } else { "n/a" }
            $lines.Add("  - alias: $($item.workspace) kind=$($item.kind) family=$($item.family) trace=$trace")
        }
    }
}

$lines | Set-Content -LiteralPath $mdPath -Encoding utf8

[pscustomobject]@{
    ok = ($failed.Count -eq 0 -and -not $pathStatusFailed)
    report_json = $jsonPath
    report_md = $mdPath
    workspace_count = $workspaceReports.Count
    failed_workspace_count = $failed.Count
    path_status_dot_ok = (-not $pathStatusFailed)
}

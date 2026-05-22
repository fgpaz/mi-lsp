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

function ConvertFrom-JsonCompat {
    param([string]$Text)

    if (-not $Text) {
        return $null
    }
    try {
        return $Text | ConvertFrom-Json -Depth 20
    } catch [System.Management.Automation.ParameterBindingException] {
        return $Text | ConvertFrom-Json
    }
}

function Invoke-MiLspJson {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [string]$Workspace = "",
        [string]$Root = ""
    )

    $operation = ($Arguments -join " ")
    $dbPath = if ($Root) { Join-Path $Root ".mi-lsp/index.db" } else { "" }
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $output = & $Cli @Arguments --format json 2>&1
        $exitCode = $LASTEXITCODE
        $timer.Stop()
        $text = ($output | Out-String).Trim()
        if ($exitCode -ne 0) {
            return [pscustomobject]@{
                ok = $false
                operation = $operation
                workspace = $Workspace
                root = $Root
                db_path = $dbPath
                duration_ms = [int64]$timer.Elapsed.TotalMilliseconds
                error = $text
            }
        }
        $parsed = ConvertFrom-JsonCompat -Text $text
        return [pscustomobject]@{
            ok = $true
            operation = $operation
            workspace = $Workspace
            root = $Root
            db_path = $dbPath
            duration_ms = [int64]$timer.Elapsed.TotalMilliseconds
            result = $parsed
        }
    } catch {
        $timer.Stop()
        return [pscustomobject]@{
            ok = $false
            operation = $operation
            workspace = $Workspace
            root = $Root
            db_path = $dbPath
            duration_ms = [int64]$timer.Elapsed.TotalMilliseconds
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
        return New-GoVersionMetadata -Name $Name -Path $Path -Ok $false -Output "missing"
    }
    $output = & go version -m $Path 2>&1
    return New-GoVersionMetadata -Name $Name -Path $Path -Ok ($LASTEXITCODE -eq 0) -Output (($output | Out-String).Trim())
}

function Get-WslGoVersionMetadata {
    $script = @'
p="$(command -v mi-lsp 2>/dev/null || true)"
if [ -z "$p" ] && [ -x "$HOME/.local/bin/mi-lsp" ]; then
  p="$HOME/.local/bin/mi-lsp"
fi
if [ -z "$p" ]; then
  echo "path=missing"
  exit 3
fi
echo "path=$p"
go version -m "$p"
'@
    $encoded = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($script))
    $output = & wsl sh -lc "printf %s $encoded | base64 -d | sh" 2>&1
    $text = ($output | Out-String).Trim()
    $path = "wsl:mi-lsp"
    foreach ($line in ($text -split "`r?`n")) {
        if ($line -like "path=*") {
            $path = "wsl:" + $line.Substring(5)
            break
        }
    }
    return New-GoVersionMetadata -Name "wsl-global" -Path $path -Ok ($LASTEXITCODE -eq 0) -Output $text
}

function New-GoVersionMetadata {
    param(
        [string]$Name,
        [string]$Path,
        [bool]$Ok,
        [string]$Output
    )

    $revision = ""
    $modified = ""
    $goos = ""
    $goarch = ""
    foreach ($line in ($Output -split "`r?`n")) {
        $trimmed = $line.Trim()
        if ($trimmed -like "build`tGOOS=*") { $goos = $trimmed.Substring("build`tGOOS=".Length) }
        if ($trimmed -like "build`tGOARCH=*") { $goarch = $trimmed.Substring("build`tGOARCH=".Length) }
        if ($trimmed -like "build`tvcs.revision=*") { $revision = $trimmed.Substring("build`tvcs.revision=".Length) }
        if ($trimmed -like "build`tvcs.modified=*") { $modified = $trimmed.Substring("build`tvcs.modified=".Length) }
    }

    $rid = ""
    if ($goos -and $goarch) {
        $rid = "$goos-$goarch"
    }

    return [pscustomobject]@{
        name = $Name
        path = $Path
        ok = $Ok
        revision = $revision
        modified = $modified
        goos = $goos
        goarch = $goarch
        rid = $rid
        output = $Output
    }
}

function Get-VersionDrift {
    param($Metadata)

    $windows = @($Metadata | Where-Object { $_.name -eq "windows-global" }) | Select-Object -First 1
    $wsl = @($Metadata | Where-Object { $_.name -eq "wsl-global" }) | Select-Object -First 1
    $revisionMismatch = $false
    if ($windows -and $wsl -and $windows.ok -and $wsl.ok -and $windows.revision -and $wsl.revision) {
        $revisionMismatch = ($windows.revision -ne $wsl.revision)
    }
    $windowsPath = ""
    $windowsRevision = ""
    $windowsRid = ""
    if ($windows) {
        $windowsPath = $windows.path
        $windowsRevision = $windows.revision
        $windowsRid = $windows.rid
    }
    $wslPath = ""
    $wslRevision = ""
    $wslRid = ""
    if ($wsl) {
        $wslPath = $wsl.path
        $wslRevision = $wsl.revision
        $wslRid = $wsl.rid
    }
    return [pscustomobject]@{
        checked = [bool]($windows -and $wsl)
        windows_path = $windowsPath
        windows_revision = $windowsRevision
        windows_rid = $windowsRid
        wsl_path = $wslPath
        wsl_revision = $wslRevision
        wsl_rid = $wslRid
        revision_mismatch = $revisionMismatch
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
    $metadata += New-GoVersionMetadata -Name "wsl-global" -Path "wsl:mi-lsp" -Ok $false -Output $_.Exception.Message
}
$versionDrift = Get-VersionDrift -Metadata $metadata

$rootGroups = @($workspaceReports | Group-Object -Property root)
$aliasesPerRoot = @{}
foreach ($group in $rootGroups) {
    $aliasesPerRoot[$group.Name] = @($group.Group | ForEach-Object { $_.workspace })
}
$pathStatusFailed = Test-OperationFailed -Result $pathStatus
$slowOperations = New-Object System.Collections.Generic.List[object]
foreach ($workspaceReport in $workspaceReports) {
    foreach ($operationName in @("status", "wiki_search", "wiki_pack", "wiki_trace")) {
        $operationResult = $workspaceReport.$operationName
        if ($operationResult -and $operationResult.PSObject.Properties["duration_ms"] -and $operationResult.duration_ms -ge 15000) {
            $slowOperations.Add([pscustomobject]@{
                workspace = $workspaceReport.workspace
                operation = $operationName
                duration_ms = $operationResult.duration_ms
            })
        }
    }
}

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
    cross_environment_version = $versionDrift
    slow_operations = $slowOperations.ToArray()
}

Write-Host "=== Federated wiki smoke (--all-workspaces) ===" -ForegroundColor Cyan

$federatedCmds = @(
    @{ Name = "search";    Args = @("nav", "wiki", "search", "governance", "--all-workspaces", "--format", "toon") },
    @{ Name = "route";     Args = @("nav", "wiki", "route", "governance", "--all-workspaces", "--format", "toon") },
    @{ Name = "trace";     Args = @("nav", "wiki", "trace", "--all", "--all-workspaces", "--format", "toon") },
    @{ Name = "pack";      Args = @("nav", "wiki", "pack", "indexing", "--all-workspaces", "--format", "toon") },
    @{ Name = "inventory"; Args = @("nav", "wiki", "inventory", "--format", "toon") }
)

$federatedResults = New-Object System.Collections.Generic.List[object]
foreach ($cmd in $federatedCmds) {
    $output = & $Cli @($cmd.Args) 2>&1 | Out-String
    $exitCode = $LASTEXITCODE

    $passed = $false
    $reason = ""

    if ($exitCode -ne 0) {
        $reason = "exit code $exitCode"
    } elseif ($output -notmatch "ok:\s*true") {
        $reason = "envelope missing ok=true"
    } elseif ($output -notmatch "workspaces_queried") {
        $reason = "missing workspaces_queried in stats"
    } else {
        $passed = $true
    }

    if ($passed) {
        Write-Host "[PASS] nav wiki $($cmd.Name) --all-workspaces" -ForegroundColor Green
    } else {
        Write-Host "[FAIL] nav wiki $($cmd.Name) --all-workspaces ($reason)" -ForegroundColor Red
        Write-Host $output.Substring(0, [Math]::Min(500, $output.Length)) -ForegroundColor Yellow
        exit 1
    }

    $federatedResults.Add([pscustomobject]@{
        name = $cmd.Name
        exit_code = $exitCode
        ok = $passed
        reason = $reason
    })
}

Write-Host "=== Federated wiki smoke complete ===" -ForegroundColor Cyan

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
    $lines.Add("- $($item.name): ok=$($item.ok) path=$($item.path) revision=$($item.revision) rid=$($item.rid)")
}
$lines.Add("- windows_wsl_revision_mismatch: $($versionDrift.revision_mismatch)")
$lines.Add("")
$lines.Add("## Slow Operations")
if ($slowOperations.Count -eq 0) {
    $lines.Add("- none over 15000ms")
} else {
    foreach ($item in @($slowOperations | Sort-Object duration_ms -Descending)) {
        $lines.Add("- $($item.workspace) $($item.operation): $($item.duration_ms)ms")
    }
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

[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$Cli = ".\mi-lsp.exe",
    [string[]]$MissingProjectAliases = @("compras", "gastos-deploy-main", "gastos-uxfix"),
    [string[]]$CorruptIndexAliases = @("mi-desktop-agent", "turismo", "vendimia-tech"),
    [string]$OutDir = "artifacts/release-regression"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-MiLspJson {
    param([string[]]$Arguments)
    $output = & $Cli @Arguments --format json 2>&1
    return [pscustomobject]@{
        exit_code = $LASTEXITCODE
        output = ($output | Out-String).Trim()
    }
}

function Get-WorkspaceRoot {
    param([string]$Alias)
    $status = Invoke-MiLspJson -Arguments @("workspace", "status", $Alias, "--no-auto-sync")
    if ($status.exit_code -ne 0) {
        throw "workspace status failed for ${Alias}: $($status.output)"
    }
    $parsed = $status.output | ConvertFrom-Json -Depth 50
    return [string]$parsed.items[0].root
}

function Get-WorkspaceLanguages {
    param([string]$Alias)
    $status = Invoke-MiLspJson -Arguments @("workspace", "status", $Alias, "--no-auto-sync")
    $parsed = $status.output | ConvertFrom-Json -Depth 50
    return @(@($parsed.items[0].languages) | ForEach-Object { [string]$_ })
}

function Convert-TomlStringArray {
    param([string[]]$Items)
    $itemsArray = @($Items)
    if (-not $Items -or $itemsArray.Count -eq 0) {
        return "[]"
    }
    return "[" + (($itemsArray | ForEach-Object { '"' + ($_ -replace '"', '\"') + '"' }) -join ", ") + "]"
}

function Set-MinimalProjectFile {
    param([string]$Alias)
    $root = Get-WorkspaceRoot $Alias
    $languages = @(Get-WorkspaceLanguages $Alias)
    if ($languages.Count -eq 0) {
        $languages = @("typescript")
    }
    $projectDir = Join-Path $root ".mi-lsp"
    $projectPath = Join-Path $projectDir "project.toml"
    New-Item -ItemType Directory -Force -Path $projectDir | Out-Null
    $tomlLanguages = Convert-TomlStringArray $languages
    $content = @"
[project]
  name = "$Alias"
  languages = $tomlLanguages
  kind = "container"
  default_repo = "$Alias"

[ignore]

[[repo]]
  id = "$Alias"
  name = "$Alias"
  root = "."
  languages = $tomlLanguages
"@
    if ($PSCmdlet.ShouldProcess($projectPath, "write minimal mi-lsp project file")) {
        Set-Content -LiteralPath $projectPath -Value $content -Encoding utf8
    }
    return [pscustomobject]@{ alias = $Alias; root = $root; project_path = $projectPath; action = "project" }
}

function Remove-GeneratedIndex {
    param([string]$Alias)
    $root = Get-WorkspaceRoot $Alias
    $miLspDir = Join-Path $root ".mi-lsp"
    $resolvedRoot = [System.IO.Path]::GetFullPath($root)
    $resolvedDir = [System.IO.Path]::GetFullPath($miLspDir)
    if (-not $resolvedDir.StartsWith($resolvedRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "refusing to remove index outside workspace root: $resolvedDir"
    }
    $removed = New-Object System.Collections.Generic.List[string]
    foreach ($name in @("index.db", "index.db-wal", "index.db-shm")) {
        $path = Join-Path $miLspDir $name
        if (Test-Path -LiteralPath $path) {
            if ($PSCmdlet.ShouldProcess($path, "remove generated SQLite index")) {
                Remove-Item -LiteralPath $path -Force
            }
            $removed.Add($path)
        }
    }
    return [pscustomobject]@{ alias = $Alias; root = $root; removed = $removed.ToArray(); action = "remove_index" }
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$targetDir = Join-Path $OutDir "$timestamp-index-followup-repair"
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

$actions = New-Object System.Collections.Generic.List[object]
foreach ($alias in $MissingProjectAliases) {
    $actions.Add((Set-MinimalProjectFile -Alias $alias))
}
foreach ($alias in $CorruptIndexAliases) {
    $actions.Add((Remove-GeneratedIndex -Alias $alias))
}

$indexResults = New-Object System.Collections.Generic.List[object]
foreach ($alias in @($MissingProjectAliases + $CorruptIndexAliases)) {
    $index = Invoke-MiLspJson -Arguments @("index", "--workspace", $alias, "--docs-only")
    $status = Invoke-MiLspJson -Arguments @("workspace", "status", $alias, "--no-auto-sync")
    $indexResults.Add([pscustomobject]@{
        alias = $alias
        index_exit = $index.exit_code
        status_exit = $status.exit_code
        index_output = $index.output
        status_output = $status.output
    })
}

$result = [pscustomobject]@{
    generated_at = (Get-Date).ToString("o")
    actions = $actions.ToArray()
    index_results = $indexResults.ToArray()
}
$jsonPath = Join-Path $targetDir "repair.json"
$mdPath = Join-Path $targetDir "repair.md"
$result | ConvertTo-Json -Depth 80 | Set-Content -LiteralPath $jsonPath -Encoding utf8

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Index Follow-up Repair")
$lines.Add("")
$lines.Add("- generated_at: $($result.generated_at)")
$lines.Add("- actions: $($actions.Count)")
$lines.Add("")
foreach ($item in $indexResults) {
    $ok = $item.index_exit -eq 0 -and $item.status_exit -eq 0
    $lines.Add("- $($item.alias): ok=$ok index_exit=$($item.index_exit) status_exit=$($item.status_exit)")
}
$lines | Set-Content -LiteralPath $mdPath -Encoding utf8

[pscustomobject]@{
    ok = (@($indexResults | Where-Object { $_.index_exit -ne 0 -or $_.status_exit -ne 0 }).Count -eq 0)
    report_json = $jsonPath
    report_md = $mdPath
}

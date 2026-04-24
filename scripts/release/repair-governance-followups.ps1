[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$Cli = ".\mi-lsp.exe",
    [string]$ReportJson = "artifacts/release-regression/20260424-140010/report.json",
    [string]$OutDir = "artifacts/release-regression"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Convert-ToYamlList {
    param([string[]]$Items)
    if (-not $Items -or $Items.Count -eq 0) {
        return "[]"
    }
    return ($Items | ForEach-Object { "  - $_" }) -join "`n"
}

function Test-HasFile {
    param([string]$Root, [string]$RelativePath)
    return Test-Path -LiteralPath (Join-Path $Root $RelativePath)
}

function New-GovernanceEntry {
    param(
        [string]$Id,
        [string]$Label,
        [string]$Layer,
        [string]$Family,
        [string]$PackStage,
        [string[]]$Paths
    )
    return [pscustomobject]@{
        id = $Id
        label = $Label
        layer = $Layer
        family = $Family
        pack_stage = $PackStage
        paths = $Paths
    }
}

function Get-ExistingPaths {
    param([string]$Root, [string[]]$Candidates)
    $existing = New-Object System.Collections.Generic.List[string]
    foreach ($candidate in $Candidates) {
        if ($candidate -like "*`**") {
            $dir = Split-Path $candidate -Parent
            $pattern = Split-Path $candidate -Leaf
            $absDir = Join-Path $Root $dir
            if (Test-Path -LiteralPath $absDir) {
                $matches = @(Get-ChildItem -LiteralPath $absDir -Filter $pattern -File -ErrorAction SilentlyContinue)
                if ($matches.Count -gt 0) {
                    $existing.Add($candidate)
                }
            }
            continue
        }
        if (Test-HasFile $Root $candidate) {
            $existing.Add($candidate)
        }
    }
    return $existing.ToArray()
}

function New-GovernanceYaml {
    param([string]$Root)

    $entries = New-Object System.Collections.Generic.List[object]
    $entries.Add((New-GovernanceEntry "governance" "Gobierno documental" "00" "functional" "governance" @(".docs/wiki/00_gobierno_documental.md")))

    $specFullSignals = @(
        ".docs/wiki/10_manifiesto_marca_experiencia.md",
        ".docs/wiki/11_manifiesto_marca_experiencia.md",
        ".docs/wiki/10_identidad_visual.md",
        ".docs/wiki/11_identidad_visual.md",
        ".docs/wiki/11_UXR.md",
        ".docs/wiki/17_UXR.md",
        ".docs/wiki/24_uxui/INDEX.md"
    )
    $hasUX = @($specFullSignals | Where-Object { Test-HasFile $Root $_ }).Count -gt 0
    $hasTechnical = @(@(
        ".docs/wiki/07_baseline_tecnica.md",
        ".docs/wiki/08_baseline_tecnica.md",
        ".docs/wiki/08_modelo_fisico_datos.md",
        ".docs/wiki/09_modelo_fisico_datos.md",
        ".docs/wiki/09_contratos_tecnicos.md",
        ".docs/wiki/10_contratos_tecnicos.md"
    ) | Where-Object { Test-HasFile $Root $_ })

    $scope = @(Get-ExistingPaths $Root @(".docs/wiki/01_alcance_funcional.md", ".docs/wiki/01_*.md"))
    if ($scope.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "scope" "Alcance funcional" "01" "functional" "scope" @($scope[0])))
    }

    $outcome = @(Get-ExistingPaths $Root @(".docs/wiki/02_resultados_soluciones_usuario.md", ".docs/wiki/02_resultados/*.md"))
    if ($outcome.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "outcome" "Resultados y soluciones de usuario" "RS" "functional" "outcome" $outcome))
    }

    $architecture = @(Get-ExistingPaths $Root @(".docs/wiki/02_arquitectura.md", ".docs/wiki/03_arquitectura.md"))
    if ($architecture.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "architecture" "Arquitectura" "02" "functional" "architecture" @($architecture[0])))
    }

    $flow = @(Get-ExistingPaths $Root @(".docs/wiki/03_FL.md", ".docs/wiki/04_FL.md", ".docs/wiki/03_FL/*.md", ".docs/wiki/04_FL/*.md", ".docs/wiki/FL/*.md"))
    if ($flow.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "flow" "Flujos funcionales" "03" "functional" "flow" $flow))
    }

    $requirements = @(Get-ExistingPaths $Root @(".docs/wiki/04_RF.md", ".docs/wiki/05_RF.md", ".docs/wiki/02_RF.md", ".docs/wiki/04_RF/*.md", ".docs/wiki/05_RF/*.md", ".docs/wiki/RF/*.md"))
    if ($requirements.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "requirements" "Requerimientos funcionales" "04" "functional" "requirements" $requirements))
    }

    $data = @(Get-ExistingPaths $Root @(".docs/wiki/05_modelo_datos.md", ".docs/wiki/06_modelo_datos.md"))
    if ($data.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "data" "Modelo de datos" "05" "functional" "data" @($data[0])))
    }

    $tests = @(Get-ExistingPaths $Root @(".docs/wiki/06_matriz_pruebas_RF.md", ".docs/wiki/07_matriz_pruebas_RF.md", ".docs/wiki/06_pruebas/*.md", ".docs/wiki/07_pruebas/*.md", ".docs/wiki/pruebas/*.md"))
    if ($tests.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "tests" "Matriz y protocolos de prueba" "06" "functional" "tests" $tests))
    }

    $technical = @(Get-ExistingPaths $Root @(".docs/wiki/07_baseline_tecnica.md", ".docs/wiki/08_baseline_tecnica.md", ".docs/wiki/07_tech/*.md", ".docs/wiki/08_tech/*.md"))
    if ($technical.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "technical_baseline" "Baseline tecnica" "07" "technical" "technical_baseline" $technical))
    }

    $physical = @(Get-ExistingPaths $Root @(".docs/wiki/08_modelo_fisico_datos.md", ".docs/wiki/09_modelo_fisico_datos.md", ".docs/wiki/08_db/*.md", ".docs/wiki/09_db/*.md"))
    if ($physical.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "physical_data" "Modelo fisico de datos" "08" "technical" "physical_data" $physical))
    }

    $contracts = @(Get-ExistingPaths $Root @(".docs/wiki/09_contratos_tecnicos.md", ".docs/wiki/10_contratos_tecnicos.md", ".docs/wiki/09_contratos/*.md", ".docs/wiki/10_contratos/*.md"))
    if ($contracts.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "contracts" "Contratos tecnicos" "09" "technical" "contracts" $contracts))
    }

    $uxGlobal = @(Get-ExistingPaths $Root @(
        ".docs/wiki/10_manifiesto_marca_experiencia.md",
        ".docs/wiki/11_manifiesto_marca_experiencia.md",
        ".docs/wiki/10_identidad_visual.md",
        ".docs/wiki/11_identidad_visual.md",
        ".docs/wiki/12_identidad_visual.md",
        ".docs/wiki/10_lineamientos_interfaz_visual.md",
        ".docs/wiki/12_lineamientos_interfaz_visual.md",
        ".docs/wiki/13_lineamientos_interfaz_visual.md",
        ".docs/wiki/13_voz_tono.md",
        ".docs/wiki/14_voz_tono.md",
        ".docs/wiki/10_patrones_ui.md",
        ".docs/wiki/16_patrones_ui.md",
        ".docs/wiki/17_patrones_ui.md"
    ))
    if ($uxGlobal.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "ux_global" "Canon UX/UI global" "10" "ux" "ux_global" $uxGlobal))
    }

    $uxResearch = @(Get-ExistingPaths $Root @(".docs/wiki/11_UXR.md", ".docs/wiki/17_UXR.md", ".docs/wiki/18_UXR.md", ".docs/wiki/12_UXI.md", ".docs/wiki/18_UXI.md", ".docs/wiki/19_UXI.md", ".docs/wiki/13_UJ.md", ".docs/wiki/19_UJ.md", ".docs/wiki/20_UJ.md"))
    if ($uxResearch.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "ux_research" "Indices UX investigacion, intencion y journey" "18" "ux" "ux_research" $uxResearch))
    }

    $uxSpec = @(Get-ExistingPaths $Root @(".docs/wiki/14_UXS.md", ".docs/wiki/20_UXS.md", ".docs/wiki/21_UXS.md", ".docs/wiki/15_UXV.md", ".docs/wiki/15_matriz_validacion_ux.md", ".docs/wiki/21_matriz_validacion_ux.md", ".docs/wiki/22_matriz_validacion_ux.md", ".docs/wiki/16_aprendizaje_ux_ui_spec_driven.md", ".docs/wiki/22_aprendizaje_ux_ui_spec_driven.md", ".docs/wiki/23_aprendizaje_ux_ui_spec_driven.md", ".docs/wiki/24_uxui/*.md"))
    if ($uxSpec.Count -gt 0) {
        $entries.Add((New-GovernanceEntry "ux_spec" "Especificacion, validacion y aprendizaje UX/UI" "21" "ux" "ux_spec" $uxSpec))
    }

    if ($hasUX) {
        $profile = "spec_full"
        $overlays = @("spec_core", "technical", "uxui")
    } elseif ($hasTechnical.Count -gt 0) {
        $profile = "spec_backend"
        $overlays = @("spec_core", "technical")
    } else {
        $profile = "ordered_wiki"
        $overlays = @("spec_core")
    }

    $context = @($entries | ForEach-Object { $_.id })
    $closurePreferred = @("governance", "outcome", "flow", "requirements", "technical_baseline", "contracts", "tests", "ux_spec")
    $auditPreferred = @("governance", "outcome", "requirements", "technical_baseline", "physical_data", "contracts", "tests", "ux_global", "ux_research", "ux_spec")
    $entryIDs = @($entries | ForEach-Object { $_.id })
    $closure = @($closurePreferred | Where-Object { $entryIDs -contains $_ })
    if ($closure.Count -eq 0) { $closure = @("governance") }
    $audit = @($auditPreferred | Where-Object { $entryIDs -contains $_ })
    if ($audit.Count -eq 0) { $audit = @("governance") }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("version: 1")
    $lines.Add("profile: $profile")
    $lines.Add("overlays:")
    foreach ($overlay in $overlays) { $lines.Add("  - $overlay") }
    $lines.Add("numbering_recommended: true")
    $lines.Add("hierarchy:")
    foreach ($entry in $entries) {
        $lines.Add("  - id: $($entry.id)")
        $lines.Add("    label: $($entry.label)")
        $lines.Add("    layer: `"$($entry.layer)`"")
        $lines.Add("    family: $($entry.family)")
        $lines.Add("    pack_stage: $($entry.pack_stage)")
        $lines.Add("    paths:")
        foreach ($path in $entry.paths) { $lines.Add("      - $path") }
    }
    $lines.Add("context_chain:")
    $lines.Add((Convert-ToYamlList $context))
    $lines.Add("closure_chain:")
    $lines.Add((Convert-ToYamlList $closure))
    $lines.Add("audit_chain:")
    $lines.Add((Convert-ToYamlList $audit))
    $lines.Add("blocking_rules:")
    foreach ($rule in @("missing_human_governance_doc", "missing_governance_yaml", "invalid_governance_schema", "projection_out_of_sync", "workspace_index_stale")) {
        $lines.Add("  - $rule")
    }
    $lines.Add("projection:")
    $lines.Add("  output: .docs/wiki/_mi-lsp/read-model.toml")
    $lines.Add("  format: toml")
    $lines.Add("  auto_sync: true")
    $lines.Add("  versioned: true")

    return [pscustomobject]@{
        yaml = ($lines -join "`n")
        profile = $profile
        hierarchy_count = $entries.Count
    }
}

function Set-GovernanceDoc {
    param(
        [string]$Root,
        [string]$Yaml
    )
    $wikiDir = Join-Path $Root ".docs/wiki"
    $docPath = Join-Path $wikiDir "00_gobierno_documental.md"
    New-Item -ItemType Directory -Force -Path $wikiDir | Out-Null

    $block = '```yaml' + "`n" + $Yaml + "`n" + '```'
    if (Test-Path -LiteralPath $docPath) {
        $content = Get-Content -Raw -LiteralPath $docPath
        if ($content -match '(?s)```yaml\s+.*?```') {
            $content = [regex]::Replace($content, '(?s)```yaml\s+.*?```', [System.Text.RegularExpressions.MatchEvaluator]{ param($m) $block }, 1)
        } else {
            $content = $content.TrimEnd() + "`n`n---`n`n## Governance Source`n`n$block`n"
        }
        Set-Content -LiteralPath $docPath -Value $content -Encoding utf8
    } else {
        $content = @"
# 00. Gobierno documental

## Proposito

Este documento define la autoridad humana y la fuente machine-readable minima para que `mi-lsp` pueda validar, enrutar y auditar este workspace.

## Governance Source

$block

## Autoridad canonica

- `00_gobierno_documental.md` manda.
- `_mi-lsp/read-model.toml` es una proyeccion ejecutable derivada.
- Si la gobernanza queda invalida o stale, solo se permite diagnostico y reparacion.
"@
        Set-Content -LiteralPath $docPath -Value $content -Encoding utf8
    }
}

function Invoke-MiLsp {
    param([string[]]$Arguments)
    $output = & $Cli @Arguments --format json 2>&1
    return [pscustomobject]@{
        exit_code = $LASTEXITCODE
        output = ($output | Out-String).Trim()
    }
}

if (-not (Test-Path -LiteralPath $ReportJson)) {
    throw "ReportJson not found: $ReportJson"
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$targetDir = Join-Path $OutDir "$timestamp-governance-followup-repair"
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

$report = Get-Content -Raw -LiteralPath $ReportJson | ConvertFrom-Json -Depth 80
$targets = @($report.workspaces |
    Where-Object { $_.status.result.items[0].governance_blocked } |
    Group-Object root |
    ForEach-Object { $_.Group | Select-Object -First 1 })

$results = New-Object System.Collections.Generic.List[object]
foreach ($target in $targets) {
    $root = [string]$target.root
    $alias = [string]$target.workspace
    $before = $target.status.result.items[0]
    $generated = New-GovernanceYaml -Root $root

    if ($PSCmdlet.ShouldProcess($root, "repair governance for alias $alias")) {
        Set-GovernanceDoc -Root $root -Yaml $generated.yaml
        $governance = Invoke-MiLsp -Arguments @("nav", "governance", "--workspace", $alias)
        $index = Invoke-MiLsp -Arguments @("index", "--workspace", $alias, "--docs-only")
        $status = Invoke-MiLsp -Arguments @("workspace", "status", $alias, "--no-auto-sync")
    } else {
        $governance = [pscustomobject]@{ exit_code = 0; output = "whatif" }
        $index = [pscustomobject]@{ exit_code = 0; output = "whatif" }
        $status = [pscustomobject]@{ exit_code = 0; output = "whatif" }
    }

    $results.Add([pscustomobject]@{
        alias = $alias
        root = $root
        before_sync = $before.governance_sync
        before_index_sync = $before.governance_index_sync
        profile = $generated.profile
        hierarchy_count = $generated.hierarchy_count
        governance_exit = $governance.exit_code
        index_exit = $index.exit_code
        status_exit = $status.exit_code
        governance_output = $governance.output
        index_output = $index.output
        status_output = $status.output
    })
}

$jsonPath = Join-Path $targetDir "repair.json"
$mdPath = Join-Path $targetDir "repair.md"
$results | ConvertTo-Json -Depth 80 | Set-Content -LiteralPath $jsonPath -Encoding utf8

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Governance Follow-up Repair")
$lines.Add("")
$lines.Add("- generated_at: $(Get-Date -Format o)")
$lines.Add("- target_roots: $($targets.Count)")
$lines.Add("")
foreach ($item in $results) {
    $ok = ($item.governance_exit -eq 0 -and $item.index_exit -eq 0 -and $item.status_exit -eq 0)
    $lines.Add("- $($item.alias): ok=$ok profile=$($item.profile) hierarchy=$($item.hierarchy_count) root=$($item.root)")
}
$lines | Set-Content -LiteralPath $mdPath -Encoding utf8

[pscustomobject]@{
    ok = (@($results | Where-Object { $_.governance_exit -ne 0 -or $_.index_exit -ne 0 -or $_.status_exit -ne 0 }).Count -eq 0)
    target_roots = $targets.Count
    report_json = $jsonPath
    report_md = $mdPath
}

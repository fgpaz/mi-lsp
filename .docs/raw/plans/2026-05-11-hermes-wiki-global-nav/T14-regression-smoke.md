---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - TP-WIKI
allowed_paths:
  - scripts/release/regression-smoke.ps1
forbidden_paths:
  - .docs/wiki/**
  - internal/**
  - worker-dotnet/**
verify:
  - pwsh scripts/release/regression-smoke.ps1 -> exit 0 (con dos o más workspaces docs_ready=true en el registry)
stop_if:
  - regression-smoke.ps1 no existe (governance issue — alertar)
  - el script tiene una estructura distinta a la documentada en CLAUDE.md
secret_scan: clean
---

# Task T14: Ampliar regression-smoke.ps1 con los cinco subcomandos federados

## Shared Context
**Goal:** Extender el smoke de release para incluir los cinco subcomandos wiki con `--all-workspaces`.
**Stack:** PowerShell.
**Architecture:** `scripts/release/regression-smoke.ps1` ya recorre aliases del registry e invoca varios `nav wiki *`. Se le agregan invocaciones con el flag.

## Locked Decisions
- NO duplicar el script — extender el existente.
- Agregar smoke para: search, route, trace, pack, inventory con `--all-workspaces`.
- Validaciones: envelope parsea como TOON, `ok=true`, `stats.workspaces_queried > 0`, no falla con governance_blocked en alguno.

## Task Metadata
```yaml
id: T14
depends_on: [T8, T9, T10, T11, T12]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "regression-smoke.ps1 cubre los cinco subcomandos federados y pasa contra el registry actual."
files:
  - modify: scripts/release/regression-smoke.ps1
  - read: scripts/release/regression-smoke.ps1
complexity: low
done_when:
  - "el script invoca los cinco subcomandos con --all-workspaces"
  - "pwsh scripts/release/regression-smoke.ps1 exit 0"
  - "el script reporta cada invocación con un mensaje claro [PASS]/[FAIL]"
evidence_expected:
  - "Output del script ejecutado"
  - "Diff del script"
stop_if:
  - "el script no existe — alertar al humano"
```

## Reference
- `scripts/release/regression-smoke.ps1` (existente).
- Patrón mencionado en CLAUDE.md: "scripts/release/regression-smoke.ps1: recorre aliases registrados con nav wiki search, nav wiki pack, nav wiki trace".

## Prompt

Sos el ejecutor de T14 (ps-worker). Extender un script existente.

1. Leer `scripts/release/regression-smoke.ps1` para entender estructura actual.
2. Agregar UNA sección "Federated wiki smoke" después de las invocaciones existentes:
   ```powershell
   Write-Host "=== Federated wiki smoke (--all-workspaces) ===" -ForegroundColor Cyan
   
   $cmds = @(
       @{ Name = "search"; Args = @("nav", "wiki", "search", "governance", "--all-workspaces", "--format", "toon") },
       @{ Name = "route"; Args = @("nav", "wiki", "route", "governance", "--all-workspaces", "--format", "toon") },
       @{ Name = "trace"; Args = @("nav", "wiki", "trace", "--all-workspaces", "--format", "toon") },
       @{ Name = "pack"; Args = @("nav", "wiki", "pack", "--all-workspaces", "--format", "toon") },
       @{ Name = "inventory"; Args = @("nav", "wiki", "inventory", "--all-workspaces", "--format", "toon") }
   )
   
   foreach ($c in $cmds) {
       $output = & mi-lsp @($c.Args)
       if ($LASTEXITCODE -ne 0) {
           Write-Host "[FAIL] nav wiki $($c.Name) --all-workspaces" -ForegroundColor Red
           exit 1
       }
       if ($output -notmatch "ok: true") {
           Write-Host "[FAIL] nav wiki $($c.Name) -- envelope did not contain ok=true" -ForegroundColor Red
           exit 1
       }
       if ($output -notmatch "workspaces_queried") {
           Write-Host "[FAIL] nav wiki $($c.Name) -- missing workspaces_queried in stats" -ForegroundColor Red
           exit 1
       }
       Write-Host "[PASS] nav wiki $($c.Name) --all-workspaces" -ForegroundColor Green
   }
   ```
3. Correr:
   ```powershell
   pwsh scripts/release/regression-smoke.ps1
   ```
   Verificar exit 0.
4. Commit: `test(release): cover federated nav wiki subcommands in regression smoke`.
5. Reportar output al orquestador.

## Execution Procedure
1. Leer el script.
2. Agregar la sección.
3. Ejecutar y verificar PASS.
4. Commit.
5. Reportar.

## Skeleton
(ver el bloque PowerShell del Prompt arriba)

## Verify
`pwsh scripts/release/regression-smoke.ps1` -> exit 0 con los cinco subcomandos marcados [PASS]

## Commit
`test(release): cover federated nav wiki subcommands in regression smoke`

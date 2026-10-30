---
wiki_contract: WIKI-CONTRACT-v1
id: RF-HARNESS-DIAG-001
kind: requirement
status: draft
title: "Consultar diagnósticos semánticos reales después de editar"
links:
  implements: [FL-ALLHARNESS-001]
  code:
    - path: internal/cli/nav_diagnostics.go
    - path: internal/model/diagnostics.go
    - path: internal/service/diagnostics.go
    - path: internal/service/diagnostics_budget.go
    - path: internal/worker/diagnostics.go
    - path: worker-dotnet/MiLsp.Worker/RoslynService.cs
---

## Propósito
El harness consulta errores y advertencias del backend semántico sin lanzar una compilación, suite o búsqueda textual. Una lista vacía solo puede provenir de un informe recibido; falta de soporte, timeout o archivos eliminados nunca significan «sin errores».

## Superficies
`mi-lsp nav diagnostics <archivo>` o `mi-lsp nav diagnostics --changed [--base <ref>]`. La integración de `lsp.diagnostics` pertenece a surface; el servicio expone `nav.diagnostics` y el constructor es `newDiagnosticsCommand(state *rootState) *cobra.Command`.

## Entradas/salidas tipadas
Un archivo relativo al workspace o absoluto dentro de él, o cambios Git en ese worktree. Flags locales: `--changed`, `--base`, `--max-files` (32 por defecto, 1–64), `--offset` (0 por defecto). Se conservan los presupuestos globales. JSON `diagnostics/v1`: `items` con `path`, `line`, `column`, `severity`, `message`, `source`/`code` opcionales, coordenadas UTF-16 desde uno. El bloque `diagnostics` cuenta hallazgos antes de recortar la salida y archivos efectivamente consultados.

## Comportamiento ordenado
La consulta selecciona la superficie semántica y conserva presupuestos, degradación y omisiones tipadas antes de interpretar el resultado.

## Criterios de aceptación
1. Selecciona gopls, tsserver, Roslyn, Pyright o rust-analyzer con el routing normal por extensión. No usa catálogo/texto como reemplazo de diagnósticos.
2. `--changed` une staged, unstaged y untracked no ignorados, con nombres delimitados por NUL. Lee los bytes actuales del working tree, no blobs del índice. `--base` agrega el diff entre `merge-base(ref, HEAD)` y working tree; no sustituye el conjunto local. Una eliminación queda con `file_missing`.
3. No lee fuera del workspace ni sigue symlinks externos; un worktree enlazado conserva su raíz física en vez de consultar el checkout principal.
4. Trabajo acotado: 30 s globales, 8 s por archivo, 256 paths descubiertos, 32 archivos por defecto y hasta 64; cada fuente hasta 2 MiB. Backends independientes arrancan en paralelo, archivos del mismo backend se serializan.
5. Ordena errores antes que warnings, luego información/hints, path y posición. Los límites no permiten que warnings numerosos desplacen errores. Totales, `truncated`, omisiones y `next_hint` explican lo recortado.
6. Un push sin barrera de finalización es siempre `partial`; sin publicación no hay resultado limpio. Rust reporta solo diagnósticos nativos: no ejecuta cargo check, build scripts ni proc macros. Roslyn reporta diagnósticos del compilador del documento, no analizadores externos.

## Errores
`unsupported_operation`, `unavailable`, `timeout`, `invalid_path`, `file_missing`, `work_limit`, `invalid_response`, `explicit_incomplete` viven en `omissions[].error_code`. Si ningún archivo pudo consultarse, `ok=false`; un diagnóstico de severidad error no implica fallo de transporte (`ok=true` con error semántico).

## Integración pendiente de surface
Este RF no inventa RS/FL comunes. El padre debe añadir `links.implements` al FL real del objetivo y enlazar ese FL con su RS; no se modifican governance, RS ni FL desde esta lane. Estado `draft` hasta la integración/verificación del objetivo.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-HARNESS-DIAG-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-ALLHARNESS-001.md
exports:
- RF-HARNESS-DIAG-001
- '"Consultar diagnósticos semánticos reales después de editar"'
agent_must_read:
- .docs/wiki/05_requirements/RF-HARNESS-DIAG-001.md
- .docs/wiki/04_flows/FL-ALLHARNESS-001.md
- .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md
- .docs/wiki/06_tests/TP-HARNESS-FIND-001.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-HARNESS-DIAG-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/nav_diagnostics.go
- internal/model/diagnostics.go
- internal/service/diagnostics.go
- internal/service/diagnostics_budget.go
- internal/worker/diagnostics.go
- worker-dotnet/MiLsp.Worker/RoslynService.cs
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md sin
  inferir que fueron ejecutados.
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-HARNESS-DIAG-001.
evidence:
- .docs/wiki/05_requirements/RF-HARNESS-DIAG-001.md
- .docs/wiki/06_tests/TP-HARNESS-DIAG-001.md
- .docs/wiki/06_tests/TP-HARNESS-FIND-001.md
```

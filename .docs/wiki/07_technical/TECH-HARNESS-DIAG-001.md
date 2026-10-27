---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-HARNESS-DIAG-001
kind: technical
status: draft
title: "Contrato del bridge de diagnósticos semánticos"
links:
  supports: [RF-HARNESS-DIAG-001]
  code:
    - path: internal/service/diagnostics.go
    - path: internal/service/diagnostics_budget.go
    - path: internal/model/diagnostics.go
    - path: internal/worker/diagnostics.go
    - path: internal/worker/lsp_transport.go
    - path: internal/worker/lsp_client.go
    - path: internal/worker/tsserver.go
    - path: internal/worker/rust_analyzer.go
    - path: worker-dotnet/MiLsp.Worker/RoslynService.cs
    - path: internal/output/diagnostics.go
---

## Resumen
`newDiagnosticsCommand(*rootState)` → `nav.diagnostics` → routing semántico habitual → worker `Method="diagnostics"`. No añade método público de filesystem arbitrario, contenido del documento ni argv suministrables por el consumidor. El servicio omite enriquecimiento wiki/catálogo e indexación para esta operación. `Workspace` se resuelve read-only; los worktrees se distinguen por raíz física y solo se corrige el alias al comprobar igualdad de `git-common-dir`.

## Payload para surface/provider
```json
{"file":"src/example.go","changed":false,"base":"","max_files":32,"offset":0}
```
`file` y `changed=true` son excluyentes. Para cambios: `{"changed":true,"base":"origin/main","max_files":32,"offset":0}`. Sin `base`, unión de cambios staged/unstaged y untracked no ignorados; con base, se suma el diff desde merge-base. Fuente: bytes actuales del worktree, no blobs staged. Descubrimiento NUL-safe, sin ext-diff/textconv, sin renames para conservar paths eliminados y nuevos. Un límite de descubrimiento conserva `work_limit`.

## JSON
`contract_version="diagnostics/v1"`; `items` siempre lista de `model.Diagnostic`. `path` relativo al workspace con `/`, `line`/`column` uno-based UTF-16, `severity=error|warning|information|hint`, `message` string, `source` string opcional, `code` string o número opcional. Esquema de referencia: `testdata/diagnostics/schema.json`.

`diagnostics` contiene `errors`, `warnings`, `information`, `hints`, `files_requested`, `files_checked`, `returned`, `backends` (archivos por backend). Totales antes del recorte; si se alcanza el límite de 256 paths, `files_requested` es el número descubierto retenido, no un total inventado del repo. `ok` expresa transporte/consulta, no ausencia de errores. `partial`/`degraded` y omisiones impiden interpretar una lista vacía como limpia. `truncated` y `next_hint` describen trabajo/salida omitidos. Offset vuelve a consultar un snapshot nuevo: no promete cursor estable entre ediciones.

Si ningún archivo pudo consultarse, el envelope incluye `error.code`, `error.reason_code`, `error.message` y `error.detail` con la causa de las omisiones (por ejemplo `unsupported_operation`: lenguaje sin backend). No se devuelve solo `explicit_incomplete`. Causas heterogéneas se conservan por archivo en `omissions`; un snapshot push sigue siendo parcial y nunca acredita limpieza.

## Backends implementados
- **gopls:** `textDocument/diagnostic` si anuncia `diagnosticProvider`; en versiones push, `publishDiagnostics` tras didOpen/didChange. Push siempre parcial por falta de barrera de finalización.
- **Pyright:** mismo bridge LSP; normalmente push. Publicación sin versión añade razón de frescura no verificable. Silencio → timeout/unavailable, nunca éxito limpio.
- **rust-analyzer:** pull anunciado o push; runtime separado por sufijo de entrypoint `::diagnostics-native` para que no herede un proceso warm con cargo check activado. `checkOnSave=false`, build scripts y proc macros desactivados. No sustituye rustc/clippy: la cobertura siempre es parcial.
- **tsserver:** `syntacticDiagnosticsSync`, `semanticDiagnosticsSync`, `suggestionDiagnosticsSync` con `includeLinePosition=true`; lee `message` y `startLocation` (no el formato `text` de eventos geterr). Close/open con contenido actual antes de consultar. Si falla una etapa posterior, conserva los hallazgos de las etapas anteriores con `partial` y motivo. No dispara tsc ni geterr con espera indeterminada.
- **Roslyn:** método nuevo en el worker distribuido; `Document.GetSemanticModelAsync` y `SemanticModel.GetDiagnostics`, filtrado al archivo, solución recargada como en las consultas existentes. Sin comando dotnet build ni suite. No ejecuta analizadores externos. Worker viejo sin método → unavailable, exige publicar el worker junto al CLI.
- **Otros (incluido Kotlin):** `unsupported_operation`, sin arranque ni fallback textual.

## Presupuestos y estado
30 s por operación, 8 s por archivo, 1 MiB de salida Git por llamada, 256 paths descubiertos, 32 fuentes por defecto (máximo 64), 2 MiB por fuente, máximo 200 hallazgos devueltos por página. Cinco backends independientes concurrentes; una lane por backend y cola serial. El primer timeout detiene nuevas admisiones en esa lane. Inicio/descubrimiento de un backend que ignore contexto no retiene la respuesta agregada; se conserva el timeout y no se abren más requests en esa lane. `npm root -g` tiene deadline de 3 s.

El transporte LSP mantiene solo la URI solicitada; didChange avanza versión incluso con mismos bytes. Las publicaciones de versión distinta no satisfacen una consulta. No hay barrera de finalización para push: aunque tenga versión, devuelve `partial`. Los timeouts de tsserver cierran los pipes/proceso; Roslyn invalida el proceso para que un frame tardío no satisfaga otra consulta.

Errores primero antes de aplicar max-items/token-budget/max-chars. Si el propio metadata excede un presupuesto minúsculo, el envelope mínimo sigue marcando truncamiento y recomienda aumentarlo; no promete caber en menos bytes que su esquema obligatorio. Texto/agent muestran conteos y hallazgos principales; formatos estructurados conservan el esquema.

## Mapa de componentes
- `newDiagnosticsCommand(*rootState)` → `nav.diagnostics` → routing semántico → worker `Method="diagnostics"`.
- Los backends gopls, Pyright, rust-analyzer, tsserver y Roslyn alimentan el bridge LSP; `internal/output/diagnostics.go` proyecta la salida.
- Los archivos y constructores concretos permanecen enumerados en los `links.code` de este documento.

## Integración y evidencia
Surface registra el constructor en nav.go, publica `lsp.diagnostics`, clasifica la operación para daemon/AXI y conserva budgets/omisiones. Esta lane no modifica nav.go/root.go/provider_manifest.go. La suite y el smoke real quedan para [[TP-HARNESS-DIAG-001]] dentro de la FINAL_VERIFY del padre; ningún backend se declara observado en runtime por agregar su bridge.

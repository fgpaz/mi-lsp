---
id: RF-IDX-001
title: Construir y refrescar el indice repo-local
implements:
  - internal/cli/index.go
  - internal/service/index_jobs.go
  - internal/indexer/indexer.go
  - internal/store/index_jobs.go
  - internal/store/index_publish.go
tests:
  - internal/service/app_test.go
  - internal/store/store_test.go
  - internal/store/index_jobs_test.go
  - internal/store/index_lock_test.go
  - internal/indexer/indexer_progress_test.go
  - internal/workspace/gitignore_test.go
  - internal/indexer/extractor_python_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-IDX-001"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-IDX-001]]'
exports:
  - 'RF-IDX-001'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-IDX-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-IDX-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-IDX-001.md
```

# RF-IDX-001 - Construir y refrescar el indice repo-local

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-IDX-001 |
| Titulo | Construir y refrescar el indice repo-local |
| Actores | Desarrollador, Skill, CLI/Core, Indexer |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-IDX-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble por path o alias | funcional | obligatorio |
| `<repo>/.mi-lsp/` escribible | tecnica | obligatorio |
| Reglas de ignore disponibles | operativa | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI resuelve el workspace objetivo y carga `project.toml`.
2. El indexer obtiene ignores desde defaults internos, `.gitignore`, `.milspignore` y `[ignore].extra_patterns`, respetando el orden del archivo y los re-includes negados (`!pattern`) sobre paths normalizados con `/`.
   Los ignores operacionales hard-coded, incluyendo `.mi-lsp/` y `**/.mi-lsp/**`, se aplican al final y no pueden ser re-habilitados por negaciones del repo.
3. La CLI crea un job durable y una generacion candidata en `index.db`.
4. Si `--clean` esta activo, fuerza recomposicion completa del modo elegido, sin borrar `index.db` antes de construir el nuevo resultado.
5. El walker enumera archivos y asigna ownership por `repo_id`.
6. El extractor prepara `workspace_repos`, `workspace_entrypoints`, `FileRecord`, `SymbolRecord` y `WorkspaceMeta`, reportando progreso vivo durante loops largos de catalogo/docs. En Go, conserva la ruta exacta del entrypoint relativa al workspace (incluido `runtime/go.mod`). El selector que consume el observador puede ser una ruta de módulo repo-local explícita (`go.mod`) o un ID de `WorkspaceEntrypoint`; un ID se resuelve con coincidencia exacta de repo y entrypoint, se rebasa su ruta workspace-relative al root del repo seleccionado y se revalida como `go.mod` repo-local seguro y regular. Si el selector repo-local está vacío, usa el fallback al `go.mod` del root; un ID desconocido o malformado, una selección explícita insegura, inexistente o no regular, incluido un symlink final, falla cerrado y queda como omisión/diagnóstico (`""`), sin activar el fallback. `go.work` no tiene extracción de grafo en este slice y un selector explícito `go.work` se rechaza.
7. El indexador documental prepara `DocRecord`, `DocEdge` y `DocMention` a partir de `.docs/wiki`, `README*`, `docs/` y `.docs/`.
8. El runtime publica catalogo, docs y memoria de reentrada en una unica transaccion SQLite para `mode=full`.
9. La CLI devuelve job status, stats y warnings no fatales si encontro ruido, archivos sin match de repo o problemas de docs.

## 4. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `index.db` | archivo | repo local | catalogo derivado listo para discovery |
| `stats.files` | numero | usuario/skill | total procesado |
| `stats.symbols` | numero | usuario/skill | total persistido |
| `warnings` | lista | usuario/skill | sugerencias de limpieza, ownership dudoso o problemas de docs |
| `job_id` | string | usuario/skill | identificador durable para status/cancel |
| `generation_id` | string | workspace_meta | generacion publicada o candidata |
| `current_stage` / `current_path` / `files_total` | campos de job | usuario/skill | progreso vivo mientras el job corre |

## 5. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `IDX_WORKSPACE_NOT_FOUND` | workspace no resoluble | alias/path invalido | abortar sin crear indice |
| `IDX_WALK_FAILED` | fallo de exploracion | error de lectura en repo | abortar con mensaje y contexto |
| `IDX_DB_WRITE_FAILED` | fallo SQLite | error de apertura o escritura | abortar con error sin ocultar causa |

## 6. Special Cases and Variants

- El indice guarda ownership por repo incluso en `container`.
- Si el indice detecta ruido en `.docs`, `old/`, `temp/` u otros paths no ignorados, debe sugerir `.milspignore`.
- Si la wiki canonica existe en disco pero `doc_records` quedo solo con docs `generic`, un `index` incremental sin cambios detectados debe degradar a full re-index en vez de responder `no changes detected`.
- Si el proceso cae antes del commit de publicacion, SQLite debe conservar el estado previo y el job queda diagnosticable como stale/failed en la siguiente inspeccion.
- Los entrypoints auxiliares bajo `.docs/` o `template(s)` no deben convertirse en el default semantico solo por estar presentes en el repo.
- El indice documental resuelve primero links markdown y doc IDs explicitos; las heuristicas solo completan contexto, no reemplazan trazabilidad explicita.
- El indice nunca persiste refs profundas ni ASTs.
- Para Go (`.go`), el extractor usa `go/parser`/AST nativo para catalogar funciones, métodos, tipos, structs, interfaces, consts y vars. Un módulo anidado puede conservarse como entrypoint con ruta exacta relativa al workspace (por ejemplo, `runtime/go.mod`); el selector del observador puede ser la ruta repo-local explícitamente configurada (`go.mod`) o un ID de `WorkspaceEntrypoint`. Un ID se resuelve con coincidencia exacta de repo y entrypoint, se rebasa la ruta workspace-relative al root del repo seleccionado y se revalida como `go.mod` repo-local seguro y regular. IDs desconocidos o malformados, rutas inseguras, absolutas, con `..`, fuera del root, inexistentes, no regulares o con symlink final fallan cerrado y producen omisión/diagnóstico (`""`) sin activar el fallback; `go.work` se rechaza como selector.
- Si el selector Go repo-local está vacío, la selección permanece determinista en `go.mod` del root. Un root solo con `go.work` no selecciona Go ni se envía al parser de `go.mod`; el contenido raíz no relacionado sigue indexándose y conserva su ownership; el indexador no sustituye este fallback por descubrimiento recursivo arbitrario.
- Para Python (`.py`, `.pyi`), el extractor usa una pasada lexical acotada por lineas e indentacion. Prioriza catalogo navegable y cancelabilidad por archivo; formas no reconocidas quedan cubiertas por `nav search` textual o Pyright opcional.
- `index cancel` sin `--force` marca `requested_cancel`; el worker debe detenerse cooperativamente al volver al loop de indexacion. `--force` queda como escape para procesos colgados y debe limpiar el lock asociado si el PID ya no vive.
- `.mi-lsp/**` es estado operacional del workspace, no input indexable. El walker y los matchers deben excluirlo tambien cuando aparece anidado dentro de repos hijos o cuando `.gitignore` intenta re-incluirlo con `!`.
## 7. Data Model Impact

- `WorkspaceRepo`
- `WorkspaceEntrypoint`
- `SymbolRecord`
- `FileRecord`
- `DocRecord`
- `DocEdge`
- `DocMention`
- `WorkspaceMeta`
- `IndexJob`
- `IndexGeneration`

## 8. Test Traceability

- Positivo: `TP-IDX / TC-IDX-001`
- Positivo: `TP-IDX / TC-IDX-012`
- Positivo: `TP-IDX / TC-IDX-025`
- Negativo: `TP-IDX / TC-IDX-003`

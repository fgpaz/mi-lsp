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
  - internal/store/index_lock_test.go
---

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
3. La CLI crea un job durable y una generacion candidata en `index.db`.
4. Si `--clean` esta activo, fuerza recomposicion completa del modo elegido, sin borrar `index.db` antes de construir el nuevo resultado.
5. El walker enumera archivos y asigna ownership por `repo_id`.
6. El extractor prepara `workspace_repos`, `workspace_entrypoints`, `FileRecord`, `SymbolRecord` y `WorkspaceMeta`.
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
- Para Python (`.py`, `.pyi`), el extractor usa tree-sitter pure Go (`gotreesitter`) en lugar de regex. Si el parsing falla, el archivo se omite sin error fatal.

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

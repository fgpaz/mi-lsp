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
3. Si `--clean` esta activo, elimina y recrea el estado derivado del indice.
4. El walker enumera archivos y asigna ownership por `repo_id`.
5. El extractor persiste `workspace_repos`, `workspace_entrypoints`, `FileRecord`, `SymbolRecord` y `WorkspaceMeta`.
6. El indexador documental persiste `DocRecord`, `DocEdge` y `DocMention` a partir de `.docs/wiki`, `README*`, `docs/` y `.docs/`.
7. La CLI devuelve stats y warnings no fatales si encontro ruido, archivos sin match de repo o problemas de docs.

## 4. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `index.db` | archivo | repo local | catalogo derivado listo para discovery |
| `stats.files` | numero | usuario/skill | total procesado |
| `stats.symbols` | numero | usuario/skill | total persistido |
| `warnings` | lista | usuario/skill | sugerencias de limpieza, ownership dudoso o problemas de docs |

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

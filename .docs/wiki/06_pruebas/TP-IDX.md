# TP-IDX

## Cobertura objetivo

- RF-IDX-001
- RF-IDX-002
- RF-IDX-003

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-IDX-001 | positivo | RF-IDX-001 | crea `index.db` repo-local y reporta stats |
| TC-IDX-002 | positivo | RF-IDX-001 | reindexa con `--clean` reconstruyendo datos derivados |
| TC-IDX-003 | negativo | RF-IDX-001 | falla con workspace no resoluble o error de escritura SQLite |
| TC-IDX-004 | positivo | RF-IDX-001 | indexa clases, funciones y metodos Python con tree-sitter |
| TC-IDX-005 | positivo | RF-IDX-001 | persiste `doc_records`, `doc_edges` y `doc_mentions` a partir de la wiki |
| TC-IDX-006 | positivo | RF-IDX-001 | usa el fallback documental generico (`README*`, `docs/`, `.docs/`) cuando no existe `.docs/wiki` |
| TC-IDX-007 | positivo | RF-IDX-002 | index detecta archivos cambiados via git y solo re-indexa codigo |
| TC-IDX-008 | positivo | RF-IDX-002 | index sin git disponible hace full re-index con warning |
| TC-IDX-009 | positivo | RF-IDX-002 | index con cambios en docs o `read-model.toml` fuerza full re-index |
| TC-IDX-010 | positivo | RF-IDX-003 | `00_gobierno_documental.md` proyecta automaticamente `.docs/wiki/_mi-lsp/read-model.toml` |
| TC-IDX-011 | negativo | RF-IDX-003 | un bloque YAML de gobernanza invalido bloquea la proyeccion y obliga reparacion |
| TC-IDX-012 | positivo | RF-IDX-001 | el indexador documental honra re-includes negados de `.gitignore`/`.milspignore` y no excluye `.docs/wiki/**` cuando fue re-habilitada |
| TC-IDX-013 | positivo | RF-IDX-002 | si `doc_records` quedo solo con docs `generic` aunque la wiki canonica existe, `index` degrada a full re-index y reconstruye el corpus documental |
| TC-IDX-014 | positivo | RF-IDX-001 | `TestIndexRunDocsOnlyRebuildsDocsWithoutReplacingCatalog`: `index --docs-only` reconstruye docs/memory sin borrar simbolos ya catalogados |
| TC-IDX-015 | negativo | RF-IDX-001 | `TestWithWorkspaceIndexLockRejectsConcurrentIndexRun`: dos indexaciones simultaneas del mismo workspace quedan bloqueadas por `.mi-lsp/index.lock` |
| TC-IDX-016 | positivo | RF-IDX-001 | `TestDefaultIgnoreMatcherSkipsGeneratedDependencyCaches`: el indexador ignora `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.turbo`, `.next` y `node_modules` |
| TC-IDX-017 | positivo | RF-IDX-001 | `TestReplaceWorkspaceIndexPublishesGenerationMetadata`: publica punteros `active_*_generation_id` solo tras commit exitoso |
| TC-IDX-018 | positivo | RF-IDX-001 | `TestWithWorkspaceIndexLockRemovesStaleLock`: un lock con PID inexistente se recupera antes de ejecutar |
| TC-IDX-019 | positivo | RF-IDX-001 | `TestIndexStartWaitCreatesSucceededJobAndGeneration`: `index.start --wait` crea job, publica generacion y deja status `succeeded` |

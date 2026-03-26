# TP-IDX

## Cobertura objetivo

- RF-IDX-001
- RF-IDX-002

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

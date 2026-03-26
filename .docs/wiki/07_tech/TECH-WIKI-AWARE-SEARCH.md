# TECH-WIKI-AWARE-SEARCH

## Proposito

Describir el pipeline tecnico de `nav ask` y del indice documental repo-local.
Esta capa existe para que la respuesta docs-first sea reproducible, explainable y barata en tiempo de ejecucion.

## Pipeline

1. `workspace init` o `index` construye `doc_records`, `doc_edges` y `doc_mentions`.
2. `docgraph.LoadProfile()` carga `.docs/wiki/_mi-lsp/read-model.toml` si existe; si no, usa el perfil embebido.
3. `nav ask` clasifica la pregunta por familia (`functional`, `technical`, `ux` o fallback generico).
4. El ranker pondera familia, capa (`01-09`/`10-16`), `doc_id` explicito y tokens de pregunta.
5. El documento primario se completa con supporting docs via `doc_edges` antes de volver al ranking textual.
6. La evidencia de codigo se deriva desde `doc_mentions` (`file_path`, `symbol`) y solo despues usa fallback textual.

## Reglas clave

- Docs primero, codigo despues.
- Links y doc IDs explicitos tienen prioridad sobre similitud textual.
- El `read-model` solo gobierna seleccion y orden; no persiste dentro de SQLite.
- Si no hay docs indexados, `nav ask` degrada a search textual del workspace.
- Si existen docs pero el match es debil, la respuesta igual debe ser explainable con `why` y `next_queries`.

## Fallback generico

Cuando el repo no usa `.docs/wiki`, el indexador considera como corpus documental:
- `README.md`
- `README*.md`
- `docs/`
- `.docs/`

Ese modo permite usar `nav ask` en repos sin gobierno documental estricto, aunque con menor calidad de ranking.

## Costos y tradeoffs

- Persistir el grafo documental evita volver a parsear toda la wiki en cada pregunta.
- El incremental por archivo no alcanza para docs: cambios documentales se tratan como disparador de full re-index.
- No se usan embeddings ni servicios externos en esta version.

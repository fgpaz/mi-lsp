# DB-DOC-INDEX

## Proposito

Detallar las estructuras SQLite usadas para soportar `nav ask` y el ranking wiki-aware.

## Tablas

### `doc_records`

Campos canonicos:
- `path`
- `title`
- `doc_id`
- `layer`
- `family`
- `snippet`
- `search_text`
- `content_hash`
- `indexed_at`
- `is_snapshot`

Uso:
- corpus base para ranking
- lookup rapido por path
- soporte para stats y debugging

### `doc_edges`

Campos canonicos:
- `from_path`
- `to_path`
- `to_doc_id`
- `kind`
- `label`

Uso:
- supporting docs explicitos
- trazabilidad doc -> doc
- prioridad de links markdown y doc IDs antes de heuristicas

### `doc_mentions`

Campos canonicos:
- `doc_path`
- `mention_type`
- `mention_value`

Tipos actuales:
- `doc_id`
- `doc_path`
- `file_path`
- `symbol`
- `command`

Uso:
- derivar evidencia de codigo para `nav ask`
- futuras ayudas de navegacion y onboarding

## Estrategia de refresco

- `ReplaceDocs()` reemplaza el snapshot documental completo en una sola transaccion.
- `ReplaceWorkspaceDocs()` reemplaza el snapshot documental y `memory_snapshot_json` en una sola transaccion; actualiza `active_docs_generation_id` y `active_memory_generation_id` cuando hay job/generacion.
- `index --docs-only` y `index start --mode docs` ejecutan esa publicacion sin modificar el catalogo de codigo.
- `ReplaceWorkspaceIndex()` publica catalogo, docs y memoria juntos para evitar estados mixtos tras un crash o cancelacion.
- Cambios en docs o en `read-model.toml` fuerzan full re-index.
- `workspace_meta.doc_count` guarda un agregado simple para diagnostico.

## Operaciones de lectura

- `CountDocRecords()` alimenta `workspace status.doc_count` y `docs_index_ready`.
- `FindDocRecordsByMention("doc_id", value)` resuelve documentos agregados que mencionan un RF/FL sin tener ese ID como `doc_records.doc_id` primario.

## No objetivos

- versionado historico de docs
- embeddings o vectores
- store remoto compartido

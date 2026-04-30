# DB-DOC-INDEX

```yaml
harness_protocol: SDD-HARNESS-v1
id: "DB-DOC-INDEX"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[DB-DOC-INDEX]]'
exports:
  - 'DB-DOC-INDEX'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/08_db/DB-DOC-INDEX.md
agent_may_edit:
  - .docs/wiki/08_db/DB-DOC-INDEX.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/08_db/DB-DOC-INDEX.md
```

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
- `source_protocol`
- `block_id`
- `record_id`
- `source_import`
- `source_export`
- `source_audience`

Uso:
- derivar evidencia de codigo para `nav ask`
- futuras ayudas de navegacion y onboarding

### `doc_source_blocks`

Campos canonicos:
- `doc_path`
- `block_id`
- `doc_id`
- `kind`
- `source_format`
- `ordinal`
- `start_line`
- `end_line`
- `content_hash`
- `indexed_at`

Uso:
- lookup exacto de bloques `toon` normativos declarados con `SDD-WIKI-SOURCE-v1`
- soporte para `nav wiki validate-source`
- degradacion compatible via `doc_mentions.block_id`

### `doc_source_records`

Campos canonicos:
- `doc_path`
- `block_id`
- `record_id`
- `record_type`
- `ordinal`
- `start_line`
- `end_line`
- `content_hash`
- `indexed_at`

Uso:
- lookup exacto de records referenciables (`RF-*`, `CT-*`, `TECH-*`, etc.) embebidos en bloques fuente
- soporte para `nav wiki search <record_id>` y `nav wiki trace <record_id>`
- degradacion compatible via `doc_mentions.record_id`

## Estrategia de refresco

- `ReplaceDocs()` reemplaza el snapshot documental completo en una sola transaccion y preserva compatibilidad llamando a la variante sin source rows.
- `ReplaceDocsWithSources()` reemplaza `doc_records`, `doc_edges`, `doc_mentions`, `doc_source_blocks` y `doc_source_records` en una sola transaccion.
- `ReplaceWorkspaceDocs()` reemplaza el snapshot documental, source rows y `memory_snapshot_json` en una sola transaccion; actualiza `active_docs_generation_id` y `active_memory_generation_id` cuando hay job/generacion.
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

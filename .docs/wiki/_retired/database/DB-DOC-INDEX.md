---
wiki_graph_protocol: SDD-WIKI-GRAPH-v1
document_lifecycle: active
doc_id: DB-DOC-INDEX
kind: DB
imports: [RF-GPH-003, 05_modelo_datos]
---

# DB-DOC-INDEX

```yaml
harness_protocol: SDD-HARNESS-v1
id: "DB-DOC-INDEX"
kind: "DB"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-GPH-003]]'
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
- prioridad de links markdown, wikilinks Obsidian (`wikilink`/`embed`), hierarchy estructural y doc IDs antes de heuristicas

### `doc_mentions`

Campos canonicos:
- `doc_path`
- `mention_type`
- `mention_value`
- `source_block` (nullable)

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

La identidad de una mención sigue siendo la clave única `(doc_path, mention_type, mention_value)`; `source_block` es proveniencia opcional del bloque que la declaró y no altera esa identidad. `doc_path` es el `source_document` de la mención en el índice documental.

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

En esta tabla, `doc_path` identifica el `source_document` y `block_id` identifica el `source_block` normativo; ambos forman la proveniencia exacta del bloque y no deben confundirse con la identidad de una mención.

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

Un record conserva la misma proveniencia `(doc_path, block_id)` del bloque que lo contiene. El `record_id` permite resolver el record referenciable; no convierte una fila de `doc_mentions` en una identidad de `GraphUnresolved`.

## Binding y estado de artefactos del puente wiki ↔ código

```toon
doc_id: DB-DOC-INDEX
block_id: DB-DOC-INDEX.wiki-code-binding-state
kind: physical-read-model-contract
source_of_truth: this
status: implemented_slice
doc_artifact_bindings:
  owner_column: doc_path
  identity: [doc_path, block_id, relation, target_path, target_symbol, target_kind, binding_ref]
  provenance: [authoring_origin, binding_status, doc_lifecycle, superseded_by, source_content_hash, start_line, end_line]
  unique: [doc_path_block_id_ordinal, binding_ref]
  indexes:
    - [doc_path, block_id]
    - [target_path, target_symbol]
    - [relation, target_kind]
doc_artifact_states:
  primary_key: path
  fields: [size, mtime_nsec, content_sha256, parser_version, authority_config_hash, indexed_at, last_docs_gen_at, lifecycle]
  lifecycle: [active, deprecated, retired]
  indexes: [lifecycle]
ownership:
  rows_replaced_or_deleted_by: explicit_repo_relative_doc_path
  tables: [doc_binding_sources, doc_binding_source_documents, doc_artifact_bindings, doc_artifact_states, doc_source_records, doc_source_blocks, doc_mentions, doc_edges, doc_records]
generation_domains:
  docs: active_docs_generation_id
  memory: active_memory_generation_id
  catalog: active_catalog_generation_id
  graph: active_graph_generation_id
  graph_rollback: previous_graph_generation_id
publication:
  full_and_docs_only: atomic_docs_sources_bindings_transaction
  incremental_docs: advances_docs_memory_and_changed_artifact_states
  incremental_owner_replacement: explicit_doc_path-preserves-source-block-provenance
  source_provenance:
    source_document: doc_path
    source_block: [doc_source_blocks.block_id, doc_mentions.source_block]
    unavailable: NULL
  catalog_pointer_on_docs_refresh: preserved
  graph_on_docs_refresh: stale_not_published
  query_time_publication: forbidden
deletion:
  accepted_proof: [disk_absent, scope_changed]
  alive_but_excluded: retain_rows_and_emit_omission
  retired_state: preserve_historical_rows
shrink_guard:
  enabled: true
  invariant: unrelated_owner_rows_unchanged
  unexplained_loss: reject_transaction
query:
  database: published_snapshot_read_only
  writes: forbidden
  migrations: forbidden
  repair: forbidden
  publication: forbidden
verify:
  - "FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes)"
  - "wiki-code-bridge-runner/v1: no_query_writes PASS"
evidence:
  - internal/store/schema.go
  - internal/store/doc_artifact_state.go
  - internal/store/queries_incremental.go
  - internal/store/index_publish.go
  - internal/livecontext/types.go
  - .docs/wiki/06_pruebas/TP-GPH.md
```

## Colección fuente independiente del agregado WCB

```toon
doc_id: DB-DOC-INDEX
block_id: DB-DOC-INDEX.canonical-binding-sources
kind: physical-source-collection-contract
source_of_truth: normative
status: authored
verification_status: PARTIAL_scoped
verification_basis: FINAL_VERIFY1_PARTIAL_audit1_Changes_requested_H1_H2_repair_unique_RECHECK1_H1_H2_PASS_scoped_overall_PARTIAL
publication_scope: parent_authorized_H1_H2_scoped_not_full_B_C
incremental_evidence: explicit_incomplete_Git_AM_not_consumed_processed_0_no_changes_no_owner_transition_no_PRODUCT_FAIL
q_evidence: valid_pipeline_same_corpus_DB_one_governance_item_parity_only
q_uncovered: [multi_item_sort, cursor_resume, native_digest, source_variation]
global_readiness: Graph_Harness_fullwiki_conformance_not_accredited
remaining_evidence: explicit_incomplete_coord_milsp_due_2026_10_15
legacy_source_owner_ranges: partial_visible_zero_ranges_not_exact
memory_atomicity_TC_IDX_028: not_accredited_snapshot_docs_catalog_are_not_memory_ID
memory_debt: owner_coord_milsp_due_before_full_acceptance_or_next_memory_sensitive_change
schema: MILSP-DBS/v1
doc_binding_sources:
  owner_column: doc_path
  primary_key: [doc_path, binding_id]
  source_fields: [binding_id, kind, role, path, surface, binding_lifecycle, symbol]
  wrapper_fields: [doc_id, block_id, document_lifecycle, provenance, binding_ref, source_content_hash, start_line, end_line]
  provenance: declared
  resolution_column: absent
  bridge_ref: optional_many_to_one_not_identity
  binding_lifecycle: [active, deprecated]
  document_lifecycle: [active, deprecated, retired]
  unknown_object_ranges: zero_not_exact
  index: [binding_id, doc_path]
doc_binding_source_documents:
  primary_key: doc_path
  fields: [collection_version, document_lifecycle]
  purpose: parsed_empty_availability_and_binding_free_document_lifecycle
  published_version: MILSP-DBS/v1
ownership:
  full: replace_snapshot_preserving_omitted_retired_owners
  excluded_alive: retained_by_docgraph_prior_snapshot_with_warning
  incremental: explicit_owner_replacement_all_families_in_one_transaction
  shrink_guard: both_source_tables_in_unrelated_owner_fingerprint
  retired: preserve_on_delete_reject_reactivation
  changed_editions: replacement_not_event_sourcing
availability:
  states: [available, explicit_incomplete]
  empty_parsed_owner: available_zero_declarations
  old_table_absent: explicit_incomplete_canonical_source_collection_not_persisted
  owner_not_published: explicit_incomplete_source_owner_not_published
  global_ID_partial_corpus: explicit_incomplete_source_owners_not_published
reader:
  function: ReadDocBindingSources
  selectors: doc_path_AND_binding_id_when_both_present
  mandatory_selector: doc_path_OR_binding_id
  limits: default_100_maximum_200_plus_one_truncation_detection
  order: [doc_path, binding_id]
  all_lifecycles: preserved_for_explicit_consumer_selection
  ambiguous_ID: multiple_owners_no_winner
  transaction: read_only
  DDL_backfill_repair: forbidden
transport:
  document: DocRecord.BindingSources_internal_not_public_JSON
  snapshot: same_read_transaction_as_docs_bridge_and_identity_marker
  RAM: WikiCodeOverlay.SourceCollections_internal_owner_replacements
  no_op_detection: includes_complete_source_collection_and_availability
artifact_bindings:
  - {binding_id: DB-DOC-INDEX-source-schema, kind: schema, role: db_schema, path: internal/store/schema.go, symbol: docBindingSourcesDDL, surface: data, binding_lifecycle: active}
verify: [parent_single_FINAL_VERIFY_after_implementation_complete]
evidence: [internal/store/schema.go, internal/store/queries_doc_binding_sources.go, internal/store/queries_docs.go, internal/store/queries_incremental.go, internal/store/doc_snapshot.go]
```

El DDL es evolución aditiva del código del productor; no acredita ni ejecuta migración runtime. Un lector de DB anterior no llama a `EnsureSchema`, no inventa IDs y no rellena desde WCB. La ausencia de colección no afecta la disponibilidad del bridge. `ReadCurrentDocSnapshot` conserva su firma y adjunta la colección/omission en cada `DocRecord`; un marker de identidad vigente no garantiza por sí solo que el owner fuente haya sido publicado. La reutilización de bytes idénticos puede seguir preservando esa omission hasta una operación explícita que vuelva a parsear. No hay un nuevo puntero graph ni una promesa de atomicidad universal docs+Graph.

## Estrategia de refresco

- `ReplaceDocs()` reemplaza el snapshot documental completo en una sola transacción y preserva compatibilidad llamando a la variante sin source rows.
- `ReplaceDocsWithSources()` reemplaza las familias documentales, el bridge y ambas tablas fuente en una sola transacción. Conserva owners retirados omitidos; el productor conserva owners vivos excluidos del scan. Un caller legacy sin `BindingSources` no declara una colección vacía válida.
- `ReplaceWorkspaceDocs()` reemplaza el snapshot documental, source rows, bindings y `memory_snapshot_json` en una sola transacción; actualiza `active_docs_generation_id` y `active_memory_generation_id` cuando hay job/generación.
- `index --docs-only` y `index start --mode docs` ejecutan esa publicación sin modificar el catálogo de código.
- `ReplaceWorkspaceIndex()` publica catálogo, docs y memoria juntos para evitar estados mixtos tras un crash o cancelación.
- La publicación incremental (`applyIncrementalDocChangesTx`) elimina y reinserta solo los owners documentales explícitos; conserva `doc_mentions.source_block`, `doc_source_blocks.block_id` y `doc_source_records.block_id` de cada documento reemplazado, y deja intactos los owners no afectados.
- Cambios en docs o en `read-model.toml` fuerzan full re-index.
- `workspace_meta.doc_count` guarda un agregado simple para diagnóstico.

## Operaciones de lectura

- `CountDocRecords()` alimenta `workspace status.doc_count` y `docs_index_ready`.
- Una mención con un `TP-*` canónico exacto resuelve el documento cuyo `doc_records.doc_id` coincide una sola vez; si falta o es ambiguo, el docgraph conserva un `GraphUnresolved` tipado con `target_kind=document`, `target_value` igual al TP y la proveniencia `source_document`/`source_block` cuando está disponible, sin reutilizar la identidad de la fila `doc_mentions`.

## No objetivos

- versionado temporal de cada edición de docs; preservar declaraciones deprecated/retired no es event-sourcing
- embeddings o vectores
- store remoto compartido

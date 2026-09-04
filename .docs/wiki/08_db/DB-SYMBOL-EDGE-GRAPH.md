---
doc_id: DB-SYMBOL-EDGE-GRAPH
title: Persistencia SQLite del grafo nativo
layer: DB
family: EDGE-GRAPH
status: accepted-design
implements:
  - internal/store/schema.go
  - internal/store/graph.go
  - internal/store/graph_recovery.go
  - internal/indexer/graph_staging.go
tests:
  - internal/store
---

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "DB-SYMBOL-EDGE-GRAPH"
doc_id: "DB-SYMBOL-EDGE-GRAPH"
kind: "db-spec"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[08_modelo_fisico_datos]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-003]]'
  - '[[RF-GPH-004]]'
  - '[[RF-GPH-009]]'
  - '[[RF-GPH-010]]'
  - '[[TECH-GRAPH-NATIVE]]'
exports:
  - 'DB-SYMBOL-EDGE-GRAPH'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/05_modelo_datos.md
  - .docs/wiki/08_modelo_fisico_datos.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-003.md
  - .docs/wiki/04_RF/RF-GPH-004.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
agent_may_edit:
  - .docs/wiki/08_modelo_fisico_datos.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md --format toon
  - go test ./internal/store -run Graph
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - NodeKey-v1-unresolved=true
  - rollback-not-proven=true
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
```

Volver a [08_modelo_fisico_datos.md](../08_modelo_fisico_datos.md).

## Resumen y ownership

Este documento reemplaza la propuesta monolítica `symbol_edges` por el schema graph-native v1 de `index.db`. `08_modelo_fisico_datos.md` conserva ownership y safety stance; aquí vive el mecanismo físico. El slice P0-P2 del schema v1 ya se persiste y publica en el binario vigente; `accepted-design` conserva el contrato aceptado para las extensiones que quedan fuera de ese slice.

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.ownership
kind: database-boundary
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
store: <workspace>/.mi-lsp/index.db
engine: SQLite-WAL
owner: workspace-Graph-Store
write_owners: [indexer-generation-builder, graph-publisher, migration-recovery]
read_owners: [direct-query-engine, daemon-query-engine, context-optimizer, MILX-pack-materializer]
forbidden:
  - query-time-write
  - query-time-migration
  - extension-write
  - external-graph-store-authority
  - full-graph-RAM-copy
legacy:
  files-symbols-docgraph: preserved
  symbol_edges-proposal: superseded-never-migrate-as-authority
```

## Schema version 1

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.schema-v1
kind: physical-schema
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
schema_version: 1
workspace_meta_additions:
  graph_schema_version: INTEGER
  active_graph_generation_id: BLOB-32-nullable
  previous_graph_generation_id: BLOB-32-nullable
tables:
  graph_generations:
    primary_key: generation_id-BLOB32
    fields:
      - schema_version-INTEGER
      - workspace_identity-TEXT
      - source_fingerprint-BLOB32
      - config_fingerprint-BLOB32
      - backend_manifest_digest-BLOB32
      - content_digest-BLOB32
      - status-TEXT-staged-active-retired-invalid
      - node_count-INTEGER
      - edge_count-INTEGER
      - evidence_count-INTEGER
      - unresolved_count-INTEGER
      - previous_generation_id-BLOB32-nullable
      - created_at-TEXT
      - published_at-TEXT-nullable
      - error_code-TEXT-nullable
  graph_nodes:
    primary_key: [generation_id, node_id]
    unique: [generation_id, node_key]
    fields:
      - node_id-INTEGER-deterministic-ordinal
      - node_key-BLOB32
      - identity_schema-TEXT
      - repository_identity-TEXT
      - backend_type-TEXT
      - language-TEXT
      - project_or_module-TEXT
      - owner_path-TEXT
      - symbol_kind-TEXT
      - semantic_identity-TEXT
      - display_name-TEXT
      - source_digest-BLOB32
      - claim_status-TEXT
      - cross_rid-TEXT
      - sort_key-TEXT
  graph_edges:
    primary_key: [generation_id, edge_id]
    unique: [generation_id, edge_key]
    fields:
      - edge_id-INTEGER-deterministic-ordinal
      - edge_key-BLOB32
      - from_node_id-INTEGER
      - to_node_id-INTEGER
      - relation-TEXT
      - claim_scope-TEXT
      - claim_status-TEXT
      - owner_path-TEXT
      - source_backend-TEXT
      - cross_rid-TEXT
  graph_evidence:
    primary_key: [generation_id, evidence_id]
    unique: [generation_id, evidence_key]
    subject: exactly-one-of-node_id-or-edge_id
    fields:
      - evidence_id-INTEGER-deterministic-ordinal
      - evidence_key-BLOB32
      - subject_kind-TEXT-node-or-edge
      - node_id-INTEGER-nullable
      - edge_id-INTEGER-nullable
      - source_uri-TEXT-repo-relative
      - start_line-INTEGER-nullable
      - start_column-INTEGER-nullable
      - end_line-INTEGER-nullable
      - end_column-INTEGER-nullable
      - backend-TEXT
      - extractor_version-TEXT
      - source_digest-BLOB32
      - claim_kind-TEXT
      - observed_claim_digest-BLOB32
      - claim_status-TEXT
      - cross_rid-TEXT
  graph_unresolved:
    primary_key: [generation_id, unresolved_id]
    unique: [generation_id, unresolved_key]
    identity:
      surrogate: unresolved_id-generation-local-deterministic-ordinal
      key: hash-of-[owner_path, subject_kind, selector_digest, reason_code, candidates, backend, source_digest, recovery_hint_code]
      cross_rid: UnresolvedRID(unresolved_key)
      excludes: [source_document, source_block, target_kind, target_value]
    fields:
      - unresolved_id-INTEGER-deterministic-ordinal
      - unresolved_key-BLOB32
      - owner_path-TEXT
      - subject_kind-TEXT
      - selector_digest-BLOB32
      - reason_code-TEXT
      - candidates_json-TEXT-bounded-sanitized
      - backend-TEXT
      - source_digest-BLOB32-nullable
      - cross_rid-TEXT
      - recovery_hint_code-TEXT-nullable
      - source_document-TEXT-nullable-bounded-sanitized-repo-relative
      - source_block-TEXT-nullable-bounded-sanitized
      - target_kind-TEXT-nullable-bounded-sanitized
      - target_value-TEXT-nullable-bounded-sanitized
    diagnostics:
      candidates_max_items: 64
      candidates_max_bytes: 4096
      context_total_max_bytes: 4096
      unavailable: NULL
```
`node_id`, `edge_id`, `evidence_id` y `unresolved_id` son ordinales locales deterministas dentro de una generation. En `graph_unresolved`, `unresolved_key` se deriva de los campos de la reclamación y `cross_rid` de ese key; `source_document`, `source_block`, `target_kind` y `target_value` son contexto diagnóstico y quedan fuera de ambas identidades, pero los cuatro campos se incluyen con framing determinista en `content_digest` y `facts_digest`, y por tanto en los fingerprints `source`/`config`/`backend` y `generation_id`. Las FKs se validan durante staging y antes del pointer swap.

## Migraciones y analysis cache

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.auxiliary-schema
kind: physical-schema
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
tables:
  graph_migrations:
    primary_key: migration_id-TEXT
    fields: [from_version, to_version, status, preflight_digest, backup_digest, prior_active_generation_id, started_at, completed_at, error_code]
    status: [prepared, applying, validated, committed, rolled_back, failed]
  graph_analysis:
    primary_key: analysis_key-BLOB32
    fields: [generation_id, extension_id, extension_version, executable_digest, operation, parameters_digest, authority_profile_digest, output_schema, result_json_bounded, result_digest, provenance_json_sanitized, omissions_json_sanitized, status, created_at]
    authority: none-derived-cache-only
constraints:
  blob-identities: CHECK-length-equals-32
  generation-status: CHECK-enum
  edge-endpoints: composite-FK-to-graph_nodes-same-generation
  evidence-subject: CHECK-exactly-one-node-or-edge
  nonnegative-counts: CHECK
  paths: application-normalized-and-validated-before-insert
  JSON: bounded-and-schema-validated-before-insert
indexes:
  - unique-active-generation-partial-index
  - graph_nodes-generation-owner_path
  - graph_nodes-generation-kind-sort_key
  - graph_nodes-generation-cross_rid
  - graph_edges-generation-from-relation-to
  - graph_edges-generation-to-relation-from
  - graph_edges-generation-owner_path
  - graph_evidence-generation-edge
  - graph_evidence-generation-node
  - graph_unresolved-generation-owner-reason
  - graph_analysis-generation-extension-operation
```


## Staging, publish y recovery

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.publication
kind: transaction-contract
source_of_truth: RF-GPH-002
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
staging:
  transaction: write-generation-records
  visibility: excluded-from-default-readers
  copy_forward: accepted-design-SQL-insert-select-for-unchanged-owners
  owner_replacement: accepted-design-delete-and-insert-inside-staged-generation
  owner_replacement_preserves: [source_document, source_block, target_kind, target_value]
  graph_unresolved_context:
    fields: [source_document, source_block, target_kind, target_value]
    persistence: carried-through-staging-validation-publish
    policy: bounded-sanitized; unavailable-null
  incremental_graph_refresh_path: complete-bundle-reassembly-before-stage
  incremental_docs_only_path: publish-docs-and-mark-graph-stale-no-graph-stage
validation:
  - generation-content-digest-and-counts
  - NodeKey-uniqueness-and-collision-payload-compare
  - edge-endpoints-and-relation-kind
  - evidence-subject-digest-provenance
  - unresolved-key-cross-rid-id-exclude-diagnostic-context
  - unresolved-diagnostic-context-bounded-sanitized
  - cross-RID-format-and-conflicts
  - source-config-backend-fingerprints
  active_prior_replacement:
    current: strict_generation_validation
    legacy:
      schema_version: 1
      pointer_equals_non_nil_expected_prior: true
      active_row_count: 1
      structural_checks: [rows, counts, FKs, enums, JSON, edge-evidence]
      digest_framing: pre-provenance-unresolved-fields-only
      action: retire_prior_and_activate_new_atomically
    unknown_digest_or_corruption: blocked
publish:
  transaction: BEGIN-IMMEDIATE
  compare_and_swap: expected-prior-active-pointer
  actions:
    - prior-active-to-retired
    - staged-to-active
    - graph-unresolved-context-columns-committed-with-generation
    - workspace-meta-active-pointer-to-new
    - workspace-meta-previous-pointer-to-prior
  commit_before_success_response: true
recovery:
  inspect: [nonterminal-migrations, staged-generations, pointer-target-seal, dead-owner]
  incomplete-staging: mark-invalid-and-clean-only-that-generation
  invalid-pointer: restore-previous-valid-or-block-graph-queries
  choose-by-latest-timestamp: forbidden
La ruta `full` fenced (`ReplaceWorkspaceIndexForJob`) entrega el bundle graph-native al staging y mantiene sus columnas de contexto dentro de la misma transacción que catálogo, documentos, memoria y pointer graph. La ruta `full` foreground publica el grafo con `PublishGraphObservationBatches` antes de `ReplaceWorkspaceIndex`; por eso la activación del grafo y la transacción posterior de catálogo, documentos y memoria son separadas. En el refresco gráfico incremental y en los cambios de código, la ruta vigente (`PublishIncrementalGenerationWithChanges` y su variante fenced) reensambla un bundle completo antes de `StageGraphGenerationTx`; no usa un writer alternativo que pueda perder el contexto. La ruta incremental docs-only publica los cambios documentales, marca el grafo stale y no hace staging gráfico. `ValidateGraphGeneration` rehidrata esas columnas, vuelve a validar el registro tipado y solo después permite el compare-and-swap del pointer.

La regla de copy-forward/reemplazo por owner del diseño aceptado conserva el mismo payload de diagnóstico: si una fila se copia o se reinsertan filas del owner dentro de staging, `source_document`, `source_block`, `target_kind` y `target_value` viajan con ella, sin entrar en `unresolved_key`, `Cross-RID` ni en el ordinal local. La consulta de omisiones limita la salida a 50 filas, sanea rutas/texto y omite o devuelve `NULL` cuando el valor no está disponible.

Para el docgraph, una mención con un TP canónico exacto resuelve un único nodo `GraphNode(kind=document)`; si falta o hay más de un destino, permanece como `GraphUnresolved` tipado con `target_kind=document`, `target_value` y, cuando existen, `source_document`/`source_block`.

Para la observación Go, una topología `single` o repo seleccionada puede declarar en `DefaultEntrypoint` un selector de módulo repo-local explícito (`go.mod`) o un ID de `WorkspaceEntrypoint`. Un ID se resuelve por coincidencia exacta de repo y entrypoint, se rebasa desde su ruta workspace-relative al root del repo seleccionado y se revalida como un `go.mod` repo-local, seguro y regular. Un selector explícito inseguro (absoluto, con `\`, `..`, basename inválido, symlink, directorio, archivo inexistente u otra entrada no regular), o un ID desconocido o malformado, falla cerrado y omite Go sin activar el fallback. El fallback a `go.mod` en la raíz del repo solo aplica cuando el selector está vacío; un root solo con `go.work` no selecciona módulo ni se envía a `ObserveGoGraph`. `go.work` se rechaza como selector de grafo. En topología `container` se consulta la raíz del workspace con ese fallback; no se elige un módulo por timestamp ni se acepta una ruta absoluta o fuera del root.
## Migracion v0 a v1

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.migration-v1
kind: migration-plan
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
from: graph-schema-absent
steps:
  - preflight-SQLite-version-and-existing-schema
  - add-graph-tables-indexes-and-workspace-meta-keys-transactionally
  - keep-files-symbols-docgraph-and-index-generation-contracts
  - enable-dual-read-write-only-behind-explicit-compatibility-state
  - build-v1-generation-from-source-not-from-untrusted-symbol_edges
  - validate-v1-generation-and-clean-equivalence
  - atomically-activate-v1
  - retain-prior-catalog-doc-generations-and-rollback-metadata
rollback:
  - stop-new-writes
  - restore-prior-active-graph-pointer-or-null
  - continue-legacy-query-contract-with-visible-backend
  - retain-v1-tables-as-inactive-until-gated-cleanup
forbidden:
  - destructive-in-place-conversion
  - migrate-during-query
  - treat-old-symbol_edges-proposal-as-authoritative-source
exit_compatibility_window_requires: [TP-GPH-002-PASS, release-gate-PASS, rollback-rehearsal-PASS]
```

## Access patterns

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.access
kind: access-patterns
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
queries:
  active-generation: workspace_meta-single-key-read
  node-by-key-or-cross-RID: unique-index-lookup
  scoped-name-selector: generation-kind-sort-key-index-with-bounded-limit
  outbound-frontier: generation-from-relation-to-index
  inbound-frontier: generation-to-relation-from-index
  owner-invalidation: generation-owner-path-index
  evidence-for-edge-or-node: subject-index
  unresolved-stats: generation-owner-reason-index
  unresolved-diagnostics: generation-owner-reason-index; max-rows-50; typed-sanitized-context
rules:
  reader-transaction: fixes-one-generation
  frontier: bounded-page-at-a-time
  recursive-CTE: allowed-only-with-hard-depth-row-limits-and-plan-tests
  full-table-graph-load: forbidden
  write-through-query-connection: query_only-ON
```

## Retencion y limpieza

```toon
doc_id: DB-SYMBOL-EDGE-GRAPH
block_id: DB-SYMBOL-EDGE-GRAPH.retention
kind: lifecycle
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_pruebas/TP-GPH.md
retain:
  - active-generation
  - previous-valid-generation-until-release-rollback-window-closes
  - migration-metadata-required-for-rollback
  - sanitized-failure-reason
cleanup:
  invalid-or-abandoned-staging: after-recovery-and-no-reader
  older-retired-generations: bounded-policy-after-backup-and-release-gate
  graph-analysis: LRU-or-generation-invalidated
  evidence-and-unresolved: same-lifecycle-as-owning-generation
never-delete:
  - only-valid-active-generation
  - only-proven-rollback-generation-during-window
VACUUM: maintenance-only-not-query-path
```

## Sync

Semantic owner: `[[05_modelo_datos]]`. RF: `[[RF-GPH-001]]` a `[[RF-GPH-004]]`, `[[RF-GPH-009]]`, `[[RF-GPH-010]]`. Runtime: `[[TECH-GRAPH-NATIVE]]`. Contracts: `[[CT-GRAPH-CLI]]`, `[[CT-MILX-V1]]`. Tests: `[[TP-GPH]]`.

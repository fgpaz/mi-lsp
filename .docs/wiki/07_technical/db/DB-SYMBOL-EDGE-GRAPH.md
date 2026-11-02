---
wiki_contract: WIKI-CONTRACT-v1
id: DB-SYMBOL-EDGE-GRAPH
kind: database
status: active
title: DB-SYMBOL-EDGE-GRAPH
links:
  materializes:
  - data-model
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: DB-SYMBOL-EDGE-GRAPH
kind: data-model-contract
audience: dual
imports: [.docs/wiki/07_technical/db/DB-PHYSICAL-MODEL.md, .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md, .docs/wiki/07_technical/contracts/CT-GRAPH-CLI.md]
exports: [DB-SYMBOL-EDGE-GRAPH]
agent_must_read: [.docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md, internal/store/schema.go, internal/store/graph.go]
agent_may_edit: [.docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md]
agent_must_not_edit: [internal/store/, .mi-lsp/index.db, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md y .docs/wiki/07_technical/db/DB-PHYSICAL-MODEL.md; cotejar las afirmaciones con la fuente publicada internal/store/schema.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Una query escriba/migre, extensión MILX obtenga write, se copie el grafo entero a RAM o se trate symbol_edges legacy como autoridad.]
evidence: [.docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md, .docs/wiki/07_technical/db/DB-PHYSICAL-MODEL.md, internal/store/schema.go]
```

## Almacén

Este documento reemplaza la propuesta monolítica `symbol_edges` por el schema graph-native v1 de `index.db`. `07_technical/db/DB-PHYSICAL-MODEL.md` conserva ownership y safety stance; aquí vive el mecanismo físico. El slice P0-P2 del schema v1 ya se persiste y publica en el binario vigente; `accepted-design` conserva el contrato aceptado para las extensiones que quedan fuera de ese slice.

- **Documento propietario**: DB-SYMBOL-EDGE-GRAPH
- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.ownership
- **Clase**: database-boundary
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **store**: <workspace>/.mi-lsp/index.db
- **engine**: SQLite-WAL
- **owner**: workspace-Graph-Store
- **write_owners**:
  - indexer-generation-builder
  - graph-publisher
  - migration-recovery
- **read_owners**:
  - direct-query-engine
  - daemon-query-engine
  - context-optimizer
  - MILX-pack-materializer
- **forbidden**:
  - query-time-write
  - query-time-migration
  - extension-write
  - external-graph-store-authority
  - full-graph-RAM-copy
- **legacy**:
  - **files-symbols-docgraph**: preserved
  - **symbol_edges-proposal**: superseded-never-migrate-as-authority

### Resumen y ownership

Este documento reemplaza la propuesta monolítica `symbol_edges` por el schema graph-native v1 de `index.db`. `07_technical/db/DB-PHYSICAL-MODEL.md` conserva ownership y safety stance; aquí vive el mecanismo físico. El slice P0-P2 del schema v1 ya se persiste y publica en el binario vigente; `accepted-design` conserva el contrato aceptado para las extensiones que quedan fuera de ese slice.

- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.ownership
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **store**: <workspace>/.mi-lsp/index.db
- **engine**: SQLite-WAL
- **owner**: workspace-Graph-Store
- **write_owners**:
  - indexer-generation-builder
  - graph-publisher
  - migration-recovery
- **read_owners**:
  - direct-query-engine
  - daemon-query-engine
  - context-optimizer
  - MILX-pack-materializer
- **forbidden**:
  - query-time-write
  - query-time-migration
  - extension-write
  - external-graph-store-authority
  - full-graph-RAM-copy
- **legacy**:
  - **files-symbols-docgraph**: preserved
  - **symbol_edges-proposal**: superseded-never-migrate-as-authority

## Esquema o convención

Los campos, claves, índices y convenciones físicas se conservan en el detalle siguiente.

Volver a [07_technical/db/DB-PHYSICAL-MODEL.md](DB-PHYSICAL-MODEL.md).

### Esquema versión 1
- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.schema-v1
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **schema_version**: 1
- **workspace_meta_additions**:
  - **graph_schema_version**: INTEGER
  - **active_graph_generation_id**: BLOB-32-nullable
  - **previous_graph_generation_id**: BLOB-32-nullable
- **tables**:
  - **graph_generations**:
    - **primary_key**: generation_id-BLOB32
    - **fields**:
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
  - **graph_nodes**:
    - **primary_key**:
      - generation_id
      - node_id
    - **unique**:
      - generation_id
      - node_key
    - **fields**:
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
  - **graph_edges**:
    - **primary_key**:
      - generation_id
      - edge_id
    - **unique**:
      - generation_id
      - edge_key
    - **fields**:
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
  - **graph_evidence**:
    - **primary_key**:
      - generation_id
      - evidence_id
    - **unique**:
      - generation_id
      - evidence_key
    - **subject**: exactly-one-of-node_id-or-edge_id
    - **fields**:
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
  - **graph_unresolved**:
    - **primary_key**:
      - generation_id
      - unresolved_id
    - **unique**:
      - generation_id
      - unresolved_key
    - **identity**:
      - **surrogate**: unresolved_id-generation-local-deterministic-ordinal
      - **key**: hash-of-[owner_path, subject_kind, selector_digest, reason_code, candidates, backend, source_digest, recovery_hint_code]
      - **cross_rid**: UnresolvedRID(unresolved_key)
      - **excludes**:
        - source_document
        - source_block
        - target_kind
        - target_value
    - **fields**:
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
    - **diagnostics**:
      - **candidates_max_items**: 64
      - **candidates_max_bytes**: 4096
      - **context_total_max_bytes**: 4096
      - **unavailable**: null
`node_id`, `edge_id`, `evidence_id` y `unresolved_id` son ordinales locales deterministas dentro de una generation. En `graph_unresolved`, `unresolved_key` se deriva de los campos de la reclamación y `cross_rid` de ese key; `source_document`, `source_block`, `target_kind` y `target_value` son contexto diagnóstico y quedan fuera de ambas identidades, pero los cuatro campos se incluyen con framing determinista en `content_digest` y `facts_digest`, y por tanto en los fingerprints `source`/`config`/`backend` y `generation_id`. Las FKs se validan durante staging y antes del pointer swap.

### Access patterns

- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.access
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **queries**:
  - **active-generation**: workspace_meta-single-key-read
  - **node-by-key-or-cross-RID**: unique-index-lookup
  - **scoped-name-selector**: generation-kind-sort-key-index-with-bounded-limit
  - **outbound-frontier**: generation-from-relation-to-index
  - **inbound-frontier**: generation-to-relation-from-index
  - **owner-invalidation**: generation-owner-path-index
  - **evidence-for-edge-or-node**: subject-index
  - **unresolved-stats**: generation-owner-reason-index
  - **unresolved-diagnostics**: generation-owner-reason-index; max-rows-50; typed-sanitized-context
- **rules**:
  - **reader-transaction**: fixes-one-generation
  - **frontier**: bounded-page-at-a-time
  - **recursive-CTE**: allowed-only-with-hard-depth-row-limits-and-plan-tests
  - **full-table-graph-load**: forbidden
  - **write-through-query-connection**: query_only-ON

### Sync

Semantic owner: `[[data-model]]`. RF: `[[RF-GPH-001]]` a `[[RF-GPH-004]]`, `[[RF-GPH-009]]`, `[[RF-GPH-010]]`. Runtime: `[[TECH-GRAPH-NATIVE]]`. Contracts: `[[CT-GRAPH-CLI]]`, `[[CT-MILX-V1]]`. Tests: `[[TP-GPH]]`.

### Admisión Kotlin aditiva

- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.kotlin-admission-v1
- **owner**: coord-milsp
- **bindings**:
  - internal/model/graph.go
  - internal/model/graph_observation.go
  - internal/indexer/extractor_kotlin.go
- **Estado del comportamiento**: experimental_unverified_explicit_incomplete
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-GPH.md::TP-GPH.kotlin-compiler-export-v1
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md

[[ADR-KOTLIN-GRAPH-ADMISSION-V1]] registra exactamente `kotlin-language-server` / `kotlin` / `kotlin/binding-context` para declarations nombradas y references compiler locales main-jvm. Decisión delegada coord-milsp, no promoción ni aprobación humana/global inferida. NodeKey-v1/framing/tablas/ReadyForStaging intactos; sin DDL/querybackfill/migración en query. Lectores viejos rechazan el enum nuevo; mínimo lector con SHA/versión de primera release todavía pendiente. Rollback conserva prior generation y binario compatible. Raw partial/experimental actual no publica; autoridad Gradle y conformance pendientes coord-milsp, 2026-10-17.

## Riesgos de migración

Se conservan las reglas específicas de publicación, retención, rollback y compatibilidad. No se migra ningún almacén durante esta migración documental.

### Migraciones y analysis cache

- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.auxiliary-schema
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **tables**:
  - **graph_migrations**:
    - **primary_key**: migration_id-TEXT
    - **fields**:
      - from_version
      - to_version
      - status
      - preflight_digest
      - backup_digest
      - prior_active_generation_id
      - started_at
      - completed_at
      - error_code
    - **Estado del comportamiento**:
      - prepared
      - applying
      - validated
      - committed
      - rolled_back
      - failed
  - **graph_analysis**:
    - **primary_key**: analysis_key-BLOB32
    - **fields**:
      - generation_id
      - extension_id
      - extension_version
      - executable_digest
      - operation
      - parameters_digest
      - authority_profile_digest
      - output_schema
      - result_json_bounded
      - result_digest
      - provenance_json_sanitized
      - omissions_json_sanitized
      - status
      - created_at
    - **authority**: none-derived-cache-only
- **constraints**:
  - **blob-identities**: CHECK-length-equals-32
  - **generation-status**: CHECK-enum
  - **edge-endpoints**: composite-FK-to-graph_nodes-same-generation
  - **evidence-subject**: CHECK-exactly-one-node-or-edge
  - **nonnegative-counts**: CHECK
  - **paths**: application-normalized-and-validated-before-insert
  - **JSON**: bounded-and-schema-validated-before-insert
- **indexes**:
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

### Staging, publish y recovery

- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.publication
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **staging**:
  - **transaction**: write-generation-records
  - **visibility**: excluded-from-default-readers
  - **copy_forward**: accepted-design-SQL-insert-select-for-unchanged-owners
  - **owner_replacement**: accepted-design-delete-and-insert-inside-staged-generation
  - **owner_replacement_preserves**: [source_document, source_block, target_kind, target_value]
  - **graph_unresolved_context**:
    - **fields**: [source_document, source_block, target_kind, target_value]
    - **persistence**: carried-through-staging-validation-publish
    - **policy**: bounded-sanitized; unavailable-null
  - **incremental_graph_refresh_path**: complete-bundle-reassembly-before-stage
  - **incremental_docs_only_path**: publish-docs-and-mark-graph-stale-no-graph-stage
- **validation**:
  - generation-content-digest-and-counts
  - NodeKey-uniqueness-and-collision-payload-compare
  - edge-endpoints-and-relation-kind
  - evidence-subject-digest-provenance
  - unresolved-key-cross-rid-id-exclude-diagnostic-context
  - unresolved-diagnostic-context-bounded-sanitized
  - cross-RID-format-and-conflicts
  - source-config-backend-fingerprints
  - **active_prior_replacement**:
    - **current**: strict_generation_validation
    - **legacy**:
      - **schema_version**: 1
      - **pointer_equals_non_nil_expected_prior**: true
      - **active_row_count**: 1
      - **structural_checks**: [rows, counts, FKs, enums, JSON, edge-evidence]
      - **digest_framing**: pre-provenance-unresolved-fields-only
      - **action**: retire_prior_and_activate_new_atomically
    - **unknown_digest_or_corruption**: blocked
- **publish**:
  - **transaction**: BEGIN-IMMEDIATE
  - **compare_and_swap**: expected-prior-active-pointer
  - **actions**:
    - prior-active-to-retired
    - staged-to-active
    - graph-unresolved-context-columns-committed-with-generation
    - workspace-meta-active-pointer-to-new
    - workspace-meta-previous-pointer-to-prior
  - **commit_before_success_response**: true
- **recovery**:
  - **inspect**: [nonterminal-migrations, staged-generations, pointer-target-seal, dead-owner]
  - **incomplete-staging**: mark-invalid-and-clean-only-that-generation
  - **invalid-pointer**: restore-previous-valid-or-block-graph-queries
  - **choose-by-latest-timestamp**: forbidden
- La ruta `full` fenced (`ReplaceWorkspaceIndexForJob`) entrega el bundle graph-native al staging y mantiene sus columnas de contexto dentro de la misma transacción que catálogo, documentos, memoria y pointer graph. La ruta `full` foreground publica el grafo con `PublishGraphObservationBatches` antes de `ReplaceWorkspaceIndex`; por eso la activación del grafo y la transacción posterior de catálogo, documentos y memoria son separadas. En el refresco gráfico incremental y en los cambios de código, la ruta vigente (`PublishIncrementalGenerationWithChanges` y su variante fenced) reensambla un bundle completo antes de `StageGraphGenerationTx`; no usa un writer alternativo que pueda perder el contexto. La ruta incremental docs-only publica los cambios documentales, marca el grafo stale y no hace staging gráfico. `ValidateGraphGeneration` rehidrata esas columnas, vuelve a validar el registro tipado y solo después permite el compare-and-swap del pointer.
- La regla de copy-forward/reemplazo por owner del diseño aceptado conserva el mismo payload de diagnóstico: si una fila se copia o se reinsertan filas del owner dentro de staging, `source_document`, `source_block`, `target_kind` y `target_value` viajan con ella, sin entrar en `unresolved_key`, `Cross-RID` ni en el ordinal local. La consulta de omisiones limita la salida a 50 filas, sanea rutas/texto y omite o devuelve `NULL` cuando el valor no está disponible.
- Para el docgraph, una mención con un TP canónico exacto resuelve un único nodo `GraphNode(kind=document)`; si falta o hay más de un destino, permanece como `GraphUnresolved` tipado con `target_kind=document`, `target_value` y, cuando existen, `source_document`/`source_block`.
- Para la observación Go, una topología `single` o repo seleccionada puede declarar en `DefaultEntrypoint` un selector de módulo repo-local explícito (`go.mod`) o un ID de `WorkspaceEntrypoint`. Un ID se resuelve por coincidencia exacta de repo y entrypoint, se rebasa desde su ruta workspace-relative al root del repo seleccionado y se revalida como un `go.mod` repo-local, seguro y regular. Un selector explícito inseguro (absoluto, con `\`, `..`, basename inválido, symlink, directorio, archivo inexistente u otra entrada no regular), o un ID desconocido o malformado, falla cerrado y omite Go sin activar el fallback. El fallback a `go.mod` en la raíz del repo solo aplica cuando el selector está vacío; un root solo con `go.work` no selecciona módulo ni se envía a `ObserveGoGraph`. `go.work` se rechaza como selector de grafo. En topología `container` se consulta la raíz del workspace con ese fallback; no se elige un módulo por timestamp ni se acepta una ruta absoluta o fuera del root.
## Migración v0 a v1

- **Marcador estable**: `DB-SYMBOL-EDGE-GRAPH.migration-v1`
- **Comprobación prevista**: `mi-lsp nav wiki validate-source --workspace mi-lsp --format toon`
- **Evidencia referenciada**: `.docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md`; `.docs/wiki/06_tests/TP-GPH.md`
- **Origen**: `graph-schema-absent`
- **Pasos**:
  1. Ejecutar preflight de versión SQLite y esquema existente.
  2. Agregar tablas, índices y claves de workspace-meta del grafo transaccionalmente.
  3. Conservar los contratos de archivos, símbolos, docgraph y generaciones de índice.
  4. Habilitar dual-read/write únicamente detrás de un estado de compatibilidad explícito.
  5. Construir la generación v1 desde la fuente, no desde `symbol_edges` no confiable.
  6. Validar la generación v1 y la equivalencia limpia.
  7. Activar v1 atómicamente.
  8. Retener generaciones previas de catálogo/documentos y metadata de rollback.
- **Rollback**:
  1. Detener nuevas escrituras.
  2. Restaurar el puntero de grafo activo previo o `null`.
  3. Continuar con el contrato de consulta legacy y backend visible.
  4. Retener las tablas v1 como inactivas hasta la limpieza autorizada.
- **Prohibido**:
  - conversión destructiva in-place;
  - migrar durante una consulta;
  - tratar la propuesta antigua de `symbol_edges` como fuente autoritativa.
- **La ventana de compatibilidad solo termina cuando**: `TP-GPH-002-PASS`, `release-gate-PASS` y `rollback-rehearsal-PASS`.

### Retención y limpieza

- **Marcador estable**: DB-SYMBOL-EDGE-GRAPH.retention
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/db/DB-SYMBOL-EDGE-GRAPH.md
  - .docs/wiki/06_tests/TP-GPH.md
- **retain**:
  - active-generation
  - previous-valid-generation-until-release-rollback-window-closes
  - migration-metadata-required-for-rollback
  - sanitized-failure-reason
- **cleanup**:
  - **invalid-or-abandoned-staging**: after-recovery-and-no-reader
  - **older-retired-generations**: bounded-policy-after-backup-and-release-gate
  - **graph-analysis**: LRU-or-generation-invalidated
  - **evidence-and-unresolved**: same-lifecycle-as-owning-generation
- **never-delete**:
  - only-valid-active-generation
  - only-proven-rollback-generation-during-window
- **VACUUM**: maintenance-only-not-query-path

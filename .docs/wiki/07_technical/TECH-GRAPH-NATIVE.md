---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-GRAPH-NATIVE
kind: technical
status: active
title: TECH-GRAPH-NATIVE
links:
  supports:
  - RF-GPH-001
  - RF-GPH-002
  - RF-GPH-003
  - RF-GPH-004
  - RF-GPH-005
  - RF-GPH-006
  - RF-GPH-007
  - RF-GPH-008
  - RF-GPH-009
  - RF-GPH-010
  - RF-GPH-011
  - RF-WKS-007
  - RF-ROADMAP-001
  code:
  - path: internal/cli/git_frontier.go
    role: implementation
  - path: internal/docgraph/docgraph.go
    role: implementation
  - path: internal/indexer/extractor_go.go
    role: implementation
  - path: internal/indexer/extractor_kotlin.go
    role: implementation
  - path: internal/indexer/graph_pipeline.go
    role: implementation
  - path: internal/indexer/graph_staging.go
    role: implementation
  - path: internal/indexer/indexer.go
    role: implementation
  - path: internal/livecontext/manifest.go
    role: implementation
  - path: internal/livecontext/overlay.go
    role: implementation
  - path: internal/milx/host.go
    role: implementation
  - path: internal/milx/pack.go
    role: implementation
  - path: internal/model/graph.go
    role: implementation
  - path: internal/model/graph_observation.go
    role: implementation
  - path: internal/service/app.go
    role: implementation
  - path: internal/service/graph_impact.go
    role: implementation
  - path: internal/service/graph_observer.go
    role: implementation
  - path: internal/service/graph_query.go
    role: implementation
  - path: internal/service/wiki_code_context.go
    role: implementation
  - path: internal/service/trace.go
    role: implementation
  - path: internal/store/doc_artifact_state.go
    role: implementation
  - path: internal/store/graph.go
    role: implementation
  - path: internal/store/graph_impact.go
    role: implementation
  - path: internal/store/graph_query.go
    role: implementation
  - path: internal/store/graph_recovery.go
    role: implementation
  - path: internal/store/schema.go
    role: implementation
  - path: internal/workspace/registry.go
    role: implementation
  - path: internal/workspace/repository_identity.go
    role: implementation
  - path: internal/docgraph/governance.go
    role: implementation
---

## Resumen

Este documento expande la arquitectura objetivo del grafo nativo. El root `07_technical.md` conserva ownership y decisiones transversales; este detalle fija componentes, flujo de datos, dependencias y fallos. `accepted-design` no afirma implementacion: cada componente pasa a stable solo con su TP y Victory gate.

### Resumen y boundary

Este documento expande la arquitectura objetivo del grafo nativo. El root `07_technical.md` conserva ownership y decisiones transversales; este detalle fija componentes, flujo de datos, dependencias y fallos. `accepted-design` no afirma implementacion: cada componente pasa a stable solo con su TP y Victory gate.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Grafo único wiki + código shipped

Desde v0.14.0, `nav wiki graph` consulta una generación del grafo único: no mantiene un grafo paralelo ni convierte Markdown/Mermaid en evidencia de compilador. Las relaciones autoradas de WIKI-CONTRACT-v1 son `fulfills`, `implements`, `verifies`, `affects`, `supports`, `materializes`, `supersedes`, `experiences`, `stages`, `maps`, `grounds_in` y `code`. Las inversas se derivan de las declaraciones; nunca se escriben en `links`. Los enlaces legacy `doc_*` siguen siendo compatibilidad de entrada, no sustituyen los links tipados del header.

La procedencia distingue `declared|observed|derived|candidate`: declarar un target no lo resuelve ni lo transforma en observación estructural. Los targets ausentes, ambiguos o no resueltos conservan candidatos/omisiones explícitos. `generation_id`, `doc_generation_id`, `code_generation_id` y `graph_freshness` distinguen los snapshots y su frescura; un catálogo documental actualizado no acredita una generación de código current.

Los modos `neighbors|path|explain|affected|change-pack|matrix` reutilizan las consultas de vecinos, caminos, explicación, impacto, paquete de cambio y matriz. El envelope `wiki-graph/v1` conserva nodos, aristas, unresolved y omisiones. `render=mermaid` produce solo una proyección de la respuesta; para el grafo ordinario exige frescura current y aristas resueltas, o informa que Mermaid no está disponible. No se persiste como canon ni se usa para completar relaciones faltantes.

### Roadmap local derivado

Desde v0.14.1, `.docs/roadmap/roadmap.md` es una fuente local opcional fuera del canon, con `canonical=false`. Su tabla tiene `id`, `rs`, `horizon`, `status` y opcionalmente `title`, `depends_on` (IDs separados por comas). Horizontes `short|medium|long`; estados `todo|doing|done|blocked`. `nav wiki graph --mode roadmap --render mermaid` deriva nodos por horizonte, dependencias y enlaces `roadmap_for` solo hacia RS existentes. El Mermaid es efímero, nunca Gantt ni archivo persistido.

Los avisos `roadmap_*` son no bloqueantes; ausencia del archivo da proyección vacía. Sin generación disponible se informa degradación, sin fabricar RS ni evidencia current. Roadmap no cambia active/readiness, trazabilidad ni cierre. Los contratos detallados siguen en TECH-ROADMAP-001 y CT-ROADMAP-001; esta reconciliación no reimplementa ese runtime ni embeddings v0.14.3/v0.14.4.

### Fuentes kernel_v2 como candidatos documentales

Desde 8ccc06d, cuando el caller declara `ae_canon.mode=kernel_v2` y el canon externo es válido, sus cinco módulos validados entran como candidatos docs con path lógico `ae_kernel_v2/canon/<módulo>`, family generic y origen físico protegido bajo `<kernel_home>/canon`. Se conservan las guardas de archivo regular y resolución segura. No se copian módulos a esta wiki, no se promueven candidatos a evidencia compiler-first y no se recupera autoridad de la historia `.docs/wiki/ae/`. Si el canon externo no valida o no es accesible, se omiten esos candidatos sin inventar fuentes.

### Componentes y ownership

- **Marcador estable**: TECH-GRAPH-NATIVE.components
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
- **Estado del comportamiento**: accepted-design
- **components**:
  - **graph_model**:
    - **owner**: Graph-Kernel
    - **planned_paths**:
      - internal/model/graph_kernel.go
    - **owns**:
      - NodeKey-v1
      - GraphGeneration-records
      - typed-status-reasons
      - cross-RID
  - **adapters**:
    - **owner**: Semantic-backends
    - **planned_paths**:
      - internal/indexer/graph_adapters.go
    - **Entradas**:
      - Roslyn
      - Go-parser-types-gopls
      - tsserver-gated
      - Pyright-gated
    - **output**: GraphObservationBatch-only
  - **go_extractor**:
    - **owner**: Semantic-backends
    - **path**: internal/indexer/extractor_go.go
    - **type_check**: shared_importer_and_package_cache_during_typeCheckAll
    - **identity_rule**: imported_packages_share_types_identity
    - **local_unrepresented**: omission_unsupported_symbol_kind
    - **partial_rule**: never_promote_partial_to_complete
    - **staging_gate**: ReadyForStaging_remains_strict
  - **generation_builder_validator**:
    - **owner**: Graph-Kernel
    - **planned_paths**:
      - internal/indexer/graph_staging.go
    - **owns**:
      - owner-path-incrementality
      - staging
      - validation
  - **graph_store_publisher**:
    - **owner**: Store
    - **planned_paths**:
      - internal/store/graph_schema.go
      - internal/store/graph_persistence.go
    - **authority**: repo-local-SQLite
  - **query_engine**:
    - **owner**: Service
    - **planned_paths**:
      - internal/graph
      - internal/service/graph_query.go
    - **properties**:
      - read-only
      - storage-backed
      - bounded
      - direct-daemon-parity
  - **wiki_authority_bridge**:
    - **owner**: Docgraph-and-service
    - **rule**: wiki-authority-before-code-evidence
  - **knowledge_wiki_docs**:
    - **owner**: Docgraph-and-indexer
    - **canonical_paths**:
      - wiki/
      - bibliotecas/
      - .docs/wiki/
      - docs/
      - README*.md
    - **docs_only_publish**: documentation-graph-without-compiler-batches
    - **doc_edge_kinds**:
      - wikilink
      - embed
      - hierarchy
      - markdown_link
    - **map_command**: nav.wiki.map
  - **federation**:
    - **owner**: Core-with-optional-daemon-cache
    - **authority**: derived-view-only
  - **context_optimizer**:
    - **owner**: Core
    - **output**: ContextPack
  - **milx_host**:
    - **owner**: Extensions-runtime
    - **isolation**: separate-process
    - **contract**: MILX-v1
  - **victory_lab**:
    - **owner**: Quality-gate
    - **role**: deterministic-comparison-not-runtime

### Pipeline y publicación

- **Marcador estable**: TECH-GRAPH-NATIVE.pipeline
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
- **sequence**:
  - resolve-one-repository-identity-explicit-then-local-vcs-origin-fail-closed
  - compute-source-and-config-fingerprints
  - container-go-observe-once-from-workspace-root-go-mod-fallback; selected-repo-go-observe-from-safe-explicit-or-root-entrypoint
  - container-csharp-fanout-over-declared-csproj-entrypoints-only
  - collect-GraphObservationBatch-per-owner
  - derive-NodeKeys-edge-keys-evidence-and-unresolved
  - type-check-Go-packages-with-shared-importer-and-package-cache
  - build-staged-generation-by-copy-forward-plus-owner-replacement
  - validate-collisions-endpoints-digests-cross-RIDs-and-coverage
  - persist-and-seal-staging-in-SQLite
  - atomically-swap-active-graph-generation-pointer
  - serve-read-only-bounded-queries
- **publication**:
  - **visible_states**:
    - active
    - explicitly-selected-retired
  - **invisible_states**:
    - staged
    - invalid
  - **transitions**:
    - staged_to_active
    - active_to_retired
    - staged_to_invalid
    - retired_to_active
  - **reactivation_rule**: exact_immutable_canonical_only_preserving_predecessor_via_cas
  - **crash_rule**: prior-valid-pointer-remains-or-is-restored
  - **query_time_migration**: forbidden
  - **prior_validation_on_replacement**:
    - **current_digest**: strict_graph_generation_validation
    - **legacy_digest**:
      - **schema_version**: 1
      - **pointer_equals_non_nil_expected_prior**: true
      - **active_row_count**: 1
      - **structural_validation**: strict_rows_counts_fks_enums_json_and_edge_evidence
      - **framing**: pre-provenance-unresolved-fields-only
      - **action**: retire_prior_and_activate_new_atomically
    - **unknown_digest_or_corruption**: blocked

- **Marcador estable**: TECH-GRAPH-NATIVE.knowledge-wiki-docs-only
- **Estado del comportamiento**: implemented
- **docs_only**:
  - **publishes_graph**: when_canonical_markdown_present
  - **preserves_code_catalog**: true
  - **catalog_generation_pointer**: reused_not_replaced
- **wikilinks**:
  - **parse**: target de enlace Obsidian
  - **embed**: target embebido con alias opcional
  - **resolve_under_wiki_slash**: wiki/<name>.md
- **structural_hierarchy**:
  - dir-readme
  - wiki-00-gobierno
- **not_authority**:
  - Tedi JavaScript graph
  - Tedi memory v1
- **Comprobación prevista**:
  - go test ./internal/indexer ./internal/docgraph -count=1 -run 'TestCanonicalGraphDocAcceptsKnowledgeWikiPaths|TestExtractReferencesParsesWikilinks|TestAppendStructuralDocEdges'
- **Evidencia referenciada**:
  - internal/indexer/indexer.go
  - internal/indexer/graph_staging.go
  - internal/docgraph/docgraph.go

Los adapters no conocen SQLite ni publican. Store no inventa identidad. Durante una query, la mutación persistente, la migración, la reparación y la publicación siguen prohibidas; el servicio solo puede reconciliar archivos canónicos relevantes en un overlay acotado de RAM. El daemon únicamente conserva warm state/cache y no cambia la semántica.

### Consulta live y overlay efímero

- **Marcador estable**: TECH-GRAPH-NATIVE.live-query-overlay
- **Estado del comportamiento**: implemented_slice
- **query**:
  - **persistent_mutation**: forbidden
  - **schema_migration**: forbidden
  - **repair**: forbidden
  - **publication**: forbidden
  - **source_mutation**: forbidden
  - **lock_acquisition**: forbidden
- **snapshot**:
  - **docs_generation**: pinned_at_request_start
  - **catalog_generation**: pinned_at_request_start
  - **graph_generation**: pinned_at_request_start_when_current
  - **fixed**: true
  - **immutable**: true
- **overlay**:
  - **allowed**: bounded_deterministic_relevant_file_parsing
  - **storage**: ram_only
  - **scope**: query_relevant_canonical_documents
  - **parser**: existing_wikisource_parser_only
  - **max_documents**:
    - **exact_wiki**: 16
    - **reverse_code**: 256
  - **max_bytes**: 4194304
  - **unchanged_inputs**: reuse_published_rows_without_body_parse
  - **changed_inputs**: parse_new_changed_or_racy_only
  - **deletion**: positive_disk_absence_only
  - **excluded_alive**: omission_without_tombstone
  - **tombstones**: request_view_only
  - **digest**: required_deterministic
  - **digest_excludes**:
    - timestamps
    - cost_counters
    - host_paths
- **freshness**:
  - **domains**:
    - docs_manifest
    - bindings
    - catalog
    - graph
    - authority
  - **statuses**:
    - current
    - overlay
    - stale
    - unknown
    - concurrent_change
- **cost**:
  - **explicit**: true
  - **counters**:
    - metadata_checked
    - files_hashed
    - files_parsed
    - unchanged_reused
    - bytes_read
- **watcher**:
  - **role**: acceleration_only
  - **correctness**: query_overlay_covers_lost_events
  - **source_mutation**: forbidden
- **compatibility**:
  - **graph_v1**: consumed_read_only_subset
  - **wiki_authority**: unchanged
- **Comprobación prevista**:
  - FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes)
  - wiki-code-bridge-runner/v1: PASS; dirty overlay p95=75ms
- **Evidencia referenciada**:
  - internal/livecontext/manifest.go
  - internal/livecontext/overlay.go
  - internal/service/app.go
  - internal/store/doc_artifact_state.go
  - .docs/wiki/06_tests/TP-GPH.md

### Identidad VCS y fan-out de observacion

`ResolveRepositoryIdentity` normaliza todas las identidades explícitas y exige que sean una sola; si faltan, ejecuta unicamente `git -C <workspace> rev-parse --show-toplevel` y `git -C <git-root> config --local --get-all remote.origin.url`. Se exige exactamente un origin HTTPS/SSH normalizable; no hay fetch, red, alias, basename ni fallback de path. La identidad resuelta es simultaneamente `WorkspaceIdentity` y `RepositoryIdentity` para todos los batches y queda en la generation, sin escribir `.mi-lsp/project.toml`.

En un container, el módulo Go se observa una sola vez desde el checkout y usa el fallback del root (`go.mod`). En una topología single/no-container con un repo Go seleccionado, el core puede elegir el `go.mod` configurado como ruta de módulo repo-local exacta o resolver un ID de `WorkspaceEntrypoint` mediante coincidencia exacta de repo y entrypoint; para un ID, rebasa la ruta workspace-relative declarada al root del repo seleccionado y revalida que el resultado sea un `go.mod` repo-local, seguro y regular. Un `--entrypoint` explícito puede seleccionar además una ruta segura existente (`go.mod` o `.csproj`) dentro del repositorio resuelto; su backend se determina por el tipo solicitado (Go o Roslyn) y nunca se sustituye silenciosamente por otro default. Las soluciones `.sln` explícitas fallan cerrado porque no existe un helper autoritativo de membresía de proyectos. Solo cuando no hay selector explícito se conserva el fallback vigente del root del repo (`go.mod`) y la selección C# configurada. Un selector explícito faltante, ID desconocido/malformado, no regular, symlink o inseguro falla cerrado con diagnóstico accionable. `go.work` no es compatible con la observación de grafo y se rechaza. La ruta declarada se mantiene relativa al workspace para la topología, mientras el batch recibe el selector repo-local rebasado.
- Todo selector explícito que contenga el separador `\` se rechaza antes de cualquier normalización de separadores; el fallback a `go.mod` solo aplica al selector vacío.
- Los campos embebidos Go de structs (`T`, `*T`, `pkg.T`, `*pkg.T` e instanciaciones genéricas) se emiten como declaraciones respaldadas por `go/types` (`*types.Var`) y conservan una referencia tipada al tipo embebido cuando es local. Si falta información del compilador, se registra omisión o unresolved y el batch permanece parcial; `ReadyForStaging` no se relaja.

### Frontera pública de Git y generaciones

`mi-lsp nav git-frontier --workspace <alias|path> --format json [--base <ref>]` expone una instantánea read-only para consumidores que necesitan comparar el checkout observado con las generaciones publicadas. Con **path explícito** read-only hacia un linked worktree no registrado, Git (`git_commit`, `git_base`, `git_frontier`) y la DB de store se abren en la raíz física resuelta, sin fallback al main registrado ni IDs de generación inferidos. Selector **omitido** o **alias** explícito no cambian la resolución vigente. No indexa, repara ni muta el workspace.

- **Marcador estable**: TECH-GRAPH-NATIVE.git-frontier
- **Estado del comportamiento**: implemented_slice
- **command**: mi-lsp nav git-frontier --workspace <alias|path> --format json [--base <ref>]
- **result**:
  - **git_commit**: resolved_HEAD_or_null
  - **git_base**: resolved_explicit_base_or_upstream_then_origin_main_or_null
  - **git_frontier**: sha256_protocol_v1_or_null
  - **generations**:
    - **docs**: published_id_or_null
    - **catalog**: published_id_or_null
    - **graph**: published_id_or_null
  - **generation_reasons**: typed_reason_per_missing_generation_or_freshness
  - **coherence**:
    - coherent
    - degraded
    - incompatible
  - **graph_freshness**:
    - current
    - stale
    - unavailable
  - **unavailable**: typed_reason_by_unavailable_field
- **frontier_digest**:
  - **domain_separator**: mi-lsp-git-frontier-v1
  - **fields**:
    - base
    - merge_base
    - head
    - committed_diff_since_merge_base
    - tracked_diff_against_HEAD
    - untracked_nonignored_files
  - **diff_mode**:
    - binary
    - no_external_diff
  - **untracked_paths**: normalized_slash_and_sorted
  - **untracked_content**: regular_file_bytes_or_symlink_target_without_following
  - **framing**: length_prefixed_name_and_content_size
  - **algorithm**: sha256
- **read_semantics**:
  - **resolve_workspace**: read_only
  - **explicit_physical_path_linked_worktree**: git_and_store_at_resolved_physical_root_without_main_fallback
  - **omitted_or_explicit_alias**: unchanged_registered_main_resolution_when_applicable
  - **explicit_path_mutating**: unchanged_main_alias_without_worktree_registration
  - **index_database**: open_existing_read_only
  - **refresh_repair_or_reindex**: forbidden
  - **missing_evidence**: null_plus_typed_reason
  - **inferred_generation_ids**: forbidden
- **coherence_rule**:
  - **coherent**: docs_and_catalog_published_and_graph_current
  - **incompatible**: explicit_invalid_graph_generation_only
  - **otherwise**: degraded
- **freshness_mapping**:
  - **current**: graph_current
  - **stale**: graph_stale_or_lagging
  - **unavailable**: absent_or_unreadable_or_invalid_freshness
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-GPH.md::TC-GPH-077
  - .docs/wiki/06_tests/TP-GPH.md::TC-GPH-078
  - .docs/wiki/06_tests/TP-WKS.md::TC-WKS-068
- **Evidencia referenciada**:
  - internal/cli/git_frontier.go
  - internal/cli/git_frontier_test.go
  - internal/workspace/registry.go
  - .docs/wiki/05_requirements/RF-WKS-007.md

`unavailable` nunca equivale a éxito: los consumidores comparan solo datos observados y fallan cerrado si el digest, una generación o la frescura requerida no está disponible. Los oráculos y casos negativos están en [[TP-GPH]].

### Observacion Roslyn, sellado y normalización de unresolved

El worker entrega el `GraphObservationBatch` ya canónico pero no sellado. El adapter puede validar canonicalidad de entrada; el core, después del rebase al namespace global, es el único owner de `SealGraphObservationBatch` y del gate `ReadyForStaging`. No se acepta que un batch no canónico o no sellado llegue al store. En Roslyn, instancias repetidas de símbolos semánticamente equivalentes emiten un único nodo canónico y su evidencia; los registros exactamente duplicados se normalizan defensivamente, mientras que una misma referencia con identidad contractual distinta sigue siendo inválida y falla cerrado.

- **Marcador estable**: TECH-GRAPH-NATIVE.observation-and-unresolved
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
  - internal/service/graph_observer.go
  - internal/indexer/graph_pipeline.go
  - internal/workspace/repository_identity.go
  - internal/model/graph_observation.go
  - internal/indexer/graph_staging.go
- **Comprobación prevista**:
  - go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  - dotnet run --project worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj
- **Condiciones de rechazo**:
  - worker_batch_noncanonical=true
  - worker_batch_sealed_before_core=true
  - graph_observation_not_ready_for_staging=true
  - unresolved_ids_assigned_before_key_dedupe=true
- **worker**:
  - **output**: GraphObservationBatch
  - **canonical**: true
  - **sealed**: false
  - **node_order**: Ref
- **core**:
  - **sequence**:
    - ValidateCanonical
    - SealGraphObservationBatch
    - ReadyForStaging
  - **seal_owner**: core
- **symbol_resolution**:
  - **typed_omissions**:
    - local
    - lambda
    - anonymous
    - synthesized_or_implicitly_declared
    - external
    - unsupported
  - **unresolved**: eligible_endpoints_really_missing_only
  - **go_entrypoint**:
    - **container**: workspace_root_fallback_only
    - **selected_repo**: exact_configured_nested_or_root_fallback
    - **explicit_nested**: selected_repo_only
    - **accepted**:
      - relative_go.mod
    - **selector_form**: repo_local_path_or_workspace_entrypoint_id
    - **entrypoint_ids**: resolve_exact_repo_and_entrypoint
    - **entrypoint_rebase**: workspace_relative_to_selected_repo_root
    - **entrypoint_revalidate**: safe_repo_local_go_mod_before_observation
    - **unknown_or_malformed_ids**: fail_closed
    - **declared_entrypoint**: workspace_relative
    - **selected_module**: repo_root_relative
    - **fallback_when**: selector_empty_only
    - **invalid_explicit**: typed_actionable_error_no_fallback
    - **backslash_rejected_before_normalization**: true
  - **csharp_entrypoint**:
    - **accepted**:
      - declared_or_safe_relative_csproj
    - **solution_selection**: typed_unavailable_no_membership_helper
    - **backend**: roslyn
    - **scope**: selected_repo_and_entrypoint_only
    - **invalid_explicit**: typed_actionable_error_no_fallback
- **graph_unresolved**:
  - **order**: key
  - **dedupe**: key
  - **assign_ids**: after_sort_and_dedupe
  - **cross_rid**: derived_from_key
  - **diagnostic_context**:
    - **fields**:
      - source_document
      - source_block
      - target_kind
      - target_value
    - **nullable**: true
    - **validation**:
      - utf8_valid
      - no_control_chars
    - **persistence_trim**: whitespace
    - **source_document_path**: relative_slash_only
    - **max_bytes**: 4096
    - **privacy**:
      - no_arbitrary_payload
      - no_secrets
      - no_raw_compiler_logs
    - **excluded_from**:
      - unresolved_key
      - unresolved_rid
    - **included_in**:
      - content_digest
      - facts_digest
      - source_fingerprint
      - config_fingerprint
      - backend_manifest_digest
      - generation_id
    - **key_fields**:
      - owner_path
      - subject_kind
      - selector_digest
      - reason_code
      - candidates
      - backend
      - source_digest
      - recovery_hint
    - **target_value_preserves_canonical_tp_ids**: true
  - **reason_contract**:
    - **missing_doc_target**: required_document_target_unresolved
    - **missing_code_target**: required_code_target_unresolved
    - **typed_omissions**: omission_without_unresolved_or_edge
  - **candidate_normalization**:
    - **trim**: true
    - **slash_normalization**: filepath.ToSlash
    - **dedupe**: true
    - **sort**: lexical
    - **max_count**: 64
    - **max_bytes**: 4096
- **doc_mention**:
  - **source_block**:
    - **normalize_before_hashing**: trim
    - **included_in**:
      - claim_digest
      - facts_digest
      - content_digest
      - source_fingerprint
      - config_fingerprint
      - backend_manifest_digest
      - generation_id
    - **input_order**: lexical_after_normalization
    - **canonical_equivalents**:
      - outer_whitespace
      - input_order
    - **unresolved_ids**: excluded


La regla de elegibilidad separa una omision tipada —sin falso edge ni falsa incompletitud— de un endpoint que el contrato si exige y que realmente no pudo resolverse. `GraphUnresolved` se ordena y deduplica por `key` antes de asignar `UnresolvedID`, `UnresolvedKey` y `CrossRID`; los candidatos documentales se normalizan antes de entrar en ese key.

### Runtime y memoria

- **Marcador estable**: TECH-GRAPH-NATIVE.runtime-constraints
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
- **storage**:
  - **authority**: .mi-lsp/index.db
  - **adjacency**: indexed-SQL-frontiers
  - **full-graph-in-memory**: forbidden
  - **persisted-transitive-closures**: forbidden
  - **networkx-core-dependency**: forbidden
- **query**:
  - **snapshot**: fixed-at-request-start
  - **writes**: forbidden
  - **bounds**:
    - **depth-default**: 1
    - **depth-max**: 6
    - **path-depth-max**: 12
    - **result-default**: 50
    - **result-max**: 500
    - **token-default**: 4000
    - **token-max**: 20000
- **cache**:
  - **keys**:
    - generation-digest
    - operation-and-parameters
    - authority-profile-digest
    - extension-or-pack-version-digest
  - **stale-cache-behavior**: discard-or-explicit-omission
- **sqlite**:
  - **write_max_open_conns**: 1
  - **read_only_pool**:
    - **max_open_conns**: 8
    - **max_idle_conns**: 4
  - **snapshot_generation**: fixed-at-request-start

### Analisis graph-native: freshness, rank, communities y utility

El runtime actual ya expone la infraestructura de analisis como enriquecimiento acotado. La freshness es un gate de autoridad: solo `current` permite claims exactos; `lagging`, `stale`, `invalid` y `unknown` deben producir omision o degradacion visible. El rank es determinista y advisory; no reemplaza identidad, evidencia ni autoridad wiki.

La consulta de freshness por request (`store.GraphFreshness`) valida solo el encabezado de la generación activa: identidad derivada (`generation_id` recalculado desde el `content_digest` almacenado), fingerprints presentes y conteos de filas (nodos, aristas, evidencia, unresolved). No re-lee ni re-hashea el grafo completo en cada query, porque las filas publicadas son inmutables y el digest de contenido ya se verificó al activar. La validación estricta no se debilita: sigue obligatoria en activación, reemplazo (`prior_validation_on_replacement`), recovery, read snapshots y en `ValidateGraphGeneration` explícito. Un encabezado o conteo inconsistente clasifica como `unavailable`/`invalid`, nunca como `current`.

- **Marcador estable**: TECH-GRAPH-NATIVE.analysis
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-GPH.md
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
- **Comprobación prevista**:
  - go test ./internal/model/... ./internal/service/... ./internal/store/...
  - mi-lsp nav graph stats --help
  - mi-lsp nav graph validate --help
  - mi-lsp nav explain-change --help
  - mi-lsp nav wiki validate-source --workspace <alias> --format toon
- **algorithm**: bounded-deterministic-v1
- **profile**: exact-extracted-only
- **freshness**:
  - **states**:
    - current
    - lagging
    - stale
    - invalid
    - unknown
  - **exact_claims_allowed_only**: current
  - **per_query_check**: header_identity_fingerprints_and_row_counts
  - **strict_validation_retained_on**:
    - activation
    - replacement
    - recovery
    - read_snapshot
    - explicit_ValidateGraphGeneration
- **rank**:
  - **score**: 0.45*authority + 0.25*impact + 0.20*centrality + 0.10*boundary
  - **authority_sources**:
    - exact
    - extracted
  - **inferred_nodes**: excluded_from_authority
  - **tie_breakers**:
    - authority
    - impact
    - centrality
    - boundary
    - utility
    - node_key
  - **digest**: sorted_canonical_output
- **communities**:
  - **algorithm**: deterministic_connected_components_v1
  - **id**: sha256_sorted_node_keys
- **bounds**:
  - **max_nodes**: 10000
  - **max_edges**: 50000
  - **max_bytes**: 65536
  - **truncation**: visible_and_not_cacheable
- **cache**:
  - **status_to_persist**: complete_only
  - **invalidation_fields**:
    - generation
    - algorithm
    - version
    - profile
    - parameter_digest
    - authority_digest
    - result_digest
  - **result_digest**: validated_before_acceptance
- **utility**:
  - **signals**:
    - continuation_followed
    - feedback_positive
    - feedback_negative
    - result_selected
  - **effect**: final_tie_break_only
  - **max_events_per_candidate**: 4096
  - **max_events_per_scope_intent_operation**: 4096
  - **retention_days**: 30
  - **candidate_identity**: node_key_only
- **telemetry**:
  - **sanitized_only**: true
  - **forbidden**:
    - query
    - prompt
    - argv
    - payload
    - snippet
    - content
    - secret
    - raw_backend_log
  - **allowed**:
    - planner_version
    - operation
    - selector_kind
    - candidate_count
    - section_count
    - fallback
    - omission_count

Si el backend graph no esta disponible, `nav intent` y `nav explain-change` conservan la preview y etiquetan el fallback; no presentan heuristica como precision graph-native ni usan un timeout silencioso para cambiar de herramienta.

### Backend maturity

- **Marcador estable**: TECH-GRAPH-NATIVE.backend-maturity
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
- **stable-order**:
  - csharp-roslyn
  - go-parser-types-gopls
  - typescript-tsserver
  - python-pyright
- **rules**:
  - **csharp**: compiler-backed-relations-are-primary; partial-project-is-omission-only
  - **go**: AST-is-extracted-and-types-or-gopls-required-for-semantic-promotion
  - **go_type_check**: shared-importer-and-cache-preserve-imported-package-identity
  - **go_local_unrepresented**: omission-unsupported-symbol-kind; no-semantic-edge
  - **go_partial**: partial-remains-partial-and-fails-ReadyForStaging
  - **typescript**: gated-until-lifecycle-and-edge-oracles-pass
  - **python**: lexical-catalog-never-becomes-semantic-edge
  - **text**: candidates-doc-mentions-or-unresolved-only

### Fallos y recovery

- **Marcador estable**: TECH-GRAPH-NATIVE.failure-modes
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
- **failures**:
  - **identity-or-hash-collision**: invalidate-generation
  - **dangling-or-stale-edge**: unresolved-and-reject-exact-claim
  - **adapter-timeout-or-crash**: discard-partial-batch
  - **go-unsupported-local-declaration**: omission-without-false-semantic-edge
  - **go-partial-observation**: preserve-partial-and-keep-ReadyForStaging-strict
  - **incremental-fanout-unknown**: full-rebuild-required
  - **SQLite-write-or-pointer-conflict**: rollback-transaction
  - **process-crash-during-publish**: recovery-keeps-prior-valid-generation
  - **daemon-unavailable**: direct-mode
  - **graph-unavailable**: legacy-or-docs-first-fallback-with-warning
  - **wiki-governance-blocked**: diagnosis-and-repair-only
  - **MILX-timeout-crash-malformed**: terminate-extension-preserve-core
- **rollback**:
  - **unit**: atomic-wave-commit-plus-immutable-prior-generation
  - **evidence_class**: preserve-sanitized-reason-and-gate-result

### Implementacion por slices

- **Marcador estable**: TECH-GRAPH-NATIVE.slices
- **plan_anchor**: RF-GPH-011
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md
  - .docs/wiki/06_tests/TP-GPH.md
- **order**:
  - G1-identity-records
  - G2-Roslyn-Go-adapters
  - G3-staging-validation
  - G4-SQLite-publish-recovery
  - G5-bounded-query
  - G6-explain-impact-wiki
  - G7-MILX-host
  - G8-CLI-no-MCP
  - G9-victory
  - G10-closure
- **join_rule**: next-slice-opens-only-after-current-TP-and-regression-gates-pass
- **planned_paths**:
  - **Estado del comportamiento**: slice-targets
  - **by_slice**:
    - **G1-identity-records**:
      - internal/model/graph.go
      - internal/model/graph_observation.go
      - internal/store/schema.go
    - **G2-Roslyn-Go-adapters**:
      - internal/indexer/graph_pipeline.go
      - internal/indexer/extractor_go.go
      - internal/service/graph_observer.go
    - **G3-staging-validation**:
      - internal/indexer/graph_staging.go
      - internal/model/graph.go
    - **G4-SQLite-publish-recovery**:
      - internal/store/schema.go
      - internal/store/graph.go
      - internal/store/graph_recovery.go
      - internal/indexer/graph_staging.go
    - **G5-bounded-query**:
      - internal/store/graph_query.go
      - internal/service/graph_query.go
    - **G6-explain-impact-wiki**:
      - internal/store/graph_impact.go
      - internal/service/graph_impact.go
      - internal/service/wiki_code_context.go
    - **G7-MILX-host**:
      - internal/milx/host.go
      - internal/milx/pack.go
    - **G8-CLI-no-MCP**:
      - internal/service/graph_query.go
      - internal/service/graph_impact.go
    - **G9-victory**:
    - **G10-closure**:
- **implementation_paths**:
  - **Estado del comportamiento**: current-tree-verified
  - **files**:
    - internal/model/graph.go
    - internal/model/graph_observation.go
    - internal/indexer/graph_pipeline.go
    - internal/indexer/extractor_go.go
    - internal/indexer/graph_staging.go
    - internal/store/schema.go
    - internal/store/graph.go
    - internal/store/graph_recovery.go
    - internal/store/graph_query.go
    - internal/store/graph_impact.go
    - internal/service/graph_observer.go
    - internal/service/graph_query.go
    - internal/service/graph_impact.go
    - internal/service/wiki_code_context.go
    - internal/milx/host.go
    - internal/milx/pack.go

### Kotlin compiler raw v2 — admisión aditiva reversible

- **Marcador estable**: TECH-GRAPH-NATIVE.kotlin-source-boundary
- **Estado del comportamiento**: experimental_unverified_explicit_incomplete
- **owner**: Semantic-backends
- **Evidencia referenciada**:
  - worker-kotlin/upstream.patch
  - internal/indexer/extractor_kotlin.go
  - internal/service/graph_observer.go
  - internal/indexer/graph_pipeline.go
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-GPH.md::TP-GPH.kotlin-compiler-export-v1
- **source**: fwcd_1.3.13_fork_1.3.13+mi-lsp.obs2_compiler_2.1.0
- **selection**: safe_explicit_Gradle_module_only_no_default_replacement
- **export**: BindingContext_and_ModuleDescriptor_raw_descriptors_signatures_receivers_resolution
- **calls**: unsupported
- **configuration**: explicit_main_jvm_hashed_sources_JARs_build_inputs_and_actual_JDK_target
- **stock_initialize**: not_compiler_extension
- **compiler_only_initialize**: before_DB_setup_Gradle_and_indexing
- **batch**: canonical_unsealed_Kotlin_subset_partial_experimental
- **gate**: ReadyForStaging_strict_complete_stable_coverage_digest
- **pipeline**: ValidateCanonical_rebase_seal_Ready_then_existing_staging
- **locks**:
  - NodeKey_v1_unchanged
  - exact_Kotlin_registry_only
  - ReadyForStaging_strict
  - Store_unchanged
- **decision**: ADR-KOTLIN-GRAPH-ADMISSION-V1
- **admission_owner**: coord-milsp_and_Graph-Kernel
- **decision_due**: 2026-10-17

La decisión del parent coord-milsp, [[ADR-KOTLIN-GRAPH-ADMISSION-V1]], sustituye específicamente `no_new_Graph_enums` de K2 por el registro exacto Kotlin. No cierra Graph-K. El recibo del exporter pineado acredita la configuración efectiva o el loader rechaza; conformance y los seis oráculos de promoción siguen explicit_incomplete. Raw v2 conserva partial/experimental; no stringflip, no NodeKey desde PSI ni aliases C#/Go. Calls unsupported se informa fuera del batch; este solo declara declarations/references. La secuencia estricta existente conserva la generación previa y no publica parciales.

### Sync y documentos relacionados

- Modelo funcional: `[[data-model]]`, `[[RF-GPH-001]]` a `[[RF-GPH-011]]`, `[[TP-GPH]]`.
- Persistencia: `[[DB-SYMBOL-EDGE-GRAPH]]`.
- CLI y envelopes: `[[CT-GRAPH-CLI]]`.
- Extensiones: `[[CT-MILX-V1]]`.
- Release: `[[AE-RELEASE-DISTRIBUTION]]`.

Cambiar ownership, pipeline, authority, backend maturity o degradacion visible exige sincronizar RF/TP. Cambiar tablas/migracion exige sincronizar `08`; cambiar comando/envelope/protocolo exige sincronizar `09`.

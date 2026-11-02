---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-003
kind: requirement
status: active
title: RF-GPH-003 - Persistir aristas tipadas con evidencia y unresolved explicito
links:
  implements:
  - FL-GPH-01
  code:
  - path: internal/docgraph/docgraph.go
    role: behavior
  - path: internal/model/doc_binding_source.go
    role: behavior
  - path: internal/model/wiki_code_binding.go
    role: behavior
  - path: internal/service/wiki_code_bindings.go
    role: behavior
  - path: internal/store/queries_doc_binding_sources.go
    role: behavior
  - path: internal/store/schema.go
    role: behavior
  - path: internal/wikisource/parser.go
    role: behavior
---

## Propósito

RF-GPH-003 - Persistir aristas tipadas con evidencia y unresolved explícito

## Comportamiento ordenado

### 1. Resultado requerido

Representar relaciones dirigidas sin inflar precision ni ocultar ambiguedad. Toda arista publicable debe enlazar dos `NodeKey` existentes en la misma generación y tener al menos una evidencia versionada. Una relacion no resuelta se persiste como `GraphUnresolved`; nunca como edge dangling.

### 2. Taxonomia inicial

`contains`, `imports`, `references`, `calls`, `implements`, `extends`, `tests`, `route_to_handler`, `publishes`, `consumes`, `reads`, `writes` y `doc_mentions`.

Agregar un tipo exige version de schema, semántica de direccion, backends habilitados, evidencia mínima y tests negativos. `doc_mentions` expresa una referencia documental; no convierte el documento en autoridad sobre una relacion de codigo.

- **Marcador estable**: RF-GPH-003.knowledge-wiki-doc-edges
- **Estado del comportamiento**: implemented
- **doc_edge_kinds**:
  - markdown_link
  - wikilink
  - embed
  - hierarchy
- **graph_relation_for_doc_edges**: doc_mentions
- **wikilink_syntax**: enlace Obsidian por target, con alias y embed opcionales
- **hierarchy**:
  - markdown en un directorio apunta a README.md de ese directorio si existe
  - wiki/*.md de primer nivel apunta a wiki/00-gobierno*.md si existe
- **Comprobación prevista**:
  - go test ./internal/docgraph -count=1 -run 'TestExtractReferencesParsesWikilinks|TestAppendStructuralDocEdges'
- **Condiciones de rechazo**:
  - wikilink_promoted_to_compiler_exact_code_edge=true
- **Evidencia referenciada**:
  - internal/docgraph/docgraph.go
  - internal/docgraph/docgraph_test.go

### 3. Estado y confianza

- **Estado**: `exact`; **Significado**: compiler/LSP resolvio ambos símbolos y relacion; **Puede presentarse como hecho del compilador**: si, con provenance
- **Estado**: `extracted`; **Significado**: parser estructural versionado resolvio la forma; **Puede presentarse como hecho del compilador**: no como compiler fact; si como extracted
- **Estado**: `inferred`; **Significado**: regla derivativa sobre evidencia publicada; **Puede presentarse como hecho del compilador**: no; siempre etiquetada
- **Estado**: `ambiguous`; **Significado**: existen destinos/candidatos multiples; **Puede presentarse como hecho del compilador**: no se publica edge activa

`ambiguous`, `unsupported`, `stale_source`, `identity_missing` y `endpoint_missing` producen unresolved/omission. Un score numerico opcional no puede elevar `extracted` o `inferred` a `exact`.

### 4. Identidad de edge y evidencia

- `edge_key` = SHA-256 de serializacion `MILSP-EK/v1` con `from_node_key`, `relation`, `to_node_key` y `claim_scope` canonicos. Es dirigido; para una relacion simetrica se materializan reglas explícitas, no sorting accidental de endpoints.
- Varias observaciones del mismo claim comparten edge y se conservan como evidencias separadas.
- `GraphEvidence` contiene `source_uri` repo-relative, rango, backend, compiler/extractor version, source digest, claim observado, confidence/status, generation y cross-RID.
- `evidence_digest` excluye timestamps y usa bytes de fuente normalizados según el backend; no normaliza semántica arbitrariamente.
- `edge_cross_rid` = `milsp:gph-edge:v1:<lowercase-hex-edge-key>`; cada evidencia tiene un cross-RID derivado de edge key + evidence digest + ordinal canonico.

### 5. Validación

1. Resolver ambos endpoints por `NodeKey` dentro del staging actual.
2. Validar que relation y backend admitan la combinacion de kinds.
3. Validar source fingerprint y evidence digest.
4. Deduplicar por `edge_key` sin descartar evidencias distintas.
5. Ordenar evidencias por source URI, rango, backend y digest.
6. Enviar casos incompletos a `GraphUnresolved` con reason y recovery hint.

### 6. Invariantes

- Cero edges silenciosamente dangling.
- Toda edge activa tiene uno o más `GraphEvidence` y el mismo `generation_id` que sus endpoints.
- Una edge inferred nunca se devuelve sin `status=inferred` y regla derivativa.
- Texto libre solo puede producir candidatos, `doc_mentions` o unresolved; no `calls`, `implements`, `extends`, `reads` o `writes` exactos.
- Orden y deduplicacion son deterministas entre RIDs.
- Imports Graphify u otra fuente externa viven en namespace/provenance `external:*` y no sustituyen evidencia nativa.

### 7. GraphUnresolved

Debe conservar `reason_code`, input/selector sanitizado, candidatos bounded, backend, owner path, generation, source digest, cross-RID y `recovery_hint`. Puede incluir, como contexto diagnóstico nullable (persistido como `null`/ausente cuando no aplica), `source_document`, `source_block`, `target_kind` y `target_value`. Si se informan, se validan como texto UTF-8 válido y sin caracteres de control; la proyección persistida recorta espacios, y `source_document` además debe ser un path relativo con `/`. El contexto completo queda acotado a 4096 bytes y los candidatos a 64 elementos/4096 bytes.

Los cuatro campos de contexto de `GraphUnresolved` son diagnósticos: participan, con framing determinista y orden estable, en el `content_digest` y en el digest de facts que alimenta los fingerprints `source`/`config`/`backend` y, por tanto, el `generation_id`; no participan en `GraphUnresolvedKey` ni en `UnresolvedRID`: la identidad unresolved usa `owner_path`, `subject_kind`, `selector_digest`, `reason_code`, candidatos, backend, `source_digest` y `recovery_hint`. En consecuencia, enriquecer la atribución cambia los digests de contenido/facts/generación, pero no cambia key/RID, y `target_value` conserva los IDs canónicos `TP-*` sin sustituirlos por un path.

En una `DocMention` canónica que resuelve a un único destino, `SourceBlock` se recorta antes del hashing y participa, con framing y orden deterministas, tanto en el digest de claim de la mención como en el mention portion de `graphDocFactsDigest`; un cambio exclusivo de `SourceBlock` cambia los digests `content`/`facts` y los fingerprints `source`/`config`/`backend`/`generation`, mientras que whitespace exterior y orden de entrada canónicos conservan la identidad. Este caso resuelto no crea ni modifica `GraphUnresolvedKey`/`UnresolvedRID`; esos IDs siguen reservados para claims unresolved.

`missing_doc_target` y `missing_code_target` solo describen un destino elegible que el contrato exige y que no pudo resolverse; sus diagnósticos quedan tipados y bounded. Una omisión tipada (`local`, `lambda`, `anonymous`, `synthesized_or_implicitly_declared`, `external` o `unsupported`) no es un unresolved: no crea edge ni falsa incompletitud. No contiene payload arbitrario, secretos ni raw compiler logs. Los unresolved participan en stats y explican recall perdido.

### 10. Binding declarado y provenance del puente live

- **Marcador estable**: RF-GPH-003.declared-binding-provenance
- **Estado del comportamiento**: implemented_slice
- **scope**:
  - **canonical_declaration**: artifact_bindings
  - **persisted_projection**: doc_artifact_bindings
  - **path_semantics**: repository_relative_posix
  - **source_range**: [start_line, end_line]
  - **identity_fields**: [doc_path, block_id, doc_id, relation, target_path, target_symbol, target_kind, binding_ref]
- **provenance_fields**:
  - authoring_origin
  - binding_status
  - doc_lifecycle
  - superseded_by
  - source_content_hash
  - start_line
  - end_line
- **vocabulary**:
  - **relation**: [implements, tests, configures, operates]
  - **target_kind**: [file, symbol, test, config]
  - **authoring_origin**: [canonical, legacy]
  - **binding_status**: [exact, planned]
  - **doc_lifecycle**: [active, deprecated, retired]
- **resolution_order**:
  - exact_declared_binding
  - current_target_path_and_hash
  - catalog_exact_symbol
  - lsp_or_graph_supporting_evidence_when_available
  - bounded_lexical_candidates
- **candidate_rule**:
  - **textual_candidate**: candidate_only
  - **provenance**: text_candidate
  - **direct_edge**: forbidden
  - **semantic_relation_from_text**: forbidden
- **graph_v1**: strict_canonical_source_reader_universal
- **graph_v1_authoring**:
  - **authority**: shared_SDD-WIKI-GRAPH-v1
  - **adoption**: explicit_wiki_graph_protocol_SDD-WIKI-GRAPH-v1_in_document_definition
  - **required_document_metadata**: [doc_id, kind, document_lifecycle]
  - **document_owner**: flat_definition_kind_never_block_kind_or_folder
  - **source_locations**: [document_root_header_frontmatter, toon_root, surfaces_nested_repeated_inline]
  - **toon_bracket_lists**: en bloques TOON, el cierre de corchete delimita la lista explícita; objetos multilínea entre llaves no requieren sangría ni comas YAML
  - **object_fields_required**: [binding_id, kind, role, path, surface, binding_lifecycle]
  - **object_fields_optional**: [symbol]
  - **other_object_fields**: rejected_with_binding_omission
  - **kinds**: [code, test, schema, migration, contract, config, ui]
  - **roles**: [behavior_implementation, orchestration_entrypoint, executable_test, handler, adapter, contract_schema, db_schema, db_migration, shared_component, infrastructure, runtime_config]
  - **surfaces**: [frontend, backend, shared, integration, data, operations]
  - **binding_lifecycle**: [active, deprecated]
  - **document_lifecycle**: [active, deprecated, retired]
  - **ownership_minimum**:
    - **RF**: [behavior_implementation]
    - **TP**: [executable_test]
    - **CT**: [handler, adapter, contract_schema]
    - **DB**: [db_schema, db_migration]
    - **TECH**: [shared_component, infrastructure, runtime_config]
    - **TECH-DS**: [shared_component, infrastructure, runtime_config]
    - **FL**: [orchestration_entrypoint]
    - **other_document_kinds**: no_direct_binding
  - **ownership_note**: validar role por owner; no inferir una matriz exhaustiva de pares kind_role de ejemplos
  - **source_metadata**: [binding_id, kind, surface, binding_lifecycle, document_lifecycle]
  - **lifecycle_axes**: independent_document_active_with_binding_deprecated_is_legal
  - **legacy_projection_lifecycle**: document_lifecycle_unless_active_binding_deprecated_then_deprecated
  - **aggregate_policy**: non_active_either_axis_excludes_bridge; equivalent_WCB_rows_deduplicated_conservatively
  - **duplicate_binding_id**: all_ambiguous_declarations_rejected_document_wide
  - **response_and_planned_payload**: not_declarations
  - **source_ranges**: object_exact_range_not_claimed_currently_zero
  - **persisted_source_collection**: doc_binding_sources
  - **source_availability_owners**: doc_binding_source_documents
  - **source_identity**: [doc_path, binding_id]
  - **independent_source_axes**: [document_lifecycle, binding_lifecycle]
  - **omissions**: parser_diagnostics_preserved_except_transport_omission_in_collection_carrying_docgraph
  - **collection_status**: PARTIAL_scoped_unique_RECHECK1_H1_H2_PASS_not_full_B_C
  - **public_source_lookup**: nav_trace_or_wiki_trace_with_binding_sources_flag
  - **public_selectors**: document_ID_or_doc_path_AND_optional_binding_ID_or_naked_binding_ID
  - **public_history**: historical_flag_only_separate_from_active_evidence
  - **query_projection**: wiki_code_context.binding_sources
  - **identity_note**: binding_ref_remains_MILSP-WCB/v1_unchanged_no_new_ID_or_schema
- **derived_association_mapping**:
  - **code_and_ui**:
    - **behavior_implementation**: implements
    - **executable_test**: tests
    - **other_permitted_roles**: operates
    - **target_kind**: symbol_when_symbol_present_otherwise_file
  - **test**: tests_test
  - **config**: configures_config
  - **schema_migration_contract**: operates_file
  - **semantics**: documentary_bridge_associations_not_compiler_edges_or_new_GraphV1_relations
- **precedence**:
  - adopted_GraphV1_is_strict_no_legacy_fields_inside_canonical_objects
  - legacy_not_adopted_is_advisory_preserves_existing_aliases_defaults_and_conflicts
  - no_target_path_authoring_workaround_required
  - path_and_symbol_safety_required_before_projection
  - default_provenance_and_resolution_consumers_remain_unchanged
- **legacy**:
  - **aliases**: [implementation_anchors, code_links, test_links, implements, tests]
  - **normalization**: derived_binding_with_authoring_origin_legacy
  - **authority**: advisory
- **lifecycle**:
  - **planned**: stored_but_non_navigable_via_binding_status
  - **binding_status_planned**: excluded in wiki_code_bindings.bindingNavigable with omission planned_binding_excluded
  - **doc_lifecycle_non_active**: excluded from navigable exact evidence; deprecated and retired follow retired policy
  - **retired**: history_only_by_default
  - **explicit_historical**: legacy_bounded_redirect_preserved_plus_opt_in_source_metadata_including_terminal_retired
  - **source_history**: explicit_historical_flag_required_never_active_direct_evidence
- **Invariantes**:
  - exact_binding_precedes_graph_lsp_and_lexical
  - **candidates_never_become_direct_edges**: true
  - **source_digest_and_binding_ref_are_stable**: true
  - **query_writes**: forbidden
- **verification_status**: manual_evidence_pending
- **Comprobación prevista**:
  - parent_single_FINAL_VERIFY_after_implementation_complete
- **historical_verification_not_reexecuted**:
  - "FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes); no valida esta ampliación"
  - "wiki-code-bridge-runner/v1: status PASS; inventory=37; stable_digest_runs=30; histórica"
- **Evidencia referenciada**:
  - internal/wikisource/parser.go
  - internal/model/wiki_code_binding.go
  - internal/service/wiki_code_bindings.go
  - internal/store/schema.go
  - .docs/wiki/06_tests/TP-GPH.md

### Colección fuente GraphV1 (autoría B)

El objeto canónico no es la fila agregada WCB: dos declaraciones con IDs o superficies diferentes pueden asociarse al mismo `binding_ref`. Se conservan `kind` y `symbol` originales incluso si la asociación `schema`, `migration` o `contract` deriva a un archivo sin símbolo. No se agregan aristas Graph ni se modifica `MILSP-WCB/v1` o `q-v1`.

- **Marcador estable**: RF-GPH-003.canonical-source-collection
- **Estado del comportamiento**: authored
- **verification_status**: PARTIAL_scoped
- **verification_basis**: FINAL_VERIFY1_PARTIAL_audit1_Changes_requested_H1_H2_repair_unique_RECHECK1_H1_H2_PASS_scoped_overall_PARTIAL
- **publication_scope**: parent_authorized_H1_H2_scoped_not_full_B_C
- **incremental_evidence**: explicit_incomplete_Git_AM_not_consumed_processed_0_no_changes_no_owner_transition_no_PRODUCT_FAIL
- **q_evidence**: valid_pipeline_same_corpus_DB_one_governance_item_parity_only
- **q_uncovered**:
  - multi_item_sort
  - cursor_resume
  - native_digest
  - source_variation
- **global_readiness**: Graph_Harness_fullwiki_conformance_not_accredited
- **remaining_evidence**: explicit_incomplete_coord_milsp_due_2026_10_15
- **legacy_source_owner_ranges**: partial_visible_zero_ranges_not_exact
- **source_object**:
  - **required**:
    - binding_id
    - kind
    - role
    - path
    - surface
    - binding_lifecycle
  - **optional**:
    - symbol
  - **extras**: forbidden
- **wrapper**:
  - **identity**:
    - doc_path
    - binding_id
  - **owner**: doc_path
  - **authority**: doc_id
  - **location**: block_id
  - **provenance**: declared
  - **document_lifecycle**: independent_of_binding_lifecycle
  - **observed_only**:
    - source_content_hash
    - start_line
    - end_line
  - **resolution**: derived_at_query_time_not_persisted
- **bridge**:
  - **association**: optional_many_declarations_to_one_WCB
  - **identity**: unchanged_MILSP-WCB/v1
  - **legacy**: advisory_only_no_invented_binding_ID_or_surface
- **publication**:
  - **producer**: DocRecord.BindingSources_internal_transport
  - **paths**:
    - full
    - docs_only
    - incremental
    - snapshot_read
    - request_RAM
  - **parsed_empty**: available_collection_with_zero_declarations
  - **duplicate_ID**: reject_all_ambiguous_objects_keep_omissions
  - **inactive**: preserve_source_declarations_no_active_evidence_promotion
  - **old_database**: explicit_incomplete_without_DDL_or_backfill
  - **reparse**: explicit_future_producer_operation_only_never_from_WCB
- **acceptance_refs**:
  - TC-GPH-SRC-001
  - TC-GPH-SRC-002
  - TC-GPH-SRC-003
  - TC-GPH-SRC-004
  - TC-GPH-SRC-005
  - TC-GPH-SRC-006
- **artifact_bindings**:
  - **binding_id**: RF-GPH-003-source-producer
  - **Clase**: code
  - **role**: behavior_implementation
  - **path**: internal/docgraph/docgraph.go
  - **symbol**: parseDocContent
  - **surface**: backend
  - **binding_lifecycle**: active
  - **binding_id**: RF-GPH-003-source-conversion
  - **Clase**: code
  - **role**: behavior_implementation
  - **path**: internal/wikisource/parser.go
  - **symbol**: SourceBindingDeclarations
  - **surface**: backend
  - **binding_lifecycle**: active
- **Comprobación prevista**:
  - parent_single_FINAL_VERIFY_after_B_and_C_writers_idle
- **Evidencia referenciada**:
  - internal/model/doc_binding_source.go
  - internal/wikisource/parser.go
  - internal/docgraph/docgraph.go
  - internal/store/queries_doc_binding_sources.go
  - .docs/wiki/06_tests/TP-GPH.md

La advertencia de A permanece en el parser sin persistencia. Docgraph la descarta únicamente al transportar la colección completa; las declaraciones rechazadas mantienen sus omissions. Un snapshot anterior conserva `explicit_incomplete` aunque el bridge exista: su contenido perdido no se reconstruye. Una operación explícita posterior del productor (por ejemplo, índice limpio) puede volver a parsear las fuentes; esta autoría no ejecuta esa operación ni una migración instalada. Las referencias históricas y sus selectores externos corresponden a C; guardar declaraciones inactivas no las convierte en `direct_code`, ranking o conformance activos.

### Relaciones documentales explícitas

La extracción Markdown persiste, mediante `DocEdge.Kind`, las relaciones `doc_related`, `doc_depends`, `doc_supports`, `doc_contradicts` y `doc_supersedes`. Solo se reconocen como líneas de lista con un wikilink o markdown-link local; frontmatter, cercas, similitud y embeddings no producen estas aristas. La resolución conserva rutas, `doc_id` y estado unresolved para destinos ausentes o ambiguos.

## Superficies

- backend

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 8. Errores tipados

- **Código**: `GPH_EDGE_ENDPOINT_MISSING`; **Causa**: source o target no existe; **Resultado**: unresolved; generación inválida si se marco exact
- **Código**: `GPH_EDGE_RELATION_UNSUPPORTED`; **Causa**: combinacion no registrada; **Resultado**: omission/unresolved
- **Código**: `GPH_EDGE_EVIDENCE_MISSING`; **Causa**: edge sin evidencia; **Resultado**: reject
- **Código**: `GPH_EDGE_SOURCE_STALE`; **Causa**: digest no coincide; **Resultado**: unresolved; no publicar claim
- **Código**: `GPH_EDGE_AMBIGUOUS`; **Causa**: multiples destinos no desambiguados; **Resultado**: unresolved con candidatos bounded
- **Código**: `GPH_EDGE_PROVENANCE_INVALID`; **Causa**: backend/version/claim incompleto; **Resultado**: reject

## Criterios de aceptación (numerados)

1. Fixtures positivos, negativos, ambiguos, unresolved y not-comparable.
2. Precision/recall por relation; `negative_violations=0` es gate independiente.
3. Validación de orden, deduplicacion, cross-RID y cero dangling edges en 30 reruns.

### 9. Aceptacion y trazabilidad

- Fixtures positivos, negativos, ambiguos, unresolved y not-comparable.
- Precision/recall por relation; `negative_violations=0` es gate independiente.
- Validación de orden, deduplicacion, cross-RID y cero dangling edges en 30 reruns.
- `TP-GPH / TP-GPH-003 / TC-GPH-015..018`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-GPH-003
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-GPH-01.md
exports:
- RF-GPH-003
- RF-GPH-003 - Persistir aristas tipadas con evidencia y unresolved explicito
agent_must_read:
- .docs/wiki/05_requirements/RF-GPH-003.md
- .docs/wiki/04_flows/FL-GPH-01.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-GPH-003.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/docgraph/docgraph.go
- internal/model/doc_binding_source.go
- internal/model/wiki_code_binding.go
- internal/service/wiki_code_bindings.go
- internal/store/queries_doc_binding_sources.go
- internal/store/schema.go
- internal/wikisource/parser.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-GPH.md, ejecutar el
  comando publicado: go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  ./internal/daemon/... (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-GPH-003.
evidence:
- .docs/wiki/05_requirements/RF-GPH-003.md
- .docs/wiki/06_tests/TP-GPH.md
```

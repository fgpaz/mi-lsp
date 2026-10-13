---
id: RF-GPH-003
title: Persistir aristas tipadas con evidencia y unresolved explicito
status: planned
flows:
  - FL-GPH-01
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-003"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[05_modelo_datos]]'
exports:
  - 'RF-GPH-003'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/05_modelo_datos.md
  - .docs/wiki/04_RF/RF-GPH-003.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-003.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-003.md
```

# RF-GPH-003 - Persistir aristas tipadas con evidencia y unresolved explicito

## 1. Resultado requerido

Representar relaciones dirigidas sin inflar precision ni ocultar ambiguedad. Toda arista publicable debe enlazar dos `NodeKey` existentes en la misma generacion y tener al menos una evidencia versionada. Una relacion no resuelta se persiste como `GraphUnresolved`; nunca como edge dangling.

## 2. Taxonomia inicial

`contains`, `imports`, `references`, `calls`, `implements`, `extends`, `tests`, `route_to_handler`, `publishes`, `consumes`, `reads`, `writes` y `doc_mentions`.

Agregar un tipo exige version de schema, semantica de direccion, backends habilitados, evidencia minima y tests negativos. `doc_mentions` expresa una referencia documental; no convierte el documento en autoridad sobre una relacion de codigo.

```toon
doc_id: RF-GPH-003
block_id: RF-GPH-003.knowledge-wiki-doc-edges
kind: edge-taxonomy-slice
source_of_truth: this
status: implemented
doc_edge_kinds:
  - markdown_link
  - wikilink
  - embed
  - hierarchy
graph_relation_for_doc_edges: doc_mentions
wikilink_syntax: "enlace Obsidian por target, con alias y embed opcionales"
hierarchy:
  - markdown en un directorio apunta a README.md de ese directorio si existe
  - wiki/*.md de primer nivel apunta a wiki/00-gobierno*.md si existe
verify:
  - go test ./internal/docgraph -count=1 -run 'TestExtractReferencesParsesWikilinks|TestAppendStructuralDocEdges'
stop_if:
  - wikilink_promoted_to_compiler_exact_code_edge=true
evidence:
  - internal/docgraph/docgraph.go
  - internal/docgraph/docgraph_test.go
```

## 3. Estado y confianza

| Estado | Significado | Puede presentarse como hecho del compilador |
|---|---|---|
| `exact` | compiler/LSP resolvio ambos simbolos y relacion | si, con provenance |
| `extracted` | parser estructural versionado resolvio la forma | no como compiler fact; si como extracted |
| `inferred` | regla derivativa sobre evidencia publicada | no; siempre etiquetada |
| `ambiguous` | existen destinos/candidatos multiples | no se publica edge activa |

`ambiguous`, `unsupported`, `stale_source`, `identity_missing` y `endpoint_missing` producen unresolved/omission. Un score numerico opcional no puede elevar `extracted` o `inferred` a `exact`.

## 4. Identidad de edge y evidencia

- `edge_key` = SHA-256 de serializacion `MILSP-EK/v1` con `from_node_key`, `relation`, `to_node_key` y `claim_scope` canonicos. Es dirigido; para una relacion simetrica se materializan reglas explicitas, no sorting accidental de endpoints.
- Varias observaciones del mismo claim comparten edge y se conservan como evidencias separadas.
- `GraphEvidence` contiene `source_uri` repo-relative, rango, backend, compiler/extractor version, source digest, claim observado, confidence/status, generation y cross-RID.
- `evidence_digest` excluye timestamps y usa bytes de fuente normalizados segun el backend; no normaliza semantica arbitrariamente.
- `edge_cross_rid` = `milsp:gph-edge:v1:<lowercase-hex-edge-key>`; cada evidencia tiene un cross-RID derivado de edge key + evidence digest + ordinal canonico.

## 5. Validacion

1. Resolver ambos endpoints por `NodeKey` dentro del staging actual.
2. Validar que relation y backend admitan la combinacion de kinds.
3. Validar source fingerprint y evidence digest.
4. Deduplicar por `edge_key` sin descartar evidencias distintas.
5. Ordenar evidencias por source URI, rango, backend y digest.
6. Enviar casos incompletos a `GraphUnresolved` con reason y recovery hint.

## 6. Invariantes

- Cero edges silenciosamente dangling.
- Toda edge activa tiene uno o mas `GraphEvidence` y el mismo `generation_id` que sus endpoints.
- Una edge inferred nunca se devuelve sin `status=inferred` y regla derivativa.
- Texto libre solo puede producir candidatos, `doc_mentions` o unresolved; no `calls`, `implements`, `extends`, `reads` o `writes` exactos.
- Orden y deduplicacion son deterministas entre RIDs.
- Imports Graphify u otra fuente externa viven en namespace/provenance `external:*` y no sustituyen evidencia nativa.

## 7. GraphUnresolved

Debe conservar `reason_code`, input/selector sanitizado, candidatos bounded, backend, owner path, generation, source digest, cross-RID y `recovery_hint`. Puede incluir, como contexto diagnóstico nullable (persistido como `null`/ausente cuando no aplica), `source_document`, `source_block`, `target_kind` y `target_value`. Si se informan, se validan como texto UTF-8 válido y sin caracteres de control; la proyección persistida recorta espacios, y `source_document` además debe ser un path relativo con `/`. El contexto completo queda acotado a 4096 bytes y los candidatos a 64 elementos/4096 bytes.

Los cuatro campos de contexto de `GraphUnresolved` son diagnósticos: participan, con framing determinista y orden estable, en el `content_digest` y en el digest de facts que alimenta los fingerprints `source`/`config`/`backend` y, por tanto, el `generation_id`; no participan en `GraphUnresolvedKey` ni en `UnresolvedRID`: la identidad unresolved usa `owner_path`, `subject_kind`, `selector_digest`, `reason_code`, candidatos, backend, `source_digest` y `recovery_hint`. En consecuencia, enriquecer la atribución cambia los digests de contenido/facts/generación, pero no cambia key/RID, y `target_value` conserva los IDs canónicos `TP-*` sin sustituirlos por un path.

En una `DocMention` canónica que resuelve a un único destino, `SourceBlock` se recorta antes del hashing y participa, con framing y orden deterministas, tanto en el digest de claim de la mención como en el mention portion de `graphDocFactsDigest`; un cambio exclusivo de `SourceBlock` cambia los digests `content`/`facts` y los fingerprints `source`/`config`/`backend`/`generation`, mientras que whitespace exterior y orden de entrada canónicos conservan la identidad. Este caso resuelto no crea ni modifica `GraphUnresolvedKey`/`UnresolvedRID`; esos IDs siguen reservados para claims unresolved.

`missing_doc_target` y `missing_code_target` solo describen un destino elegible que el contrato exige y que no pudo resolverse; sus diagnósticos quedan tipados y bounded. Una omisión tipada (`local`, `lambda`, `anonymous`, `synthesized_or_implicitly_declared`, `external` o `unsupported`) no es un unresolved: no crea edge ni falsa incompletitud. No contiene payload arbitrario, secretos ni raw compiler logs. Los unresolved participan en stats y explican recall perdido.

## 8. Errores tipados

| Codigo | Causa | Resultado |
|---|---|---|
| `GPH_EDGE_ENDPOINT_MISSING` | source o target no existe | unresolved; generacion invalida si se marco exact |
| `GPH_EDGE_RELATION_UNSUPPORTED` | combinacion no registrada | omission/unresolved |
| `GPH_EDGE_EVIDENCE_MISSING` | edge sin evidencia | reject |
| `GPH_EDGE_SOURCE_STALE` | digest no coincide | unresolved; no publicar claim |
| `GPH_EDGE_AMBIGUOUS` | multiples destinos no desambiguados | unresolved con candidatos bounded |
| `GPH_EDGE_PROVENANCE_INVALID` | backend/version/claim incompleto | reject |

## 9. Aceptacion y trazabilidad

- Fixtures positivos, negativos, ambiguos, unresolved y not-comparable.
- Precision/recall por relation; `negative_violations=0` es gate independiente.
- Validacion de orden, deduplicacion, cross-RID y cero dangling edges en 30 reruns.
- `TP-GPH / TP-GPH-003 / TC-GPH-015..018`.

## 10. Binding declarado y provenance del puente live

```toon
doc_id: RF-GPH-003
block_id: RF-GPH-003.declared-binding-provenance
kind: declared-binding-contract
source_of_truth: this
status: implemented_slice
scope:
  canonical_declaration: artifact_bindings
  persisted_projection: doc_artifact_bindings
  path_semantics: repository_relative_posix
  source_range: [start_line, end_line]
  identity_fields: [doc_path, block_id, doc_id, relation, target_path, target_symbol, target_kind, binding_ref]
provenance_fields:
  - authoring_origin
  - binding_status
  - doc_lifecycle
  - superseded_by
  - source_content_hash
  - start_line
  - end_line
vocabulary:
  relation: [implements, tests, configures, operates]
  target_kind: [file, symbol, test, config]
  authoring_origin: [canonical, legacy]
  binding_status: [exact, planned]
  doc_lifecycle: [active, deprecated, retired]
resolution_order:
  - exact_declared_binding
  - current_target_path_and_hash
  - catalog_exact_symbol
  - lsp_or_graph_supporting_evidence_when_available
  - bounded_lexical_candidates
candidate_rule:
  textual_candidate: candidate_only
  provenance: text_candidate
  direct_edge: forbidden
  semantic_relation_from_text: forbidden
graph_v1: strict_canonical_source_reader_universal
graph_v1_authoring:
  authority: shared_SDD-WIKI-GRAPH-v1
  adoption: explicit_wiki_graph_protocol_SDD-WIKI-GRAPH-v1_in_document_definition
  required_document_metadata: [doc_id, kind, document_lifecycle]
  document_owner: flat_definition_kind_never_block_kind_or_folder
  source_locations: [document_root_header_frontmatter, toon_root, surfaces_nested_repeated_inline]
  toon_bracket_lists: en bloques TOON, el cierre de corchete delimita la lista explícita; objetos multilínea entre llaves no requieren sangría ni comas YAML
  object_fields_required: [binding_id, kind, role, path, surface, binding_lifecycle]
  object_fields_optional: [symbol]
  other_object_fields: rejected_with_binding_omission
  kinds: [code, test, schema, migration, contract, config, ui]
  roles: [behavior_implementation, orchestration_entrypoint, executable_test, handler, adapter, contract_schema, db_schema, db_migration, shared_component, infrastructure, runtime_config]
  surfaces: [frontend, backend, shared, integration, data, operations]
  binding_lifecycle: [active, deprecated]
  document_lifecycle: [active, deprecated, retired]
  ownership_minimum:
    RF: [behavior_implementation]
    TP: [executable_test]
    CT: [handler, adapter, contract_schema]
    DB: [db_schema, db_migration]
    TECH: [shared_component, infrastructure, runtime_config]
    TECH-DS: [shared_component, infrastructure, runtime_config]
    FL: [orchestration_entrypoint]
    other_document_kinds: no_direct_binding
  ownership_note: validar role por owner; no inferir una matriz exhaustiva de pares kind_role de ejemplos
  source_metadata: [binding_id, kind, surface, binding_lifecycle, document_lifecycle]
  lifecycle_axes: independent_document_active_with_binding_deprecated_is_legal
  legacy_projection_lifecycle: document_lifecycle_unless_active_binding_deprecated_then_deprecated
  aggregate_policy: non_active_either_axis_excludes_bridge; equivalent_WCB_rows_deduplicated_conservatively
  duplicate_binding_id: all_ambiguous_declarations_rejected_document_wide
  response_and_planned_payload: not_declarations
  source_ranges: object_exact_range_not_claimed_currently_zero
  not_persisted: [binding_id, surface, kind, binding_lifecycle, document_lifecycle_separate]
  omissions: binding_omission_mentions_via_existing_docgraph_producer
  collection_status: explicit_incomplete_canonical_source_collection_not_persisted_owned_by_Unit_B
  identity_note: binding_ref_remains_MILSP-WCB/v1_unchanged_no_new_ID_or_schema
derived_association_mapping:
  code_and_ui:
    behavior_implementation: implements
    executable_test: tests
    other_permitted_roles: operates
    target_kind: symbol_when_symbol_present_otherwise_file
  test: tests_test
  config: configures_config
  schema_migration_contract: operates_file
  semantics: documentary_bridge_associations_not_compiler_edges_or_new_GraphV1_relations
precedence:
  - adopted_GraphV1_is_strict_no_legacy_fields_inside_canonical_objects
  - legacy_not_adopted_is_advisory_preserves_existing_aliases_defaults_and_conflicts
  - no_target_path_authoring_workaround_required
  - path_and_symbol_safety_required_before_projection
  - default_provenance_and_resolution_consumers_remain_unchanged
legacy:
  aliases: [implementation_anchors, code_links, test_links, implements, tests]
  normalization: derived_binding_with_authoring_origin_legacy
  authority: advisory
lifecycle:
  planned: stored_but_non_navigable_via_binding_status
  binding_status_planned: excluded in wiki_code_bindings.bindingNavigable with omission planned_binding_excluded
  doc_lifecycle_non_active: excluded from navigable exact evidence; deprecated and retired follow retired policy
  retired: history_only_by_default
  explicit_historical: bounded_redirect_when_superseded_by_exists
invariants:
  exact_binding_precedes_graph_lsp_and_lexical
  candidates_never_become_direct_edges: true
  source_digest_and_binding_ref_are_stable: true
  query_writes: forbidden
verification_status: manual_evidence_pending
verify:
  - parent_single_FINAL_VERIFY_after_implementation_complete
historical_verification_not_reexecuted:
  - "FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes); no valida esta ampliación"
  - "wiki-code-bridge-runner/v1: status PASS; inventory=37; stable_digest_runs=30; histórica"
evidence:
  - internal/wikisource/parser.go
  - internal/model/wiki_code_binding.go
  - internal/service/wiki_code_bindings.go
  - internal/store/schema.go
  - .docs/wiki/06_pruebas/TP-GPH.md
```

## Relaciones documentales explícitas

La extracción Markdown persiste, mediante `DocEdge.Kind`, las relaciones `doc_related`, `doc_depends`, `doc_supports`, `doc_contradicts` y `doc_supersedes`. Solo se reconocen como líneas de lista con un wikilink o markdown-link local; frontmatter, cercas, similitud y embeddings no producen estas aristas. La resolución conserva rutas, `doc_id` y estado unresolved para destinos ausentes o ambiguos.

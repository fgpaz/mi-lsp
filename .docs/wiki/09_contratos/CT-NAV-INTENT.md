---
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
wiki_graph_protocol: SDD-WIKI-GRAPH-v1
document_lifecycle: active
doc_id: CT-NAV-INTENT
id: CT-NAV-INTENT
title: Contrato CLI nav intent
layer: CT
family: NAV-INTENT
status: implemented_slice
kind: contract
audience: llm-first
imports:
  - 00_gobierno_documental
  - RF-QRY-001
  - RF-QRY-011
  - RF-QRY-014
  - RF-QRY-015
  - RF-QRY-016
  - TP-QRY
  - CT-GRAPH-CLI
  - TECH-DOC-ROUTER
  - TECH-GRAPH-NATIVE
exports:
  - 'CT-NAV-INTENT.surface'
  - 'CT-NAV-INTENT.execution-routing'
  - 'CT-NAV-INTENT.selectors'
  - 'CT-NAV-INTENT.legacy-envelope'
  - 'CT-NAV-INTENT.planner-preview'
  - 'CT-NAV-INTENT.continuation-and-enrichment'
  - 'CT-NAV-INTENT.artifact-bindings'
  - 'CT-NAV-INTENT.compatibility'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-QRY.md
  - .docs/wiki/07_tech/TECH-DOC-ROUTER.md
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
  - go test ./internal/service/... ./internal/telemetry/...
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - direct_daemon_semantics_diverge=true
  - ambiguous_selector_auto_selected=true
  - timeout_without_typed_diagnostic=true
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-QRY.md
  - internal/service/intent.go
  - internal/service/intent_test.go
  - internal/service/wiki_code_context.go
  - internal/service/wiki_code_enrich.go
  - internal/service/wiki_code_enrich_test.go
---

# CT-NAV-INTENT

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav intent`.

```toon
block_id: CT-NAV-INTENT.surface
kind: cli-contract
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
boundary: usuario_o_agente -> mi-lsp nav intent
invocation: mi-lsp nav intent <question> [--workspace <alias>] [--repo <name>] [--top N] [--offset N] [--full]
payload:
  question: required_string
  workspace: resolvable_alias_or_path
  repo: optional_repo_selector_for_container_workspaces
  top: optional_integer
  offset: optional_integer
  full: optional_preview_expansion_flag
response:
  backend: intent|planner
  mode: docs|code|preview
  fields: [items, warnings, stats]
```

`nav intent` es una superficie directa y repo-local. No agrega API HTTP, MCP, daemon nuevo ni runtime nuevo.

## Semántica legacy

`nav intent` conserva `backend=intent` y expone `mode=docs|code`.

- `mode=docs`: consultas capability-like, contract-like, flow-like o docs-first. Usa el scorer documental owner-aware compartido con `nav route`, `nav ask` y `nav pack`.
- `mode=code`: consultas symbol-like o implementation-like. Conserva el ranking BM25 actual sobre `search_text`.
- El contrato no mezcla documentos y símbolos en la misma lista.
- La ruta documental no interpola la pregunta original en `next_queries`: continúa únicamente con `doc_id`/rutas canónicas o con un diagnóstico fijo de workspace.

## Ejecución y deadlines

```toon
block_id: CT-NAV-INTENT.execution-routing
kind: runtime-routing-contract
source_of_truth: normative
verify:
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
routing:
  current_route: direct
  no_daemon:
    flag: --no-daemon
    semantics: force_direct_local_execution
    connects_to_daemon: false
    starts_daemon: false
  no_auto_daemon:
    flag: --no-auto-daemon
    semantics: suppress_auto_start_only
    daemon_aware_operations_may_connect_existing: true
    unavailable_daemon: direct_fallback
  dial_timeout:
    helper: ExecuteWithDialTimeout
    scope: dial_only
    post_dial_context: original_request_context
deadlines:
  dial_attempt: short_timeout_only
  write_read_processing: original_request_context
  cancellation: original_request_context
```

`nav intent` permanece en el lane directo definido por la política actual. `--no-daemon` no conecta ni inicia daemon. `--no-auto-daemon` solo impide el auto-start; no convierte por sí solo una operación daemon-aware en direct mode. Cuando existe un intento daemon-aware, el timeout corto se limita al dial y no acorta write, read ni procesamiento bajo el contexto original.

## Selectores y resolución

```toon
block_id: CT-NAV-INTENT.selectors
kind: selector-contract
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - internal/service/intent.go
  - internal/service/intent_test.go
selector_precedence:
  payload:
    explicit_fields_win: [selector, symbol, from, to, edge]
    normalization: repository_relative_separators_to_slash
  question:
    fallback: only_when_payload_selector_fields_are_empty
    symbol_shape: capitalized_symbol_only
    ignored_route_words: [Callers, Caller, Callees, Callee, Explain, Edge, Path, Between, Neighborhood, Related, Change, Impact, What, Who]
relative_path:
  accepted: exactly_one_explicit_repository_relative_file_path
  separators: slash_or_backslash
  normalized: slash
  quoted: accepted_when_safe_after_sanitization
  unsafe: [absolute_path, traversal, url, shell_payload, unsafe_characters]
  ambiguity: bounded_no_auto_selection
  capitalized_fragment_substitution: forbidden
resolution:
  ambiguous: candidates_without_auto_selection
  missing: preserve_INTENT_ARGUMENT_MISSING_behavior
```

En una pregunta como `neighborhood of internal/service/wiki_code_context.go`, una única ruta relativa explícita se conserva como selector. Las rutas con backslash se normalizan a slash y las rutas entre comillas siguen el mismo saneamiento. Varias rutas, rutas absolutas, traversal, URLs y payloads shell no habilitan la selección de un fragmento capitalizado. Un símbolo CamelCase existente conserva su comportamiento y un selector ausente conserva el diagnóstico de argumento faltante.

## Envelope legacy

```toon
block_id: CT-NAV-INTENT.legacy-envelope
kind: response-contract
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/06_pruebas/TP-QRY.md
legacy:
  backend: intent
  modes: [docs, code]
  docs_item_fields: [doc_path, doc_id, title, family, layer, score, evidence, next_queries]
  code_item_fields: [file, line, symbol, kind, qualified_name, score, evidence, snippet]
  repo_scope:
    docs: validate_but_do_not_redefine_document_lane
    code: filter_symbol_universe_in_container_workspace
```

### Ejemplo documental compatible

```json
{
  "ok": true,
  "workspace": "mi-lsp",
  "backend": "intent",
  "mode": "docs",
  "items": [{
    "doc_path": ".docs/wiki/09_contratos/CT-NAV-INTENT.md",
    "doc_id": "CT-NAV-INTENT",
    "title": "Contrato CLI nav intent",
    "family": "CT",
    "layer": "09",
    "score": 10,
    "evidence": ["tier1_canonical_route"],
    "next_queries": [
      "mi-lsp nav search \"CT-NAV-INTENT\" --include-content --workspace mi-lsp",
      "mi-lsp nav multi-read \".docs/wiki/09_contratos/CT-NAV-INTENT.md:1-120\" --workspace mi-lsp"
    ]
  }],
  "warnings": [],
  "stats": {"files": 1},
  "truncated": false
}
```

### Ejemplo de código compatible

Para consultas de símbolos, el mismo envelope usa `mode=code` y los items contienen `file`, `line`, `symbol`, `kind`, `qualified_name`, `score`, `evidence` y `snippet`.

## Planner graph-native

```toon
block_id: CT-NAV-INTENT.planner-preview
kind: intent-plan-contract
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
  - go test ./internal/service/... ./internal/telemetry/...
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-QRY.md
routing:
  entrypoints: [nav.intent, nav.ask]
  policy: automatic_mi_lsp_first_no_opt_out
  supported: [callers, callees, affected-change, path-between, explain-edge, neighborhood, explain-change]
plan:
  type: IntentPlan
  required_fields: [intent, operation, arguments, confidence, freshness, preview, omissions, fallbacks, expansions, telemetry]
  mode: preview
  digest: deterministic_excluding_telemetry
  freshness: graph_generation_bound_or_catalog_current
preview:
  explain_change_sections: [change, affected, callers, callees, tests, contracts, wiki]
  empty_section: omission_or_fallback_required
  completeness_claim: forbidden_without_evidence
all_workspaces:
  supported: true
  result_type: []IntentPlan
  workspace_field: required_on_every_plan
  ordering: deterministic_by_workspace_then_intent_operation_digest_arguments
expansions:
  item_shape: [command, reason]
  command_prefix: mi-lsp nav
  split_command_reason: forbidden
arguments:
  explain_change:
    paths: repeatable_normalized_workspace_relative_values
    ref: preserved_and_safely_quoted_when_present
  graph_operations:
    generation: preserved_when_present
  incomplete_path: executable_search_discovery_only
fallbacks:
  terminal_allowlist: [unsupported_operation, unavailable_binary, invalid_workspace, explicit_incomplete]
  detail: canonical_fixed_derived_from_reason_code_never_raw_input
  internal_degradation: omission_only
  timeout_without_diagnostic: blocked
telemetry:
  persisted: derived_metadata_only
  forbidden: [query, prompt, argv, payload, snippet, content, raw_path, raw_error]
```

Las expansiones no usan `strconv.Quote`, `%q` ni quoting dependiente de shell. Valores que no pertenecen a una allowlist portable se reemplazan en `command` por placeholders inertes como `__MI_LSP_ARG_SELECTOR__` y se entregan en `arguments` estructurados para binding sin shell. Esto también aplica a `workspace`, `repo`, paths, selectors y refs.

`IntentFallback` solo representa una degradación terminal externa. Las omisiones internas —backend heurístico, generation ausente/stale, consulta parcial, timeout diagnosticado o miembro no disponible— permanecen en `omissions[]` y no se publican como fallback externo. Timeout, silencio, `DONE` o `PASS` sin diagnóstico fresco no habilitan ningún fallback.

### Ejemplo planner compatible

```json
{
  "ok": true,
  "workspace": "mi-lsp",
  "backend": "planner",
  "mode": "preview",
  "items": [{
    "intent": "callers",
    "operation": "callers",
    "arguments": {"selector": "HandleRequest"},
    "confidence": 0.9,
    "freshness": "graph-generation-bound",
    "preview": [{"section": "callers", "items": [], "count": 0}],
    "omissions": [],
    "fallbacks": [],
    "expansions": [{"command": "mi-lsp nav callers HandleRequest --workspace mi-lsp --format toon --full", "reason": "expand the selected graph section with the same selector and generation"}],
    "telemetry": {"planner_version": "intent-v1", "operation": "callers"}
  }],
  "warnings": ["automatic intent routing selected a local deterministic planner"],
  "truncated": false
}
```

## Continuación, preview y enriquecimiento wiki-code

```toon
block_id: CT-NAV-INTENT.continuation-and-enrichment
kind: continuation-contract
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - internal/service/wiki_code_context.go
  - internal/service/wiki_code_enrich.go
preview:
  cheap: true
  next_hint: existing_bounded_full_expansion
  eager_wiki_code_context_for_nav_intent: false
continuation:
  next_queries: canonical_doc_id_or_path_only_for_legacy
  planner_expansions: executable_existing_nav_surface
wiki_code_context:
  enriched_operations: [nav.ask, nav.route, nav.wiki.route, nav.pack, nav.wiki.pack, nav.context, nav.affected, nav.diff-context, nav.workspace-map]
  primary: active_canonical_wiki_document_selected_by_existing_operation
  query_only: true
  writes: forbidden
  direct_code: declared_bindings_only
  tests: separate_from_direct_code
  supporting: graph_current_only
  candidates: never_promoted_to_direct_code
  freshness: independent_docs_and_graph_generations
  unavailable_graph: typed_omission_with_docs_first_authority
compatibility:
  public_BuildWikiCodeContext: preserved
  enrichment_catalog: one_request_local_document_set_reused
```

En AXI preview, `nav intent` conserva `backend` y `mode` y anuncia mediante `next_hint` una continuación ejecutable y acotada hacia `--full`. El harness puede seguir esa continuación hacia las superficies existentes de wiki pack, trace o related. `nav.intent` no activa enriquecimiento wiki-code de forma anticipada para cada intent.

El enriquecimiento reutiliza el conjunto documental ya cargado por la operación. No introduce caché, estado nuevo, snapshot compartido ni debilitamiento de resolución. Un documento canónico primario no se reemplaza por un documento conectado; la evidencia supporting solo se publica cuando el grafo actual la respalda; frescura, omisiones tipadas y autoridad docs-first permanecen sin cambios.

## Bindings de implementación

```toon
block_id: CT-NAV-INTENT.artifact-bindings
kind: contract-bindings
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - internal/service/intent.go
  - internal/service/intent_test.go
  - internal/service/wiki_code_context.go
  - internal/service/wiki_code_enrich.go
  - internal/service/wiki_code_enrich_test.go
artifact_bindings:
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: implements
    role: handler
    target_path: internal/service/intent.go
    target_symbol: (*App).intent
    target_kind: symbol
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: 3a687ae014914f608ef11e5370121b5efb2d95cab553c4418891b17f68ab25c7
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: tests
    role: executable_test
    target_path: internal/service/intent_test.go
    target_symbol: TestExtractIntentSelectorSupportsExplicitRelativePathsWithoutGuessing
    target_kind: test
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: cedb0f806bc3d2b8b9923943d062d69c657c7225ebde02db3b1e1d3ab8093f89
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: implements
    role: selector
    target_path: internal/service/intent.go
    target_symbol: extractIntentSelector
    target_kind: symbol
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: 47ce5f0de97590939fee500217ce47e2b245f9ab41e3a758bb4e82252bf67972
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: implements
    role: adapter
    target_path: internal/service/wiki_code_context.go
    target_symbol: BuildWikiCodeContext
    target_kind: symbol
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: e0ec3b2827604ab588eb800b8c75d768c4f361a8df8dfcaf33cecb5f696fe71c
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: implements
    role: adapter
    target_path: internal/service/wiki_code_context.go
    target_symbol: buildWikiCodeContextWithDocs
    target_kind: symbol
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: 62805352aacf33ccabcd1c61a62b4f8cd4c2ab0ca92c76cc289408bf0429e99d
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: implements
    role: adapter
    target_path: internal/service/wiki_code_enrich.go
    target_symbol: (*App).enrichWikiCodeContext
    target_kind: symbol
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: 6bed744ca2589cbfe99003533519a295e75b9f8a486e2911237dfb5ba35479c4
  - doc_path: .docs/wiki/09_contratos/CT-NAV-INTENT.md
    block_id: CT-NAV-INTENT.artifact-bindings
    doc_id: CT-NAV-INTENT
    relation: tests
    role: executable_test
    target_path: internal/service/wiki_code_enrich_test.go
    target_symbol: TestEnrichWikiCodeContextMatchesDirectBuilderAndRejectsUnavailablePrimaries
    target_kind: test
    authoring_origin: canonical
    binding_status: exact
    doc_lifecycle: active
    binding_ref: cc1318da6504030cefafbecbebe679a36e1cace551b8a301e311dd348df22a88
binding_rules:
  exact_paths:
    required_at: implementation_close
  bindings_are_not_conformance: true
  query_writes: forbidden
```

## Reglas observables y compatibilidad

```toon
block_id: CT-NAV-INTENT.compatibility
kind: compatibility-contract
source_of_truth: normative
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
rules:
  docs_repo_selector: validate_but_do_not_redefine_document_lane
  code_repo_selector: filter_container_workspace_symbols
  canonical_doc_positive: README_and_generic_docs_cannot_lead
  graph_omission_reason: stable_code_only_after_sanitization
  explain_change_ref: normalize_ref_to_changed_ref_and_use_git_snapshot_only_without_explicit_paths
  explain_change_sections: exactly_seven
  planner_fallbacks: explicit_array_even_when_empty
  selector_ambiguity: bounded_candidates_without_auto_selection
  no_new_public_command: true
  no_general_docs_as_sdd_canon: true
diagnostics:
  doc_ranking: MI_LSP_DOC_RANKING=owner|legacy
  telemetry: [doc_ranker, intent_mode]
```

- `mode=docs` valida `--repo` pero no redefine el lane documental; puede emitir warning visible.
- `mode=code` filtra el universo de símbolos en workspaces `container`.
- Si existe un candidato documental canónico positivo, `README` y otros docs `generic` no lideran la lista.
- En `backend=planner`, `explain-change` conserva exactamente las siete secciones `change`, `affected`, `callers`, `callees`, `tests`, `contracts` y `wiki`; las secciones vacías conservan omisión explícita y `expansions[]` conserva comandos ejecutables con razón.
- Las expansiones planner preservan el `--repo` seleccionado dentro de `arguments` y en el comando. Una expansión `affected-change` con rutas explícitas conserva esas rutas y su quoting determinista; no las sustituye por `--from-git-diff`.
- Los warnings de catálogo/SQLite que cruzan el envelope planner usan códigos estables y nunca incluyen `err.Error()`, roots, rutas de DB, secretos o PII.

## RF y TP asociados

- RF-QRY-001
- RF-QRY-011
- RF-QRY-014
- RF-QRY-015
- TP-QRY

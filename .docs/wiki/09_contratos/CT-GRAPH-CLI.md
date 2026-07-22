---
doc_id: CT-GRAPH-CLI
title: Contrato CLI graph-native
layer: CT
family: GRAPH-CLI
status: implemented
---

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
id: "CT-GRAPH-CLI"
doc_id: "CT-GRAPH-CLI"
kind: "contract"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[09_contratos_tecnicos]]'
  - '[[RF-GPH-005]]'
  - '[[RF-GPH-006]]'
  - '[[RF-GPH-007]]'
  - '[[RF-GPH-009]]'
  - '[[RF-GPH-011]]'
  - '[[TECH-GRAPH-NATIVE]]'
  - '[[DB-SYMBOL-EDGE-GRAPH]]'
exports:
  - 'CT-GRAPH-CLI'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos_tecnicos.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-006.md
  - .docs/wiki/04_RF/RF-GPH-007.md
  - .docs/wiki/04_RF/RF-GPH-009.md
  - .docs/wiki/04_RF/RF-GPH-011.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
agent_may_edit:
  - .docs/wiki/09_contratos_tecnicos.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-GRAPH-CLI.md --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-GRAPH-CLI.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - direct_daemon_semantics_diverge=true
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
```

Volver a [09_contratos_tecnicos.md](../09_contratos_tecnicos.md).

## Boundary

Contrato de CLI para consultar generations publicadas y enrutar intenciones graph-native. La CLI es publica; daemon, SQLite y adapters son internos. La superficie de comandos esta implementada en el runtime actual; la disponibilidad de una generation o backend graph concreto puede degradar de forma visible y no convierte heuristica en equivalencia exacta.

## Comandos v1

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.commands
kind: cli-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
commands:
  nav.neighbors:
    shape: mi-lsp nav neighbors <selector> [common-graph-flags]
    semantics: bounded-neighborhood
  nav.callers:
    shape: mi-lsp nav callers <selector> [common-graph-flags]
    edge: calls-inbound
  nav.callees:
    shape: mi-lsp nav callees <selector> [common-graph-flags]
    edge: calls-outbound
  nav.path:
    shape: mi-lsp nav path <from-selector> <to-selector> [common-graph-flags]
    semantics: shortest-edge-count-with-stable-tiebreak
  nav.explain:
    shape: mi-lsp nav explain <selector-or-edge-cross-rid> [common-graph-flags]
    semantics: evidence-chain
  nav.graph.stats:
    shape: mi-lsp nav graph stats [--generation <id>] [--all-workspaces]
  nav.graph.validate:
    shape: mi-lsp nav graph validate [--generation <id>]
    writes: forbidden
  nav.affected:
    compatibility: existing-command-enriched-additively
    graph_flags: [--generation, --mode-direct-or-transitive, --depth, --limit, --token-budget]
  nav.diff-context:
    compatibility: existing-command-enriched-additively
  nav.related:
    compatibility: existing-shape-preserved-graph-native-backend-visible
  nav.workspace-map:
    compatibility: existing-shape-preserved-graph-summary-additive
  nav.intent:
    shape: mi-lsp nav intent <question> [common-graph-flags]
    semantics: automatic-intent-first; supported graph intents have no opt-out
  nav.explain-change:
    shape: mi-lsp nav explain-change [question] [--path <path> ...] [--ref <git-ref>] [common-graph-flags]
    status: implemented
    sections: [change, affected, callers, callees, tests, contracts, wiki]
    preview: bounded-seven-section-with-executable-expansions
    full: expansion-request; may remain preview when no additional evidence is available
```

## Routing de intencion y explain-change

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.intent-routing
kind: intent-contract
source_of_truth: this
evidence:
  - .docs/wiki/06_pruebas/TP-GPH.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
verify:
  - go run ./cmd/mi-lsp nav intent --help
  - go run ./cmd/mi-lsp nav explain-change --help
  - go run ./cmd/mi-lsp nav explain-change --path internal/service/intent.go --workspace <alias> --format toon
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --ids CT-GRAPH-CLI --format toon
intent_routing:
  policy: automatic_mi_lsp_first_no_opt_out
  supported: [callers, callees, affected-change, path-between, explain-edge, neighborhood, explain-change]
  seven_sections: [change, affected, callers, callees, tests, contracts, wiki]
  required_plan_fields: [intent, operation, arguments, confidence, freshness, preview, omissions, fallbacks, expansions, telemetry]
  expansion_fields: [command, reason]
  terminal_fallbacks: [unsupported_operation, unavailable_binary, invalid_workspace, explicit_incomplete]
  timeout_without_diagnostic: blocked
  selector_ambiguity: candidates_without_auto_selection
  wiki: must_read_governance_and_contracts_before_may_read_specs_or_tests
```

`nav graph stats` y `nav graph validate` son comandos read-only reales; `nav neighbors`, `nav callers`, `nav callees`, `nav path` y `nav explain` tambien tienen wiring CLI real. La consulta puede devolver `GPH_QUERY_BACKEND_UNAVAILABLE` o fallback explicito si no existe una generation publicada; eso es disponibilidad runtime, no ausencia del comando.

## Selector y flags comunes

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.inputs
kind: input-contract
source_of_truth: RF-GPH-005
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
selector_precedence:
  - NodeKey-lowercase-hex
  - cross-RID
  - exact-symbol-or-document-id
  - scoped-name
ambiguity: return-bounded-candidates-never-auto-pick
flags:
  generation:
    default: active-fixed-at-request-start
    allowed: published-generation-only
  depth:
    default: 1
    max: 6
    path_max: 12
  limit:
    default: 50
    max: 500
  token_budget:
    default: 4000
    max: 20000
  direction:
    values: [in, out, both]
  edge:
    repeatable: true
    values: registered-relation-types
  cursor:
    binding: [generation, operation, selectors, filters, ordering]
  all_workspaces:
    default: false
    authority: per-workspace-governance
invalid_budget: reject-before-store-read
```

`--format compact|json|text|toon|yaml`, `--compress`, workspace/repo selectors y flags globales conservan su contrato existente. No se agrega modo MCP ni HTTP.

## Seleccion de daemon y ciclo de request

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.execution-routing
kind: runtime-routing-contract
source_of_truth: this
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
verify:
  - go test ./internal/cli/... ./internal/daemon/...
routing:
  no_daemon:
    flag: --no-daemon
    semantics: force_direct_local_execution
    connects_to_daemon: false
    starts_daemon: false
  no_auto_daemon:
    flag: --no-auto-daemon
    semantics: suppress_auto_start_only
    existing_daemon_connection: allowed
    unavailable_daemon: direct_fallback
  daemon_attempt_timeout:
    helper: ExecuteWithDialTimeout
    scope: dial_only
    write_read_processing_context: original_request_context
  cancellation:
    connection_close_hook: original_request_context
    write_read_processing: obey_original_request_context
```

`--no-daemon` es una orden de ejecucion directa: no intenta conectar al daemon y tampoco lo inicia. `--no-auto-daemon` solo evita el auto-start; una operacion daemon-aware aun puede conectar a un daemon ya existente y, si no esta disponible, degradar a direct mode. El timeout corto de intento se aplica unicamente al dial mediante `ExecuteWithDialTimeout`; despues de conectar, write, read, procesamiento y cancelacion siguen gobernados por el contexto original de la request.

## Envelope v1

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.envelope
kind: output-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
envelope:
  inherited_required: [ok, backend, workspace, items, warnings, stats, truncated]
  graph_required_when-executed:
    - operation
    - generation_id
    - graph_schema_version
    - determinism_digest
    - omissions
  backend_values:
    - sqlite-direct
    - daemon/sqlite
    - graph-native+heuristic
    - legacy-heuristic
item:
  common:
    - kind
    - cross_rid
    - display
    - status
    - distance
    - evidence_refs
  node:
    - node_key
    - symbol_kind
    - owner_path
  edge:
    - edge_cross_rid
    - from_cross_rid
    - to_cross_rid
    - relation
    - confidence_class
  impact:
    - path
    - reason
    - evidence_path
    - trigger_path
    - change_type
stats:
  - visited
  - frontier
  - returned
  - depth_reached
  - token_units
  - unresolved
  - elapsed_ms-nondeterministic-not-in-digest
continuation:
  required_when: truncated
  cursor: opaque-generation-bound
```

Ordering canonico: distance, confidence class (`exact`, `extracted`, `inferred`, `heuristic`), relation, display casefold + original, NodeKey/edge key y evidence digest. Timings, host path y daemon runtime metadata quedan fuera del determinism digest.

## Errores

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.errors
kind: error-contract
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
errors:
  GPH_QUERY_GENERATION_NOT_FOUND: generation-missing-invalid-or-cleaned
  GPH_QUERY_SELECTOR_NOT_FOUND: empty-result-with-hint
  GPH_QUERY_SELECTOR_AMBIGUOUS: candidates-and-refinement-required
  GPH_QUERY_BUDGET_INVALID: reject-before-read
  GPH_QUERY_CURSOR_STALE: restart-query
  GPH_QUERY_GRAPH_INVALID: graph-native-blocked
  GPH_QUERY_BACKEND_UNAVAILABLE: actionable-store-error
  GPH_IMPACT_SEED_UNRESOLVED: omission-or-visible-legacy-fallback
  GPH_IMPACT_BASELINE_REGRESSION: quality-gate-fail
  GPH_WIKI_GOVERNANCE_BLOCKED: diagnosis-and-repair-only
  GPH_WIKI_CODE_DRIFT: report-both-sides-no-override
  GPH_GLOBAL_MEMBER_UNAVAILABLE: partial-result-with-member-omission
  GPH_GLOBAL_CROSS_RID_CONFLICT: reject-cross-edge
shape:
  ok: false
  error_fields: [kind, code, message, stage, retryable, hint]
  raw_backend_log: forbidden
```

Empty result, truncation y unavailable member no son automaticamente process failure. El envelope debe diferenciar success parcial, error de input, graph invalid y fallback legacy.

## Compatibilidad y degradacion

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.compatibility
kind: compatibility
source_of_truth: this
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
rules:
  direct-daemon: same-canonical-items-order-errors-and-omissions
  no-daemon: direct-only-without-daemon-connect-or-start
  no-auto-daemon: suppress-start-but-existing-daemon-connect-remains-allowed
  daemon-dial-timeout: dial-only; original-request-context-after-connect
  daemon-unavailable: direct-mode
  graph-not-yet-built: existing-command-may-use-legacy-with-explicit-backend-warning
  graph-stale-or-invalid: no-silent-legacy-equivalence
  existing-fields: preserved
  new-fields: additive
  query-time-schema-migration: forbidden
  graph-store-write: forbidden
  wiki-authority: unchanged
  MCP: forbidden
  implicit-network: forbidden
federation:
  member-generations: required-in-envelope
  unavailable-members: explicit
  scope: registered-and-caller-authorized-only
```

## Rank, cache, utility y comportamiento bounded

```toon
doc_id: CT-GRAPH-CLI
block_id: CT-GRAPH-CLI.analysis-runtime
kind: graph-analysis-contract
source_of_truth: TECH-GRAPH-NATIVE
imports:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md
evidence:
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
  - .docs/wiki/06_pruebas/TP-GPH.md
verify:
  - go test ./internal/model/... ./internal/service/... ./internal/store/...
  - mi-lsp nav graph stats --workspace <alias> --format toon
  - mi-lsp nav graph validate --workspace <alias> --format toon
stop_if:
  - graph_claim_without_freshness_current=true
  - truncated_analysis_persisted=true
  - generation_mismatch_accepted=true
rank:
  algorithm: bounded-deterministic-v1
  version: "1"
  profile: exact-extracted-only
  score: 0.45*authority + 0.25*impact + 0.20*centrality + 0.10*boundary
  utility: last_tie_break_only
  community: deterministic_connected_components_v1
  community_id: sha256_sorted_node_keys
freshness:
  states: [current, lagging, stale, invalid, unknown]
  exact_claims_allowed_only: current
generation:
  fixed: request_start
  cache_must_match: [generation, algorithm, version, profile, parameter_digest, authority_digest, result_digest]
bounds:
  max_nodes: 10000
  max_edges: 50000
  max_bytes: 65536
  query: [depth, path_depth, result_limit, token_budget]
cache:
  operation: rank
  persist_status: complete_only
  truncated_results: never_persist
  miss_on: [generation, analysis_key, algorithm, version, profile, parameter_digest, authority_digest, result_digest, byte_limit]
utility:
  signals: [continuation_followed, feedback_positive, feedback_negative, result_selected]
  max_events_per_candidate: 4096
  max_events_per_scope_intent_operation: 4096
  retention_days: 30
  candidate_identity: node_key_only
snapshot:
  reads: bounded_storage_backed
  full_graph_in_memory: forbidden
  transitive_closures_persisted: forbidden
  query_time_migration: forbidden
sqlite:
  write_max_open_conns: 1
  read_only_pool: {max_open_conns: 8, max_idle_conns: 4}
parity:
  direct_daemon: same_items_order_errors_omissions
```

## Ejemplo compacto

```json
{
  "ok": true,
  "backend": "sqlite-direct",
  "workspace": "mi-lsp",
  "operation": "nav.callers",
  "generation_id": "<sha256-hex>",
  "graph_schema_version": 1,
  "items": [
    {
      "kind": "edge",
      "edge_cross_rid": "milsp:gph-edge:v1:<hex>",
      "from_cross_rid": "milsp:gph-node:v1:<hex>",
      "to_cross_rid": "milsp:gph-node:v1:<hex>",
      "relation": "calls",
      "status": "exact",
      "evidence_refs": ["milsp:gph-evidence:v1:<hex>"]
    }
  ],
  "warnings": [],
  "omissions": [],
  "stats": {"visited": 2, "returned": 1, "token_units": 42},
  "truncated": false,
  "determinism_digest": "sha256:<hex>"
}
```

## Sync

RF owners: `[[RF-GPH-005]]`, `[[RF-GPH-006]]`, `[[RF-GPH-007]]`, `[[RF-GPH-009]]`, `[[RF-GPH-011]]`. Tests: `[[TP-GPH]]`. Runtime: `[[TECH-GRAPH-NATIVE]]`. Store: `[[DB-SYMBOL-EDGE-GRAPH]]`.

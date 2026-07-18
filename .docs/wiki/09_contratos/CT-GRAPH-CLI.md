---
doc_id: CT-GRAPH-CLI
title: Contrato CLI graph-native
layer: CT
family: GRAPH-CLI
status: accepted-design
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

Contrato aditivo de CLI para consultar generations publicadas. La CLI es publica; daemon, SQLite y adapters son internos. El estado `accepted-design` no habilita comandos hasta que sus slices y goldens esten implementados.

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
```

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

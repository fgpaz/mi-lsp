# CT-NAV-INTENT

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-INTENT"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-NAV-INTENT]]'
exports:
  - 'CT-NAV-INTENT'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
```

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav intent`

## Forma de invocacion

```text
mi-lsp nav intent <question> [--workspace <alias>] [--repo <name>] [--top N] [--offset N] [--full]
```

## Semantica

`nav intent` conserva `backend=intent`, pero expone `mode=docs|code`.

- `mode=docs`: consultas capability-like, contract-like, flow-like o docs-first. Usa el scorer documental owner-aware compartido con `nav route`, `nav ask` y `nav pack`.
- `mode=code`: consultas symbol-like o implementation-like. Conserva el ranking BM25 actual sobre `search_text`.

El contrato no mezcla docs y simbolos en la misma lista.

## Payload logico

- `question`: string requerido
- `workspace`: alias o path resoluble
- `repo`: selector opcional de repo para workspaces `container`
- `top`: entero opcional
- `offset`: entero opcional

## Respuesta

El envelope incluye:

- `backend=intent`
- `mode=docs|code`
- `items`
- `warnings`
- `stats`

En `mode=docs`, cada item contiene:

- `doc_path`
- `doc_id`
- `title`
- `family`
- `layer`
- `score`
- `evidence`
- `next_queries`

En `mode=code`, cada item contiene:

- `file`
- `line`
- `symbol`
- `kind`
- `qualified_name`
- `score`
- `evidence`
- `snippet`

## Reglas observables

- Si `mode=docs`, `--repo` se valida pero no redefine el lane documental; puede quedar warning visible.
- Si `mode=code`, `--repo` filtra el universo de simbolos en workspaces `container`.
- Si ya existe un candidato documental canonico positivo, `README` y otros docs `generic` no deben liderar la lista.
- En AXI preview, `nav intent` mantiene `backend` y `mode`, y anuncia expansion via `next_hint` hacia `--full`.

## Diagnostico

- `MI_LSP_DOC_RANKING=owner|legacy` permite comparar el scorer owner-aware contra el camino legacy sin cambiar el contrato publico.
- La telemetria puede registrar solo metadata derivada: `doc_ranker` e `intent_mode`.

## RF asociados

- RF-QRY-001
- RF-QRY-011
- RF-QRY-014
- RF-QRY-015

## Contrato Harness-first de planificacion

```toon
doc_id: CT-NAV-INTENT
block_id: CT-NAV-INTENT.harness-first-plan
kind: intent-plan-contract
source_of_truth: this
imports:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
exports:
  - IntentPlan
  - automatic_graph_native_routing
  - explain_change_preview
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/06_pruebas/TP-QRY.md
verify:
  - go test ./internal/service/... ./internal/telemetry/...
  - mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-INTENT --format toon
stop_if:
  - governance_blocked=true
  - timeout_without_typed_diagnostic=true
  - ambiguous_selector_auto_selected=true
routing:
  entrypoints: [nav.intent, nav.ask]
  policy: automatic_mi_lsp_first_no_opt_out
  supported: [callers, callees, affected-change, path-between, explain-edge, neighborhood, explain-change]
plan:
  type: IntentPlan
  required_fields: [intent, operation, arguments, confidence, freshness, preview, omissions, fallbacks, expansions, telemetry]
  mode: preview
  digest: deterministic_excluding_telemetry
all_workspaces:
  supported: true
  result_type: []IntentPlan
  workspace_field: required_on_every_plan
  ordering: deterministic_by_workspace_then_intent_operation_digest_arguments
preview:
  explain_change_sections: [change, affected, callers, callees, tests, contracts, wiki]
  empty_section: omission_or_fallback_required
  completeness_claim: forbidden_without_evidence
expansions:
  item_shape: {command: string, reason: string}
  command_prefix: "mi-lsp nav "
  split_command_reason: forbidden
arguments:
  explain_change:
    paths: repeatable_normalized_workspace_relative_values
    ref: preserved_and_safely_quoted_when_present
  graph_operations:
    generation: preserved_when_present
  incomplete_path: executable_search_discovery_only
selectors:
  ambiguous: bounded_candidates_without_auto_selection
fallbacks:
  terminal: [unsupported_operation, unavailable_binary, invalid_workspace, explicit_incomplete]
  timeout_without_diagnostic: blocked
telemetry:
  persisted: derived_metadata_only
  forbidden: [query, prompt, argv, payload, snippet, content, raw_path, raw_error]
```

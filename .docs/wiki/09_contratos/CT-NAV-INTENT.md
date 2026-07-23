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

## Ejecucion y deadlines

```toon
doc_id: CT-NAV-INTENT
block_id: CT-NAV-INTENT.execution-routing
kind: runtime-routing-contract
source_of_truth: this
evidence:
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/09_contratos/CT-GRAPH-CLI.md
routing:
  current_route: direct
  no_daemon:
    semantics: force_direct_local_execution
    connects_to_daemon: false
    starts_daemon: false
  no_auto_daemon:
    semantics: suppress_auto_start_only
    daemon_aware_operations_may_connect_existing: true
  dial_timeout:
    helper: ExecuteWithDialTimeout
    scope: dial_only
    post_dial_context: original_request_context
```

`nav intent` permanece en el lane directo definido por la politica actual. El flag global `--no-daemon` conserva la garantia fuerte de no conectar ni iniciar daemon; `--no-auto-daemon` no convierte por si solo una operacion daemon-aware en direct mode, solo impide su auto-start. Cuando existe un intento daemon-aware, el timeout corto se limita al dial y no acorta write, read ni procesamiento bajo el contexto original.

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

## Envelopes de `nav intent`

La superficie conserva dos envelopes publicos y no mezcla sus lanes. El primer envelope es el camino legacy documental/codigo; el segundo es el planner graph-native automatico.

### Legacy: `backend=intent`, `mode=docs|code`

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
    "next_queries": ["mi-lsp nav search \"CT-NAV-INTENT\" --include-content --workspace mi-lsp", "mi-lsp nav multi-read \".docs/wiki/09_contratos/CT-NAV-INTENT.md:1-120\" --workspace mi-lsp"]
  }],
  "warnings": [],
  "stats": {"files": 1},
  "truncated": false
}
```

Para consultas de simbolos, el mismo envelope usa `mode=code` y los items contienen `file`, `line`, `symbol`, `kind`, `qualified_name`, `score`, `evidence` y `snippet`. El lane legacy nunca interpola la pregunta original en `next_queries`: continua únicamente con `doc_id`/ruta canónicos o con un diagnóstico fijo de workspace.

### Graph-native: `backend=planner`, `mode=preview`

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

Las expansiones no usan `strconv.Quote`, `%q` ni quoting dependiente de shell. Valores que no pertenecen a una allowlist portable se reemplazan en `command` por placeholders inertes como `__MI_LSP_ARG_SELECTOR__` y se entregan en `arguments` estructurados para binding sin shell. Esto aplica también a `workspace`, `repo`, paths, selectors y refs.

`IntentFallback` solo representa una degradacion terminal externa. Su forma es `reason_code` (allowlist exacta: `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, `explicit_incomplete`) y `detail` separado. El `detail` no es entrada libre: el constructor y `MarshalJSON` rechazan códigos fuera de la allowlist y serializan únicamente un detalle canónico fijo derivado del `reason_code`, nunca el valor arbitrario de `Detail`. En todo planner preview, `fallbacks` aparece como array explícito (`[]` cuando está vacío). Una degradacion interna no terminal —backend heuristico, generation ausente/stale, consulta parcial, timeout diagnosticado o miembro no disponible— se expresa en `omissions[]`; no se publica como fallback externo. Timeout, silencio, `DONE` o `PASS` sin diagnostico fresco no habilitan ningun fallback.

## Reglas observables

- Si `mode=docs`, `--repo` se valida pero no redefine el lane documental; puede quedar warning visible.
- Si `mode=code`, `--repo` filtra el universo de simbolos en workspaces `container`.
- Si ya existe un candidato documental canonico positivo, `README` y otros docs `generic` no deben liderar la lista.
- En AXI preview, `nav intent` mantiene `backend` y `mode`, y anuncia expansion via `next_hint` hacia `--full`.
- En `backend=planner`, `explain-change` conserva exactamente siete secciones: `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, `wiki`; las secciones vacias se mantienen con omission explicita y `expansions[]` siempre conserva comandos ejecutables con razon.
- Las expansiones planner preservan el `--repo` seleccionado dentro de `arguments` y en el comando. Una expansion `affected-change` con rutas explicitas conserva esas rutas y su quoting determinista; no las sustituye por `--from-git-diff`.
- Los warnings de catalogo/SQLite que cruzan el envelope planner usan codigos estables y nunca incluyen `err.Error()`, roots, rutas de DB, secretos o PII.
- Las omisiones `graph_unresolved`/`GPH_*` publican sólo el código estable; su `Reason` no cruza desde el backend sin sanitización.
- `--ref` de `explain-change` se normaliza a `changed_ref` para la expansión affected y activa el snapshot Git sólo cuando no hay paths explícitos.

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
  terminal:
    allowlist: [unsupported_operation, unavailable_binary, invalid_workspace, explicit_incomplete]
    fields: [reason_code, detail]
    detail: canonical_fixed_derived_from_reason_code_never_raw_input
  internal_degradation: omission_only
  timeout_without_diagnostic: blocked
telemetry:
  persisted: derived_metadata_only
  forbidden: [query, prompt, argv, payload, snippet, content, raw_path, raw_error]
```

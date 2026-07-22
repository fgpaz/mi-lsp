---
linear_parent: not_applicable_user_authorized
linear_child: not_applicable_user_authorized
anchors: [FL-QRY-01, FL-WIKI-01, RF-QRY-017, RF-GPH-006, RF-GPH-011, CT-GRAPH-CLI, CT-NAV-WIKI]
allowed_paths: [internal/model/**, internal/service/intent.go, internal/service/ask.go, internal/service/related.go, internal/service/wiki_code_context.go, internal/service/affected.go, internal/service/diff_context.go, internal/cli/nav.go, internal/daemon/**]
forbidden_paths: [network clients, MCP, external Graphify runtime]
verify: [go test ./internal/service/... ./internal/cli/... ./internal/daemon/...]
stop_if: [intent planner requires network, fallback is silent, preview has no executable continuation]
secret_scan: {required: true, evidence: names-only-no-values}
---
# Task T3: Routing automático y explain-change

## Shared Context
**Goal:** G3, convertir una intención en síntesis graph-native + wiki.
**Stack:** Go service/CLI/daemon.
**Architecture:** planner local determinista selecciona operaciones; preview compacto y expansión explícita comparten generation snapshot.

## Locked Decisions
- Routing automático por intención es obligatorio y sin opt-out para intenciones soportadas.
- Primer journey: explicar cambio con impacto, riesgos, callers/callees, tests, contratos y wiki dirigida.

## Task Metadata
```yaml
id: T3
depends_on: [T1, T2]
agent_type: claudex-writer
goal_id: G3
github_issues: []
expected_outcome: "una consulta explain-change devuelve preview útil y próximos comandos razonados."
files:
  - modify: internal/service/intent.go
  - modify: internal/service/ask.go
  - modify: internal/service/related.go
  - modify: internal/service/wiki_code_context.go
  - modify: internal/cli/nav.go
  - modify: internal/daemon/**
complexity: high
done_when:
  - "intent golden tests exit 0"
  - "direct and daemon envelopes are equivalent"
evidence_expected:
  - "golden preview"
  - "fallback and omission fixtures"
stop_if:
  - "ambiguous selector is auto-chosen"
  - "wiki relevance cannot cite evidence"
```

## Reference
`internal/service/intent.go` current docs/BM25 routing; `internal/service/graph_impact.go`; `internal/service/wiki_code_context.go`.

## Prompt
Implementa un planificador de intenciones para callers, callees, affected-change, path-between, explain-edge, neighborhood y explain-change. El output debe declarar operación elegida, argumentos extraídos, confianza, generación/freshness, preview seccionado, omissions/fallbacks y `expansions[]` con comando exacto + razón. Explain-change compone diff/affected, callers/callees, tests, contratos y `must_read`/`may_read` de wiki. Selectores ambiguos devuelven candidatos.

## Execution Procedure
1. Usa `mi-lsp nav related Intent --workspace milsp-harness-first --format toon` para ubicar wiring.
2. Define modelos aditivos del planner y preview.
3. Añade golden tests antes del wiring CLI/daemon.
4. Reusa servicios graph/wiki; no dupliques BFS ni ranking.
5. Registra routing telemetry sanitizada sin raw prompt.
6. Verifica parity directa/daemon y budgets de tokens.

## Skeleton
```go
type IntentPlan struct { Intent, Operation string; Confidence float64; Expansions []Expansion }
type Expansion struct { Command, Reason string }
```

## Test
Goldens para cambio Go, contrato wiki, selector ambiguo, graph stale, graph absent y truncation con segunda consulta.

## Verify
`go test ./internal/service/... ./internal/cli/... ./internal/daemon/...` -> PASS

## Commit
`feat(nav): add automatic explain-change synthesis - Gabriel Paz -`

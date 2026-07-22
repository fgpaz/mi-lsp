---
linear_parent: not_applicable_user_authorized
linear_child: not_applicable_user_authorized
anchors: [RF-GPH-005, RF-GPH-006, RF-GPH-008, CT-GRAPH-CLI, TECH-GRAPH-NATIVE, TP-GPH]
allowed_paths: [internal/model/graph_*.go, internal/store/graph*.go, internal/store/schema.go, internal/service/graph_*.go, internal/service/affected.go, internal/service/diff_context.go]
forbidden_paths: [internal/milx/**, scripts/bench/**, .mi-lsp/**]
verify: [go test ./internal/model/... ./internal/store/... ./internal/service/...]
stop_if: [full graph materialization, persisted transitive closure, cursor not generation-bound]
secret_scan: {required: true, evidence: names-only-no-values}
---
# Task T2: Robustez y performance graph-native

## Shared Context
**Goal:** G2, consultas paginables, correctas y bounded.
**Stack:** Go + SQLite snapshots.
**Architecture:** una transacción read-only fija generación; SQL frontier + hidratación batch; continuation firmada y ligada a digest.

## Locked Decisions
- Impacto debe propagar truncation/continuation al envelope exterior.
- Limitar seeds, validar generación y eliminar N+1 dominante sin cargar el grafo completo.

## Task Metadata
```yaml
id: T2
depends_on: []
agent_type: claudex-writer
goal_id: G2
github_issues: []
expected_outcome: "impact/path/neighbors conservan completitud explícita bajo límites y reducen round trips."
files:
  - modify: internal/model/graph_impact.go
  - modify: internal/model/graph_query.go
  - modify: internal/store/graph_query.go
  - modify: internal/store/graph_impact.go
  - modify: internal/store/schema.go
  - modify: internal/service/graph_query.go
  - modify: internal/service/graph_impact.go
  - modify: internal/service/affected.go
  - modify: internal/service/diff_context.go
complexity: high
done_when:
  - "focused graph tests exit 0"
  - "EXPLAIN QUERY PLAN shows generation-scoped selector indexes"
  - "100k input paths terminate at configured seed cap"
evidence_expected:
  - "round-trip counter before/after fixture"
  - "cursor and corrupted-generation negative tests"
stop_if:
  - "schema migration cannot preserve existing DB compatibility"
  - "continuation needs unbounded visited state"
```

## Reference
`internal/store/graph_query.go` snapshot/cursor patterns y `internal/model/graph_query.go` normalización existente.

## Prompt
Añade continuation determinista y acotada para impacto, límite explícito de seeds con omission summary, resolución batch, hidratación batch de nodes/evidence, validación/attestation de generación antes de servir claims, índices `(generation_id, semantic_identity)` y `(generation_id, display_name)`, fairness para direction=both y shortest-path por `(distance, canonical_key)`. Separa budget de expansión de `path` del límite de salida y reporta `search_budget_exhausted`.

## Execution Procedure
1. Usa `mi-lsp nav related GraphImpact --workspace milsp-harness-first --format toon` y `nav related BeginGraphQuerySnapshot`.
2. Escribe negativos para seeds masivas, corrupción, paginación retired-generation, path mínimo y both-direction.
3. Extiende modelos/cursor reutilizando firma/checksum existente.
4. Añade queries batch e índices; conserva orden determinista.
5. Propaga generation/truncated/continuation/warnings a affected y diff-context.
6. Ejecuta suites focalizadas y `EXPLAIN QUERY PLAN` en fixture temporal.

## Skeleton
```go
type GraphImpactContinuation struct { GenerationID, RequestDigest, FrontierDigest string }
const MaxImpactSeeds = 2048
func (s *GraphReadSnapshot) HydrateEdges(ctx context.Context, ids []int64) (...)
```

## Test
Casos adversariales acotados: 100k paths, active generation alterada, cursor retired, shortest path, high-degree both, display/semantic selector, filename con espacios/rename.

## Verify
`go test ./internal/model/... ./internal/store/... ./internal/service/...` -> PASS

## Commit
`perf(graph): bound impact and batch query hydration - Gabriel Paz -`

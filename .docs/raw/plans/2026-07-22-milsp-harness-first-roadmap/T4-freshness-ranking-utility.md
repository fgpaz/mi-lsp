---
linear_parent: not_applicable_user_authorized
linear_child: not_applicable_user_authorized
anchors: [RF-GPH-009, RF-GPH-010, RF-GPH-011, CT-GRAPH-CLI, TECH-GRAPH-NATIVE]
allowed_paths: [internal/model/**, internal/store/**, internal/service/graph_*.go, internal/service/workspace_map.go, internal/service/related.go, internal/daemon/**]
forbidden_paths: [raw query persistence, full graph cache, network, MCP]
verify: [go test ./internal/model/... ./internal/store/... ./internal/service/... ./internal/daemon/...]
stop_if: [utility outranks authority, stale cache is served, analysis is nondeterministic]
secret_scan: {required: true, evidence: names-only-no-values}
---
# Task T4: Freshness, ranking, comunidades y utility memory

## Shared Context
**Goal:** G4, relevancia arquitectónica actual con aprendizaje seguro.
**Stack:** Go + SQLite derived analysis + daemon telemetry.
**Architecture:** `graph_analysis` cachea JSON bounded por generation/algorithm/profile; utility es prior/tie-break sanitizado.

## Locked Decisions
- Freshness es gate de claims exactos.
- Centrality/community y utility nunca reemplazan wiki authority, confidence o exact identity.

## Task Metadata
```yaml
id: T4
depends_on: [T2]
agent_type: claudex-writer
goal_id: G4
github_issues: []
expected_outcome: "workspace-map/related/impact priorizan hubs y boundaries actuales con memoria útil acotada."
files:
  - modify: internal/model/**
  - modify: internal/store/schema.go
  - modify: internal/store/graph*.go
  - modify: internal/service/graph_*.go
  - modify: internal/service/workspace_map.go
  - modify: internal/service/related.go
  - modify: internal/daemon/**
complexity: high
done_when:
  - "freshness negative tests pass"
  - "ranking digest is stable"
  - "utility storage contains no raw prompt/content"
evidence_expected:
  - "determinism fixture"
  - "memory allocation/row-count bound"
stop_if:
  - "analysis requires persisted transitive closure"
  - "cache key omits generation/profile/version"
```

## Reference
`TECH-GRAPH-NATIVE` cache key contract y `graph_analysis` schema existente.

## Prompt
Añade `graph_freshness` state current|lagging|stale|invalid|unknown a envelopes. Implementa ranking authority|impact|centrality|boundary y communities sobre exact/extracted edges únicamente, con algoritmo/version/digest. Añade utility events workspace+intent scoped, decayed/capped y sin raw query/snippets/argv; automatic continuation-followed y feedback explícito son señales, no autoridad. Añade routing telemetry por intent/operation/fallback/failure stage.

## Execution Procedure
1. Usa `mi-lsp nav search "graph_analysis" --workspace milsp-harness-first --format toon --include-content`.
2. Escribe tests de stale invalidación, determinismo y sanitización.
3. Añade schema migrations compatibles y caches generation-bound.
4. Implementa análisis bounded y exposición aditiva en graph status/rank y surfaces existentes.
5. Integra utility como tie-break posterior a autoridad/freshness/confidence.
6. Ejecuta suite focalizada y benchmark de allocaciones local sólo como test, no campaña final.

## Skeleton
```go
type GraphFreshness struct { State, GenerationID, ReasonCode string }
type GraphRank struct { CommunityID, RankReason, AlgorithmVersion, DeterminismDigest string; Score float64 }
```

## Test
Freshness mismatch, stale cache discard, deterministic community IDs, no heuristic-edge authority, utility decay/cap/sanitization y memory bound.

## Verify
`go test ./internal/model/... ./internal/store/... ./internal/service/... ./internal/daemon/...` -> PASS

## Commit
`feat(graph): add freshness ranking and utility signals - Gabriel Paz -`

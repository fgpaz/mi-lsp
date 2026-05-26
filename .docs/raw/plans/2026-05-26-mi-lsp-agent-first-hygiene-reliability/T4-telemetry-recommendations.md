---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-DAE-01"], rf: ["RF-DAE-002"], ct: ["CT-CLI-DAEMON-ADMIN"]}
allowed_paths: ["internal/daemon/**", "internal/telemetry/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["go test ./internal/daemon ./internal/telemetry"]
stop_if: ["recommendation would persist raw query or payload"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T4: Telemetry Recommendations

## Shared Context
**Goal:** Make admin summary point agents to hygiene and narrowing actions.
**Stack:** Go daemon export summary and sanitized telemetry.
**Architecture:** Recommendations are derived from counters, hint codes, failure stages, and top error codes only.

## Locked Decisions
- Do not persist or expose raw query text.
- Recommend `workspace hygiene --format toon` for stale registry/workspace resolution signals.

## Task Metadata
```yaml
id: T4
depends_on: ["T0"]
agent_type: codex
goal_id: G2
github_issues: []
expected_outcome: "Admin export summary recommends workspace hygiene for stale registry and resolution failures."
files:
  - modify: internal/daemon/export.go
complexity: low
done_when:
  - "go test ./internal/daemon exits 0"
evidence_expected:
  - "daemon export test output"
stop_if:
  - "raw query or payload would be required"
```

## Reference
`internal/daemon/export.go` function `ComputeUsageRecommendations`.

## Prompt
Add an agent-first `workspace_hygiene` recommendation when summary contains `workspace_resolution_failed`, missing workspace root top errors, or stale registry signals. Keep existing recommendations.

## Execution Procedure
1. Open `ComputeUsageRecommendations`.
2. Add a recommendation with command `mi-lsp workspace hygiene --format toon`.
3. Update tests that assert recommendations.

## Skeleton
```go
UsageRecommendation{ID: "workspace_hygiene", Command: "mi-lsp workspace hygiene --format toon"}
```

## Verify
`go test ./internal/daemon` -> PASS

## Commit
`feat(telemetry): recommend workspace hygiene`

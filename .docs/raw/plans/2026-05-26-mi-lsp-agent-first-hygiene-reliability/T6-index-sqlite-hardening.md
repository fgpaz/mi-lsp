---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-IDX-01"], rf: ["RF-IDX-003"], ct: ["CT-NAV-WIKI"]}
allowed_paths: ["internal/store/**", "internal/service/**", "internal/daemon/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["go test ./internal/store ./internal/service ./internal/daemon"]
stop_if: ["schema migration is required"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T6: Index SQLite Hardening

## Shared Context
**Goal:** Reduce repeated failures from missing indexed files and SQLite lock errors.
**Stack:** Repo-local SQLite index, service file reads, doc query access.
**Architecture:** Prefer diagnostics and existing WAL/busy-timeout config over schema changes.

## Locked Decisions
- No schema migration in this task.
- Missing indexed files become stale-index diagnostics.

## Task Metadata
```yaml
id: T6
depends_on: ["T2", "T3", "T4"]
agent_type: codex
goal_id: G2
github_issues: []
expected_outcome: "Missing indexed files and doc DB locks produce actionable diagnostics instead of noisy generic errors."
files:
  - modify: internal/service
  - modify: internal/store
complexity: medium
done_when:
  - "go test ./internal/store ./internal/service exits 0"
evidence_expected:
  - "store/service test output"
stop_if:
  - "fix requires schema migration"
```

## Reference
`internal/store/db.go` `configureWorkspaceDB` and service code that opens indexed files.

## Prompt
Patch only the minimal read paths. If a file from the index no longer exists, return a warning/hint that index is stale and suggest `mi-lsp index --workspace <alias>`. For SQLite lock, ensure read connections use the configured busy timeout path.

## Execution Procedure
1. Find file-open errors in service read/enrichment paths.
2. Convert not-exist errors into stale-index warnings where the envelope can still succeed.
3. Verify DB open path uses `configureWorkspaceDB`.

## Skeleton
```go
if errors.Is(err, os.ErrNotExist) {
    warnings = append(warnings, "indexed file is missing; rerun mi-lsp index")
}
```

## Verify
`go test ./internal/store ./internal/service` -> PASS

## Commit
`fix(index): surface stale indexed paths`

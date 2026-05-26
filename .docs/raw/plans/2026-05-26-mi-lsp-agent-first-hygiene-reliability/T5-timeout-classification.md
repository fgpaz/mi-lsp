---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-QRY-01"], rf: ["RF-QRY-002"], ct: ["CT-NAV-WIKI"]}
allowed_paths: ["internal/service/**", "internal/nav/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["go test ./internal/service ./internal/nav"]
stop_if: ["partial results cannot be proven safe"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T5: Timeout Classification

## Shared Context
**Goal:** Make timeout outcomes typed and useful instead of generic failures.
**Stack:** Go search service, wiki fanout, envelopes, telemetry enrichment.
**Architecture:** Safe partial search timeouts should return `ok=true`; hard timeouts must emit typed diagnostics.

## Locked Decisions
- Preserve existing `search_timeout` hint semantics.
- Do not retry blindly inside the command.

## Task Metadata
```yaml
id: T5
depends_on: ["T2", "T3", "T4"]
agent_type: codex
goal_id: G2
github_issues: []
expected_outcome: "Search/wiki timeout diagnostics are typed and actionable."
files:
  - modify: internal/service/search.go
  - modify: internal/service/app.go
  - modify: internal/service/wiki_search.go
complexity: medium
done_when:
  - "go test ./internal/service exits 0"
evidence_expected:
  - "search timeout test output"
stop_if:
  - "timeout handling would hide a real backend failure"
```

## Reference
`internal/service/search.go` `searchTimeoutResult` and `internal/service/app.go` timeout hint construction.

## Prompt
Improve classification only. Keep the existing successful partial-result contract. Add clearer warning/hint code paths for no-partial timeout and wiki timeout where practical.

## Execution Procedure
1. Locate existing timeout tests.
2. Add or update focused assertions for hint/warning/coach/failure stage.
3. Patch classification with minimal code.

## Skeleton
```go
if diagnostics.TimedOut {
    hint = "search timed out..."
}
```

## Verify
`go test ./internal/service` -> PASS

## Commit
`fix(search): clarify timeout diagnostics`

---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-IDX-01"], rf: ["RF-WKS-005", "RF-IDX-003"], ct: ["CT-NAV-GOVERNANCE"]}
allowed_paths: ["internal/docgraph/**", "internal/service/**", "internal/model/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["go test ./internal/docgraph ./internal/service"]
stop_if: ["--no-auto-sync would start writing projection files"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T3: Status Governance Diagnostics

## Shared Context
**Goal:** Explain stale governance/index state with actionable details.
**Stack:** Go docgraph governance status and workspace status envelope.
**Architecture:** Index stale is computed by comparing `.mi-lsp/index.db` with governance doc and read-model projection.

## Locked Decisions
- Preserve `--no-auto-sync`.
- Include paths and timestamps only, no file contents.

## Task Metadata
```yaml
id: T3
depends_on: ["T0"]
agent_type: codex
goal_id: G2
github_issues: []
expected_outcome: "Stale index diagnostics show the compared paths and timestamps."
files:
  - modify: internal/docgraph/governance.go
  - modify: internal/model/types.go
  - modify: internal/service/workspace_ops.go
complexity: medium
done_when:
  - "go test ./internal/docgraph ./internal/service exits 0"
evidence_expected:
  - "focused test output"
stop_if:
  - "diagnostics require reading or persisting doc contents"
```

## Reference
`internal/docgraph/governance.go` function `indexSyncState`.

## Prompt
Replace the string-only stale calculation with details that preserve the existing public fields and add a detailed object/list for diagnostics. Use the details in `workspace status` and `nav governance` items.

## Execution Procedure
1. Add a model type or map output for compared index paths.
2. Update governance inspection to populate details when index is missing/stale/current.
3. Update status output to include the details.
4. Add/adjust tests.

## Skeleton
```go
type GovernanceIndexSyncDetail struct {
    IndexPath string `json:"index_path,omitempty"`
    Compared []GovernanceIndexComparedPath `json:"compared,omitempty"`
}
```

## Verify
`go test ./internal/docgraph ./internal/service` -> PASS

## Commit
`feat(governance): expose stale index diagnostics`

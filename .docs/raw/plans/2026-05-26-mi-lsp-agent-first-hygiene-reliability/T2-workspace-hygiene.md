---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-BOOT-01"], rf: ["RF-WKS-004"], ct: ["CT-CLI-DAEMON-ADMIN"]}
allowed_paths: ["internal/cli/**", "internal/service/**", "internal/workspace/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["go test ./internal/workspace ./internal/service"]
stop_if: ["existing prune safety would need to be bypassed"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T2: Workspace Hygiene

## Shared Context
**Goal:** Add a one-stop agent-first hygiene command.
**Stack:** Go, Cobra CLI, service envelope, workspace registry helpers.
**Architecture:** Compose existing `DoctorWorkspaces` and `PruneStaleWorkspaces`; do not duplicate registry mutation logic.

## Locked Decisions
- Backend string is `registry-hygiene`.
- Flag name is `--apply-safe`.
- No deletion of files, worktrees, indexes, branches, or processes.

## Task Metadata
```yaml
id: T2
depends_on: ["T0"]
agent_type: codex
goal_id: G1
github_issues: []
expected_outcome: "Users can run `mi-lsp workspace hygiene` for a concise hygiene summary and `--apply-safe` for safe registry cleanup."
files:
  - modify: internal/cli/workspace.go
  - modify: internal/service/workspace_ops.go
  - modify: internal/workspace/registry.go
complexity: medium
done_when:
  - "go test ./internal/workspace ./internal/service exits 0"
evidence_expected:
  - "focused test output"
stop_if:
  - "command implementation needs to delete filesystem paths"
```

## Reference
`internal/service/workspace_ops.go` functions `workspaceDoctor` and `workspacePrune`.

## Prompt
Add `workspace hygiene` as a CLI subcommand. It should call a new service operation `workspace.hygiene`. The service must build a summary from `DoctorWorkspaces`; when `apply_safe=true`, it must call `PruneStaleWorkspaces(true)` and include applied action data.

## Execution Procedure
1. Add Cobra subcommand with bool flag `--apply-safe`.
2. Add service dispatch for `workspace.hygiene` where operations are routed.
3. Implement `workspaceHygiene` using existing doctor/prune helpers.
4. Add tests for dry-run and apply-safe behavior.

## Skeleton
```go
func (a *App) workspaceHygiene(request model.CommandRequest) (model.Envelope, error) {
    applySafe, _ := request.Payload["apply_safe"].(bool)
    // compose doctor + optional prune
    return model.Envelope{Ok: true, Backend: "registry-hygiene", Items: []map[string]any{item}}, nil
}
```

## Verify
`go test ./internal/workspace ./internal/service` -> PASS

## Commit
`feat(workspace): add agent-first hygiene command`

# Task T4: --all-workspaces Flag for nav ask

## Shared Context
**Goal:** Agregar flag `--all-workspaces` a `nav ask` para buscar docs en todos los workspaces registrados, no solo el actual.
**Stack:** Go, Cobra CLI, internal/service + internal/cli
**Architecture:** `nav find` y `nav search` ya tienen `--all-workspaces` (nav.go:35-52, 164-168). `nav ask` no tiene ningún flag (nav.go:110-120). El patron es: flag -> payload map -> service layer loop.

## Task Metadata
```yaml
id: T4
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/cli/nav.go:110-120           # add --all-workspaces flag to askCommand
  - modify: internal/service/ask.go               # handle all_workspaces in Ask function
  - read: internal/cli/nav.go:35-57               # reference: how findCommand uses --all-workspaces
  - read: internal/service/search.go              # reference: how search handles all_workspaces
complexity: low
done_when: "go build ./... exits 0 AND mi-lsp nav ask 'question' --all-workspaces returns results from multiple workspaces"
```

## Reference
`internal/cli/nav.go:35-57` -- `findCommand` with `--all-workspaces` flag pattern:
```go
findCommand.Flags().Bool("all-workspaces", false, "Search across all registered workspaces")
```
And in RunE:
```go
allWS, _ := cmd.Flags().GetBool("all-workspaces")
payload["all_workspaces"] = allWS
```

## Prompt
Follow the exact pattern of `findCommand` to add `--all-workspaces` to `askCommand`.

**Step 1: CLI flag** (`internal/cli/nav.go:110-120`)

After the `askCommand` definition (around line 120), before it's added to the nav command, register the flag:
```go
askCommand.Flags().Bool("all-workspaces", false, "Search docs across all registered workspaces")
```

In the `RunE` function, before calling `executeOperation`, read the flag and add to payload:
```go
allWS, _ := cmd.Flags().GetBool("all-workspaces")
payload := map[string]any{"question": question}
if allWS {
    payload["all_workspaces"] = true
}
return state.executeOperation(cmd, "nav.ask", payload, true)
```

**Step 2: Service layer** (`internal/service/ask.go`)

In the `Ask` function (or `Execute` for nav.ask), check if `all_workspaces` is true in the payload:
- If true: iterate over all registered workspaces (use the same mechanism as `search.go` does for all_workspaces), call the doc ranking logic for each workspace's store, merge results by score, and return the combined top results.
- If false: current behavior (single workspace).

Look at how `search.go` handles `all_workspaces` and follow that exact pattern. The key is: get list of workspace stores, run the ask logic against each store, merge results.

**Important:**
- Do NOT change the `AskResult` struct -- add a `Workspace string` field to each `DocEvidence` item if it doesn't exist, so the user knows which workspace each result came from.
- Respect `--token-budget` and `--max-items` for the merged result, not per-workspace.
- If only one workspace is registered, `--all-workspaces` should behave identically to the default.

## Skeleton
```go
// In nav.go askCommand RunE:
allWS, _ := cmd.Flags().GetBool("all-workspaces")
payload := map[string]any{"question": question}
if allWS {
    payload["all_workspaces"] = true
}
return state.executeOperation(cmd, "nav.ask", payload, true)
```

## Verify
```bash
go build ./... && go test ./internal/service/ -run TestAsk -v
```

## Commit
`feat(ask): add --all-workspaces flag to search docs across all registered workspaces`

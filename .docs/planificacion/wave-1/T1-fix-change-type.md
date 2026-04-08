# Task T1: Fix change-type Hardcode in diff_context.go

## Shared Context
**Goal:** Parsear el tipo de cambio real (added/modified/deleted) desde git diff en vez de hardcodear "modified".
**Stack:** Go, os/exec (git commands), internal/service
**Architecture:** `diff_context.go` ejecuta `git diff --name-only` y `git diff --unified=0` para obtener archivos y hunks cambiados.

## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/service/diff_context.go:95-105  # hardcoded changeType
  - modify: internal/service/diff_context.go:20-25   # ChangedSymbol struct (ChangeType field)
  - read: internal/service/diff_context.go            # full context
complexity: low
done_when: "go build ./... exits 0 AND go test ./internal/service/ -run TestDiffContext passes"
```

## Reference
`internal/service/diff_context.go:21` -- `ChangeType string // "modified", "added", "deleted"` already documented.

## Prompt
Open `internal/service/diff_context.go`.

**Step 1:** Find the function that calls `git diff --name-only` (around line 60-80). This returns a list of changed file paths. Change this call to `git diff --name-status` instead, which returns lines like:
```
M       src/foo.go
A       src/bar.go
D       src/baz.go
R100    src/old.go  src/new.go
```

**Step 2:** Parse the status prefix from each line:
- `M` or `M\t` -> "modified"
- `A` or `A\t` -> "added"  
- `D` or `D\t` -> "deleted"
- `R` followed by digits -> "modified" (rename, treat as modified)
- `C` followed by digits -> "added" (copy, treat as added)
- Default -> "modified"

Store the mapping in a `fileChangeTypes map[string]string` keyed by file path.

**Step 3:** At line 100, replace:
```go
changeType := "modified"
```
with:
```go
changeType := fileChangeTypes[relFile]
if changeType == "" {
    changeType = "modified"
}
```

**Step 4:** Remove the comment `// Determine change type (for now all are "modified")` at line 99.

Do NOT change the `ChangedSymbol` struct definition -- `ChangeType` field already supports the new values.
Do NOT change any other function.
Do NOT add new dependencies.

## Skeleton
```go
// parseNameStatus parses "git diff --name-status" output into a map[relativePath]changeType.
func parseNameStatus(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		filePath := parts[1]
		switch {
		case status == "M":
			result[filePath] = "modified"
		case status == "A":
			result[filePath] = "added"
		case status == "D":
			result[filePath] = "deleted"
		case strings.HasPrefix(status, "R"):
			if len(parts) >= 3 {
				result[parts[2]] = "modified" // new name
			}
		case strings.HasPrefix(status, "C"):
			if len(parts) >= 3 {
				result[parts[2]] = "added"
			}
		default:
			result[filePath] = "modified"
		}
	}
	return result
}
```

## Verify
`go build ./... && go test ./internal/service/ -run TestDiffContext -v` -> all pass

## Commit
`fix(diff-context): parse real change type from git diff --name-status instead of hardcoded "modified"`

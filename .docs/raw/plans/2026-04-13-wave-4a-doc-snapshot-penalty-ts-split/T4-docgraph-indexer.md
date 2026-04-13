# Task T4: docgraph — add isSnapshotPath and set IsSnapshot on DocRecord

## Shared Context
**Goal:** Add `isSnapshotPath(path string) bool` function and set `IsSnapshot: isSnapshotPath(candidate.relativePath)` when constructing each `DocRecord` in `IndexWorkspaceDocs`.
**Stack:** Go, `internal/docgraph/docgraph.go`
**Architecture:** Detection is by path segment containing `/old/`, `/archive/`, `/deprecated/`, `/historico/`, `/legacy/` (case-insensitive). The function is pure and has no side effects.

## Task Metadata
```yaml
id: T4
depends_on: [T3]
agent_type: ps-worker
files:
  - modify: internal/docgraph/docgraph.go:138-148
complexity: low
done_when: "go build ./... exits 0"
```

## Reference
`internal/docgraph/docgraph.go:138-148` — DocRecord construction inside the loop:
```go
doc := model.DocRecord{
    Path:        candidate.relativePath,
    Title:       title,
    DocID:       docID,
    Layer:       candidate.layer,
    Family:      candidate.family,
    Snippet:     extractSnippet(content),
    SearchText:  normalizeSearchText(title + "\n" + candidate.relativePath + "\n" + string(content)),
    ContentHash: digest(content),
    IndexedAt:   time.Now().Unix(),
}
```

## Prompt
Open `internal/docgraph/docgraph.go`.

1. Add a helper function `isSnapshotPath` near the top of the file (after the var block, around line 22):
```go
func isSnapshotPath(path string) bool {
    lower := strings.ToLower(path)
    snapshots := []string{"/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"}
    for _, seg := range snapshots {
        if strings.Contains(lower, seg) {
            return true
        }
    }
    return false
}
```

2. In the `IndexWorkspaceDocs` function, inside the loop where `doc` is constructed (around line 138-148), add `IsSnapshot: isSnapshotPath(candidate.relativePath)` to the struct literal:
```go
doc := model.DocRecord{
    Path:        candidate.relativePath,
    Title:       title,
    DocID:       docID,
    Layer:       candidate.layer,
    Family:      candidate.family,
    Snippet:     extractSnippet(content),
    SearchText:  normalizeSearchText(title + "\n" + candidate.relativePath + "\n" + string(content)),
    ContentHash: digest(content),
    IndexedAt:   time.Now().Unix(),
    IsSnapshot:  isSnapshotPath(candidate.relativePath),
}
```

## Skeleton
```go
func isSnapshotPath(path string) bool {
    lower := strings.ToLower(path)
    snapshots := []string{"/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"}
    for _, seg := range snapshots {
        if strings.Contains(lower, seg) {
            return true
        }
    }
    return false
}

// In DocRecord construction:
IsSnapshot: isSnapshotPath(candidate.relativePath),
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(docgraph): add isSnapshotPath detection and set IsSnapshot on DocRecord`

# Task T8: Test — isSnapshotPath detection

## Shared Context
**Goal:** Write a test for `isSnapshotPath` function in `internal/docgraph/docgraph_test.go` (or create the file if it doesn't exist).
**Stack:** Go test, `internal/docgraph/docgraph_test.go`
**Architecture:** Pure function test — no DB, no network.

## Task Metadata
```yaml
id: T8
depends_on: [T5]
agent_type: ps-worker
files:
  - create: internal/docgraph/docgraph_test.go
  - read: internal/docgraph/docgraph.go:22-30
complexity: low
done_when: "go test ./internal/docgraph/... -run IsSnapshot -count=1 exits 0"
```

## Reference
`internal/docgraph/docgraph.go:22-30` — `isSnapshotPath` function to test.

## Prompt
Check if `internal/docgraph/docgraph_test.go` exists. If not, create it with a `package docgraph` header.

Write a test function `TestIsSnapshotPath` that covers:
- Positive cases: paths containing `/old/`, `/archive/`, `/deprecated/`, `/historico/`, `/legacy/`
- Case-insensitive: `/Old/`, `/ARCHIVE/`, `/Deprecated/`
- Negative cases: normal paths like `.docs/wiki/01_alcance.md`, `src/main.go`
- Edge cases: empty string, path with segment at start (`old/foo.md`)

```go
func TestIsSnapshotPath(t *testing.T) {
    tests := []struct {
        name string
        path string
        want bool
    }{
        {"old segment", "docs/wiki/old/foo.md", true},
        {"archive segment", "docs/archive/bar.md", true},
        {"deprecated segment", "docs/deprecated/baz.md", true},
        {"historico segment", "docs/historico/qux.md", true},
        {"legacy segment", "legacy/docs/readme.md", true},
        {"case insensitive Old", "docs/Old/foo.md", true},
        {"case insensitive ARCHIVE", "docs/ARCHIVE/bar.md", true},
        {"case insensitive Deprecated", "docs/Deprecated/baz.md", true},
        {"normal doc", "docs/wiki/01_alcance.md", false},
        {"code file", "src/main.go", false},
        {"empty string", "", false},
        {"old at start", "old/foo.md", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := isSnapshotPath(tt.path)
            if got != tt.want {
                t.Errorf("isSnapshotPath(%q) = %v, want %v", tt.path, got, tt.want)
            }
        })
    }
}
```

## Verify
`go test ./internal/docgraph/... -run IsSnapshot -count=1` -> `PASS`

## Commit
`test(docgraph): add TestIsSnapshotPath`

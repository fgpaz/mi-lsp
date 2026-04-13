# Task T2: Model — add IsSnapshot to DocRecord struct

## Shared Context
**Goal:** Add `IsSnapshot bool` field to `model.DocRecord` struct in `internal/model/types.go`.
**Stack:** Go, `internal/model/types.go`
**Architecture:** The struct currently has 9 fields. Adding the bool field at the end.

## Task Metadata
```yaml
id: T2
depends_on: [T1]
agent_type: ps-worker
files:
  - modify: internal/model/types.go:97-107
complexity: low
done_when: "go build ./... exits 0"
```

## Reference
`internal/model/types.go:97-107` — current DocRecord struct:
```go
type DocRecord struct {
    Path        string `json:"path"`
    Title       string `json:"title,omitempty"`
    DocID       string `json:"doc_id,omitempty"`
    Layer       string `json:"layer,omitempty"`
    Family      string `json:"family,omitempty"`
    Snippet     string `json:"snippet,omitempty"`
    SearchText  string `json:"search_text,omitempty"`
    ContentHash string `json:"content_hash,omitempty"`
    IndexedAt   int64  `json:"indexed_at,omitempty"`
}
```

## Prompt
Open `internal/model/types.go` and find the `DocRecord` struct (around line 97-107). Add a new field after `IndexedAt`:
```go
IsSnapshot bool `json:"is_snapshot,omitempty"`
```

The JSON tag uses `omitempty` because when `IsSnapshot` is `false` (default), it won't be marshalled into JSON — keeping existing output compatible.

## Skeleton
```go
type DocRecord struct {
    Path        string `json:"path"`
    Title       string `json:"title,omitempty"`
    DocID       string `json:"doc_id,omitempty"`
    Layer       string `json:"layer,omitempty"`
    Family      string `json:"family,omitempty"`
    Snippet     string `json:"snippet,omitempty"`
    SearchText  string `json:"search_text,omitempty"`
    ContentHash string `json:"content_hash,omitempty"`
    IndexedAt   int64  `json:"indexed_at,omitempty"`
    IsSnapshot  bool   `json:"is_snapshot,omitempty"`
}
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(model): add IsSnapshot field to DocRecord`

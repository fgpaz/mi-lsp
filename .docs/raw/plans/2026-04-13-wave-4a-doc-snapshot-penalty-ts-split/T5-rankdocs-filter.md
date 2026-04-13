# Task T5: rankDocs — add IsSnapshot filter

## Shared Context
**Goal:** Add `if doc.IsSnapshot { continue }` at the top of the ranking loop in `rankDocs` so snapshot docs are excluded from results entirely.
**Stack:** Go, `internal/service/ask.go`
**Architecture:** Exclusion (not score penalty) — snapshot docs never become anchors or mini-pack entries.

## Task Metadata
```yaml
id: T5
depends_on: [T4]
agent_type: ps-worker
files:
  - modify: internal/service/ask.go:132-145
complexity: low
done_when: "go build ./... exits 0"
```

## Reference
`internal/service/ask.go:132-145` — start of rankDocs loop:
```go
for _, doc := range docs {
    score := 0
    reasons := make([]string, 0, 4)

    // FTS5 BM25 score is the primary signal when available
    if ftsScores != nil {
```

## Prompt
Open `internal/service/ask.go` and find the `rankDocs` function (around line 132). At the very start of the `for _, doc := range docs` loop, before any scoring, add:
```go
if doc.IsSnapshot {
    continue
}
```

This must be the first statement inside the loop, before `score := 0`.

## Skeleton
```go
for _, doc := range docs {
    if doc.IsSnapshot {
        continue
    }
    score := 0
    // ... rest of loop unchanged
}
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(ask): filter snapshot docs from rankDocs results`

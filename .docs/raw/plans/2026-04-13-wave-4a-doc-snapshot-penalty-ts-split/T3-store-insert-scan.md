# Task T3: Store — update INSERT and scan for is_snapshot

## Shared Context
**Goal:** Update `ReplaceDocs` INSERT and `ListDocRecords` scan to include the `is_snapshot` column.
**Stack:** Go, `internal/store/queries_docs.go`
**Architecture:** `ReplaceDocs` uses a prepared statement with 9 placeholders. Need to add 10th for `is_snapshot`. `ListDocRecords` uses raw `rows.Scan` with positional args.

## Task Metadata
```yaml
id: T3
depends_on: [T2]
agent_type: ps-worker
files:
  - modify: internal/store/queries_docs.go:25-30
  - modify: internal/store/queries_docs.go:78-85
complexity: low
done_when: "go build ./... exits 0"
```

## Reference
`internal/store/queries_docs.go:25-30` — current INSERT:
```go
stmt, err := tx.PrepareContext(ctx, `
    INSERT INTO doc_records(path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at)
    VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
```

`internal/store/queries_docs.go:78-85` — current ListDocRecords scan:
```go
if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &item.ContentHash, &item.IndexedAt); err != nil {
```

## Prompt
Open `internal/store/queries_docs.go`.

1. In `ReplaceDocs`, update the INSERT statement to include `is_snapshot`:
```go
INSERT INTO doc_records(path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
```
And update the `stmt.ExecContext` call inside the loop to include `doc.IsSnapshot` as the 10th argument:
```go
if _, err := stmt.ExecContext(ctx, doc.Path, doc.Title, doc.DocID, doc.Layer, doc.Family, doc.Snippet, doc.SearchText, doc.ContentHash, doc.IndexedAt, doc.IsSnapshot); err != nil {
```

2. In `ListDocRecords`, update the SELECT query columns and the `rows.Scan` call to include `is_snapshot`:
```sql
SELECT path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot
```
```go
if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &item.ContentHash, &item.IndexedAt, &item.IsSnapshot); err != nil {
```

## Skeleton
```go
// ReplaceDocs INSERT:
`INSERT INTO doc_records(path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// ExecContext call:
if _, err := stmt.ExecContext(ctx, doc.Path, doc.Title, doc.DocID, doc.Layer, doc.Family, doc.Snippet, doc.SearchText, doc.ContentHash, doc.IndexedAt, doc.IsSnapshot); err != nil {

// ListDocRecords:
SELECT ... indexed_at, is_snapshot FROM doc_records
rows.Scan(..., &item.IndexedAt, &item.IsSnapshot)
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(store): add is_snapshot to INSERT and scan for doc_records`

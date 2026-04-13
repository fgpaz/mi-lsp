# Task T1: Schema Migration — add is_snapshot to doc_records

## Shared Context
**Goal:** Add `is_snapshot INTEGER NOT NULL DEFAULT 0` column to `doc_records` table via idempotent `ensureColumn`.
**Stack:** Go + SQLite, `internal/store/schema.go`
**Architecture:** DDL lives in `docsDDL` const. Migrations via `ensureColumn` function at end of `EnsureSchema`.

## Task Metadata
```yaml
id: T1
depends_on: []
agent_type: ps-worker
files:
  - modify: internal/store/schema.go:62-74
  - modify: internal/store/schema.go:145-152
complexity: low
done_when: "go build ./... exits 0"
```

## Reference
`internal/store/schema.go:145-152` — `ensureColumn` pattern:
```go
if err := ensureColumn(db, "symbols", "repo_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
    return err
}
```
Existing `ensureColumn` calls for `symbols` and `files` tables. Follow the same pattern for `doc_records`.

## Prompt
Open `internal/store/schema.go`. 

1. Find the `docsDDL` const (around line 62-74). It currently reads:
```go
const docsDDL = `
CREATE TABLE IF NOT EXISTS doc_records (
    path TEXT PRIMARY KEY, title TEXT, doc_id TEXT,
    layer TEXT, family TEXT, snippet TEXT,
    search_text TEXT, content_hash TEXT, indexed_at INTEGER
);
`
```
Add `is_snapshot INTEGER NOT NULL DEFAULT 0` as a new column after `indexed_at INTEGER`. The table already has `IF NOT EXISTS` so it's idempotent for new installs.

2. At the end of `EnsureSchema` function (after existing `ensureColumn` calls for `symbols` and `files`), add:
```go
if err := ensureColumn(db, "doc_records", "is_snapshot", "INTEGER NOT NULL DEFAULT 0"); err != nil {
    return err
}
```
This makes the migration automatic for existing installs (the column will be added via ALTER TABLE if missing).

3. Do NOT modify FTS triggers — they only index `title, doc_id, search_text`.

## Skeleton
```go
const docsDDL = `
CREATE TABLE IF NOT EXISTS doc_records (
    path TEXT PRIMARY KEY, title TEXT, doc_id TEXT,
    layer TEXT, family TEXT, snippet TEXT,
    search_text TEXT, content_hash TEXT, indexed_at INTEGER,
    is_snapshot INTEGER NOT NULL DEFAULT 0
);
`
// ... in EnsureSchema:
if err := ensureColumn(db, "doc_records", "is_snapshot", "INTEGER NOT NULL DEFAULT 0"); err != nil {
    return err
}
```

## Verify
`go build ./...` -> `Build succeeded`

## Commit
`feat(schema): add is_snapshot column to doc_records via ensureColumn`

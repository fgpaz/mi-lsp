# Task T2: FTS5 Porter Tokenizer for nav ask

## Shared Context
**Goal:** Agregar FTS5 virtual table con porter stemmer a doc_records para eliminar zero-match failures en nav ask.
**Stack:** Go, SQLite FTS5, internal/store + internal/service
**Architecture:** `index.db` repo-local tiene `doc_records` table. `ask.go` rankea docs con scoring manual por token overlap. FTS5 reemplaza el scoring manual con BM25 nativo.

## Task Metadata
```yaml
id: T2
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/store/schema.go:62-94       # add FTS5 virtual table DDL
  - modify: internal/store/queries.go             # add FTS5 search query
  - modify: internal/service/ask.go:102-138       # replace manual scoring with FTS5 BM25
  - modify: internal/docgraph/docgraph.go         # populate FTS5 table during indexing
complexity: medium
done_when: "go build ./... exits 0 AND nav ask finds docs when query uses synonyms/stemmed forms of doc titles"
```

## Reference
`internal/store/schema.go:62-94` -- existing `doc_records` table DDL.
`internal/service/ask.go:106-129` -- current manual scoring logic to replace.

## Prompt
This task adds an FTS5 virtual table for doc_records and uses it for BM25 scoring in nav ask.

**Step 1: Schema change** (`internal/store/schema.go`)

After the `doc_records` CREATE TABLE statement (around line 70), add:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS doc_records_fts USING fts5(
    title,
    doc_id,
    search_text,
    content='doc_records',
    content_rowid='rowid',
    tokenize='porter unicode61'
);
```

This is a contentless FTS5 table (content synced from doc_records). The `porter unicode61` tokenizer handles stemming + unicode normalization.

Add triggers to keep FTS5 in sync with doc_records. Add after the FTS5 table creation:

```sql
CREATE TRIGGER IF NOT EXISTS doc_records_ai AFTER INSERT ON doc_records BEGIN
    INSERT INTO doc_records_fts(rowid, title, doc_id, search_text)
    VALUES (new.rowid, new.title, new.doc_id, new.search_text);
END;

CREATE TRIGGER IF NOT EXISTS doc_records_ad AFTER DELETE ON doc_records BEGIN
    INSERT INTO doc_records_fts(doc_records_fts, rowid, title, doc_id, search_text)
    VALUES ('delete', old.rowid, old.title, old.doc_id, old.search_text);
END;

CREATE TRIGGER IF NOT EXISTS doc_records_au AFTER UPDATE ON doc_records BEGIN
    INSERT INTO doc_records_fts(doc_records_fts, rowid, title, doc_id, search_text)
    VALUES ('delete', old.rowid, old.title, old.doc_id, old.search_text);
    INSERT INTO doc_records_fts(rowid, title, doc_id, search_text)
    VALUES (new.rowid, new.title, new.doc_id, new.search_text);
END;
```

**Step 2: Query** (`internal/store/queries.go`)

Add a new function:

```go
func (s *Store) FTSSearchDocs(question string, limit int) ([]DocRecord, error)
```

That executes:
```sql
SELECT dr.path, dr.title, dr.doc_id, dr.layer, dr.family, dr.snippet, dr.search_text,
       rank
FROM doc_records_fts
JOIN doc_records dr ON dr.rowid = doc_records_fts.rowid
WHERE doc_records_fts MATCH ?
ORDER BY rank
LIMIT ?
```

The `rank` column is FTS5's built-in BM25 score (lower = better match). Convert to positive score for compatibility with existing code: `score = -rank * 10`.

If the FTS5 table doesn't exist (old index.db without it), catch the error and return nil, nil (graceful degradation).

**Step 3: Scoring integration** (`internal/service/ask.go`)

In the `rankDocs` function (line 102):
1. Before the manual scoring loop, attempt `store.FTSSearchDocs(question, 20)`.
2. If FTS5 returns results, use those as the primary ranking (FTS5 BM25 score as base, then add family bonus +30 and layer weight).
3. If FTS5 returns no results or errors, fall back to the existing manual token scoring (keep the current code as fallback).
4. The `score > 0` gate at line 127 remains, but FTS5 matches will have score > 0 because BM25 produces non-zero scores for any match.

Do NOT remove the existing manual scoring code. Keep it as fallback when FTS5 is unavailable (old databases).
Do NOT change the `AskResult` struct or the `nav.ask` output format.

**Step 4: Index population** (`internal/docgraph/docgraph.go`)

No changes needed IF the triggers are in place -- existing INSERT/UPDATE on doc_records will auto-populate the FTS5 table. But verify that `docgraph.go` uses INSERT/UPDATE on doc_records (not REPLACE INTO or DELETE+INSERT which might skip triggers).

If `docgraph.go` uses DELETE FROM doc_records + INSERT (bulk re-index pattern), the triggers will handle it correctly.

## Skeleton
```go
// internal/store/queries.go
func (s *Store) FTSSearchDocs(question string, limit int) ([]DocRecord, error) {
	rows, err := s.db.Query(`
		SELECT dr.path, dr.title, dr.doc_id, dr.layer, dr.family, dr.snippet, dr.search_text,
		       -rank * 10 as score
		FROM doc_records_fts
		JOIN doc_records dr ON dr.rowid = doc_records_fts.rowid
		WHERE doc_records_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, question, limit)
	if err != nil {
		// FTS5 table may not exist in old databases
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	// scan into []DocRecord with score field...
}
```

## Verify
```bash
go build ./... && go test ./internal/store/ -run TestFTS -v && go test ./internal/service/ -run TestAsk -v
```

## Commit
`feat(ask): add FTS5 porter stemmer for doc search, eliminating zero-match failures on synonym queries`

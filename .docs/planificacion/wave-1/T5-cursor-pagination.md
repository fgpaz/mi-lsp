# Task T5: Cursor Pagination (--offset) for search and find

## Shared Context
**Goal:** Agregar flag `--offset N` a `nav find`, `nav search`, y `nav intent` para permitir paginacion de resultados.
**Stack:** Go, SQLite LIMIT/OFFSET, Cobra CLI
**Architecture:** Todas las queries en `queries.go` usan `LIMIT ?` pero ninguna usa `OFFSET`. El envelope de respuesta ya tiene `truncated` y `next_hint` fields.

## Task Metadata
```yaml
id: T5
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/store/queries.go:129,161,181,328  # add OFFSET ? to queries
  - modify: internal/cli/nav.go                         # add --offset flag to find, search, intent
  - modify: internal/service/find.go                    # pass offset to store
  - modify: internal/service/search.go                  # pass offset to store
  - modify: internal/service/intent.go                  # pass offset to store
  - read: internal/output/truncator.go                  # understand next_hint for pagination hint
complexity: low
done_when: "go build ./... exits 0 AND mi-lsp nav find 'X' --offset 10 skips first 10 results"
```

## Reference
`internal/store/queries.go:161` -- `FindSymbols` query:
```sql
SELECT ... FROM symbols WHERE name LIKE ? ORDER BY name ASC, file_path ASC LIMIT ?
```

## Prompt
Add OFFSET support to the 4 variable-limit queries and expose it via CLI flags.

**Step 1: Store layer** (`internal/store/queries.go`)

For each of these functions, add an `offset int` parameter and append `OFFSET ?` to the SQL:

1. `SymbolsByFile(filePath string, limit int)` (line 129) -> `SymbolsByFile(filePath string, limit, offset int)`
2. `FindSymbols(pattern string, kind string, exact bool, limit int, ...)` (line 161) -> add offset parameter
3. `OverviewByPrefix(prefix string, limit int)` (line 181) -> add offset parameter
4. `IntentSearch(question string, top int, ...)` (line 328) -> add offset parameter

For each: change `LIMIT ?` to `LIMIT ? OFFSET ?` and add `offset` to the query args.

**Do NOT change** `SymbolContainingLine` (line 195) -- it uses `LIMIT 1` hardcoded and pagination makes no sense for it.
**Do NOT change** `CandidateReposForSymbol` (line 249) -- internal use only.

**Step 2: Service layer** (find.go, search.go, intent.go)

In each service's Execute function, read `offset` from the request payload (default 0):
```go
offset := payloadInt(request.Payload, "offset", 0)
```
Pass it to the store function.

**Step 3: CLI flags** (`internal/cli/nav.go`)

Add to `findCommand`, `searchCommand`, and `intentCommand`:
```go
cmd.Flags().Int("offset", 0, "Skip first N results (for pagination)")
```

In each RunE, read and add to payload:
```go
offset, _ := cmd.Flags().GetInt("offset")
if offset > 0 {
    payload["offset"] = offset
}
```

**Step 4: Pagination hint** (`internal/output/truncator.go` or envelope logic)

When truncation occurs AND offset > 0, update `next_hint` to include the next offset:
```
"next_hint": "nav find 'X' --offset 60 for next page"
```

If offset is 0 and truncation occurs:
```
"next_hint": "nav find 'X' --offset 50 for next page"
```

The offset in the hint should be `current_offset + max_items`.

## Skeleton
```go
// queries.go change pattern:
func (s *Store) FindSymbols(pattern string, kind string, exact bool, limit, offset int, repoFilter string) ([]SymbolRecord, error) {
    // ... existing query building ...
    query += " LIMIT ? OFFSET ?"
    args = append(args, limit, offset)
    // ...
}
```

## Verify
```bash
go build ./... && go test ./internal/store/ -run TestFind -v && go test ./internal/service/ -run TestFind -v
```
Manual: `mi-lsp nav find "Service" --workspace mi-lsp --offset 0` vs `--offset 5` should show different results.

## Commit
`feat(pagination): add --offset flag to nav find, search, and intent for cursor pagination`

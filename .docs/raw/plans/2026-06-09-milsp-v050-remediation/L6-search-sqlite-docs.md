# Task L6: Search / SQLite / docs perf + binary lookup

## Shared Context
**Goal:** Pragmas SQLite, cache de FTS y de doc-ranking, timeout de search 7-10s, pin de rg/git (MI_LSP_GIT).
**Stack:** Go, SQLite, ripgrep, git.
**Architecture:** Worktree `C:/wt/v050-l6-search-sqlite-docs`, branch `v050/l6-search-sqlite-docs`. Único dueño de `db.go`, `queries_docs.go`, `doc_query_context.go`, `doc_ranking.go`, `search.go`, `config.go`, `indexer/incremental.go`, `indexer/search_text.go`.

## Locked Decisions
- `configureWorkspaceDB`: agregar `PRAGMA cache_size=-40000` (~40MB), `PRAGMA mmap_size=30000000`, y `PRAGMA optimize` tras publicar índice.
- Cache de resultados de `wiki.search` por query normalizada (sync.Map con TTL 5min); invalidar en reindex.
- Cache de doc-ranking por query (mismo TTL).
- `SearchTimeout` default 5s → 8s para las primeras N filas de búsquedas de texto (AUD-06).
- Pin de binarios: respetar `MI_LSP_RG` (ya existe) y agregar `MI_LSP_GIT`; si se resuelve por PATH, loguear warning una vez.

## Task Metadata
```yaml
id: L6
depends_on: [T2]
agent_type: general-purpose
goal_id: G3
github_issues: []
expected_outcome: "wiki.search repetida cacheada; SQLite con cache/mmap; timeout 8s; rg/git pinneables."
files:
  - modify: internal/store/db.go
  - modify: internal/store/queries_docs.go
  - modify: internal/service/doc_query_context.go
  - modify: internal/service/doc_ranking.go
  - modify: internal/service/search.go
  - modify: internal/service/config.go
  - modify: internal/indexer/incremental.go
  - modify: internal/indexer/search_text.go
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/store/... ./internal/service/... ./internal/indexer/... passes"
  - "a test asserts second identical wiki.search hits cache"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L6-verdict.yaml"
stop_if:
  - "cache returns stale docs after a reindex (must invalidate on index publish)"
```

## Reference
`discovery.yaml` PERF-02/03/04, AUD-06, SEC-06. `internal/service/config.go:23` (SearchTimeout).

## Prompt
Editá SOLO tu set. No toques `index_publish.go` (es de L1); para invalidar cache en reindex, exponé un método `InvalidateDocCache()` que L1/L2 puedan llamar, o suscribite a un evento ya existente — si requiere tocar archivos de otra lane, STOP y registralo. Cambios:
1. PERF-04: pragmas en `db.go` (`configureWorkspaceDB`): cache_size, mmap_size, y `PRAGMA optimize` post-publish.
2. PERF-03: cache de FTS en `queries_docs.go`/`doc_query_context.go` (sync.Map[queryHash]result, TTL 5min).
3. PERF-02: cache de doc-ranking en `doc_ranking.go`.
4. AUD-06: `config.go` SearchTimeout 8s para primeras filas; mantené degradación elegante.
5. SEC-06: `search.go` ya respeta `MI_LSP_RG`; agregá `MI_LSP_GIT` en `incremental.go` (resolución de git); warning si se cae a PATH.

## Execution Procedure
1. `cd C:/wt/v050-l6-search-sqlite-docs`; `git merge --no-edit main`.
2. Aplicá los cambios + test de cache.
3. `go build ./... && go vet ./... && go test ./internal/store/... ./internal/service/... ./internal/indexer/...`.
4. Commit. `L6-verdict.yaml`.

## Skeleton
```go
// db.go configureWorkspaceDB
for _, p := range []string{"PRAGMA cache_size=-40000","PRAGMA mmap_size=30000000"} { db.Exec(p) }
// doc_query_context.go
if v, ok := docCache.Load(qhash); ok && !v.expired() { return v.docs, nil }
```

## Verify
`go test ./internal/store/... ./internal/service/... ./internal/indexer/...` → PASS

## Commit
`perf(search): sqlite pragmas, FTS+rank cache, 8s timeout, MI_LSP_GIT pin (PERF-02/03/04 AUD-06 SEC-06)`

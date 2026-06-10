package service

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// maxFTSCacheEntries bounds the per-process FTS score cache. Distinct queries each add
// one entry; when the cap is exceeded the cache is dropped wholesale (FTS recompute is
// cheap) so a long-lived daemon cannot grow it without bound.
const maxFTSCacheEntries = 4096

// Generation-keyed doc/FTS caches (PERF-02/03).
//
// The previous v0.5.0 caches were removed because they keyed only on the query
// (or on workspaceRoot+query) and were never invalidated on reindex, serving stale
// cross-workspace/post-reindex data. These caches key on the workspace's
// active_docs_generation_id, which the indexer rotates on every docs publish
// (store.ReplaceWorkspaceIndex). A new generation yields a new cache key, so a stale
// generation is structurally impossible to serve — no explicit invalidation needed.
// When no generation is recorded (unindexed/docs-empty), the cache is bypassed.

type docRecordsCacheEntry struct {
	generation string
	docs       []model.DocRecord
}

type ftsCacheEntry struct {
	generation string
	scores     map[string]float64
}

var (
	docRecordsCache sync.Map     // workspaceRoot -> docRecordsCacheEntry
	ftsScoresCache  sync.Map     // workspaceRoot+"\x00"+query -> ftsCacheEntry
	ftsCacheSize    atomic.Int64 // approximate ftsScoresCache entry count for the cap
)

// docsGeneration returns the active docs generation id for the workspace, or "" when
// none is recorded yet.
func docsGeneration(ctx context.Context, db *sql.DB) string {
	gen, _, _ := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveDocsGeneration)
	return gen
}

// loadDocRecordsCached returns the full doc-record list, cached per (workspaceRoot,
// generation). Callers must treat the result as read-only (it is shared across queries
// of the same generation); the doc query path never mutates doc records.
func loadDocRecordsCached(ctx context.Context, db *sql.DB, root, generation string) ([]model.DocRecord, error) {
	if generation != "" {
		if v, ok := docRecordsCache.Load(root); ok {
			if entry, ok := v.(docRecordsCacheEntry); ok && entry.generation == generation {
				return entry.docs, nil
			}
		}
	}
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return nil, err
	}
	if generation != "" {
		docRecordsCache.Store(root, docRecordsCacheEntry{generation: generation, docs: docs})
	}
	return docs, nil
}

// ftsScoresCached returns FTS5 scores for a query, cached per (workspaceRoot, query,
// generation).
func ftsScoresCached(ctx context.Context, db *sql.DB, root, query, generation string) map[string]float64 {
	key := root + "\x00" + query
	if generation != "" {
		if v, ok := ftsScoresCache.Load(key); ok {
			if entry, ok := v.(ftsCacheEntry); ok && entry.generation == generation {
				return entry.scores
			}
		}
	}
	_, scores, _ := store.FTSSearchDocs(ctx, db, query, 20)
	if generation != "" {
		// Bound the cache: when the cap is exceeded, drop everything and start over.
		if ftsCacheSize.Add(1) > maxFTSCacheEntries {
			ftsScoresCache.Range(func(k, _ any) bool { ftsScoresCache.Delete(k); return true })
			ftsCacheSize.Store(1)
		}
		ftsScoresCache.Store(key, ftsCacheEntry{generation: generation, scores: scores})
	}
	return scores
}

// PurgeWorkspaceCaches drops cached doc records and FTS scores for a workspace root.
// Call after unregistering a workspace so its cache entries do not linger.
func PurgeWorkspaceCaches(root string) {
	docRecordsCache.Delete(root)
	prefix := root + "\x00"
	ftsScoresCache.Range(func(k, _ any) bool {
		if s, ok := k.(string); ok && strings.HasPrefix(s, prefix) {
			ftsScoresCache.Delete(k)
			ftsCacheSize.Add(-1)
		}
		return true
	})
}

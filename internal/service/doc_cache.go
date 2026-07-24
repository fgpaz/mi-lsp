package service

import (
	"context"
	"database/sql"
	"strings"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// maxFTSCacheEntries bounds the per-process FTS score cache with LRU eviction.
const maxFTSCacheEntries = 4096

// Generation-keyed doc/FTS caches (PERF-02/03).
//
// These caches key on the workspace's active_docs_generation_id, which the indexer
// rotates on every docs publish (store.ReplaceWorkspaceIndex). A new generation yields
// a new cache key, so a stale generation is structurally impossible to serve.
// FTS keys additionally normalize the query (case/space) so semantic duplicates share
// one entry, and use LRU eviction instead of wholesale purge.

type docRecordsCacheEntry struct {
	generation string
	docs       []model.DocRecord
}

type ftsCacheEntry struct {
	generation string
	scores     map[string]float64
}

type ftsLRUNode struct {
	key   string
	value ftsCacheEntry
	prev  *ftsLRUNode
	next  *ftsLRUNode
}

type ftsLRUCache struct {
	mu      sync.Mutex
	max     int
	entries map[string]*ftsLRUNode
	head    *ftsLRUNode // most recent
	tail    *ftsLRUNode // least recent
}

func newFTSLRUCache(max int) *ftsLRUCache {
	if max <= 0 {
		max = maxFTSCacheEntries
	}
	return &ftsLRUCache{
		max:     max,
		entries: make(map[string]*ftsLRUNode, max),
	}
}

func (c *ftsLRUCache) get(key string) (ftsCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	node, ok := c.entries[key]
	if !ok {
		return ftsCacheEntry{}, false
	}
	c.moveToFrontLocked(node)
	return node.value, true
}

func (c *ftsLRUCache) put(key string, value ftsCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if node, ok := c.entries[key]; ok {
		node.value = value
		c.moveToFrontLocked(node)
		return
	}
	node := &ftsLRUNode{key: key, value: value}
	c.entries[key] = node
	c.pushFrontLocked(node)
	for len(c.entries) > c.max {
		c.evictTailLocked()
	}
}

func (c *ftsLRUCache) purgePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, node := range c.entries {
		if strings.HasPrefix(key, prefix) {
			c.removeLocked(node)
			delete(c.entries, key)
		}
	}
}

func (c *ftsLRUCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *ftsLRUCache) moveToFrontLocked(node *ftsLRUNode) {
	if c.head == node {
		return
	}
	c.detachLocked(node)
	c.pushFrontLocked(node)
}

func (c *ftsLRUCache) pushFrontLocked(node *ftsLRUNode) {
	node.prev = nil
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}
}

func (c *ftsLRUCache) detachLocked(node *ftsLRUNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}
	node.prev = nil
	node.next = nil
}

func (c *ftsLRUCache) removeLocked(node *ftsLRUNode) {
	c.detachLocked(node)
}

func (c *ftsLRUCache) evictTailLocked() {
	if c.tail == nil {
		return
	}
	node := c.tail
	c.detachLocked(node)
	delete(c.entries, node.key)
}

var (
	docRecordsCache sync.Map // workspaceRoot -> docRecordsCacheEntry
	ftsScoresCache  = newFTSLRUCache(maxFTSCacheEntries)
)

// normalizeFTSQuery collapses case and whitespace so semantic duplicates share a cache key.
func normalizeFTSQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(query))), " ")
}

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

// ftsScoresCached returns FTS5 scores for a query, cached per (workspaceRoot, normalized query,
// generation). The underlying FTS call still uses the original query text.
func ftsScoresCached(ctx context.Context, db *sql.DB, root, query, generation string) map[string]float64 {
	key := root + "\x00" + normalizeFTSQuery(query)
	if generation != "" {
		if entry, ok := ftsScoresCache.get(key); ok && entry.generation == generation {
			return entry.scores
		}
	}
	_, scores, _ := store.FTSSearchDocs(ctx, db, query, 20)
	if generation != "" {
		ftsScoresCache.put(key, ftsCacheEntry{generation: generation, scores: scores})
	}
	return scores
}

// PurgeWorkspaceCaches drops cached doc records and FTS scores for a workspace root.
// Call after unregistering a workspace so its cache entries do not linger.
func PurgeWorkspaceCaches(root string) {
	docRecordsCache.Delete(root)
	ftsScoresCache.purgePrefix(root + "\x00")
}

package daemon

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/service"
)

const (
	resultCacheTTL     = 10 * time.Minute
	resultCacheMaxSize = 256
)

// minInt returns the minimum of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// resultCache holds cached operation results keyed by workspace, generation, op, and query hash.
type resultCache struct {
	mu      sync.Mutex
	entries map[string]resultCacheEntry
	lru     []string // LRU order for eviction
	hits    int64
	misses  int64
}

type resultCacheEntry struct {
	envelope  []byte // serialized model.Envelope
	expiresAt time.Time
}

func newResultCache() *resultCache {
	return &resultCache{
		entries: make(map[string]resultCacheEntry),
		lru:     make([]string, 0, resultCacheMaxSize),
		hits:    0,
		misses:  0,
	}
}

// isCacheableOp returns true if the operation is deterministic and read-only.
func isCacheableOp(op string) bool {
	switch op {
	case "nav.ask", "nav.search", "nav.pack", "nav.governance", "nav.route", "nav.prepare",
		"nav.graph.rank", "nav.graph.stats", "nav.graph.status", "nav.neighbors",
		"nav.callers", "nav.callees", "nav.path", "nav.explain", "nav.flow-slice":
		return true
	default:
		return false
	}
}

// resultCacheKey constructs a cache key from workspace root, generation, op, and canonical args.
func resultCacheKey(workspaceRoot, op string, generation any, canonicalArgs map[string]any) (string, error) {
	// Serialize args to deterministic JSON
	argsJSON, err := json.Marshal(canonicalArgs)
	if err != nil {
		return "", err
	}

	// Hash: workspace_root + \x00 + generation + \x00 + op + \x00 + args_json
	hash := sha256.New()
	hash.Write([]byte(workspaceRoot))
	hash.Write([]byte{0})
	hash.Write([]byte(cacheGenerationValue(generation)))
	hash.Write([]byte{0})
	hash.Write([]byte(op))
	hash.Write([]byte{0})
	hash.Write(argsJSON)

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func cacheGenerationValue(generation any) string {
	return fmt.Sprint(generation)
}

func indexGeneration(workspaceRoot string) (string, string, error) {
	return service.PreparationCacheIdentity(workspaceRoot)
}

func (rc *resultCache) get(key string) ([]byte, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	debugEnabled := os.Getenv("MI_LSP_DAEMON_RESULT_CACHE_DEBUG") != ""

	entry, ok := rc.entries[key]
	if !ok {
		rc.misses++
		if debugEnabled {
			fmt.Fprintf(os.Stderr, "[cache:miss] key=%s entries=%d\n", key[:minInt(12, len(key))], len(rc.entries))
		}
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		delete(rc.entries, key)
		// Remove from LRU
		for i, k := range rc.lru {
			if k == key {
				rc.lru = append(rc.lru[:i], rc.lru[i+1:]...)
				break
			}
		}
		rc.misses++
		if debugEnabled {
			fmt.Fprintf(os.Stderr, "[cache:miss:expired] key=%s\n", key[:minInt(12, len(key))])
		}
		return nil, false
	}

	rc.hits++
	// Move to end of LRU (most recently used)
	for i, k := range rc.lru {
		if k == key {
			rc.lru = append(rc.lru[:i], rc.lru[i+1:]...)
			break
		}
	}
	rc.lru = append(rc.lru, key)

	if debugEnabled {
		fmt.Fprintf(os.Stderr, "[cache:hit] key=%s hits=%d misses=%d entries=%d\n",
			key[:minInt(12, len(key))], rc.hits, rc.misses, len(rc.entries))
	}

	return entry.envelope, true
}

func (rc *resultCache) set(key string, envelope []byte, ttl time.Duration) {
	if os.Getenv("MI_LSP_DAEMON_RESULT_CACHE") == "0" {
		return
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// If entry already exists, just update expiry and touch LRU
	if _, ok := rc.entries[key]; ok {
		rc.entries[key] = resultCacheEntry{
			envelope:  envelope,
			expiresAt: time.Now().Add(ttl),
		}
		// Move to end of LRU
		for i, k := range rc.lru {
			if k == key {
				rc.lru = append(rc.lru[:i], rc.lru[i+1:]...)
				break
			}
		}
		rc.lru = append(rc.lru, key)
		return
	}

	// New entry: check LRU capacity and evict if needed
	if len(rc.lru) >= resultCacheMaxSize {
		// Evict LRU (oldest entry)
		victim := rc.lru[0]
		rc.lru = rc.lru[1:]
		delete(rc.entries, victim)
	}

	rc.entries[key] = resultCacheEntry{
		envelope:  envelope,
		expiresAt: time.Now().Add(ttl),
	}
	rc.lru = append(rc.lru, key)
}

func (rc *resultCache) stats() (hits, misses, entries int64) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.hits, rc.misses, int64(len(rc.entries))
}

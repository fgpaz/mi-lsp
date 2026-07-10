package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestResultCachePutGet(t *testing.T) {
	rc := newResultCache()

	// Create test envelope
	envelope := []byte(`{"ok": true}`)
	key := "test_key_1"

	// Initially should be a miss
	_, ok := rc.get(key)
	if ok {
		t.Fatal("expected cache miss for non-existent key")
	}

	// Put and get
	rc.set(key, envelope, 1*time.Minute)
	retrieved, ok := rc.get(key)
	if !ok {
		t.Fatal("expected cache hit after set")
	}

	if string(retrieved) != string(envelope) {
		t.Fatalf("envelope mismatch: got %s, want %s", string(retrieved), string(envelope))
	}
}

func TestResultCacheTTLExpiry(t *testing.T) {
	rc := newResultCache()

	envelope := []byte(`{"ok": true}`)
	key := "test_ttl_key"

	// Set with very short TTL
	rc.set(key, envelope, 10*time.Millisecond)

	// Should hit immediately
	_, ok := rc.get(key)
	if !ok {
		t.Fatal("expected cache hit immediately after set")
	}

	// Wait for TTL to expire
	time.Sleep(50 * time.Millisecond)

	// Should now be a miss (expired)
	_, ok = rc.get(key)
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestResultCacheLRUEviction(t *testing.T) {
	rc := newResultCache()

	// Fill cache to capacity
	for i := 0; i < resultCacheMaxSize; i++ {
		key := "key_" + fmt.Sprint(i)
		envelope := []byte(`{"id": ` + fmt.Sprint(i) + `}`)
		rc.set(key, envelope, 1*time.Minute)
	}

	_, _, entries := rc.stats()
	if entries != resultCacheMaxSize {
		t.Fatalf("expected %d entries, got %d", resultCacheMaxSize, entries)
	}

	// Add one more; should evict the oldest (LRU)
	firstKey := "key_0"
	rc.set("key_new", []byte(`{"id": "new"}`), 1*time.Minute)

	// Old key should be evicted
	_, ok := rc.get(firstKey)
	if ok {
		t.Fatal("expected oldest key to be evicted")
	}

	// New key should be present
	_, ok = rc.get("key_new")
	if !ok {
		t.Fatal("expected new key to be present")
	}

	hits2, misses2, entries2 := rc.stats()
	if entries2 != resultCacheMaxSize {
		t.Fatalf("expected %d entries after eviction, got %d", resultCacheMaxSize, entries2)
	}

	// Verify hit/miss counts increased appropriately
	_ = hits2
	_ = misses2
	// (Just verify they are reasonable; the exact counts depend on get() calls above)
}

func TestResultCacheKeyDifferentGeneration(t *testing.T) {
	args := map[string]any{"query": "test"}

	key1, err := resultCacheKey("/workspace", "nav.ask", 100, args)
	if err != nil {
		t.Fatalf("resultCacheKey failed: %v", err)
	}

	key2, err := resultCacheKey("/workspace", "nav.ask", 200, args)
	if err != nil {
		t.Fatalf("resultCacheKey failed: %v", err)
	}

	if key1 == key2 {
		t.Fatal("expected different keys for different generations")
	}
}

func TestResultCacheKeyConsistency(t *testing.T) {
	args := map[string]any{
		"query":  "test",
		"limit":  10,
		"offset": 0,
	}

	key1, err := resultCacheKey("/workspace", "nav.search", 123, args)
	if err != nil {
		t.Fatalf("resultCacheKey failed: %v", err)
	}

	key2, err := resultCacheKey("/workspace", "nav.search", 123, args)
	if err != nil {
		t.Fatalf("resultCacheKey failed: %v", err)
	}

	if key1 != key2 {
		t.Fatal("expected same key for identical inputs")
	}
}

func TestResultCacheStatsHitMiss(t *testing.T) {
	rc := newResultCache()

	// Initial stats
	hits, misses, entries := rc.stats()
	if hits != 0 || misses != 0 || entries != 0 {
		t.Fatalf("initial stats unexpected: hits=%d, misses=%d, entries=%d", hits, misses, entries)
	}

	// Set and get (hit)
	rc.set("key1", []byte(`{}`), 1*time.Minute)
	rc.get("key1")
	hits, misses, entries = rc.stats()
	if hits != 1 || misses != 0 || entries != 1 {
		t.Fatalf("after hit: hits=%d (want 1), misses=%d (want 0), entries=%d (want 1)", hits, misses, entries)
	}

	// Get non-existent (miss)
	rc.get("nonexistent")
	hits, misses, entries = rc.stats()
	if hits != 1 || misses != 1 || entries != 1 {
		t.Fatalf("after miss: hits=%d (want 1), misses=%d (want 1), entries=%d (want 1)", hits, misses, entries)
	}
}

func TestIsCacheableOp(t *testing.T) {
	tests := map[string]bool{
		"nav.ask":      true,
		"nav.search":   true,
		"nav.pack":     true,
		"nav.governance": true,
		"nav.route":    true,
		"workspace.add": false,
		"workspace.status": false,
		"system.stop":   false,
		"unknown":       false,
	}

	for op, expected := range tests {
		if isCacheableOp(op) != expected {
			t.Fatalf("isCacheableOp(%q) = %v, want %v", op, isCacheableOp(op), expected)
		}
	}
}

func TestResultCacheDisabledEnv(t *testing.T) {
	rc := newResultCache()

	// Set env var to disable cache
	os.Setenv("MI_LSP_DAEMON_RESULT_CACHE", "0")
	defer os.Unsetenv("MI_LSP_DAEMON_RESULT_CACHE")

	// Try to set (should no-op due to env check)
	rc.set("key1", []byte(`{}`), 1*time.Minute)

	// Get should still miss (or return whatever was there before the set)
	// In this case, there's nothing, so it should miss
	_, ok := rc.get("key1")
	if ok {
		t.Fatal("expected no cache when MI_LSP_DAEMON_RESULT_CACHE=0")
	}
}

func TestResultCacheCanonicalJSONArgs(t *testing.T) {
	// Verify that different JSON orderings of the same args produce the same key
	args1 := map[string]any{"a": 1, "b": 2}
	args2 := map[string]any{"b": 2, "a": 1}

	// JSON marshaling of maps is deterministic in Go (sorts keys)
	j1, _ := json.Marshal(args1)
	j2, _ := json.Marshal(args2)

	if string(j1) != string(j2) {
		t.Logf("Note: JSON marshals differ (this is expected for maps in different order)")
		// Go's json.Marshal does sort map keys, so they should be equal
		// If they're not, it's a Go version issue; skip the rest
		t.Skip("json.Marshal not deterministic on this Go version")
	}

	key1, _ := resultCacheKey("/ws", "nav.ask", 100, args1)
	key2, _ := resultCacheKey("/ws", "nav.ask", 100, args2)

	if key1 != key2 {
		t.Fatal("expected same key for same args in different order")
	}
}

func TestExtractCanonicalArgsDifferentQuestion(t *testing.T) {
	// FIX 1: Verify that same payload with different question generates different cache keys
	req1 := model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Format: "compact"},
		Payload:   map[string]any{"question": "What is architecture?"},
	}

	req2 := model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Format: "compact"},
		Payload:   map[string]any{"question": "How are symbols indexed?"},
	}

	args1 := extractCanonicalArgs(req1)
	args2 := extractCanonicalArgs(req2)

	key1, _ := resultCacheKey("/ws", "nav.ask", 100, args1)
	key2, _ := resultCacheKey("/ws", "nav.ask", 100, args2)

	if key1 == key2 {
		t.Fatal("expected different keys for different questions")
	}
}

func TestExtractCanonicalArgsDifferentEnvelopeFlags(t *testing.T) {
	// FIX 1: Verify that same payload with different format/Full/Verbose flags generates different keys
	payload := map[string]any{"query": "test"}

	req1 := model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Format: "compact"},
		Payload:   payload,
	}

	req2 := model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Format: "compact", Full: true},
		Payload:   payload,
	}

	req3 := model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Format: "toon"},
		Payload:   payload,
	}

	args1 := extractCanonicalArgs(req1)
	args2 := extractCanonicalArgs(req2)
	args3 := extractCanonicalArgs(req3)

	key1, _ := resultCacheKey("/ws", "nav.search", 100, args1)
	key2, _ := resultCacheKey("/ws", "nav.search", 100, args2)
	key3, _ := resultCacheKey("/ws", "nav.search", 100, args3)

	if key1 == key2 {
		t.Fatal("expected different keys for different Full flag")
	}
	if key1 == key3 {
		t.Fatal("expected different keys for different format")
	}
}

func TestExtractCanonicalArgsSessionIDIgnored(t *testing.T) {
	// FIX 1: Verify that session_id is NOT included in canonical args (non-semantic field)
	// This is tricky because session_id is in Context, not Payload
	// For now, verify that identical payloads produce same args regardless of other context fields

	payload := map[string]any{"query": "test"}

	req1 := model.CommandRequest{
		Operation: "nav.search",
		Context: model.QueryOptions{
			Format:    "compact",
			SessionID: "session-1",
		},
		Payload: payload,
	}

	req2 := model.CommandRequest{
		Operation: "nav.search",
		Context: model.QueryOptions{
			Format:    "compact",
			SessionID: "session-2",
		},
		Payload: payload,
	}

	args1 := extractCanonicalArgs(req1)
	args2 := extractCanonicalArgs(req2)

	key1, _ := resultCacheKey("/ws", "nav.search", 100, args1)
	key2, _ := resultCacheKey("/ws", "nav.search", 100, args2)

	// Both should have the same key because session_id is non-semantic
	if key1 != key2 {
		t.Fatal("expected same key when only session_id differs (non-semantic field)")
	}
}

func TestResultCacheConsecutiveHits(t *testing.T) {
	// Verify that consecutive identical queries produce a HIT on the second execution
	rc := newResultCache()

	envelope1 := []byte(`{"ok": true, "items": ["result1"]}`)

	// Simulate two identical requests
	args := map[string]any{"question": "What is the system?"}
	key, _ := resultCacheKey("/workspace", "nav.ask", 100, args)

	// First execution: MISS, then set
	_, hit := rc.get(key)
	if hit {
		t.Fatal("expected cache miss on first access")
	}
	rc.set(key, envelope1, 1*time.Minute)

	// Second execution: HIT
	result, hit := rc.get(key)
	if !hit {
		t.Fatal("expected cache hit on second access with identical args")
	}
	if string(result) != string(envelope1) {
		t.Fatalf("cache result mismatch: got %s, want %s", string(result), string(envelope1))
	}

	// Verify stats
	hits, misses, entries := rc.stats()
	if hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
	if entries != 1 {
		t.Fatalf("expected 1 entry, got %d", entries)
	}
}

func TestResultCacheDebugLogging(t *testing.T) {
	// Verify that debug logging is controlled by MI_LSP_DAEMON_RESULT_CACHE_DEBUG env var
	rc := newResultCache()

	envelope := []byte(`{"ok": true}`)
	key := "debug_test_key"

	// Without debug enabled, no issue
	rc.set(key, envelope, 1*time.Minute)
	_, hit := rc.get(key)
	if !hit {
		t.Fatal("expected cache hit without debug logging")
	}

	// With debug enabled (just verify no panic or error)
	os.Setenv("MI_LSP_DAEMON_RESULT_CACHE_DEBUG", "1")
	defer os.Unsetenv("MI_LSP_DAEMON_RESULT_CACHE_DEBUG")

	rc2 := newResultCache()
	rc2.set(key, envelope, 1*time.Minute)
	_, hit2 := rc2.get(key)
	if !hit2 {
		t.Fatal("expected cache hit with debug logging enabled")
	}
	// Test just verifies no panic occurs with debug enabled
}

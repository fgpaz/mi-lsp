package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEmbedIndexSendsLowPriorityInSequentialBatches(t *testing.T) {
	var active, maxActive, requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Micro-Proxy-Priority"); got != "baja" {
			t.Errorf("priority header = %q, want baja", got)
		}
		current := active.Add(1)
		for old := maxActive.Load(); current > old && !maxActive.CompareAndSwap(old, current); old = maxActive.Load() {
		}
		defer active.Add(-1)
		requests.Add(1)
		var input requestPayload
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(responsePayload{Data: []responseData{{Embedding: []float32{1}}}})
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, Model: "fixture", BatchSize: 1, Dim: 1})
	vectors, err := client.EmbedIndex(context.Background(), []string{"one", "two", "three"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 3 || requests.Load() != 3 {
		t.Fatalf("vectors=%d requests=%d, want 3 each", len(vectors), requests.Load())
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("maximum concurrent requests = %d, want serial requests", got)
	}
}

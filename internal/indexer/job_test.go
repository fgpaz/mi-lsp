package indexer

import (
	"testing"
	"time"
)

func TestEmbeddingIndexTimeoutScalesWithBatchCount(t *testing.T) {
	t.Setenv("MI_LSP_INDEX_TIMEOUT", "30m")
	got := EmbeddingIndexTimeout(9254, 16, 30*time.Second, 0)
	if got <= 30*time.Minute {
		t.Fatalf("EmbeddingIndexTimeout = %v, want above base timeout", got)
	}
}

func TestEmbeddingIndexTimeoutHonorsConfiguredOverride(t *testing.T) {
	got := EmbeddingIndexTimeout(9254, 16, 30*time.Second, 45*time.Minute)
	if got != 45*time.Minute {
		t.Fatalf("EmbeddingIndexTimeout configured = %v, want 45m", got)
	}
}

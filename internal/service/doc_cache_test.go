package service

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// TestDocRecordsCacheInvalidatesOnGeneration verifies the PERF-02 cache is keyed on the
// active docs generation and that a generation change (reindex) structurally invalidates
// it — the bug class that forced removal of the previous question-only cache.
func TestDocRecordsCacheInvalidatesOnGeneration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	// Generation G1 with one doc.
	if err := store.UpsertWorkspaceMeta(ctx, db, store.WorkspaceMetaActiveDocsGeneration, "gen-1"); err != nil {
		t.Fatalf("meta gen-1: %v", err)
	}
	if err := store.ReplaceDocs(ctx, db, []model.DocRecord{
		{Path: ".docs/wiki/03_FL/FL-A.md", Title: "FL-A", DocID: "FL-A", Layer: "03", Family: "functional", SearchText: "alpha"},
	}, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs gen-1: %v", err)
	}

	d1, err := loadDocRecordsCached(ctx, db, root, "gen-1")
	if err != nil {
		t.Fatalf("load gen-1: %v", err)
	}
	if len(d1) != 1 || d1[0].DocID != "FL-A" {
		t.Fatalf("gen-1 docs = %#v, want [FL-A]", d1)
	}

	// Mutate the underlying docs WITHOUT changing the generation: the cache must still
	// return the gen-1 snapshot (cache hit within the same generation).
	if err := store.ReplaceDocs(ctx, db, []model.DocRecord{
		{Path: ".docs/wiki/03_FL/FL-A.md", Title: "FL-A", DocID: "FL-A", Layer: "03", Family: "functional", SearchText: "alpha"},
		{Path: ".docs/wiki/03_FL/FL-B.md", Title: "FL-B", DocID: "FL-B", Layer: "03", Family: "functional", SearchText: "beta"},
	}, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs (same gen): %v", err)
	}
	cached, err := loadDocRecordsCached(ctx, db, root, "gen-1")
	if err != nil {
		t.Fatalf("load gen-1 (cache hit): %v", err)
	}
	if len(cached) != 1 {
		t.Fatalf("same-generation cache should return the gen-1 snapshot (1 doc), got %d", len(cached))
	}

	// New generation (reindex): the cache key changes, so fresh records are loaded.
	if err := store.UpsertWorkspaceMeta(ctx, db, store.WorkspaceMetaActiveDocsGeneration, "gen-2"); err != nil {
		t.Fatalf("meta gen-2: %v", err)
	}
	d2, err := loadDocRecordsCached(ctx, db, root, "gen-2")
	if err != nil {
		t.Fatalf("load gen-2: %v", err)
	}
	if len(d2) != 2 {
		t.Fatalf("gen-2 should load fresh records (2 docs), got %d — cache not invalidated on generation change", len(d2))
	}
}

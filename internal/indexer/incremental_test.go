package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestIncrementalIndexFallsBackWhenCanonicalDocsAreMissingFromIndex(t *testing.T) {
	root := t.TempDir()
	mustWriteIncrementalFile(t, filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md"), "# 00. Gobierno documental\n")

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{{
		Path:   "README.md",
		Title:  "repo",
		Family: "generic",
		Layer:  "generic",
	}}, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	_ = db.Close()

	_, err = IncrementalIndex(context.Background(), root)
	if err == nil || err.Error() != "canonical docs missing from index; fallback to full index" {
		t.Fatalf("IncrementalIndex error = %v, want canonical-doc fallback", err)
	}
}

func TestIncrementalIndexNoChangesWhenCanonicalDocsAlreadyIndexed(t *testing.T) {
	root := t.TempDir()
	mustWriteIncrementalFile(t, filepath.Join(root, ".docs", "wiki", "03_FL.md"), "# FL-INDEX\n")

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{{
		Path:   ".docs/wiki/03_FL.md",
		Title:  "FL-INDEX",
		DocID:  "FL-INDEX",
		Family: "functional",
		Layer:  "03",
	}}, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	_ = db.Close()

	result, err := IncrementalIndex(context.Background(), root)
	if err != nil {
		t.Fatalf("IncrementalIndex error = %v, want nil", err)
	}
	if result.Stats.Files != 0 {
		t.Fatalf("IncrementalIndex processed %d files, want 0", result.Stats.Files)
	}
}

func mustWriteIncrementalFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

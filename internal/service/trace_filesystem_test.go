package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestTraceRFVerifiesFileOnlyLinksByWorkspaceFileExistence(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "internal/service/file_exists_impl.go", "package service\n")
	writeWorkspaceFile(t, root, "internal/service/file_exists_impl_test.go", "package service\n")

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	docPath := filepath.ToSlash(".docs/wiki/04_RF/RF-TRC-001.md")
	err = store.ReplaceDocs(context.Background(), db, []model.DocRecord{{
		Path:      docPath,
		Title:     "RF-TRC-001 - File links",
		DocID:     "RF-TRC-001",
		Layer:     "04",
		Family:    "functional",
		IndexedAt: 1,
	}}, nil, []model.DocMention{
		{
			DocPath:      docPath,
			MentionType:  "implements",
			MentionValue: "internal/service/file_exists_impl.go",
		},
		{
			DocPath:      docPath,
			MentionType:  "test_file",
			MentionValue: "internal/service/file_exists_impl_test.go",
		},
	})
	if err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}

	app := New(root, nil)
	result, err := app.traceRF(context.Background(), root, db, "RF-TRC-001")
	if err != nil {
		t.Fatalf("traceRF: %v", err)
	}
	if result == nil {
		t.Fatal("traceRF returned nil result")
	}
	if result.Status != "implemented" || result.Coverage != 1 {
		t.Fatalf("trace status = %s %.2f, want implemented 1.00", result.Status, result.Coverage)
	}
	if len(result.Explicit) != 1 || !result.Explicit[0].Verified {
		t.Fatalf("explicit trace link not verified: %#v", result.Explicit)
	}
	if len(result.Tests) != 1 || !result.Tests[0].Verified {
		t.Fatalf("test trace link not verified: %#v", result.Tests)
	}
}

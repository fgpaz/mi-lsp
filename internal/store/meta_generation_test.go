package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestReadWorkspaceGenerationSnapshotIsReadOnlyWhenDBAbsent(t *testing.T) {
	root := t.TempDir()
	generation, err := ReadWorkspaceGenerationSnapshot(context.Background(), root)
	if err != nil || generation != "none" {
		t.Fatalf("snapshot = %q, err=%v; want none", generation, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("absent DB read created state directory: err=%v", err)
	}
}

func TestReadWorkspaceGenerationSnapshotUsesActiveMetadata(t *testing.T) {
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := UpsertWorkspaceMeta(context.Background(), db, WorkspaceMetaLastIndexGeneration, "g-last"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertWorkspaceMeta(context.Background(), db, WorkspaceMetaActiveDocsGeneration, "g-docs"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadWorkspaceGenerationSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := WorkspaceMetaLastIndexGeneration + "=g-last\x00" + WorkspaceMetaActiveDocsGeneration + "=g-docs"
	if got != want {
		t.Fatalf("snapshot = %q, want %q", got, want)
	}
}

func TestWorkspaceCatalogReadyRecognizesCompleteCatalogWithoutGenerationID(t *testing.T) {
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "empty", Kind: model.WorkspaceKindSingle}}
	if err := ReplaceCatalog(context.Background(), db, project, []model.FileRecord{}, []model.SymbolRecord{}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ready, err := WorkspaceCatalogReady(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("catalog readiness = false, want true for a published zero-file catalog")
	}
}

func TestWorkspaceCatalogReadyRejectsPartialLegacyPublicationMetadata(t *testing.T) {
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertWorkspaceMeta(context.Background(), db, WorkspaceMetaIndexedAt, "123"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ready, err := WorkspaceCatalogReady(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("catalog readiness = true, want false for partial publication metadata")
	}
}

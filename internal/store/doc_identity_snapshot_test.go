package store

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docidentity"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDocumentIdentitySnapshotTrustIsStampedAndInvalidatedAtomically(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	if err := UpsertWorkspaceMeta(ctx, db, WorkspaceMetaDocIdentitySnapshotVersion, docidentity.ExtractionVersion); err != nil {
		t.Fatal(err)
	}

	docs := []model.DocRecord{{Path: "wiki/source.md", Title: "Source", ContentHash: "hash"}}
	if err := ReplaceWorkspaceDocsWithReferenceSnapshot(ctx, db, "", docs, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{}); err != nil {
		t.Fatalf("versioned publication: %v", err)
	}
	current, err := DocIdentitySnapshotCurrent(ctx, db)
	if err != nil || !current {
		t.Fatalf("current after versioned publication = %v, err=%v", current, err)
	}

	if err := ReplaceDocsWithSources(ctx, db, []model.DocRecord{{Path: "wiki/manual.md", Title: "Manual", ContentHash: "other"}}, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("unversioned publication: %v", err)
	}
	current, err = DocIdentitySnapshotCurrent(ctx, db)
	if err != nil || current {
		t.Fatalf("current after unversioned publication = %v, err=%v, want false", current, err)
	}

	if err := UpsertWorkspaceMeta(ctx, db, WorkspaceMetaDocIdentitySnapshotVersion, docidentity.ExtractionVersion); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceDocsWithSourcesTx(ctx, tx, docs, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := stampDocIdentitySnapshotTx(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	current, err = DocIdentitySnapshotCurrent(ctx, db)
	if err != nil || !current {
		t.Fatalf("current after rolled-back publication = %v, err=%v, want true", current, err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := ReplaceWorkspaceDocsWithReferenceSnapshot(canceled, db, "", docs, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{}); err == nil {
		t.Fatal("canceled publication unexpectedly succeeded")
	}
	current, err = DocIdentitySnapshotCurrent(ctx, db)
	if err != nil || !current {
		t.Fatalf("current after canceled publication = %v, err=%v, want true", current, err)
	}
}

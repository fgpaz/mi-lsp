package store

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestOpen_CreatesDocTables(t *testing.T) {
	db, _ := seedTestDB(t)
	for _, table := range []string{"doc_records", "doc_edges", "doc_mentions"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("table %s not found: %v", table, err)
		}
	}
}

func TestReplaceDocs_RoundTrip(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docs := []model.DocRecord{{Path: ".docs/wiki/07_baseline_tecnica.md", Title: "Baseline tecnica", DocID: "TECH-SEARCH", Layer: "07", Family: "technical", Snippet: "daemon routing", SearchText: "daemon routing", ContentHash: "abc", IndexedAt: 1}}
	edges := []model.DocEdge{{FromPath: ".docs/wiki/07_baseline_tecnica.md", ToDocID: "CT-NAV-ASK", ToPath: ".docs/wiki/09_contratos/CT-NAV-ASK.md", Kind: "doc_id", Label: "CT-NAV-ASK"}}
	mentions := []model.DocMention{{DocPath: ".docs/wiki/07_baseline_tecnica.md", MentionType: "file_path", MentionValue: "internal/service/ask.go"}}
	if err := ReplaceDocs(ctx, db, docs, edges, mentions); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	storedDocs, err := ListDocRecords(ctx, db)
	if err != nil {
		t.Fatalf("ListDocRecords: %v", err)
	}
	if len(storedDocs) != 1 || storedDocs[0].Path != docs[0].Path {
		t.Fatalf("stored docs = %#v", storedDocs)
	}
	storedEdges, err := DocEdgesFrom(ctx, db, docs[0].Path)
	if err != nil {
		t.Fatalf("DocEdgesFrom: %v", err)
	}
	if len(storedEdges) != 1 || storedEdges[0].ToPath != edges[0].ToPath {
		t.Fatalf("stored edges = %#v", storedEdges)
	}
	storedMentions, err := DocMentionsForPath(ctx, db, docs[0].Path)
	if err != nil {
		t.Fatalf("DocMentionsForPath: %v", err)
	}
	if len(storedMentions) != 1 || storedMentions[0].MentionValue != mentions[0].MentionValue {
		t.Fatalf("stored mentions = %#v", storedMentions)
	}
}

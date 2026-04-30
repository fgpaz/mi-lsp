package store

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestOpen_CreatesDocTables(t *testing.T) {
	db, _ := seedTestDB(t)
	for _, table := range []string{"doc_records", "doc_edges", "doc_mentions", "doc_source_blocks", "doc_source_records"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("table %s not found: %v", table, err)
		}
	}
}

func TestFTSSearchDocs_StemmerMatch(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docs := []model.DocRecord{
		{
			Path:        ".docs/wiki/03_FL/FL-BOOT-01.md",
			Title:       "FL-BOOT-01 - Flujo de bootstrap",
			DocID:       "FL-BOOT-01",
			Layer:       "03",
			Family:      "functional",
			Snippet:     "Describe como arranca el sistema",
			SearchText:  "flujo de bootstrap como arranca el sistema",
			ContentHash: "x1",
			IndexedAt:   1,
		},
	}
	if err := ReplaceDocs(ctx, db, docs, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}

	// "arranca" and "arrancar" should both stem to the same root via porter
	results, scores, err := FTSSearchDocs(ctx, db, "como arranca el sistema", 5)
	if err != nil {
		t.Fatalf("FTSSearchDocs: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FTSSearchDocs: expected at least one match for 'arranca', got none")
	}
	if results[0].Path != docs[0].Path {
		t.Fatalf("FTSSearchDocs: expected path %q, got %q", docs[0].Path, results[0].Path)
	}
	if scores[results[0].Path] <= 0 {
		t.Fatalf("FTSSearchDocs: expected positive score, got %f", scores[results[0].Path])
	}
}

func TestFTSSearchDocs_GracefulDegradation(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()

	// Drop FTS table to simulate old database
	if _, err := db.Exec("DROP TABLE IF EXISTS doc_records_fts"); err != nil {
		t.Fatalf("drop fts: %v", err)
	}

	results, scores, err := FTSSearchDocs(ctx, db, "test question", 5)
	if err != nil {
		t.Fatalf("FTSSearchDocs should degrade gracefully, got: %v", err)
	}
	if results != nil || scores != nil {
		t.Fatalf("FTSSearchDocs: expected nil results on missing table, got results=%v scores=%v", results, scores)
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

func TestReplaceDocsWithSources_RoundTrip(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docPath := ".docs/wiki/09_contratos/CT-NAV-WIKI.md"
	docs := []model.DocRecord{{Path: docPath, Title: "CT-NAV-WIKI", DocID: "CT-NAV-WIKI", Layer: "09", Family: "technical", SearchText: "SDD WIKI SOURCE", IndexedAt: 1}}
	blocks := []model.DocSourceBlock{{
		DocPath:      docPath,
		BlockID:      "CT-NAV-WIKI.source",
		DocID:        "CT-NAV-WIKI",
		Kind:         "contract",
		SourceFormat: "SDD-WIKI-SOURCE-v1",
		Ordinal:      1,
		StartLine:    10,
		EndLine:      20,
		ContentHash:  "b1",
		IndexedAt:    1,
	}}
	records := []model.DocSourceRecord{{
		DocPath:     docPath,
		BlockID:     "CT-NAV-WIKI.source",
		RecordID:    "RF-QRY-016",
		RecordType:  "RF",
		Ordinal:     1,
		StartLine:   12,
		EndLine:     18,
		ContentHash: "r1",
		IndexedAt:   1,
	}}
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, blocks, records); err != nil {
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
	storedBlocks, err := ListDocSourceBlocks(ctx, db)
	if err != nil {
		t.Fatalf("ListDocSourceBlocks: %v", err)
	}
	if len(storedBlocks) != 1 || storedBlocks[0].BlockID != blocks[0].BlockID {
		t.Fatalf("stored blocks = %#v", storedBlocks)
	}
	storedRecords, err := ListDocSourceRecords(ctx, db)
	if err != nil {
		t.Fatalf("ListDocSourceRecords: %v", err)
	}
	if len(storedRecords) != 1 || storedRecords[0].RecordID != records[0].RecordID {
		t.Fatalf("stored records = %#v", storedRecords)
	}
	found, err := FindDocRecordsBySourceID(ctx, db, "RF-QRY-016")
	if err != nil {
		t.Fatalf("FindDocRecordsBySourceID: %v", err)
	}
	if len(found) != 1 || found[0].Path != docPath {
		t.Fatalf("source lookup = %#v", found)
	}
}

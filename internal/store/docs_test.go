package store

import (
	"context"
	"strings"
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
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, blocks, records, nil); err != nil {
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

func TestListDocRecordsPathsFiltersKnowledgeRootsWithoutSearchText(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docs := []model.DocRecord{
		{Path: "wiki/00-person.md", Title: "Person", Family: "generic", SearchText: strings.Repeat("heavy", 100)},
		{Path: "wiki/10-project.md", Title: "Project", Family: "generic", SearchText: strings.Repeat("heavy", 100)},
		{Path: "bibliotecas/topic.md", Title: "Topic", Family: "generic", SearchText: strings.Repeat("heavy", 100)},
		{Path: ".docs/wiki/04_RF/RF-X.md", Title: "RF-X", Family: "functional", SearchText: strings.Repeat("heavy", 100)},
	}
	if err := ReplaceDocs(ctx, db, docs, nil, nil); err != nil {
		t.Fatal(err)
	}
	items, err := ListDocRecordsPaths(ctx, db, "wiki/")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Path != "wiki/00-person.md" || items[1].Path != "wiki/10-project.md" {
		t.Fatalf("items=%#v", items)
	}
}

func TestReplaceDocsWithSources_BindingsRoundTrip(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docPath := ".docs/wiki/09_contratos/CT-BIND.md"
	docs := []model.DocRecord{{Path: docPath, Title: "CT-BIND", DocID: "CT-BIND", Layer: "09", Family: "technical", SearchText: "binding test", IndexedAt: 1}}
	bindings := []model.DocArtifactBinding{
		{
			DocPath:         docPath,
			BlockID:         "CT-BIND.source",
			DocID:           "CT-BIND",
			Relation:        model.RelationImplements,
			TargetPath:      "src/backend/ApiService.cs",
			TargetKind:      model.TargetKindFile,
			AuthoringOrigin: model.AuthoringOriginCanonical,
			BindingStatus:   model.BindingStatusExact,
			DocLifecycle:    model.DocLifecycleActive,
			Ordinal:         1,
			StartLine:       10,
			EndLine:         20,
			BindingRef:      model.WikiCodeBindingRef(docPath, "CT-BIND.source", "CT-BIND", model.RelationImplements, "src/backend/ApiService.cs", "", model.TargetKindFile),
			IndexedAt:       1,
		},
	}
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, nil, nil, bindings); err != nil {
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
	stored, err := ListDocArtifactBindings(ctx, db)
	if err != nil {
		t.Fatalf("ListDocArtifactBindings: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(stored))
	}
	if stored[0].TargetPath != "src/backend/ApiService.cs" {
		t.Fatalf("TargetPath = %q, want %q", stored[0].TargetPath, "src/backend/ApiService.cs")
	}
}

func TestReplaceDocsWithSources_BindingOnlyChangePublishes(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docPath := "wiki/09_contratos/CT-BINDING-ONLY.md"
	docs := []model.DocRecord{{Path: docPath, Title: "CT-BINDING-ONLY", DocID: "CT-BINDING-ONLY", Layer: "09", Family: "technical", SearchText: "binding-only", ContentHash: "doc-hash", IndexedAt: 1}}
	binding := model.DocArtifactBinding{
		DocPath:         docPath,
		BlockID:         "CT-BINDING-ONLY.source",
		DocID:           "CT-BINDING-ONLY",
		Relation:        model.RelationImplements,
		TargetPath:      "src/old.cs",
		TargetKind:      model.TargetKindFile,
		AuthoringOrigin: model.AuthoringOriginCanonical,
		BindingStatus:   model.BindingStatusExact,
		DocLifecycle:    model.DocLifecycleActive,
		Ordinal:         1,
		StartLine:       1,
		EndLine:         2,
		BindingRef:      model.WikiCodeBindingRef(docPath, "CT-BINDING-ONLY.source", "CT-BINDING-ONLY", model.RelationImplements, "src/old.cs", "", model.TargetKindFile),
		IndexedAt:       1,
	}
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, nil, nil, []model.DocArtifactBinding{binding}); err != nil {
		t.Fatalf("initial ReplaceDocsWithSources: %v", err)
	}

	updated := binding
	updated.TargetPath = "src/new.cs"
	updated.BindingRef = model.WikiCodeBindingRef(docPath, "CT-BINDING-ONLY.source", "CT-BINDING-ONLY", model.RelationImplements, updated.TargetPath, "", model.TargetKindFile)
	updated.IndexedAt = 2
	docs[0].IndexedAt = 2
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, nil, nil, []model.DocArtifactBinding{updated}); err != nil {
		t.Fatalf("binding-only ReplaceDocsWithSources: %v", err)
	}
	stored, err := ListDocArtifactBindings(ctx, db)
	if err != nil {
		t.Fatalf("ListDocArtifactBindings: %v", err)
	}
	if len(stored) != 1 || stored[0].TargetPath != "src/new.cs" {
		t.Fatalf("binding-only publication stored=%+v, want new target", stored)
	}
}

func TestReplaceDocsWithSources_RepairsStaleSourceAndBindingDrift(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docPath := "wiki/09_contratos/CT-DRIFT.md"
	docs := []model.DocRecord{{Path: docPath, Title: "CT-DRIFT", DocID: "CT-DRIFT", Layer: "09", Family: "technical", SearchText: "drift", ContentHash: "doc-hash", IndexedAt: 1}}
	block := model.DocSourceBlock{DocPath: docPath, BlockID: "CT-DRIFT.source", DocID: "CT-DRIFT", Kind: "contract", SourceFormat: "SDD", Ordinal: 1, StartLine: 1, EndLine: 4, ContentHash: "block-hash", IndexedAt: 1}
	record := model.DocSourceRecord{DocPath: docPath, BlockID: block.BlockID, RecordID: "RF-DRIFT-001", RecordType: "RF", Ordinal: 1, StartLine: 2, EndLine: 3, ContentHash: "record-hash", IndexedAt: 1}
	binding := model.DocArtifactBinding{
		DocPath:         docPath,
		BlockID:         block.BlockID,
		DocID:           docs[0].DocID,
		Relation:        model.RelationImplements,
		TargetPath:      "src/current.cs",
		TargetKind:      model.TargetKindFile,
		AuthoringOrigin: model.AuthoringOriginCanonical,
		BindingStatus:   model.BindingStatusExact,
		DocLifecycle:    model.DocLifecycleActive,
		Ordinal:         1,
		StartLine:       1,
		EndLine:         4,
		SourceContentHash: block.ContentHash,
		BindingRef:      model.WikiCodeBindingRef(docPath, block.BlockID, docs[0].DocID, model.RelationImplements, "src/current.cs", "", model.TargetKindFile),
		IndexedAt:       1,
	}
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, []model.DocSourceBlock{block}, []model.DocSourceRecord{record}, []model.DocArtifactBinding{binding}); err != nil {
		t.Fatalf("initial ReplaceDocsWithSources: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE doc_source_blocks SET content_hash=? WHERE doc_path=? AND block_id=?", "stale-block", docPath, block.BlockID); err != nil {
		t.Fatalf("stale source update: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE doc_artifact_bindings SET target_path=?, binding_ref=? WHERE doc_path=? AND block_id=?", "src/stale.cs", "stale-binding", docPath, block.BlockID); err != nil {
		t.Fatalf("stale binding update: %v", err)
	}

	docs[0].IndexedAt = 2
	block.IndexedAt = 2
	record.IndexedAt = 2
	binding.IndexedAt = 2
	if err := ReplaceDocsWithSources(ctx, db, docs, nil, nil, []model.DocSourceBlock{block}, []model.DocSourceRecord{record}, []model.DocArtifactBinding{binding}); err != nil {
		t.Fatalf("drift repair ReplaceDocsWithSources: %v", err)
	}
	storedBlocks, err := ListDocSourceBlocks(ctx, db)
	if err != nil {
		t.Fatalf("ListDocSourceBlocks: %v", err)
	}
	storedBindings, err := ListDocArtifactBindings(ctx, db)
	if err != nil {
		t.Fatalf("ListDocArtifactBindings: %v", err)
	}
	if len(storedBlocks) != 1 || storedBlocks[0].ContentHash != block.ContentHash {
		t.Fatalf("stale source drift remained: %+v", storedBlocks)
	}
	if len(storedBindings) != 1 || storedBindings[0].TargetPath != binding.TargetPath || storedBindings[0].BindingRef != binding.BindingRef {
		t.Fatalf("stale binding drift remained: %+v", storedBindings)
	}
}

func TestBindingsForTarget(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docPath := ".docs/wiki/00_test.md"
	bindings := []model.DocArtifactBinding{
		{
			DocPath:      docPath,
			BlockID:      "b1",
			DocID:        "TECH-01",
			Relation:     model.RelationImplements,
			TargetPath:   "src/api/Service.cs",
			TargetKind:   model.TargetKindFile,
			AuthoringOrigin: model.AuthoringOriginCanonical,
			BindingRef:  model.WikiCodeBindingRef(docPath, "b1", "TECH-01", model.RelationImplements, "src/api/Service.cs", "", model.TargetKindFile),
			Ordinal:     1,
			IndexedAt:   1,
		},
		{
			DocPath:      docPath,
			BlockID:      "b2",
			DocID:        "TECH-02",
			Relation:     model.RelationTests,
			TargetPath:   "src/api/Service.cs",
			TargetSymbol: "Validate",
			TargetKind:   model.TargetKindSymbol,
			AuthoringOrigin: model.AuthoringOriginCanonical,
			BindingRef:  model.WikiCodeBindingRef(docPath, "b2", "TECH-02", model.RelationTests, "src/api/Service.cs", "Validate", model.TargetKindSymbol),
			Ordinal:     2,
			IndexedAt:   1,
		},
	}
	if err := ReplaceDocsWithSources(ctx, db, nil, nil, nil, nil, nil, bindings); err != nil {
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
	results, err := BindingsForTarget(ctx, db, "src/api/Service.cs", "Validate")
	if err != nil {
		t.Fatalf("BindingsForTarget: %v", err)
	}
	if len(results) != 1 || results[0].TargetSymbol != "Validate" {
		t.Fatalf("exact target lookup failed: %#v", results)
	}
}

func TestBindingsForDocID(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	bindings := []model.DocArtifactBinding{
		{
			DocPath:      "wiki/00.md",
			BlockID:      "b1",
			DocID:        "RF-OLD-001",
			Relation:     model.RelationImplements,
			TargetPath:   "src/old.cs",
			TargetKind:   model.TargetKindFile,
			AuthoringOrigin: model.AuthoringOriginCanonical,
			BindingRef:  model.WikiCodeBindingRef("wiki/00.md", "b1", "RF-OLD-001", model.RelationImplements, "src/old.cs", "", model.TargetKindFile),
			Ordinal:     1,
			IndexedAt:   1,
		},
	}
	if err := ReplaceDocsWithSources(ctx, db, nil, nil, nil, nil, nil, bindings); err != nil {
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
	results, err := BindingsForDocID(ctx, db, "RF-OLD-001")
	if err != nil {
		t.Fatalf("BindingsForDocID: %v", err)
	}
	if len(results) != 1 || results[0].DocID != "RF-OLD-001" {
		t.Fatalf("BindingsForDocID failed: %#v", results)
	}
}

func TestReplaceDocsWithSources_FullReplacementRemovesDeletedBinding(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	docPath := ".docs/wiki/00_test.md"
	bindings := []model.DocArtifactBinding{
		{
			DocPath:      docPath,
			BlockID:      "b1",
			DocID:        "TECH-01",
			Relation:     model.RelationImplements,
			TargetPath:   "src/keep.cs",
			TargetKind:   model.TargetKindFile,
			AuthoringOrigin: model.AuthoringOriginCanonical,
			BindingRef:  model.WikiCodeBindingRef(docPath, "b1", "TECH-01", model.RelationImplements, "src/keep.cs", "", model.TargetKindFile),
			Ordinal:     1,
			IndexedAt:   1,
		},
	}
	if err := ReplaceDocsWithSources(ctx, db, nil, nil, nil, nil, nil, bindings); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	// Replace with only one binding (deleted the other)
	bindings2 := []model.DocArtifactBinding{
		{
			DocPath:      docPath,
			BlockID:      "b1",
			DocID:        "TECH-02",
			Relation:     model.RelationImplements,
			TargetPath:   "src/new.cs",
			TargetKind:   model.TargetKindFile,
			AuthoringOrigin: model.AuthoringOriginCanonical,
			BindingRef:  model.WikiCodeBindingRef(docPath, "b1", "TECH-02", model.RelationImplements, "src/new.cs", "", model.TargetKindFile),
			Ordinal:     1,
			IndexedAt:   1,
		},
	}
	if err := ReplaceDocsWithSources(ctx, db, nil, nil, nil, nil, nil, bindings2); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	stored, err := ListDocArtifactBindings(ctx, db)
	if err != nil {
		t.Fatalf("ListDocArtifactBindings: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 binding after replacement, got %d", len(stored))
	}
	if stored[0].TargetPath != "src/new.cs" {
		t.Fatalf("target should be src/new.cs, got %q", stored[0].TargetPath)
	}
}

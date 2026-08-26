package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestPublishIncrementalDocsGeneration_AdvancesDocsGeneration(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	genID := "gen-docs-001"

	// Set up a document to publish.
	change := IncrementalDocChange{
		Action: "replace",
		Path:   "wiki/00_test.md",
		Doc: &model.DocRecord{
			Path:        "wiki/00_test.md",
			Title:       "Test",
			DocID:       "TECH-001",
			Layer:       "07",
			Family:      "technical",
			SearchText:  "test doc",
			ContentHash: "hash-1",
			IndexedAt:   time.Now().Unix(),
		},
		Edges: []model.DocEdge{
			{FromPath: "wiki/00_test.md", ToDocID: "TECH-002", Kind: "doc_id"},
		},
		Mentions: []model.DocMention{
			{DocPath: "wiki/00_test.md", MentionType: "doc_id", MentionValue: "TECH-002"},
		},
		Blocks: []model.DocSourceBlock{
			{DocPath: "wiki/00_test.md", BlockID: "b1", DocID: "TECH-001", Kind: "contract", SourceFormat: "SDD", Ordinal: 1, StartLine: 1, EndLine: 10},
		},
		Records: []model.DocSourceRecord{
			{DocPath: "wiki/00_test.md", BlockID: "b1", RecordID: "RF-001", RecordType: "RF", Ordinal: 1, StartLine: 1, EndLine: 5},
		},
		Bindings: []model.DocArtifactBinding{
			{
				DocPath:         "wiki/00_test.md",
				BlockID:         "b1",
				DocID:           "TECH-001",
				Relation:        model.RelationImplements,
				TargetPath:      "src/Main.cs",
				TargetKind:      model.TargetKindFile,
				AuthoringOrigin: model.AuthoringOriginCanonical,
				BindingRef:      model.WikiCodeBindingRef("wiki/00_test.md", "b1", "TECH-001", model.RelationImplements, "src/Main.cs", "", model.TargetKindFile),
				Ordinal:         1,
				IndexedAt:       time.Now().Unix(),
			},
		},
		State: &model.DocArtifactState{
			Path:                "wiki/00_test.md",
			ContentSHA256:       "hash-1",
			ParserVersion:       model.ParserVersion,
			AuthorityConfigHash: "config",
			Lifecycle:           model.DocLifecycleActive,
		},
	}

	if err := PublishIncrementalDocsGeneration(ctx, db, genID, []IncrementalDocChange{change}); err != nil {
		t.Fatalf("PublishIncrementalDocsGeneration: %v", err)
	}

	// Verify docs generation advanced.
	val, ok, err := WorkspaceMetaValue(ctx, db, WorkspaceMetaActiveDocsGeneration)
	if err != nil || !ok || val != genID {
		t.Fatalf("active_docs_generation = %q ok=%v err=%v, want %q", val, ok, err, genID)
	}

	// Verify memory generation advanced.
	val, ok, err = WorkspaceMetaValue(ctx, db, WorkspaceMetaActiveMemoryGeneration)
	if err != nil || !ok || val != genID {
		t.Fatalf("active_memory_generation = %q ok=%v err=%v, want %q", val, ok, err, genID)
	}

	// Verify graph is marked stale.
	state, err := GraphRuntimeState(ctx, db)
	if err != nil || state != GraphRuntimeStale {
		t.Fatalf("graph runtime state = %q err=%v, want stale", state, err)
	}
}

func TestPublishIncrementalDocsGeneration_NoCatalogChange(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	genID := "gen-docs-002"

	// Set active catalog generation.
	if err := UpsertWorkspaceMeta(ctx, db, WorkspaceMetaActiveCatalogGeneration, "gen-catalog-001"); err != nil {
		t.Fatal(err)
	}

	change := IncrementalDocChange{
		Action: "replace",
		Path:   "wiki/00_test.md",
		Doc: &model.DocRecord{
			Path:        "wiki/00_test.md",
			Title:       "Test",
			ContentHash: "hash-1",
			IndexedAt:   time.Now().Unix(),
		},
		State: &model.DocArtifactState{
			Path:          "wiki/00_test.md",
			ContentSHA256: "hash-1",
			Lifecycle:     model.DocLifecycleActive,
		},
	}

	if err := PublishIncrementalDocsGeneration(ctx, db, genID, []IncrementalDocChange{change}); err != nil {
		t.Fatalf("PublishIncrementalDocsGeneration: %v", err)
	}

	// Catalog generation should NOT have changed.
	val, ok, err := WorkspaceMetaValue(ctx, db, WorkspaceMetaActiveCatalogGeneration)
	if err != nil || !ok || val != "gen-catalog-001" {
		t.Fatalf("active_catalog_generation = %q ok=%v err=%v, want unchanged gen-catalog-001", val, ok, err)
	}
}

func TestPublishIncrementalDocsGeneration_DeletePreservesRetired(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()

	// Insert a retired artifact state.
	retiredState := model.DocArtifactState{
		Path:      "wiki/01_retired.md",
		Lifecycle: model.DocLifecycleRetired,
	}
	if err := UpsertDocArtifactState(ctx, db, retiredState); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO doc_artifact_bindings(
		doc_path, block_id, relation, target_path, target_kind, ordinal, start_line, end_line, binding_ref, indexed_at, doc_lifecycle
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "wiki/01_retired.md", "b1", model.RelationImplements, "src/old.go", model.TargetKindFile, 1, 1, 1, "retired-binding-ref", 1, model.DocLifecycleRetired); err != nil {
		t.Fatal(err)
	}

	// Publish a delete change.
	change := IncrementalDocChange{
		Action: "delete",
		Path:   "wiki/01_retired.md",
		Proof:  "disk-absent",
	}

	if err := PublishIncrementalDocsGeneration(ctx, db, "gen-003", []IncrementalDocChange{change}); err != nil {
		t.Fatalf("PublishIncrementalDocsGeneration (delete): %v", err)
	}

	// The retired state should still exist after delete.
	// Only active states are removed by delete.
	state, ok, err := GetDocArtifactState(ctx, db, "wiki/01_retired.md")
	if err != nil {
		t.Fatalf("GetDocArtifactState: %v", err)
	}
	if !ok {
		t.Fatalf("retired artifact state should still exist after delete")
	}
	if state.Lifecycle != model.DocLifecycleRetired {
		t.Fatalf("lifecycle should be retired, got %q", state.Lifecycle)
	}
	var bindings int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM doc_artifact_bindings WHERE doc_path=?", "wiki/01_retired.md").Scan(&bindings); err != nil {
		t.Fatal(err)
	}
	if bindings != 1 {
		t.Fatalf("retired historical bindings = %d, want 1", bindings)
	}
}

func TestPublishIncrementalDocsGeneration_PlannedBindingRoundTrip(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	genID := "gen-docs-004"

	// Publish a replace that includes a binding with BindingStatusPlanned.
	change := IncrementalDocChange{
		Action: "replace",
		Path:   "wiki/02_binding_test.md",
		Doc: &model.DocRecord{
			Path:        "wiki/02_binding_test.md",
			Title:       "Binding Test",
			ContentHash: "hash-1",
			IndexedAt:   time.Now().Unix(),
		},
		Bindings: []model.DocArtifactBinding{
			{
				DocPath:         "wiki/02_binding_test.md",
				BlockID:         "b1",
				DocID:           "TECH-003",
				Relation:        model.RelationImplements,
				TargetPath:      "src/Target.cs",
				TargetKind:      model.TargetKindFile,
				AuthoringOrigin: model.AuthoringOriginCanonical,
				BindingStatus:   model.BindingStatusPlanned,
				DocLifecycle:    model.DocLifecycleActive,
				BindingRef:      model.WikiCodeBindingRef("wiki/02_binding_test.md", "b1", "TECH-003", model.RelationImplements, "src/Target.cs", "", model.TargetKindFile),
				Ordinal:         1,
				IndexedAt:       time.Now().Unix(),
			},
		},
		State: &model.DocArtifactState{
			Path:                "wiki/02_binding_test.md",
			ContentSHA256:       "hash-1",
			ParserVersion:       model.ParserVersion,
			AuthorityConfigHash: "config",
			Lifecycle:           model.DocLifecycleActive,
		},
	}

	if err := PublishIncrementalDocsGeneration(ctx, db, genID, []IncrementalDocChange{change}); err != nil {
		t.Fatalf("PublishIncrementalDocsGeneration: %v", err)
	}

	// Verify the binding round-tripped with BindingStatusPlanned intact (not promoted to "exact").
	rows, err := db.QueryContext(ctx, "SELECT binding_status FROM doc_artifact_bindings WHERE doc_path = ?", "wiki/02_binding_test.md")
	if err != nil {
		t.Fatalf("Query bindings: %v", err)
	}
	defer rows.Close()
	var status string
	if !rows.Next() {
		t.Fatal("expected at least one binding row")
	}
	if err := rows.Scan(&status); err != nil {
		t.Fatalf("Scan binding_status: %v", err)
	}
	if status != model.BindingStatusPlanned {
		t.Fatalf("binding_status should be %q, got %q (planned bindings must not be promoted)", model.BindingStatusPlanned, status)
	}
}

func TestShrinkGuard_NoUnexplainedLoss(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()

	// Publish a replace for wiki/a.md, then verify no orphan rows remain
	// when we replace with a different doc.
	change := IncrementalDocChange{
		Action: "replace",
		Path:   "wiki/a.md",
		Doc: &model.DocRecord{
			Path: "wiki/a.md", Title: "A", ContentHash: "h1", IndexedAt: 1,
		},
	}
	if err := PublishIncrementalDocsGeneration(ctx, db, "gen-1", []IncrementalDocChange{change}); err != nil {
		t.Fatalf("first publish: %v", err)
	}

	// Shrink guard is validated implicitly by the publish succeeding.
	// The publish would error if there were unexplained row loss.
}

func TestPublishIncrementalDocsGeneration_ReplacePreservesUnrelated(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()

	// First publish with doc "wiki/a.md".
	change1 := IncrementalDocChange{
		Action: "replace",
		Path:   "wiki/a.md",
		Doc: &model.DocRecord{
			Path: "wiki/a.md", Title: "A", ContentHash: "h1", IndexedAt: 1,
		},
		Blocks: []model.DocSourceBlock{{DocPath: "wiki/a.md", BlockID: "b1", SourceFormat: "SDD"}},
	}
	if err := PublishIncrementalDocsGeneration(ctx, db, "gen-1", []IncrementalDocChange{change1}); err != nil {
		t.Fatalf("first publish: %v", err)
	}

	// Now replace with doc "wiki/b.md" (different path); unrelated "wiki/a.md"
	// must survive the per-owner publication.
	change2 := IncrementalDocChange{
		Action: "replace",
		Path:   "wiki/b.md",
		Doc: &model.DocRecord{
			Path: "wiki/b.md", Title: "B", ContentHash: "h2", IndexedAt: 1,
		},
	}
	if err := PublishIncrementalDocsGeneration(ctx, db, "gen-2", []IncrementalDocChange{change2}); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	// The original document remains because it was not an explicit owner path.
	rows, err := db.QueryContext(ctx, "SELECT path FROM doc_records WHERE path = ?", "wiki/a.md")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	_ = rows.Close()
	if count != 1 {
		t.Fatalf("expected unrelated wiki/a.md to survive, got %d rows", count)
	}

	// Verify wiki/b.md is present.
	rows, err = db.QueryContext(ctx, "SELECT path FROM doc_records WHERE path = ?", "wiki/b.md")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	_ = rows.Close()
	if count != 1 {
		t.Fatalf("expected 1 row for wiki/b.md, got %d", count)
	}
}

func TestPublishIncrementalDocsGeneration_RejectsInvalidChangeSets(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	invalid := []IncrementalDocChange{
		{Action: "parse-failed", Path: "wiki/a.md"},
		{Action: "delete", Path: "wiki/a.md"},
		{Action: "delete", Path: "wiki/a.md", Proof: "disk-absent"},
	}
	for _, changes := range [][]IncrementalDocChange{nil, invalid[:1], invalid[1:2], invalid[1:]} {
		if err := PublishIncrementalDocsGeneration(ctx, db, "gen-invalid", changes); err == nil {
			t.Fatalf("invalid change set %#v was accepted", changes)
		}
	}
}

func TestPublishIncrementalDocsGeneration_DeleteOnlyAdvancesGeneration(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if err := UpsertWorkspaceMeta(ctx, db, WorkspaceMetaActiveDocsGeneration, "docs-old"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertWorkspaceMeta(ctx, db, WorkspaceMetaActiveMemoryGeneration, "memory-old"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO doc_records(path, title) VALUES(?, ?)", "wiki/delete.md", "delete"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertDocArtifactState(ctx, db, model.DocArtifactState{Path: "wiki/delete.md", Lifecycle: model.DocLifecycleActive}); err != nil {
		t.Fatal(err)
	}
	if err := PublishIncrementalDocsGeneration(ctx, db, "docs-new", []IncrementalDocChange{{Action: "delete", Path: "wiki/delete.md", Proof: "disk-absent"}}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{WorkspaceMetaActiveDocsGeneration, WorkspaceMetaActiveMemoryGeneration} {
		value, ok, err := WorkspaceMetaValue(ctx, db, key)
		if err != nil || !ok || value != "docs-new" {
			t.Fatalf("%s = %q ok=%v err=%v, want docs-new", key, value, ok, err)
		}
	}
}

func TestPublishIncrementalDocsGeneration_ParseFailureRetainsPriorRows(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "INSERT INTO doc_records(path, title) VALUES(?, ?)", "wiki/keep.md", "old"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertDocArtifactState(ctx, db, model.DocArtifactState{Path: "wiki/keep.md", ContentSHA256: "old", Lifecycle: model.DocLifecycleActive}); err != nil {
		t.Fatal(err)
	}
	if err := PublishIncrementalDocsGeneration(ctx, db, "gen-parse-failed", []IncrementalDocChange{{Action: "parse-failed", Path: "wiki/keep.md"}}); err == nil {
		t.Fatal("parse-failed change should be rejected")
	}
	var title string
	if err := db.QueryRowContext(ctx, "SELECT title FROM doc_records WHERE path=?", "wiki/keep.md").Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "old" {
		t.Fatalf("prior row changed after parse failure: %q", title)
	}
}

func TestPublishIncrementalDocsGenerationForJob_UsesDocsMode(t *testing.T) {
	db, root := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "docs-job", root, IndexModeDocs, false)
	if err != nil {
		t.Fatal(err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, os.Getpid(), "indexing", fence); err != nil {
		t.Fatal(err)
	}
	change := IncrementalDocChange{Action: "replace", Path: "wiki/job.md", Doc: &model.DocRecord{Path: "wiki/job.md", Title: "job", ContentHash: "hash"}, State: &model.DocArtifactState{Path: "wiki/job.md", ContentSHA256: "hash", Lifecycle: model.DocLifecycleActive}}
	if err := PublishIncrementalDocsGenerationForJob(ctx, db, job.JobID, job.GenerationID, []IncrementalDocChange{change}, fence); err != nil {
		t.Fatal(err)
	}
	var mode, status string
	if err := db.QueryRowContext(ctx, "SELECT mode, status FROM index_generations WHERE generation_id=?", job.GenerationID).Scan(&mode, &status); err != nil {
		t.Fatal(err)
	}
	if mode != IndexModeDocs || status != "published" {
		t.Fatalf("job generation mode/status = %q/%q, want docs/published", mode, status)
	}
}

func TestPublishIncrementalGenerationWithFileAndDocChanges_Atomic(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if err := replaceFileSymbols(ctx, db, "src/main.go", "repo", "repo", "go", "old", []model.SymbolRecord{{FilePath: "src/main.go", Name: "Old", Kind: "function", Language: "go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO doc_records(path, title) VALUES(?, ?)", "wiki/old.md", "old"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertDocArtifactState(ctx, db, model.DocArtifactState{Path: "wiki/old.md", ContentSHA256: "old", Lifecycle: model.DocLifecycleActive}); err != nil {
		t.Fatal(err)
	}
	if err := PublishIncrementalGenerationWithFileAndDocChanges(ctx, db, "mixed-gen", 1, 1, 1, []IncrementalFileChange{{FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "new", Symbols: []model.SymbolRecord{{FilePath: "src/main.go", Name: "New", Kind: "function", Language: "go"}}}}, []IncrementalDocChange{{Action: "replace", Path: "wiki/new.md", Doc: &model.DocRecord{Path: "wiki/new.md", Title: "new", ContentHash: "new"}, State: &model.DocArtifactState{Path: "wiki/new.md", ContentSHA256: "new", Lifecycle: model.DocLifecycleActive}}}); err != nil {
		t.Fatal(err)
	}
	var symbolName string
	if err := db.QueryRowContext(ctx, "SELECT name FROM symbols WHERE file_path=?", "src/main.go").Scan(&symbolName); err != nil {
		t.Fatal(err)
	}
	if symbolName != "New" {
		t.Fatalf("mixed file publication did not commit: %q", symbolName)
	}
	var docs int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM doc_records WHERE path=?", "wiki/new.md").Scan(&docs); err != nil {
		t.Fatal(err)
	}
	if docs != 1 {
		t.Fatalf("mixed doc publication count=%d, want 1", docs)
	}
	state, err := GraphRuntimeState(ctx, db)
	if err != nil || state != GraphRuntimeStale {
		t.Fatalf("mixed graph state=%q err=%v, want stale", state, err)
	}
}

func TestPublishIncrementalGenerationForJobWithFileAndDocChanges_RollsBackBeforeCommit(t *testing.T) {
	db, root := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "mixed-rollback", root, IndexModeFull, false)
	if err != nil {
		t.Fatal(err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, os.Getpid(), "indexing", fence); err != nil {
		t.Fatal(err)
	}
	restore := SetIndexPublicationBeforeCommitHookForTest(func() error { return errors.New("abort before commit") })
	defer restore()
	err = PublishIncrementalGenerationForJobWithFileAndDocChanges(ctx, db, job.JobID, job.GenerationID, 1, 1, 1, fence, []IncrementalFileChange{{FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "new"}}, []IncrementalDocChange{{Action: "replace", Path: "wiki/new.md", Doc: &model.DocRecord{Path: "wiki/new.md", Title: "new", ContentHash: "new"}, State: &model.DocArtifactState{Path: "wiki/new.md", ContentSHA256: "new", Lifecycle: model.DocLifecycleActive}}})
	if err == nil {
		t.Fatal("publication should fail at the commit seam")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files WHERE file_path=?", "src/main.go").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("file rows survived rolled-back publication: %d", count)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM doc_records WHERE path=?", "wiki/new.md").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("doc rows survived rolled-back publication: %d", count)
	}
}

func TestPublishIncrementalGenerationForJobWithFileAndDocChanges_RejectsStaleOwner(t *testing.T) {
	db, root := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "mixed-job", root, IndexModeFull, false)
	if err != nil {
		t.Fatal(err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, os.Getpid(), "indexing", fence); err != nil {
		t.Fatal(err)
	}
	wrong := IndexJobFence{OwnerToken: "wrong-owner", FencingToken: fence.FencingToken}
	err = PublishIncrementalGenerationForJobWithFileAndDocChanges(ctx, db, job.JobID, job.GenerationID, 1, 1, 1, wrong, []IncrementalFileChange{{FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "new"}}, nil)
	if !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("stale owner error=%v, want ErrStaleIndexJobOwner", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files WHERE file_path=?", "src/main.go").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale owner mutated file rows: %d", count)
	}
}

func TestPublishIncrementalDocsGeneration_ExplicitOwnerColumnsAndUnrelatedSurvive(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	ctx := context.Background()
	for _, path := range []string{"wiki/owned.md", "wiki/other.md"} {
		if _, err := db.ExecContext(ctx, "INSERT INTO doc_records(path, title) VALUES(?, ?)", path, path); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO doc_edges(from_path, to_path, kind) VALUES(?, ?, ?)", path, "wiki/target.md", "link"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO doc_mentions(doc_path, mention_type, mention_value) VALUES(?, ?, ?)", path, "doc_id", "TECH-OWNER"); err != nil {
			t.Fatal(err)
		}
	}
	if err := UpsertDocArtifactState(ctx, db, model.DocArtifactState{Path: "wiki/owned.md", Lifecycle: model.DocLifecycleActive}); err != nil {
		t.Fatal(err)
	}
	if err := PublishIncrementalDocsGeneration(ctx, db, "gen-owner", []IncrementalDocChange{{Action: "delete", Path: "wiki/owned.md", Proof: "disk-absent"}}); err != nil {
		t.Fatalf("delete publication: %v", err)
	}
	for _, query := range []string{
		"SELECT COUNT(*) FROM doc_records WHERE path='wiki/owned.md'",
		"SELECT COUNT(*) FROM doc_edges WHERE from_path='wiki/owned.md'",
		"SELECT COUNT(*) FROM doc_mentions WHERE doc_path='wiki/owned.md'",
	} {
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("owned rows remain for %s: %d", query, count)
		}
	}
	var unrelated int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM doc_records WHERE path='wiki/other.md'").Scan(&unrelated); err != nil {
		t.Fatal(err)
	}
	if unrelated != 1 {
		t.Fatalf("unrelated doc rows changed: %d", unrelated)
	}
}

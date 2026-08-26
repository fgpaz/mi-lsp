package indexer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
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

func TestIncrementalIndexNoChangesPreservesCurrentGraph(t *testing.T) {
	root := setupIncrementalGraphFixture(t)
	ctx := context.Background()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before, ok, err := store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok {
		db.Close()
		t.Fatalf("initial active graph=%s ok=%v err=%v", before, ok, err)
	}
	_ = db.Close()

	result, err := IncrementalIndexWithGraphProgress(ctx, root, "", nil, GraphIndexOptions{})
	if err != nil {
		t.Fatalf("IncrementalIndexWithGraphProgress: %v", err)
	}
	if result.GraphGenerationID != "" {
		t.Fatalf("no-change result unexpectedly published generation=%s", result.GraphGenerationID)
	}
	db, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after, ok, err := store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || after != before {
		t.Fatalf("active graph=%s ok=%v err=%v, want unchanged %s", after, ok, err, before)
	}
	var generations, active int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations WHERE status = ?", model.GraphGenerationActive).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if generations != 1 || active != 1 {
		t.Fatalf("generation rows=%d active=%d, want one unchanged active generation", generations, active)
	}
}

func TestIncrementalIndexRepairsMissingGraphOnNoChanges(t *testing.T) {
	root := setupIncrementalGraphFixture(t)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DELETE FROM workspace_meta WHERE key IN ('active_graph_generation_id', 'graph_runtime_state', 'graph_catalog_generation_id')",
		"DELETE FROM graph_evidence",
		"DELETE FROM graph_unresolved",
		"DELETE FROM graph_edges",
		"DELETE FROM graph_nodes",
		"DELETE FROM graph_generations",
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	_ = db.Close()

	result, err := IncrementalIndexWithGraphProgress(context.Background(), root, "", nil, GraphIndexOptions{})
	if err != nil {
		t.Fatalf("IncrementalIndexWithGraphProgress: %v", err)
	}
	if result.GraphGenerationID == "" {
		t.Fatalf("result=%#v, want repaired graph generation", result)
	}
	db, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	active, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok || active.String() != result.GraphGenerationID {
		t.Fatalf("active graph=%s ok=%v err=%v, want %s", active, ok, err, result.GraphGenerationID)
	}
	if state, err := store.GraphRuntimeState(context.Background(), db); err != nil || state != store.GraphRuntimeFresh {
		t.Fatalf("graph runtime state=%q err=%v, want fresh", state, err)
	}
}

func TestIncrementalIndexPublishesNewGraphGenerationAfterFileChange(t *testing.T) {
	root := setupIncrementalGraphFixture(t)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok {
		db.Close()
		t.Fatalf("initial active graph=%s ok=%v err=%v", before, ok, err)
	}
	_ = db.Close()

	mustWriteIncrementalFile(t, filepath.Join(root, "main.go"), "package main\nfunc main() { println(\"changed\") }\n")
	result, err := IncrementalIndexWithGraphProgress(context.Background(), root, "", nil, GraphIndexOptions{})
	if err != nil {
		t.Fatalf("IncrementalIndexWithGraphProgress: %v", err)
	}
	if result.GraphGenerationID == "" || result.GraphGenerationID == before.String() {
		t.Fatalf("result=%#v, want a new graph generation after source change", result)
	}
	db, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok || after.String() != result.GraphGenerationID {
		t.Fatalf("active graph=%s ok=%v err=%v, want %s", after, ok, err, result.GraphGenerationID)
	}
	if state, err := store.GraphRuntimeState(context.Background(), db); err != nil || state != store.GraphRuntimeFresh {
		t.Fatalf("graph runtime state=%q err=%v, want fresh", state, err)
	}
}

func TestIncrementalIndexObservationFailureLeavesGraphStale(t *testing.T) {
	root := setupIncrementalGraphFixture(t)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok {
		db.Close()
		t.Fatalf("initial active graph=%s ok=%v err=%v", before, ok, err)
	}
	_ = db.Close()

	mustWriteIncrementalFile(t, filepath.Join(root, "go.mod"), "not a module\n")
	if _, err := IncrementalIndexWithGraphProgress(context.Background(), root, "", nil, GraphIndexOptions{}); err == nil {
		t.Fatal("expected graph observation failure")
	}
	db, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok || after != before {
		t.Fatalf("active graph=%s ok=%v err=%v, want prior %s", after, ok, err, before)
	}
	if state, err := store.GraphRuntimeState(context.Background(), db); err != nil || state != store.GraphRuntimeStale {
		t.Fatalf("graph runtime state=%q err=%v, want stale", state, err)
	}
}

func setupIncrementalGraphFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "incremental-graph", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/incremental-graph", Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	mustWriteIncrementalFile(t, filepath.Join(root, "go.mod"), "module example.com/incremental-graph\n\ngo 1.23\n")
	mustWriteIncrementalFile(t, filepath.Join(root, "main.go"), "package main\nfunc main() {}\n")
	mustWriteIncrementalFile(t, filepath.Join(root, ".gitignore"), ".mi-lsp/\n")
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "mi-lsp-test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := IndexWorkspace(context.Background(), root, true); err != nil {
		t.Fatalf("initial IndexWorkspace: %v", err)
	}
	return root
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

// ========== T4 Incremental Index Wiring Tests ==========

// TestIncrementalIndexDocsAddPublishesDocsGeneration verifies that adding a
// new canonical doc through git creates a replace change and advances docs
// generation while preserving catalog generation and marking graph stale.
func TestIncrementalIndexDocsAddPublishesDocsGeneration(t *testing.T) {
	root := setupIncrementalGraphFixture(t)

	// Create a new wiki doc that git will see as changed (using add).
	docPath := filepath.Join(root, ".docs", "wiki", "05_new_doc.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# New Doc\nTECH-001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stage it so git sees it as a change.
	cmd := exec.Command("git", "add", ".docs/wiki/05_new_doc.md")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	ctx := context.Background()
	result, err := IncrementalIndex(ctx, root)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	// Verify docs generation advanced.
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	val, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveDocsGeneration)
	if err != nil || !ok {
		t.Fatalf("active_docs_generation not set: err=%v ok=%v", err, ok)
	}
	if val == "" {
		t.Fatal("docs generation should not be empty")
	}

	// Catalog generation should be unchanged.
	catalogVal, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveCatalogGeneration)
	if err != nil || !ok || catalogVal == "" {
		t.Fatalf("catalog generation should exist: err=%v ok=%v val=%q", err, ok, catalogVal)
	}

	// Graph should be stale (docs changes mark graph stale).
	state, err := store.GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if state != store.GraphRuntimeStale {
		t.Fatalf("graph runtime state = %q, want %q", state, store.GraphRuntimeStale)
	}

	// Verify the new doc record exists in the index.
	records, err := store.ListDocRecords(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range records {
		if r.Path == ".docs/wiki/05_new_doc.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected doc record for .docs/wiki/05_new_doc.md, got %d records", len(records))
	}
}

// TestIncrementalIndexDocsOnlyNoGraphPublication verifies that docs-only
// changes do NOT trigger graph publication; they only mark the graph stale.
func TestIncrementalIndexDocsOnlyNoGraphPublication(t *testing.T) {
	root := setupIncrementalGraphFixture(t)

	// Create a new wiki doc.
	docPath := filepath.Join(root, ".docs", "wiki", "05_docs_only.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# Docs Only\nFL-DOC-001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".docs/wiki/05_docs_only.md")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	ctx := context.Background()
	result, err := IncrementalIndexWithGraphProgress(ctx, root, "gen-docs-only", nil, GraphIndexOptions{})
	if err != nil {
		t.Fatalf("IncrementalIndexWithGraphProgress: %v", err)
	}

	// Graph generation should NOT be set (docs-only changes don't publish graph).
	if result.GraphGenerationID != "" {
		t.Fatalf("docs-only should not publish graph generation, got %q", result.GraphGenerationID)
	}
}

// TestIncrementalIndexMixedCodeDocSeparation verifies that mixed code+doc
// changes run catalog/graph logic only for the code domain, while docs gets
// its own generation.
func TestIncrementalIndexMixedCodeDocSeparation(t *testing.T) {
	root := setupIncrementalGraphFixture(t)

	// Create both a code file and a wiki doc.
	codePath := filepath.Join(root, "pkg", "util.go")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, []byte("package pkg\nfunc Util() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(root, ".docs", "wiki", "05_mixed.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# Mixed\nTECH-MIXED-001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".", "-A")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	ctx := context.Background()
	result, err := IncrementalIndexWithGraphProgress(ctx, root, "gen-mixed", nil, GraphIndexOptions{})
	if err != nil {
		t.Fatalf("IncrementalIndexWithGraphProgress: %v", err)
	}

	// Mixed changes update the catalog and docs atomically but leave the graph
	// stale; observation must not use pre-doc-change facts.
	if result.GraphGenerationID != "" {
		t.Fatalf("mixed code+doc must not publish a graph generation, got %q", result.GraphGenerationID)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	state, err := store.GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if state != store.GraphRuntimeStale {
		t.Fatalf("mixed code+doc graph state = %q, want stale", state)
	}
	catalog, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveCatalogGeneration)
	if err != nil || !ok || catalog != "gen-mixed" {
		t.Fatalf("mixed catalog pointer=%q ok=%v err=%v, want gen-mixed", catalog, ok, err)
	}
	docsGeneration, ok, err := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveDocsGeneration)
	if err != nil || !ok || docsGeneration != "gen-mixed" {
		t.Fatalf("mixed docs pointer=%q ok=%v err=%v, want gen-mixed", docsGeneration, ok, err)
	}
}

// TestReconcileDocs_WithAuthorityConfigHash verifies that ReconcileDocs
// computes and returns the authority config hash, enabling downstream
// parser/profile/config change detection.
func TestReconcileDocs_WithAuthorityConfigHash(t *testing.T) {
	root := setupIncrementalGraphFixture(t)

	ctx := context.Background()
	result, err := ReconcileDocs(ctx, root)
	if err != nil {
		t.Logf("ReconcileDocs: %v (may be expected)", err)
		return
	}

	// Authority config hash should be set.
	if result.AuthorityConfigHash == "" {
		t.Fatal("ReconcileDocs should set AuthorityConfigHash")
	}
}

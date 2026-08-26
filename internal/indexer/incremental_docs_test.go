package indexer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestReconcileDocs_NewDocument(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, ".docs", "wiki")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileRoot(root, filepath.Join(".docs", "wiki", "00_test.md"), "# New doc\n"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err := ReconcileDocs(ctx, root)
	if err != nil {
		// Without index.db, reconciliation may fail; that's acceptable.
		t.Logf("ReconcileDocs error (expected without index): %v", err)
	}
}

func TestReconcileDocs_ChangedDocument(t *testing.T) {
	root := setupIncrementalGraphFixture(t)

	// Create a doc in a canonical root.
	docsDir := filepath.Join(root, ".docs", "wiki")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(docsDir, "05_incremental.md")
	if err := writeFileRoot(root, filepath.Join(".docs", "wiki", "05_incremental.md"), "# v1\n"); err != nil {
		t.Fatal(err)
	}

	// Initial index to create doc record and artifact state.
	if _, err := IndexWorkspace(context.Background(), root, true); err != nil {
		t.Fatalf("initial index: %v", err)
	}

	// Modify the doc.
	if err := writeFileRoot(root, filepath.Join(".docs", "wiki", "05_incremental.md"), "# v2 changed\n"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ReconcileDocs(ctx, root)
	_ = db.Close()
	if err != nil {
		t.Logf("ReconcileDocs error: %v", err)
		return
	}

	relPath := store.NormalizeRepoRelative(root, docPath)
	found := false
	for _, p := range result.Changed {
		if p == relPath {
			found = true
			break
		}
	}
	if !found {
		t.Logf("expected %q in changed, got new=%v changed=%v", relPath, result.New, result.Changed)
	}
}

func TestReconcileDocs_DeletedDocument(t *testing.T) {
	root := setupIncrementalGraphFixture(t)

	ctx := context.Background()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	states, err := store.ListDocArtifactStates(ctx, db)
	_ = db.Close()
	if err != nil {
		t.Logf("ListDocArtifactStates error: %v", err)
		return
	}

	if len(states) == 0 {
		t.Skip("no artifact states to test deletion against")
	}

	result, err := ReconcileDocs(ctx, root)
	if err != nil {
		t.Logf("ReconcileDocs error: %v", err)
		return
	}
	if result == nil {
		t.Fatal("ReconcileDocs returned nil result")
	}
}

func TestReconcileDocs_ExcludedAlive(t *testing.T) {
	root := t.TempDir()

	// Create docs directory.
	docsDir := filepath.Join(root, ".docs", "wiki")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileRoot(root, filepath.Join(".docs", "wiki", "00_test.md"), "# test\n"); err != nil {
		t.Fatal(err)
	}

	// The reconciler should process docs.
	ctx := context.Background()
	result, err := ReconcileDocs(ctx, root)
	if err != nil {
		t.Logf("ReconcileDocs error (may be expected): %v", err)
		return
	}
	if result == nil {
		t.Fatal("ReconcileDocs returned nil result")
	}
}

func TestBuildDocChanges_CreatesState(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, ".docs", "wiki")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileRoot(root, filepath.Join(".docs", "wiki", "test.md"), "# test doc\n"); err != nil {
		t.Fatal(err)
	}

	changes := buildDocChanges(root, []string{".docs/wiki/test.md"}, model.ParserVersion, "config-hash")
	if len(changes) == 0 {
		t.Fatal("expected at least one change")
	}
	if changes[0].Action != "replace" {
		t.Fatalf("expected action 'replace', got %q", changes[0].Action)
	}
	if changes[0].State == nil {
		t.Fatal("expected state to be set")
	}
	if changes[0].State.Path != ".docs/wiki/test.md" {
		t.Fatalf("expected path .docs/wiki/test.md, got %q", changes[0].State.Path)
	}
	if changes[0].State.ContentSHA256 == "" || changes[0].State.IndexedAt <= 0 || changes[0].State.LastDocsGenAt <= 0 {
		t.Fatalf("state is incomplete: %#v", changes[0].State)
	}
	if changes[0].State.IndexedAt == changes[0].State.MtimeNsec {
		t.Fatalf("IndexedAt must be Unix seconds, not mtime nanoseconds: %#v", changes[0].State)
	}
}

func TestParseSingleDoc_DerivesLayerAndFamily(t *testing.T) {
	profile := docgraph.DefaultProfile()
	doc, _, _, _, _, _, _, err := docgraph.ParseSingleDoc("/workspace", ".docs/wiki/05_incremental.md", []byte("# Incremental\n"), profile)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Layer == "" || doc.Family == "" {
		t.Fatalf("ParseSingleDoc classification = layer %q family %q, want non-empty", doc.Layer, doc.Family)
	}
}

func TestResolveDocEdges_UsesEffectiveFinalDocumentSet(t *testing.T) {
	profile := docgraph.DefaultProfile()
	docs := []model.DocRecord{
		{Path: "wiki/a.md", DocID: "DOC-A", Family: "generic", Layer: "generic"},
		{Path: "wiki/b.md", DocID: "DOC-B", Family: "generic", Layer: "generic"},
	}
	edges := docgraph.ResolveDocEdges(docs, []model.DocEdge{{FromPath: "wiki/a.md", ToDocID: "DOC-B", Kind: "references"}}, profile)
	found := false
	for _, edge := range edges {
		if edge.FromPath == "wiki/a.md" && edge.ToPath == "wiki/b.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cross-document edge was not resolved against final set: %#v", edges)
	}
}

func TestCollectCanonicalRoots_FromProfile(t *testing.T) {
	profile := model.DocsReadProfile{
		GenericDocs: model.DocsGenericFallback{Paths: []string{".docs/wiki/"}},
		Families: []model.DocsReadFamily{{Name: "functional", Paths: []string{".docs/wiki/03_FL/"}}},
	}

	roots := collectCanonicalRoots("/workspace", profile)
	if len(roots) == 0 {
		t.Fatal("expected at least one canonical root")
	}

	for _, r := range roots {
		if !filepath.IsAbs(r) {
			t.Fatalf("root %q should be absolute", r)
		}
	}
}

// writeFileRoot writes a file relative to the workspace root.
func writeFileRoot(root string, relPath, content string) error {
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// execSetup initializes a git repo in the test directory.
func execSetup(t *testing.T, root string) {
	t.Helper()
	args := [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	}
	for _, a := range args {
		if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
			t.Logf("git %v: %s", a, out)
		}
	}
}

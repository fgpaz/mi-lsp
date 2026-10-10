package indexer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestIncrementalIndexRepairsStaleGraphWithDocumentationGraph(t *testing.T) {
	root := newStaleGraphWorkspace(t)

	result, err := IncrementalIndexWithGraphProgress(context.Background(), root, "repair-catalog-generation", nil, GraphIndexOptions{})
	if err != nil {
		t.Fatalf("incremental graph repair: %v", err)
	}
	if result.GraphGenerationID == "" {
		t.Fatal("incremental repair did not publish a graph generation")
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	freshness, err := store.GraphFreshness(context.Background(), db, "")
	if err != nil {
		t.Fatal(err)
	}
	if freshness.State != model.GraphFreshnessCurrent {
		t.Fatalf("graph freshness = %q (%s), want current", freshness.State, freshness.ReasonCode)
	}
	catalogGeneration, catalogOK, err := store.WorkspaceMetaValue(context.Background(), db, store.WorkspaceMetaActiveCatalogGeneration)
	if err != nil {
		t.Fatal(err)
	}
	graphCatalogGeneration, graphCatalogOK, err := store.WorkspaceMetaValue(context.Background(), db, store.GraphCatalogGenerationMeta)
	if err != nil {
		t.Fatal(err)
	}
	if !catalogOK || !graphCatalogOK || graphCatalogGeneration != catalogGeneration {
		t.Fatalf("graph/catalog generations = %q/%q (present %t/%t), want matching published generations", graphCatalogGeneration, catalogGeneration, graphCatalogOK, catalogOK)
	}
}

func TestIncrementalIndexReturnsTypedErrorWhenStaleGraphCannotBeRebuilt(t *testing.T) {
	root := newStaleGraphWorkspace(t)
	gitTestCommand(t, root, "remote", "remove", "origin")

	_, err := IncrementalIndexWithGraphProgress(context.Background(), root, "repair-catalog-generation", nil, GraphIndexOptions{})
	var observationErr *model.GraphObservationError
	if !errors.As(err, &observationErr) {
		t.Fatalf("error = %v, want typed GraphObservationError", err)
	}
	if observationErr.Code != "graph_repair_incomplete" {
		t.Fatalf("error code = %q, want graph_repair_incomplete", observationErr.Code)
	}

	_, err = IndexWorkspaceWithGraphProgress(context.Background(), root, false, "failed-full-repair", nil, GraphIndexOptions{})
	if !errors.As(err, &observationErr) || observationErr.Code != "graph_repair_incomplete" {
		t.Fatalf("full index error = %v, want graph_repair_incomplete", err)
	}
}

func newStaleGraphWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	project := `[project]
name = "graph-repair-fixture"
languages = ["go"]
kind = "single"
default_repo = "graph-repair-fixture"

[[repo]]
id = "graph-repair-fixture"
name = "graph-repair-fixture"
root = "."
languages = ["go"]
`
	projectPath := filepath.Join(root, ".mi-lsp", "project.toml")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(root, ".docs", "wiki", "architecture.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# Graph repair fixture\n\nCanonical documentation for a docs-only graph.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitTestCommand(t, root, "init", "-q")
	gitTestCommand(t, root, "config", "user.email", "mi-lsp-test@example.invalid")
	gitTestCommand(t, root, "config", "user.name", "mi-lsp test")
	gitTestCommand(t, root, "remote", "add", "origin", "https://github.com/example/graph-repair-fixture.git")
	gitTestCommand(t, root, "add", ".")
	gitTestCommand(t, root, "commit", "-m", "fixture")

	if _, err := IndexWorkspaceWithGraphProgress(context.Background(), root, true, "initial-graph-repair", nil, GraphIndexOptions{}); err != nil {
		t.Fatalf("initial index: %v", err)
	}
	markGraphStaleForTest(t, root)
	return root
}

func markGraphStaleForTest(t *testing.T, root string) {
	t.Helper()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	catalogGeneration, ok, err := store.WorkspaceMetaValue(context.Background(), db, store.WorkspaceMetaActiveCatalogGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || catalogGeneration == "" {
		t.Fatal("active catalog generation is missing")
	}
	if err := store.SetGraphRuntimeState(context.Background(), db, store.GraphRuntimeStale, catalogGeneration); err != nil {
		t.Fatal(err)
	}
}

func gitTestCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

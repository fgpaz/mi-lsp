package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func createDiffWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/S1.cs", strings.Join([]string{
		"namespace Demo;",
		"public class SvcOne",
		"{",
		"    public void Alpha() { }",
		"}",
	}, "\n"))
	runGit(t, root, "init")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "commit", "-m", "init")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func TestDiffContextUsesPublishedGraphForChangedCallee(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "diff-graph-" + filepath.Base(root)
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "go.mod", "module example.com/diffgraph\n\ngo 1.23\n")
	writeWorkspaceFile(t, root, "callee.go", "package diffgraph\n\nfunc Callee() string { return \"baseline\" }\n")
	writeWorkspaceFile(t, root, "caller.go", "package diffgraph\n\nfunc Caller() string { return Callee() }\n")
	writeWorkspaceFile(t, root, "unrelated.go", "package diffgraph\n\nfunc Unrelated() string { return \"unrelated\" }\n")
	runGit(t, root, "init")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "commit", "-m", "baseline")
	if _, err := indexer.IndexWorkspaceWithGraphProgress(context.Background(), root, true, "", nil, indexer.GraphIndexOptions{}); err != nil {
		t.Fatalf("IndexWorkspaceWithGraphProgress: %v", err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	writeWorkspaceFile(t, root, "callee.go", "package diffgraph\n\nfunc Callee() string { return \"changed\" }\n")
	app := New(root, nil)
	env, err := app.diffContext(context.Background(), model.CommandRequest{
		Context: model.QueryOptions{Workspace: alias},
		Payload: map[string]any{"mode": "direct", "edge": []string{"calls"}},
	})
	if err != nil {
		t.Fatalf("diffContext: %v", err)
	}
	if env.Backend != "graph-native" && env.Backend != "graph-native+heuristic" {
		t.Fatalf("backend = %q, want graph-native", env.Backend)
	}
	items, ok := env.Items.([]DiffContextResult)
	if !ok || len(items) != 1 || items[0].GraphImpact == nil {
		t.Fatalf("expected graph impact result, got %#v", env.Items)
	}
	impact := items[0].GraphImpact
	var caller *model.GraphImpactItem
	for i := range impact.Items {
		if impact.Items[i].Path == "caller.go" {
			caller = &impact.Items[i]
		}
		if impact.Items[i].Path == "unrelated.go" {
			t.Fatalf("unrelated graph positive: %#v", impact.Items[i])
		}
	}
	if caller == nil || len(caller.EvidencePath) == 0 || caller.CrossRID == "" {
		t.Fatalf("caller lacks graph evidence path: %#v", impact.Items)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.SetGraphRuntimeState(context.Background(), db, store.GraphRuntimeStale, ""); err != nil {
		t.Fatal(err)
	}
	_, err = app.diffContext(context.Background(), model.CommandRequest{Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"edge": []string{"calls"}}})
	var graphErr *model.GraphQueryError
	if !errors.As(err, &graphErr) || graphErr.Code != "GPH_IMPACT_GRAPH_STALE" {
		t.Fatalf("stale diffContext error = %v, want typed GPH_IMPACT_GRAPH_STALE", err)
	}
}

func TestNavDiffContextIncludesStagedAddedAndDeletedFiles(t *testing.T) {
	alias := "diff-ws-" + filepath.Base(t.TempDir())
	root := createDiffWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	writeWorkspaceFile(t, root, "src/Added.cs", strings.Join([]string{
		"namespace Demo;",
		"public class AddedOne",
		"{",
		"}",
	}, "\n"))
	if err := os.Remove(filepath.Join(root, "src", "S1.cs")); err != nil {
		t.Fatalf("Remove S1.cs: %v", err)
	}
	runGit(t, root, "add", "-A", "src")

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.diff-context",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("nav.diff-context: %v", err)
	}

	items, ok := env.Items.([]DiffContextResult)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one diff result, got %#v", env.Items)
	}
	if items[0].ChangedFiles == 0 {
		t.Fatalf("changed_files = 0, want > 0")
	}
	if len(items[0].ChangedSymbols) == 0 {
		t.Fatalf("changed_symbols empty, want at least one entry")
	}

	var sawAdded bool
	for _, sym := range items[0].ChangedSymbols {
		if sym.ChangeType == "added" && sym.File == "src/Added.cs" {
			sawAdded = true
			break
		}
	}
	if !sawAdded {
		t.Fatalf("expected added symbol for src/Added.cs, got %#v", items[0].ChangedSymbols)
	}
}

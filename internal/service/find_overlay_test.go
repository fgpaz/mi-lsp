package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestFindSymbolsWithWorktreeOverlayMasksBaseAndPreservesPhysicalPaths(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	gitTest := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitTest(base, "init", "-q")
	gitTest(base, "config", "user.name", "Overlay Test")
	gitTest(base, "config", "user.email", "overlay@example.invalid")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(base, "base.go"), "package sample\nfunc Keep() {}\nfunc Gone() {}\n")
	write(filepath.Join(base, "stable.go"), "package sample\nfunc Stable() {}\n")
	gitTest(base, "add", "base.go", "stable.go")
	gitTest(base, "commit", "-qm", "base")
	wt := filepath.Join(t.TempDir(), "linked")
	gitTest(base, "worktree", "add", "-qb", "feature", wt)
	if err := os.Rename(filepath.Join(wt, "base.go"), filepath.Join(wt, "moved.go")); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(wt, "moved.go"), "package sample\nfunc Keep() {}\nfunc BranchOnly() {}\n")
	write(filepath.Join(wt, "extra.go"), "package sample\nfunc Added() {}\n")

	project := model.ProjectFile{Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: "."}}}
	baseDB, err := store.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	var files []model.FileRecord
	var symbols []model.SymbolRecord
	for _, name := range []string{"base.go", "stable.go"} {
		path := filepath.Join(base, name)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		items, file := indexer.ExtractCatalog(base, project.Repos[0], path, content)
		files = append(files, file)
		symbols = append(symbols, items...)
	}
	if err := store.ReplaceCatalog(ctx, baseDB, project, files, symbols); err != nil {
		t.Fatal(err)
	}
	defer baseDB.Close()
	overlay, err := workspace.ResolveWorktreeOverlay(wt)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	if _, err := workspace.RegisterWorkspace("base", model.WorkspaceRegistration{Root: base}); err != nil {
		t.Fatal(err)
	}
	baseRegistration, found, err := workspace.RegisteredWorktreeBase(overlay)
	if err != nil || !found || baseRegistration.Root != base {
		t.Fatalf("registered base resolution = %#v, %v, %v", baseRegistration, found, err)
	}
	if err := os.MkdirAll(workspace.WorkspaceStateDir(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace("feature", model.WorkspaceRegistration{Root: wt}); err != nil {
		t.Fatal(err)
	}
	registration := model.WorkspaceRegistration{Name: "feature", Root: wt}
	got, err := findSymbolsWithWorktreeOverlay(ctx, baseDB, registration, project, overlay, "", "", false, 50, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]model.SymbolRecord)
	for _, item := range got {
		byName[item.Name] = item
	}
	for _, name := range []string{"Keep", "Stable", "BranchOnly", "Added"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing %s in overlay results: %#v", name, got)
		}
	}
	if _, leaked := byName["Gone"]; leaked {
		t.Errorf("deleted base symbol leaked into worktree results: %#v", byName["Gone"])
	}
	if byName["Keep"].FilePath != "moved.go" || byName["BranchOnly"].FilePath != "moved.go" || byName["Added"].FilePath != "extra.go" {
		t.Fatalf("overlay results do not preserve physical worktree paths: %#v", byName)
	}

	app := New(base, nil)
	request := model.CommandRequest{Operation: "nav.find", Payload: map[string]any{"pattern": "BranchOnly"}, Context: model.QueryOptions{Workspace: "feature", CallerCWD: wt, NoIndexRefresh: true, MaxItems: 50}}
	envelope, err := app.find(ctx, request)
	items, ok := envelope.Items.([]model.SymbolRecord)
	if err != nil || !envelope.Ok || !ok || len(items) != 1 || items[0].Name != "BranchOnly" || items[0].FilePath != "moved.go" {
		t.Fatalf("nav.find worktree overlay = %#v, %v", envelope, err)
	}
	request.Payload["pattern"] = "Gone"
	envelope, err = app.find(ctx, request)
	items, ok = envelope.Items.([]model.SymbolRecord)
	if err != nil || !envelope.Ok || !ok || len(items) != 0 {
		t.Fatalf("nav.find leaked deleted base symbol: %#v, %v", envelope, err)
	}

	symbolRequest := model.CommandRequest{Operation: "nav.symbols", Payload: map[string]any{"file": filepath.Join(wt, "moved.go")}, Context: request.Context}
	symbolsEnvelope, err := app.symbols(ctx, symbolRequest)
	symbols, symbolsOK := symbolsEnvelope.Items.([]model.SymbolRecord)
	if err != nil || !symbolsEnvelope.Ok || !symbolsOK || len(symbols) != 2 || symbols[1].Name != "BranchOnly" {
		t.Fatalf("nav.symbols changed worktree file = %#v, %v", symbolsEnvelope, err)
	}
	symbolRequest.Payload["file"] = filepath.Join(wt, "stable.go")
	symbolsEnvelope, err = app.symbols(ctx, symbolRequest)
	symbols, symbolsOK = symbolsEnvelope.Items.([]model.SymbolRecord)
	if err != nil || !symbolsEnvelope.Ok || !symbolsOK || len(symbols) != 1 || symbols[0].Name != "Stable" {
		t.Fatalf("nav.symbols unchanged base-index file = %#v, %v", symbolsEnvelope, err)
	}
	symbolRequest.Payload["file"] = filepath.Join(wt, "base.go")
	symbolsEnvelope, err = app.symbols(ctx, symbolRequest)
	symbols, symbolsOK = symbolsEnvelope.Items.([]model.SymbolRecord)
	if err != nil || !symbolsEnvelope.Ok || !symbolsOK || len(symbols) != 0 {
		t.Fatalf("nav.symbols leaked renamed/deleted base path = %#v, %v", symbolsEnvelope, err)
	}
}

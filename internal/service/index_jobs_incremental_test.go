package service

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestIncrementalIndexJobFailsWithoutFalseCurrentGraph(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "incremental-job-" + filepath.Base(root)
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "go.mod", "module example.com/incremental-job\n\ngo 1.23\n")
	writeWorkspaceFile(t, root, "main.go", "package main\nfunc main() {}\n")
	writeWorkspaceFile(t, root, ".gitignore", ".mi-lsp/\n")
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "mi-lsp-test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	if _, err := indexer.IndexWorkspace(context.Background(), root, true); err != nil {
		t.Fatalf("initial index: %v", err)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok {
		db.Close()
		t.Fatalf("initial active graph=%s ok=%v err=%v", before, ok, err)
	}
	job, err := store.CreateIndexJob(context.Background(), db, alias, root, store.IndexModeFull, false)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	_ = db.Close()

	writeWorkspaceFile(t, root, "go.mod", "not a module\n")
	app := New(root, nil)
	if _, _, err := app.runIndexJob(context.Background(), model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}, job.JobID); err == nil {
		t.Fatal("expected incremental job failure")
	}

	db, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	failed, ok, err := store.GetIndexJob(context.Background(), db, job.JobID)
	if err != nil || !ok || failed.Status != store.IndexJobFailed {
		t.Fatalf("job=%#v ok=%v err=%v, want failed", failed, ok, err)
	}
	after, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok || after != before {
		t.Fatalf("active graph=%s ok=%v err=%v, want prior %s", after, ok, err, before)
	}
	if state, err := store.GraphRuntimeState(context.Background(), db); err != nil || state != store.GraphRuntimeStale {
		t.Fatalf("graph runtime state=%q err=%v, want stale", state, err)
	}
}

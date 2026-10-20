package service

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDiagnosticsKeepsLinkedWorktreePhysicalRoot(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"a.go": "package a\n"}, nil)
	diagnosticsTestGit(t, root, "init")
	diagnosticsTestGit(t, root, "config", "user.email", "fixture@example.invalid")
	diagnosticsTestGit(t, root, "config", "user.name", "Fixture")
	diagnosticsTestGit(t, root, "add", "a.go")
	diagnosticsTestGit(t, root, "commit", "-m", "initial")
	linked := filepath.Join(t.TempDir(), "linked")
	diagnosticsTestGit(t, root, "worktree", "add", "-b", "diagnostics-fixture", linked, "HEAD")
	// The canonical checkout and linked worktree intentionally disagree.
	writeWorkspaceFile(t, linked, "a.go", "package a\nvar broken int = \"wrong\"\n")
	fake := &fakeSemanticCaller{callFn: func(_ context.Context, reg model.WorkspaceRegistration, req model.WorkerRequest) (model.WorkerResponse, error) {
		physical, _ := filepath.EvalSymlinks(linked)
		if reg.Root != physical || req.Workspace != physical || req.Payload["file"] != filepath.Join(physical, "a.go") {
			t.Errorf("escaped worktree: reg=%+v req=%+v", reg, req)
		}
		return model.WorkerResponse{Ok: true, Backend: "gopls", Items: []map[string]any{{"path": req.Payload["file"], "line": 2, "column": 1, "severity": "error", "message": "cannot assign string"}}}, nil
	}}
	env, e := New(root, fake).Execute(context.Background(), model.CommandRequest{Operation: "nav.diagnostics", Context: model.QueryOptions{Workspace: alias, CallerCWD: linked}, Payload: map[string]any{"file": "a.go"}})
	if e != nil || env.Diagnostics == nil || env.Diagnostics.Errors != 1 {
		t.Fatalf("env=%+v err=%v", env, e)
	}
}
func TestDiagnosticsReadyBackendsStartConcurrently(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go", "typescript"}, false, map[string]string{"a.go": "package a\n", "b.ts": "const b=1;\n"}, nil)
	diagnosticsTestGit(t, root, "init")
	var started atomic.Int32
	both := make(chan struct{})
	fake := &fakeSemanticCaller{callFn: func(ctx context.Context, _ model.WorkspaceRegistration, req model.WorkerRequest) (model.WorkerResponse, error) {
		if started.Add(1) == 2 {
			close(both)
		}
		select {
		case <-both:
		case <-ctx.Done():
			return model.WorkerResponse{}, ctx.Err()
		}
		return model.WorkerResponse{Ok: true, Backend: req.BackendType, Items: []map[string]any{}}, nil
	}}
	env, e := New(root, fake).Execute(context.Background(), model.CommandRequest{Operation: "nav.diagnostics", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"changed": true}})
	if e != nil || env.Diagnostics == nil || env.Diagnostics.FilesChecked != 2 {
		t.Fatalf("env=%+v err=%v", env, e)
	}
}

package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func diagnosticsTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, out)
	}
	return strings.TrimSpace(string(out))
}
func TestDiagnosticsGitUnionAndMergeBase(t *testing.T) {
	root := t.TempDir()
	diagnosticsTestGit(t, root, "init")
	diagnosticsTestGit(t, root, "config", "user.email", "fixture@example.invalid")
	diagnosticsTestGit(t, root, "config", "user.name", "Fixture")
	for _, p := range []string{"staged.go", "unstaged.ts", "deleted.py", "branch.rs"} {
		writeWorkspaceFile(t, root, p, "initial\n")
	}
	writeWorkspaceFile(t, root, ".gitignore", "ignored.py\n")
	diagnosticsTestGit(t, root, "add", ".")
	diagnosticsTestGit(t, root, "commit", "-m", "initial")
	base := diagnosticsTestGit(t, root, "rev-parse", "HEAD")
	writeWorkspaceFile(t, root, "branch.rs", "changed\n")
	diagnosticsTestGit(t, root, "add", "branch.rs")
	diagnosticsTestGit(t, root, "commit", "-m", "branch")
	writeWorkspaceFile(t, root, "staged.go", "staged\n")
	diagnosticsTestGit(t, root, "add", "staged.go")
	writeWorkspaceFile(t, root, "unstaged.ts", "unstaged\n")
	writeWorkspaceFile(t, root, "untracked name\nfile.py", "untracked\n")
	writeWorkspaceFile(t, root, "ignored.py", "ignored\n")
	if e := os.Remove(filepath.Join(root, "deleted.py")); e != nil {
		t.Fatal(e)
	}
	paths, truncated, e := diagnosticsChangedPaths(context.Background(), root, "")
	want := []string{"deleted.py", "staged.go", "unstaged.ts", "untracked name\nfile.py"}
	if e != nil || truncated || !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths=%q truncated=%v err=%v", paths, truncated, e)
	}
	paths, _, e = diagnosticsChangedPaths(context.Background(), root, base)
	if e != nil || len(paths) != 5 || paths[0] != "branch.rs" {
		t.Fatalf("base paths=%q err=%v", paths, e)
	}
	if _, _, e := diagnosticsChangedPaths(context.Background(), root, "--output=/evil"); e == nil {
		t.Fatal("option-like ref accepted")
	}
}
func TestDiagnosticsPathsRejectEscapeAndMissing(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeWorkspaceFile(t, outside, "a.go", "package a\n")
	if _, _, code := diagnosticsPath(root, filepath.Join(outside, "a.go")); code != "invalid_path" {
		t.Fatal(code)
	}
	if _, _, code := diagnosticsPath(root, "deleted.go"); code != "file_missing" {
		t.Fatal(code)
	}
	if e := os.Symlink(filepath.Join(outside, "a.go"), filepath.Join(root, "link.go")); e == nil {
		if _, _, code := diagnosticsPath(root, "link.go"); code != "invalid_path" {
			t.Fatal(code)
		}
	}
}
func TestDiagnosticsChangedPartitionsBackendsAndErrorsFirst(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go", "typescript"}, false, map[string]string{"a.go": "package a\n", "b.ts": "const x: number = 'bad';\n"}, nil)
	diagnosticsTestGit(t, root, "init")
	fake := &fakeSemanticCaller{callFn: func(ctx context.Context, reg model.WorkspaceRegistration, req model.WorkerRequest) (model.WorkerResponse, error) {
		if req.Method != "diagnostics" {
			return model.WorkerResponse{}, errors.New("wrong worker method")
		}
		severity := "warning"
		if req.BackendType == "tsserver" {
			severity = "error"
		}
		return model.WorkerResponse{Ok: true, Backend: req.BackendType, Items: []map[string]any{{"path": req.Payload["file"], "line": 1, "column": 1, "severity": severity, "message": "semantic finding"}}}, nil
	}}
	env, e := New(root, fake).Execute(context.Background(), model.CommandRequest{Operation: "nav.diagnostics", Context: model.QueryOptions{Workspace: alias, MaxItems: 1}, Payload: map[string]any{"changed": true}})
	if e != nil {
		t.Fatal(e)
	}
	items := env.Items.([]model.Diagnostic)
	if len(items) != 1 || items[0].Severity != "error" || !env.Truncated || env.Diagnostics.Errors != 1 || env.Diagnostics.Warnings != 1 {
		t.Fatalf("%+v", env)
	}
	if len(fake.requests()) != 2 {
		t.Fatalf("requests=%+v", fake.requests())
	}
}
func TestDiagnosticsUnavailableNeverClean(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"a.go": "package a\n"}, nil)
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{}, errors.New("backend absent")
	}}
	env, e := New(root, fake).Execute(context.Background(), model.CommandRequest{Operation: "nav.diagnostics", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"file": "a.go"}})
	if e != nil || env.Ok || !env.Partial || len(env.Omissions) != 1 || env.Omissions[0].ErrorCode != "unavailable" {
		t.Fatalf("env=%+v err=%v", env, e)
	}
}
func TestDiagnosticsCallerDeadline(t *testing.T) {
	fake := &fakeSemanticCaller{callFn: func(ctx context.Context, _ model.WorkspaceRegistration, _ model.WorkerRequest) (model.WorkerResponse, error) {
		<-ctx.Done()
		return model.WorkerResponse{}, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, e := New("", fake).diagnosticsCall(ctx, model.WorkspaceRegistration{}, model.WorkerRequest{})
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}
func TestDiagnosticsBudgetKeepsErrorsBeforeWarnings(t *testing.T) {
	env := model.Envelope{Ok: true, Diagnostics: &model.DiagnosticsSummary{Errors: 1, Warnings: 100}, Items: []model.Diagnostic{{Path: "a.go", Line: 1, Column: 1, Severity: "error", Message: "important"}}}
	for i := 0; i < 100; i++ {
		env.Items = append(env.Items.([]model.Diagnostic), model.Diagnostic{Path: "a.go", Line: i + 2, Column: 1, Severity: "warning", Message: strings.Repeat("w", 200)})
	}
	env = boundDiagnostics(env, model.QueryOptions{TokenBudget: 250})
	items := env.Items.([]model.Diagnostic)
	if !env.Truncated || len(items) == 0 || items[0].Severity != "error" || env.Diagnostics.Warnings != 100 {
		t.Fatalf("%+v", env)
	}
}

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func autoRegisterFixture(t *testing.T) (repo string, spawns *atomic.Int32) {
	t.Helper()
	return autoRegisterFixtureCommits(t, true)
}

func autoRegisterFixtureCommits(t *testing.T, commit bool) (repo string, spawns *atomic.Int32) {
	t.Helper()
	home, err := os.MkdirTemp(".", ".mi-lsp-service-autoregister-test-")
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(workspace.AutoRegisterEnvVar, "")
	t.Setenv(workspace.AutoRegisterModeEnvVar, "")
	repo = filepath.Join(home, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if commit {
		for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "i"}} {
			if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
	}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	spawns = &atomic.Int32{}
	old := spawnDetachedIndexJobProcess
	spawnDetachedIndexJobProcess = func(model.WorkspaceRegistration, string) (int, error) {
		spawns.Add(1)
		return 4242, nil
	}
	t.Cleanup(func() { spawnDetachedIndexJobProcess = old })
	return repo, spawns
}

func autoRegisterGitRepoOutsideHome(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	return repo
}

func hasAutoWarning(env model.Envelope, prefix string) bool {
	for _, w := range env.Warnings {
		if strings.HasPrefix(w, prefix) {
			return true
		}
	}
	return false
}

func TestExecuteAutoRegistersOnFirstQuery(t *testing.T) {
	repo, spawns := autoRegisterFixture(t)
	app := New(repo, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{CallerCWD: repo},
		Payload:   map[string]any{"pattern": "Hello"},
	})
	if err != nil {
		t.Fatalf("nav.search: %v", err)
	}
	if !hasAutoWarning(env, "auto_registered:") {
		t.Fatalf("missing auto_registered warning: %v", env.Warnings)
	}
	registry, _ := workspace.LoadRegistry()
	if registry.Workspaces["proj"].Root != repo {
		t.Fatalf("not persisted: %+v", registry.Workspaces)
	}
	if spawns.Load() != 1 {
		t.Fatalf("index spawns = %d, want 1", spawns.Load())
	}

	env, err = app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{CallerCWD: repo},
		Payload:   map[string]any{"pattern": "Hello"},
	})
	if err != nil || hasAutoWarning(env, "auto_registered:") || spawns.Load() != 1 {
		t.Fatalf("second query must not re-register: err=%v warnings=%v spawns=%d", err, env.Warnings, spawns.Load())
	}
}

func TestExecuteRetriesPartialAutoRegistrationAndStartsIndex(t *testing.T) {
	repo, spawns := autoRegisterFixture(t)
	stateDir := workspace.WorkspaceStateDir(repo)
	if err := os.WriteFile(stateDir, []byte("blocks directory creation"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := New(repo, nil)
	request := model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{CallerCWD: repo},
		Payload:   map[string]any{"pattern": "Hello"},
	}
	first, err := app.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("first nav.search: %v", err)
	}
	if !hasAutoWarning(first, "auto_register_failed:") || hasAutoWarning(first, "auto_registered:") {
		t.Fatalf("first warnings = %v; want incomplete registration failure", first.Warnings)
	}
	if err := os.Remove(stateDir); err != nil {
		t.Fatal(err)
	}

	second, err := app.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("retry nav.search: %v", err)
	}
	if !hasAutoWarning(second, "auto_registered:") || !hasAutoWarning(second, "auto_register_index:") || spawns.Load() != 1 {
		t.Fatalf("retry warnings = %v spawns=%d; want recovery and one index", second.Warnings, spawns.Load())
	}
}

func TestExecuteAutoRegisterFailureDoesNotBlockNavMultiRead(t *testing.T) {
	repo, _ := autoRegisterFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("query remains available\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := workspace.WorkspaceStateDir(repo)
	if err := os.WriteFile(stateDir, []byte("blocks directory creation"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, err := New(repo, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.multi-read",
		Context:   model.QueryOptions{CallerCWD: repo},
		Payload:   map[string]any{"items": []string{"README.md:1-1"}},
	})
	if err != nil || !env.Ok || env.Stats.Files != 1 || !hasAutoWarning(env, "auto_register_failed:") {
		t.Fatalf("nav.multi-read = %+v, %v; want successful read and auto-register warning", env, err)
	}
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil || len(registry.Workspaces) != 0 {
		t.Fatalf("failed navigation persisted workspace: %+v, %v", registry.Workspaces, err)
	}
}

func TestExecuteLinkedWorktreeWithoutRegisteredMainDoesNotIndex(t *testing.T) {
	repo, spawns := autoRegisterFixture(t)
	linked := filepath.Join(filepath.Dir(repo), "linked")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "--detach", linked).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v %s", err, out)
	}
	_, err := New(repo, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{CallerCWD: linked},
		Payload:   map[string]any{"pattern": "Hello"},
	})
	if err == nil || !strings.Contains(err.Error(), "explicit_incomplete: reason_code=invalid_workspace") {
		t.Fatalf("linked query error = %v; want explicit_incomplete invalid_workspace", err)
	}
	registry, loadErr := workspace.LoadRegistryReadOnly()
	if loadErr != nil || len(registry.Workspaces) != 0 || spawns.Load() != 0 {
		t.Fatalf("registry=%+v loadErr=%v index spawns=%d; want no registration or index", registry.Workspaces, loadErr, spawns.Load())
	}
}

func TestExecuteAutoRegisterForceOverrideIsReported(t *testing.T) {
	_, spawns := autoRegisterFixture(t)
	repo := autoRegisterGitRepoOutsideHome(t)
	t.Setenv(workspace.AutoRegisterModeEnvVar, "force")
	env, err := New(repo, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{CallerCWD: repo},
		Payload:   map[string]any{"pattern": "initial"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasAutoWarning(env, "auto_register_force:") || !hasAutoWarning(env, "auto_registered:") {
		t.Fatalf("force bypass was not registered and reported: %v", env.Warnings)
	}
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil || len(registry.Workspaces) != 1 || spawns.Load() != 1 {
		t.Fatalf("registry=%+v err=%v index spawns=%d; want forced registration and one index", registry.Workspaces, err, spawns.Load())
	}
}

func TestExecuteAutoRegisterOptOut(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, *model.QueryOptions){
		"flag": func(_ *testing.T, o *model.QueryOptions) { o.NoAutoRegister = true },
		"env":  func(t *testing.T, _ *model.QueryOptions) { t.Setenv(workspace.AutoRegisterEnvVar, "1") },
	} {
		t.Run(name, func(t *testing.T) {
			repo, spawns := autoRegisterFixture(t)
			opts := model.QueryOptions{CallerCWD: repo}
			setup(t, &opts)
			env, _ := New(repo, nil).Execute(context.Background(), model.CommandRequest{
				Operation: "nav.search", Context: opts, Payload: map[string]any{"pattern": "Hello"},
			})
			if hasAutoWarning(env, "auto_registered:") || spawns.Load() != 0 {
				t.Fatalf("opt-out ignored: %v spawns=%d", env.Warnings, spawns.Load())
			}
			registry, _ := workspace.LoadRegistryReadOnly()
			if len(registry.Workspaces) != 0 {
				t.Fatalf("registry must stay empty: %+v", registry.Workspaces)
			}
		})
	}
}

func TestExecuteAutoRegisterConcurrentQueriesIndexOnce(t *testing.T) {
	repo, spawns := autoRegisterFixture(t)
	app := New(repo, nil)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = app.Execute(context.Background(), model.CommandRequest{
				Operation: "nav.search",
				Context:   model.QueryOptions{CallerCWD: repo},
				Payload:   map[string]any{"pattern": "Hello"},
			})
		}()
	}
	wg.Wait()
	registry, _ := workspace.LoadRegistry()
	if len(registry.Workspaces) != 1 || spawns.Load() != 1 {
		t.Fatalf("workspaces=%d spawns=%d, want 1/1", len(registry.Workspaces), spawns.Load())
	}
}

func TestExecuteAutoRegisterSkipsIndexWithoutCommits(t *testing.T) {
	repo, spawns := autoRegisterFixtureCommits(t, false)
	env, err := New(repo, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{CallerCWD: repo},
		Payload:   map[string]any{"pattern": "Hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spawns.Load() != 0 || !hasAutoWarning(env, "auto_register_index_skipped:") || hasAutoWarning(env, "auto_register_index:") {
		t.Fatalf("spawns=%d warnings=%v", spawns.Load(), env.Warnings)
	}
}

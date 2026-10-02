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
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(workspace.AutoRegisterEnvVar, "")
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

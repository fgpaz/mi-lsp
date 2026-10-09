package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/query"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestExecuteQCanonicalUsageRecipes(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{
		"app.go": `package app

const MI_LSP_REFS_TIMEOUT = 8000

func Execute() {}
func Caller() { Execute() }
`,
	}, []model.SymbolRecord{
		{FilePath: "app.go", Name: "Execute", Kind: "function", StartLine: 5, EndLine: 5, Language: "go", QualifiedName: "App.Execute"},
		{FilePath: "app.go", Name: "Caller", Kind: "function", StartLine: 6, EndLine: 6, Language: "go", QualifiedName: "App.Caller"},
	})
	staleRoot := filepath.Join(t.TempDir(), "missing-workspace")
	if _, err := workspace.RegisterWorkspace("stale-q-test", model.WorkspaceRegistration{Name: "stale-q-test", Root: staleRoot, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	app := New(root, nil)
	for _, pipeline := range []string{
		`sym "App.Execute" exact | edges callers depth=2 | read ±3`,
		`text "MI_LSP_REFS_TIMEOUT" type=go | limit 5 | read ±2`,
		`docs "RF-QRY-001" | limit 3 | read`,
	} {
		parsed, err := query.Parse(pipeline)
		if err != nil {
			t.Fatalf("Parse(%q): %v", pipeline, err)
		}
		env, err := app.ExecuteQ(context.Background(), model.CommandRequest{
			Operation: "q",
			Context:   model.QueryOptions{Workspace: alias, CallerCWD: root},
			Payload:   map[string]any{"q": pipeline},
		}, nil)
		if err != nil {
			t.Fatalf("ExecuteQ(%q): %v", pipeline, err)
		}
		if len(env.Stages) != len(parsed.Stages) {
			t.Fatalf("ExecuteQ(%q) stages=%+v, want %d stages", pipeline, env.Stages, len(parsed.Stages))
		}
		for i, stage := range parsed.Stages {
			if env.Stages[i].Verb != stage.Verb {
				t.Fatalf("ExecuteQ(%q) stage[%d]=%q, want %q", pipeline, i, env.Stages[i].Verb, stage.Verb)
			}
		}
		if env.Error != nil && env.Error.Stage == "parse" {
			t.Fatalf("ExecuteQ(%q) failed to parse: %+v", pipeline, env.Error)
		}
	}
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Workspaces["stale-q-test"]; !ok {
		t.Fatal("read-only q query garbage-collected a stale registry entry")
	}
}

func TestExecuteQUnresolvedAliasSuggestsWorkspaceFromCallerCWD(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	app := New(root, nil)
	env, err := app.ExecuteQ(context.Background(), model.CommandRequest{
		Operation: "q",
		Context:   model.QueryOptions{Workspace: "wrong-alias", CallerCWD: root},
		Payload:   map[string]any{"q": `sym Hello exact`},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env.Ok || env.Error == nil {
		t.Fatalf("envelope=%+v, want workspace resolution error", env)
	}
	if env.Workspace != alias || !strings.Contains(env.Error.Detail, alias) || !strings.Contains(env.Error.Detail, "--workspace") {
		t.Fatalf("envelope=%+v, want probable alias %q in actionable detail", env, alias)
	}
}

func TestExecuteQMissingCatalogNamesExplicitIndexCommandWithoutCreatingIt(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	app := New(root, nil)
	env, err := app.ExecuteQ(context.Background(), model.CommandRequest{
		Operation: "q",
		Context:   model.QueryOptions{Workspace: alias, CallerCWD: root},
		Payload:   map[string]any{"q": `sym Hello exact`},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env.Ok || env.Error == nil || !strings.Contains(env.Error.Detail, "índice ausente") || !strings.Contains(env.Error.Detail, "mi-lsp index --workspace") {
		t.Fatalf("envelope=%+v, want explicit index guidance", env)
	}
	if _, err := os.Stat(store.WorkspaceDBPath(root)); !os.IsNotExist(err) {
		t.Fatalf("q created an index or found an unexpected one: %v", err)
	}
}

func TestExecuteQUnregisteredPathSuggestsAddWithoutRegistering(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/q\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := New(root, nil)
	env, err := app.ExecuteQ(context.Background(), model.CommandRequest{
		Operation: "q",
		Context:   model.QueryOptions{Workspace: root, CallerCWD: root},
		Payload:   map[string]any{"q": `sym Hello exact`},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env.Ok || env.Error == nil || !strings.Contains(env.Error.Detail, "workspace no registrado") || !strings.Contains(env.Error.Detail, "mi-lsp workspace add") || !strings.Contains(env.Error.Detail, "--no-index") {
		t.Fatalf("envelope=%+v, want explicit registration guidance", env)
	}
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Workspaces) != 0 {
		t.Fatalf("q registered a workspace: %+v", registry.Workspaces)
	}
	if _, err := os.Stat(store.WorkspaceDBPath(root)); !os.IsNotExist(err) {
		t.Fatalf("q created an index: %v", err)
	}
}

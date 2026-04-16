package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestExecuteWorkspaceStatusResolvesWorkspaceFromCallerCWD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	otherRoot := t.TempDir()
	callerRoot := t.TempDir()
	writeWorkspaceFile(t, callerRoot, "src/Program.cs", "class Program {}")
	writeWorkspaceFile(t, otherRoot, "src/Legacy.cs", "class Legacy {}")

	if err := workspace.SaveProjectFile(callerRoot, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "interbancarizacion_coelsa",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(callerRoot): %v", err)
	}

	registerServiceWorkspace(t, "interbancarizacion_coelsa", callerRoot)
	registerServiceWorkspace(t, "mis-cals", otherRoot)

	app := New(callerRoot, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			CallerCWD: filepath.Join(callerRoot, "src"),
		},
	})
	if err != nil {
		t.Fatalf("Execute(workspace.status): %v", err)
	}
	if env.Workspace != "interbancarizacion_coelsa" {
		t.Fatalf("env.Workspace = %q, want interbancarizacion_coelsa", env.Workspace)
	}
	items, ok := env.Items.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("Items = %#v, want one workspace status item", env.Items)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item type = %T, want map[string]any", items[0])
	}
	if item["root"] != callerRoot {
		t.Fatalf("item[root] = %#v, want %q", item["root"], callerRoot)
	}
}

func TestExecuteWorkspaceStatusAppendsSameRootAliasWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/Program.cs", "class Program {}")

	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "interbancarizacion_coelsa",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(root): %v", err)
	}

	registerServiceWorkspace(t, "coelsa", root)
	registerServiceWorkspace(t, "interbancarizacion_coelsa", root)

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			CallerCWD: filepath.Join(root, "src"),
		},
	})
	if err != nil {
		t.Fatalf("Execute(workspace.status): %v", err)
	}
	if env.Workspace != "interbancarizacion_coelsa" {
		t.Fatalf("env.Workspace = %q, want interbancarizacion_coelsa", env.Workspace)
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), "multiple registry aliases") {
		t.Fatalf("Warnings = %v, want ambiguity warning", env.Warnings)
	}
}

func registerServiceWorkspace(t *testing.T, alias string, root string) {
	t.Helper()
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(%s): %v", alias, err)
	}
	t.Cleanup(func() {
		_ = workspace.RemoveWorkspace(alias)
	})
}

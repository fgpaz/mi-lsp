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
	if item["workspace_root"] != callerRoot {
		t.Fatalf("item[workspace_root] = %#v, want %q", item["workspace_root"], callerRoot)
	}
	if item["workspace_source"] != "caller_cwd" {
		t.Fatalf("item[workspace_source] = %#v, want caller_cwd", item["workspace_source"])
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

func TestExecuteWorkspaceStatusExplicitAliasWinsOverCallerCWDWithWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mainRoot := t.TempDir()
	worktreeRoot := t.TempDir()
	writeWorkspaceFile(t, mainRoot, "src/Main.cs", "class Main {}")
	writeWorkspaceFile(t, worktreeRoot, "src/Feature.cs", "class Feature {}")

	registerServiceWorkspace(t, "mi-lsp-main", mainRoot)
	registerServiceWorkspace(t, "mi-lsp-feature", worktreeRoot)

	app := New(mainRoot, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			Workspace: "mi-lsp-main",
			CallerCWD: filepath.Join(worktreeRoot, "src"),
		},
	})
	if err != nil {
		t.Fatalf("Execute(workspace.status): %v", err)
	}
	if env.Workspace != "mi-lsp-main" {
		t.Fatalf("env.Workspace = %q, want mi-lsp-main", env.Workspace)
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), "explicit workspace") {
		t.Fatalf("Warnings = %v, want explicit workspace mismatch warning", env.Warnings)
	}
	items, ok := env.Items.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("Items = %#v, want one workspace status item", env.Items)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item type = %T, want map[string]any", items[0])
	}
	if item["workspace_source"] != "explicit" {
		t.Fatalf("item[workspace_source] = %#v, want explicit", item["workspace_source"])
	}
}

func TestExecuteWorkspaceStatusInvalidExplicitAliasReturnsDiagnosticError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/Program.cs", "class Program {}")
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "caller-workspace",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(root): %v", err)
	}
	registerServiceWorkspace(t, "caller-workspace", root)

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			Workspace: "stale-alias",
			CallerCWD: filepath.Join(root, "src"),
		},
	})
	if err == nil {
		t.Fatalf("Execute(workspace.status) err = nil, env=%#v", env)
	}
	message := err.Error()
	if !strings.Contains(message, "stale-alias") {
		t.Fatalf("error = %q, want stale alias", message)
	}
	if !strings.Contains(message, "caller-workspace") {
		t.Fatalf("error = %q, want caller cwd diagnostic alias", message)
	}
	if !strings.Contains(message, "workspace hygiene --apply-safe") {
		t.Fatalf("error = %q, want hygiene suggestion", message)
	}
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].Name != "caller-workspace" {
		t.Fatalf("workspaces = %#v, want only caller-workspace", workspaces)
	}
}

func TestExecuteWorkspaceStatusPathWorkspaceUsesPathSafeNextSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	writeWorkspaceFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	writeWorkspaceFile(t, root, "Program.cs", "class RootProgram {}")
	writeWorkspaceFile(t, root, "App.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\" />")
	writeWorkspaceFile(t, root, "src/Program.cs", "class Program {}")
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "current-repo",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(root): %v", err)
	}
	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			Workspace: ".",
			CallerCWD: root,
			AXI:       true,
		},
	})
	if err != nil {
		t.Fatalf("Execute(workspace.status): %v", err)
	}
	item := singleStatusItem(t, env)
	if item["workspace_source"] != "path" {
		t.Fatalf("item[workspace_source] = %#v, want path", item["workspace_source"])
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), "generated alias") {
		t.Fatalf("Warnings = %v, want unregistered generated alias warning", env.Warnings)
	}
	if !strings.Contains(env.Hint, "--workspace .") {
		t.Fatalf("Hint = %q, want --workspace .", env.Hint)
	}
	nextSteps, ok := item["next_steps"].([]string)
	if !ok {
		t.Fatalf("item[next_steps] = %#v, want []string", item["next_steps"])
	}
	joined := strings.Join(nextSteps, "\n")
	if !strings.Contains(joined, "--workspace .") {
		t.Fatalf("next_steps = %v, want --workspace .", nextSteps)
	}
}

func TestExecuteNavGovernanceInvalidAliasReturnsCallerCWDDiagnostic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/Program.cs", "class Program {}")
	writeSpecBackendGovernanceFixture(t, root)
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "current-repo",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(root): %v", err)
	}
	registerServiceWorkspace(t, "current-repo", root)

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context: model.QueryOptions{
			Workspace: "missing-alias",
			CallerCWD: filepath.Join(root, "src"),
		},
	})
	if err == nil {
		t.Fatalf("Execute(nav.governance) err = nil, env=%#v", env)
	}
	message := err.Error()
	for _, want := range []string{"missing-alias", "current-repo", "workspace hygiene --apply-safe"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want %q", message, want)
		}
	}
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].Name != "current-repo" {
		t.Fatalf("workspaces = %#v, want only current-repo", workspaces)
	}
}

func TestExecuteWorkspaceStatusOmittedWorkspaceIgnoresUnrelatedLastWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	otherRoot := t.TempDir()
	callerRoot := t.TempDir()
	writeWorkspaceFile(t, otherRoot, "src/Legacy.cs", "class Legacy {}")
	writeWorkspaceFile(t, callerRoot, ".git/HEAD", "ref: refs/heads/main")
	writeWorkspaceFile(t, callerRoot, "Program.cs", "class RootProgram {}")
	writeWorkspaceFile(t, callerRoot, "src/Program.cs", "class Program {}")
	writeWorkspaceFile(t, callerRoot, "App.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\" />")
	if err := workspace.SaveProjectFile(callerRoot, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "current-repo",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(callerRoot): %v", err)
	}
	registerServiceWorkspace(t, "legacy", otherRoot)

	app := New(otherRoot, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			CallerCWD: filepath.Join(callerRoot, "src"),
			AXI:       true,
		},
	})
	if err != nil {
		t.Fatalf("Execute(workspace.status): %v", err)
	}
	item := singleStatusItem(t, env)
	if item["root"] != callerRoot {
		t.Fatalf("item[root] = %#v, want %q", item["root"], callerRoot)
	}
	if item["workspace_source"] != "caller_cwd" {
		t.Fatalf("item[workspace_source] = %#v, want caller_cwd", item["workspace_source"])
	}
	warnings := strings.Join(env.Warnings, " ")
	if !strings.Contains(warnings, "ignored unrelated last_workspace") {
		t.Fatalf("Warnings = %v, want ignored unrelated last_workspace warning", env.Warnings)
	}
	if !strings.Contains(env.Hint, "--workspace .") {
		t.Fatalf("Hint = %q, want --workspace .", env.Hint)
	}
	nextSteps, ok := item["next_steps"].([]string)
	if !ok {
		t.Fatalf("item[next_steps] = %#v, want []string", item["next_steps"])
	}
	joined := strings.Join(nextSteps, "\n")
	if strings.Contains(joined, "--workspace legacy") || !strings.Contains(joined, "--workspace .") {
		t.Fatalf("next_steps = %v, want path-safe steps and no legacy alias", nextSteps)
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

func singleStatusItem(t *testing.T, env model.Envelope) map[string]any {
	t.Helper()
	items, ok := env.Items.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("Items = %#v, want one workspace status item", env.Items)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item type = %T, want map[string]any", items[0])
	}
	return item
}

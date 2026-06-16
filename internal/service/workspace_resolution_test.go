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
	if !strings.Contains(strings.Join(env.Warnings, " "), "workspace mismatch") {
		t.Fatalf("Warnings = %v, want actionable workspace mismatch warning", env.Warnings)
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

func TestExecuteWorkspaceStatusAgentRejectsExplicitAliasOutsideCallerCWD(t *testing.T) {
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
	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context: model.QueryOptions{
			Workspace:  "mi-lsp-main",
			CallerCWD:  filepath.Join(worktreeRoot, "src"),
			ClientName: "codex",
		},
	})
	if err == nil {
		t.Fatal("Execute(workspace.status) err = nil, want workspace cross-workspace refusal")
	}
	if !strings.Contains(err.Error(), "workspace cross-workspace refused") {
		t.Fatalf("err = %v, want workspace cross-workspace refusal", err)
	}
	if !strings.Contains(err.Error(), "--allow-cross-workspace") {
		t.Fatalf("err = %v, want override hint", err)
	}
}

func TestExecuteNavGovernanceAgentRejectsExplicitAliasOutsideCallerCWD(t *testing.T) {
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
	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context: model.QueryOptions{
			Workspace:  "mi-lsp-main",
			CallerCWD:  filepath.Join(worktreeRoot, "src"),
			ClientName: "codex",
		},
	})
	if err == nil {
		t.Fatal("Execute(nav.governance) err = nil, want workspace cross-workspace refusal")
	}
	if !strings.Contains(err.Error(), "workspace cross-workspace refused") {
		t.Fatalf("err = %v, want workspace cross-workspace refusal", err)
	}
	if !strings.Contains(err.Error(), "mi-lsp nav governance --workspace mi-lsp-feature --format toon") {
		t.Fatalf("err = %v, want nav governance recommendation", err)
	}
}

func TestExecuteWorkspaceStatusAgentAllowsExplicitCrossWorkspaceOverride(t *testing.T) {
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
			Workspace:           "mi-lsp-main",
			CallerCWD:           filepath.Join(worktreeRoot, "src"),
			ClientName:          "codex",
			AllowCrossWorkspace: true,
		},
	})
	if err != nil {
		t.Fatalf("Execute(workspace.status): %v", err)
	}
	if env.Workspace != "mi-lsp-main" {
		t.Fatalf("env.Workspace = %q, want mi-lsp-main", env.Workspace)
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), "--allow-cross-workspace") {
		t.Fatalf("Warnings = %v, want explicit override warning", env.Warnings)
	}
}

func TestOperationRequiresWorkspaceResolutionSkipsNavAllWorkspaces(t *testing.T) {
	if operationRequiresWorkspaceResolution(model.CommandRequest{
		Operation: "nav.ask",
		Payload:   map[string]any{"all_workspaces": true},
	}) {
		t.Fatal("nav.ask --all-workspaces should not require single-workspace resolution")
	}
	if !operationRequiresWorkspaceResolution(model.CommandRequest{
		Operation: "nav.ask",
	}) {
		t.Fatal("nav.ask without --all-workspaces should require workspace resolution")
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
	// AUD-03: When the requested alias doesn't exist but CallerCWD resolves unambiguously,
	// we auto-correct instead of erroring. So this should now succeed.
	if err != nil {
		t.Fatalf("Execute(workspace.status) err = %v, want nil (auto-corrected)", err)
	}
	if !env.Ok {
		t.Fatalf("expected envelope.Ok=true after auto-correction, got %#v", env)
	}
	// Check that it auto-corrected to caller-workspace
	if env.Workspace != "caller-workspace" {
		t.Fatalf("expected workspace=caller-workspace after auto-correction, got %s", env.Workspace)
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
	// AUD-03: When the requested alias doesn't exist but CallerCWD resolves unambiguously,
	// we auto-correct instead of erroring. So this should now succeed.
	if err != nil {
		t.Fatalf("Execute(nav.governance) err = %v, want nil (auto-corrected)", err)
	}
	if !env.Ok {
		t.Fatalf("expected envelope.Ok=true after auto-correction, got %#v", env)
	}
	// Check that it auto-corrected to current-repo
	if env.Workspace != "current-repo" {
		t.Fatalf("expected workspace=current-repo after auto-correction, got %s", env.Workspace)
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

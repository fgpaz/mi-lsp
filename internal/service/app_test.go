package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func setupTestWorkspace(t *testing.T) (string, string) {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	name := "test-ws-" + filepath.Base(root)

	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Hello.cs"), []byte("namespace Demo;\npublic class HelloWorld {\n    public void Greet() { }\n}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg := model.WorkspaceRegistration{
		Name:      name,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}
	if _, err := workspace.RegisterWorkspace(name, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() {
		_ = workspace.RemoveWorkspace(name)
	})
	return root, name
}

func ensureWritableTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

type fakeSemanticCaller struct {
	calls  []model.WorkerRequest
	callFn func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error)
}

func (f *fakeSemanticCaller) Call(ctx context.Context, workspace model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
	f.calls = append(f.calls, request)
	if f.callFn != nil {
		return f.callFn(ctx, workspace, request)
	}
	return model.WorkerResponse{}, nil
}

func (f *fakeSemanticCaller) Status() []model.WorkerStatus {
	return nil
}

func writeWorkspaceFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}

func testProject(name string) model.ProjectFile {
	return model.ProjectFile{
		Project: model.ProjectBlock{
			Name:              name,
			Languages:         []string{"csharp", "typescript"},
			Kind:              model.WorkspaceKindSingle,
			DefaultRepo:       "main",
			DefaultEntrypoint: "main::src-app-csproj",
		},
		Repos: []model.WorkspaceRepo{{
			ID:                "main",
			Name:              "main",
			Root:              ".",
			Languages:         []string{"csharp", "typescript"},
			DefaultEntrypoint: "main::src-app-csproj",
		}},
		Entrypoints: []model.WorkspaceEntrypoint{{
			ID:      "main::src-app-csproj",
			RepoID:  "main",
			Path:    "src/App.csproj",
			Kind:    model.EntrypointKindProject,
			Default: true,
		}},
	}
}

func seedCatalogSymbol(t *testing.T, root string, project model.ProjectFile, filePath string, line int, name string, kind string) {
	t.Helper()
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	files := []model.FileRecord{{
		FilePath: filePath,
		RepoID:   "main",
		RepoName: "main",
		Language: "typescript",
	}}
	symbols := []model.SymbolRecord{{
		FilePath:      filePath,
		RepoID:        "main",
		RepoName:      "main",
		Name:          name,
		Kind:          kind,
		StartLine:     line,
		EndLine:       line,
		QualifiedName: filePath + "::" + name,
		Language:      "typescript",
	}}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}
}

func TestSearchPattern_GoFallback(t *testing.T) {
	root, name := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	env, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "HelloWorld"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected search results, got %v", env.Items)
	}
}

func TestSearchPattern_Regex(t *testing.T) {
	root, name := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	env, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "Hello\\w+", "regex": true},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true")
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected regex results, got %v", env.Items)
	}
}

func TestNavContext_NonCodeFileReturnsTextSliceWithoutSemanticCall(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "notes/README.md", strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4 target",
		"line 5",
		"line 6",
	}, "\n"))

	project := testProject(name)
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	semantic := &fakeSemanticCaller{}
	app := New(root, semantic)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.context",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"file": "notes/README.md", "line": 4},
	})
	if err != nil {
		t.Fatalf("nav.context: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	if env.Backend != "text" {
		t.Fatalf("backend = %q, want text", env.Backend)
	}
	if len(semantic.calls) != 0 {
		t.Fatalf("semantic caller should not be used for non-code files, got %d calls", len(semantic.calls))
	}

	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one context item, got %#v", env.Items)
	}
	item := items[0]
	if item["focus_line"] != 4 {
		t.Fatalf("focus_line = %#v, want 4", item["focus_line"])
	}
	if item["slice_start_line"] != 2 {
		t.Fatalf("slice_start_line = %#v, want 2", item["slice_start_line"])
	}
	if item["slice_end_line"] != 6 {
		t.Fatalf("slice_end_line = %#v, want 6", item["slice_end_line"])
	}
	sliceText, _ := item["slice_text"].(string)
	if !strings.Contains(sliceText, "line 4 target") {
		t.Fatalf("slice_text = %q, want target line", sliceText)
	}
}

func TestNavContext_MergesSemanticMetadataWithSlice(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "src/Context.cs", strings.Join([]string{
		"namespace Demo;",
		"public class ContextType",
		"{",
		"    public void Run() { }",
		"}",
	}, "\n"))

	project := testProject(name)
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	semantic := &fakeSemanticCaller{
		callFn: func(_ context.Context, _ model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
			return model.WorkerResponse{
				Ok:      true,
				Backend: "roslyn",
				Items: []map[string]any{{
					"name":      "ContextType",
					"kind":      "namedtype",
					"signature": "ContextType",
					"scope":     "public",
					"line":      2,
				}},
				Stats: model.Stats{Symbols: 1},
			}, nil
		},
	}

	app := New(root, semantic)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.context",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"file": "src/Context.cs", "line": 2},
	})
	if err != nil {
		t.Fatalf("nav.context: %v", err)
	}
	if env.Backend != "roslyn" {
		t.Fatalf("backend = %q, want roslyn", env.Backend)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one context item, got %#v", env.Items)
	}
	item := items[0]
	if item["name"] != "ContextType" {
		t.Fatalf("name = %#v, want ContextType", item["name"])
	}
	if item["kind"] != "namedtype" {
		t.Fatalf("kind = %#v, want namedtype", item["kind"])
	}
	if item["focus_line"] != 2 {
		t.Fatalf("focus_line = %#v, want 2", item["focus_line"])
	}
	if _, ok := item["slice_text"].(string); !ok {
		t.Fatalf("slice_text missing from %#v", item)
	}
	if len(semantic.calls) != 1 {
		t.Fatalf("semantic caller calls = %d, want 1", len(semantic.calls))
	}
	if semantic.calls[0].BackendType != "roslyn" {
		t.Fatalf("backend type = %q, want roslyn", semantic.calls[0].BackendType)
	}
}

func TestNavContext_TsserverErrorFallsBackToSlice(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "src/app/page.tsx", strings.Join([]string{
		"export default function Page() {",
		"  return (",
		"    <main>pairing target</main>",
		"  );",
		"}",
	}, "\n"))

	project := testProject(name)
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	seedCatalogSymbol(t, root, project, "src/app/page.tsx", 1, "Page", "function")

	semantic := &fakeSemanticCaller{
		callFn: func(_ context.Context, _ model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
			if request.BackendType != "tsserver" {
				t.Fatalf("backend type = %q, want tsserver", request.BackendType)
			}
			return model.WorkerResponse{}, errors.New("tsserver is unavailable")
		},
	}

	app := New(root, semantic)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.context",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"file": "src/app/page.tsx", "line": 3},
	})
	if err != nil {
		t.Fatalf("nav.context should degrade to slice, got error: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	if env.Backend != "catalog" && env.Backend != "text" {
		t.Fatalf("backend = %q, want catalog or text", env.Backend)
	}
	if len(env.Warnings) == 0 || !strings.Contains(strings.Join(env.Warnings, " "), "tsserver") {
		t.Fatalf("expected tsserver warning, got %v", env.Warnings)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one context item, got %#v", env.Items)
	}
	sliceText, _ := items[0]["slice_text"].(string)
	if !strings.Contains(sliceText, "pairing target") {
		t.Fatalf("slice_text = %q, want target text", sliceText)
	}
}

func TestNavContext_RoslynBootstrapErrorAddsInstallHint(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "src/Context.cs", strings.Join([]string{
		"namespace Demo;",
		"public class ContextType",
		"{",
		"    public void Run() { }",
		"}",
	}, "\n"))

	project := testProject(name)
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	semantic := &fakeSemanticCaller{
		callFn: func(_ context.Context, _ model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
			if request.BackendType != "roslyn" {
				t.Fatalf("backend type = %q, want roslyn", request.BackendType)
			}
			return model.WorkerResponse{}, errors.New("Dll was not found.")
		},
	}

	app := New(root, semantic)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.context",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"file": "src/Context.cs", "line": 2},
	})
	if err != nil {
		t.Fatalf("nav.context should degrade to slice, got error: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	if env.Backend != "text" && env.Backend != "catalog" {
		t.Fatalf("backend = %q, want text or catalog", env.Backend)
	}
	warnings := strings.Join(env.Warnings, " ")
	if !strings.Contains(warnings, "mi-lsp worker install") {
		t.Fatalf("expected install hint warning, got %v", env.Warnings)
	}
	if !strings.Contains(strings.ToLower(warnings), "roslyn") {
		t.Fatalf("expected roslyn warning context, got %v", env.Warnings)
	}
}

func TestSearchPattern_RgNoMatchesReturnsEmptyResults(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)

	rgPath = ""
	rgResolved = false
	rgOnce = sync.Once{}

	scriptPath := filepath.Join(root, "fake-rg")
	scriptBody := "#!/bin/sh\nexit 1\n"
	if runtime.GOOS == "windows" {
		scriptPath += ".cmd"
		scriptBody = "@echo off\r\nexit /b 1\r\n"
	}
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write fake rg: %v", err)
	}

	items, err := searchPatternRg(context.Background(), root, root, project, "missing", false, 10, scriptPath)
	if err != nil {
		t.Fatalf("searchPatternRg: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected zero items, got %d", len(items))
	}
}

func TestSearchPattern_LiteralNoMatchesSuggestsRegex(t *testing.T) {
	root, name := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	rgPath = ""
	rgResolved = false
	rgOnce = sync.Once{}

	env, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "pairing|deep link"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true")
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 0 {
		t.Fatalf("expected zero items, got %v", env.Items)
	}
	if len(env.Warnings) == 0 || !strings.Contains(strings.Join(env.Warnings, " "), "--regex") {
		t.Fatalf("expected regex hint warning, got %v", env.Warnings)
	}
}

func TestWorkspaceAdd_And_Status(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Program.cs"), []byte("class Program {}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	app := New(root, nil)
	ctx := context.Background()
	wsName := "add-test-" + filepath.Base(root)

	env, err := app.Execute(ctx, model.CommandRequest{
		Operation: "workspace.add",
		Payload:   map[string]any{"path": root, "alias": wsName},
	})
	if err != nil {
		t.Fatalf("workspace.add: %v", err)
	}
	if !env.Ok {
		t.Fatalf("workspace.add not ok: %v", env.Warnings)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(wsName) })

	statusEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: wsName},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}
	if !statusEnv.Ok {
		t.Fatalf("status not ok: %v", statusEnv.Warnings)
	}
}

func TestFind_Symbol(t *testing.T) {
	root, name := setupTestWorkspace(t)
	ctx := context.Background()

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: name, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "src/Hello.cs", RepoID: "main", RepoName: "main", Name: "HelloWorld", Kind: "class", StartLine: 2, EndLine: 4, QualifiedName: "Demo.HelloWorld", Language: "csharp"},
	}
	files := []model.FileRecord{
		{FilePath: "src/Hello.cs", RepoID: "main", RepoName: "main", Language: "csharp"},
	}
	if err := store.ReplaceCatalog(ctx, db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}
	db.Close()

	app := New(root, nil)
	env, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "HelloWorld", "exact": true},
	})
	if err != nil {
		t.Fatalf("nav.find: %v", err)
	}
	if !env.Ok {
		t.Fatalf("find not ok: %v", env.Warnings)
	}
	if env.Stats.Symbols != 1 {
		t.Errorf("want 1 symbol, got %d", env.Stats.Symbols)
	}
}

func TestExecute_UnknownOperation(t *testing.T) {
	app := New(t.TempDir(), nil)
	_, err := app.Execute(context.Background(), model.CommandRequest{Operation: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
}
func TestWorkspaceList_PreservesRegistryAliasesForSameRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:              "canonical-project-name",
			Languages:         []string{"csharp"},
			Kind:              model.WorkspaceKindSingle,
			DefaultRepo:       "main",
			DefaultEntrypoint: "main::app",
		},
		Repos:       []model.WorkspaceRepo{{ID: "main", Name: "main", Root: ".", Languages: []string{"csharp"}, DefaultEntrypoint: "main::app"}},
		Entrypoints: []model.WorkspaceEntrypoint{{ID: "main::app", RepoID: "main", Path: "App.sln", Kind: model.EntrypointKindSolution, Default: true}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	aliases := []string{"alias-one", "alias-two"}
	for _, alias := range aliases {
		registration := model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"csharp"}, Kind: model.WorkspaceKindSingle}
		if _, err := workspace.RegisterWorkspace(alias, registration); err != nil {
			t.Fatalf("RegisterWorkspace(%s): %v", alias, err)
		}
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{Operation: "workspace.list"})
	if err != nil {
		t.Fatalf("workspace.list: %v", err)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok {
		t.Fatalf("unexpected items type: %T", env.Items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		name, _ := item["name"].(string)
		if name == "alias-one" || name == "alias-two" {
			seen[name] = true
		}
	}
	if !seen["alias-one"] || !seen["alias-two"] {
		t.Fatalf("expected both aliases in workspace.list, got %#v", items)
	}
}

func createContainerWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "frontend/src/Login.tsx", strings.Join([]string{
		"export function LoginPage() {",
		"  return <main>forgot password link</main>;",
		"}",
	}, "\n"))
	writeWorkspaceFile(t, root, "backend/src/PasswordReset.cs", strings.Join([]string{
		"namespace Demo;",
		"// forgot password token pipeline",
		"public class PasswordResetService",
		"{",
		"    public void ResetPassword() { }",
		"}",
	}, "\n"))

	registration := model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp", "typescript"},
		Kind:      model.WorkspaceKindContainer,
	}
	if _, err := workspace.RegisterWorkspace(alias, registration); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() {
		_ = workspace.RemoveWorkspace(alias)
	})

	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:        alias,
			Languages:   []string{"csharp", "typescript"},
			Kind:        model.WorkspaceKindContainer,
			DefaultRepo: "frontend",
		},
		Repos: []model.WorkspaceRepo{
			{ID: "frontend", Name: "frontend", Root: "frontend", Languages: []string{"typescript"}},
			{ID: "backend", Name: "backend", Root: "backend", Languages: []string{"csharp"}},
		},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	files := []model.FileRecord{
		{FilePath: "frontend/src/Login.tsx", RepoID: "frontend", RepoName: "frontend", Language: "typescript"},
		{FilePath: "backend/src/PasswordReset.cs", RepoID: "backend", RepoName: "backend", Language: "csharp"},
	}
	symbols := []model.SymbolRecord{
		{
			FilePath:      "frontend/src/Login.tsx",
			RepoID:        "frontend",
			RepoName:      "frontend",
			Name:          "LoginPage",
			Kind:          "function",
			StartLine:     1,
			EndLine:       3,
			QualifiedName: "frontend.LoginPage",
			Language:      "typescript",
			SearchText:    "login forgot password reset link",
		},
		{
			FilePath:      "frontend/src/Login.tsx",
			RepoID:        "frontend",
			RepoName:      "frontend",
			Name:          "PasswordResetView",
			Kind:          "function",
			StartLine:     1,
			EndLine:       3,
			QualifiedName: "frontend.PasswordResetView",
			Language:      "typescript",
			SearchText:    "password reset frontend view",
		},
		{
			FilePath:      "backend/src/PasswordReset.cs",
			RepoID:        "backend",
			RepoName:      "backend",
			Name:          "PasswordResetService",
			Kind:          "class",
			StartLine:     3,
			EndLine:       6,
			QualifiedName: "backend.PasswordResetService",
			Language:      "csharp",
			SearchText:    "password reset recovery token backend service",
		},
		{
			FilePath:      "backend/src/PasswordReset.cs",
			RepoID:        "backend",
			RepoName:      "backend",
			Name:          "PasswordResetTokenService",
			Kind:          "class",
			StartLine:     7,
			EndLine:       10,
			QualifiedName: "backend.PasswordResetTokenService",
			Language:      "csharp",
			SearchText:    "password reset token backend service",
		},
	}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}
	return root
}

func TestFind_RepoSelectorFiltersResults(t *testing.T) {
	alias := "container-find-" + filepath.Base(t.TempDir())
	root := createContainerWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 10},
		Payload:   map[string]any{"pattern": "PasswordReset", "repo": "backend"},
	})
	if err != nil {
		t.Fatalf("nav.find: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	items, ok := env.Items.([]model.SymbolRecord)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one symbol result, got %#v", env.Items)
	}
	if items[0].RepoName != "backend" {
		t.Fatalf("repo = %q, want backend", items[0].RepoName)
	}
}

func TestFind_RepoSelectorAppliesOffsetAfterFiltering(t *testing.T) {
	alias := "container-find-offset-" + filepath.Base(t.TempDir())
	root := createContainerWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 1},
		Payload:   map[string]any{"pattern": "PasswordReset", "repo": "backend", "offset": 1},
	})
	if err != nil {
		t.Fatalf("nav.find offset: %v", err)
	}
	items, ok := env.Items.([]model.SymbolRecord)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one symbol result, got %#v", env.Items)
	}
	if items[0].Name != "PasswordResetTokenService" {
		t.Fatalf("name = %q, want PasswordResetTokenService", items[0].Name)
	}
}

func TestSymbols_OffsetSkipsFirstSymbol(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	files := []model.FileRecord{{FilePath: "src/Hello.cs", RepoID: "main", RepoName: "main", Language: "csharp"}}
	symbols := []model.SymbolRecord{
		{FilePath: "src/Hello.cs", RepoID: "main", RepoName: "main", Name: "HelloWorld", Kind: "class", StartLine: 2, EndLine: 4, QualifiedName: "Demo.HelloWorld", Language: "csharp"},
		{FilePath: "src/Hello.cs", RepoID: "main", RepoName: "main", Name: "Greet", Kind: "method", StartLine: 3, EndLine: 3, QualifiedName: "Demo.HelloWorld.Greet", Language: "csharp"},
	}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.symbols",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 1},
		Payload:   map[string]any{"file": "src/Hello.cs", "offset": 1},
	})
	if err != nil {
		t.Fatalf("nav.symbols offset: %v", err)
	}
	items, ok := env.Items.([]model.SymbolRecord)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one symbol result, got %#v", env.Items)
	}
	if items[0].Name != "Greet" {
		t.Fatalf("name = %q, want Greet", items[0].Name)
	}
}

func TestSearch_RepoSelectorFiltersResults(t *testing.T) {
	alias := "container-search-" + filepath.Base(t.TempDir())
	root := createContainerWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 10},
		Payload:   map[string]any{"pattern": "forgot password", "repo": "frontend"},
	})
	if err != nil {
		t.Fatalf("nav.search: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one search result, got %#v", env.Items)
	}
	if items[0]["repo"] != "frontend" {
		t.Fatalf("repo = %#v, want frontend", items[0]["repo"])
	}
}

func TestIntent_RepoSelectorFiltersResults(t *testing.T) {
	alias := "container-intent-" + filepath.Base(t.TempDir())
	root := createContainerWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 10},
		Payload:   map[string]any{"question": "password reset backend", "top": 10, "repo": "backend"},
	})
	if err != nil {
		t.Fatalf("nav.intent: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected intent results, got %#v", env.Items)
	}
	for _, item := range items {
		if item["file"] != "backend/src/PasswordReset.cs" {
			t.Fatalf("unexpected intent result %#v", item)
		}
	}
}

func TestCatalogQueryUnknownRepoSelectorReturnsRouterEnvelope(t *testing.T) {
	alias := "container-router-" + filepath.Base(t.TempDir())
	root := createContainerWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 10},
		Payload:   map[string]any{"pattern": "PasswordResetService", "exact": true, "repo": "missing"},
	})
	if err != nil {
		t.Fatalf("nav.find: %v", err)
	}
	if env.Ok {
		t.Fatalf("expected ok=false for unknown repo selector, got %#v", env)
	}
	if env.Backend != "router" {
		t.Fatalf("backend = %q, want router", env.Backend)
	}
	if env.NextHint == nil || !strings.Contains(*env.NextHint, "--repo <name>") {
		t.Fatalf("expected rerun hint for repo selector, got %#v", env.NextHint)
	}
}

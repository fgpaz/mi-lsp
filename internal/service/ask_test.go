package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func createIndexedWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/daemon/router.cs", strings.Join([]string{
		"namespace Demo;",
		"public class DaemonRouter",
		"{",
		"    public void RouteRequest() { }",
		"}",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", strings.Join([]string{
		"# 07. Baseline tecnica",
		"",
		"El routing del daemon se apoya en `src/daemon/router.cs` y en `DaemonRouter`.",
		"",
		"- backend: daemon",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"name = \"technical\"",
		"intent_keywords = [\"daemon\", \"routing\", \"backend\"]",
		"paths = [\".docs/wiki/07_*.md\"]",
	}, "\n"))
	return root
}

func createLinkedDocsWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "internal/service/ask.go", "package service\n\nfunc docsFirst() {}\n")
	writeWorkspaceFile(t, root, ".docs/wiki/03_FL/FL-QRY-01.md", strings.Join([]string{
		"# FL-QRY-01",
		"",
		"Flujo de consultas docs-first y evidence-first.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-QRY-010.md", strings.Join([]string{
		"# RF-QRY-010 - Responder preguntas docs-first guiadas por wiki",
		"",
		"Este RF deriva de `FL-QRY-01` y se apoya en `CT-NAV-ASK`.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-NAV-ASK.md", strings.Join([]string{
		"# CT-NAV-ASK",
		"",
		"Contrato de `nav ask` conectado con `internal/service/ask.go`.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"name = \"functional\"",
		"intent_keywords = [\"rf\", \"feature\", \"flow\"]",
		"paths = [\".docs/wiki/03_FL/*.md\", \".docs/wiki/04_RF/*.md\"]",
		"",
		"[[family]]",
		"name = \"technical\"",
		"intent_keywords = [\"contract\", \"ask\", \"backend\"]",
		"paths = [\".docs/wiki/09_contratos/*.md\"]",
	}, "\n"))
	return root
}

func TestWorkspaceInitRegistersAndIndexes(t *testing.T) {
	alias := "init-ws-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	if env.Backend != "init" {
		t.Fatalf("backend = %q, want init", env.Backend)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one init item, got %#v", env.Items)
	}
	item := items[0]
	if item["name"] != alias {
		t.Fatalf("name = %#v, want %s", item["name"], alias)
	}
	if item["index_files"] == nil {
		t.Fatalf("expected init to auto-index, got %#v", item)
	}
	nextSteps, ok := item["next_steps"].([]string)
	if !ok || len(nextSteps) == 0 || !strings.Contains(nextSteps[0], "--workspace "+alias) {
		t.Fatalf("expected init next steps to include workspace alias, got %#v", item["next_steps"])
	}
	if _, err := workspace.ResolveWorkspace(alias); err != nil {
		t.Fatalf("ResolveWorkspace(%s): %v", alias, err)
	}
}

func TestNavAskUsesWikiAndCodeEvidence(t *testing.T) {
	alias := "ask-ws-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "how does daemon routing work?"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	if env.Backend != "ask" {
		t.Fatalf("backend = %q, want ask", env.Backend)
	}
	results, ok := env.Items.([]model.AskResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	result := results[0]
	if result.PrimaryDoc.Path != ".docs/wiki/07_baseline_tecnica.md" {
		t.Fatalf("primary doc = %q", result.PrimaryDoc.Path)
	}
	if len(result.CodeEvidence) == 0 {
		t.Fatalf("expected code evidence, got %#v", result)
	}
	if result.CodeEvidence[0].File != "src/daemon/router.cs" {
		t.Fatalf("first code evidence file = %q", result.CodeEvidence[0].File)
	}
	if len(result.NextQueries) == 0 {
		t.Fatalf("expected next queries, got %#v", result)
	}
}

func TestNavAskPrefersExplicitLinkedDocs(t *testing.T) {
	alias := "ask-links-" + filepath.Base(t.TempDir())
	root := createLinkedDocsWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "RF-QRY-010"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	result := results[0]
	if result.PrimaryDoc.DocID != "RF-QRY-010" {
		t.Fatalf("primary doc id = %q", result.PrimaryDoc.DocID)
	}
	if !hasDocEvidence(result.DocEvidence, ".docs/wiki/03_FL/FL-QRY-01.md") {
		t.Fatalf("expected FL evidence from explicit doc links, got %#v", result.DocEvidence)
	}
	if !hasDocEvidence(result.DocEvidence, ".docs/wiki/09_contratos/CT-NAV-ASK.md") {
		t.Fatalf("expected CT evidence from explicit doc links, got %#v", result.DocEvidence)
	}
}

func TestNavAskFallsBackWhenDocsIndexIsEmpty(t *testing.T) {
	alias := "ask-fallback-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/Program.cs", "namespace Demo;\npublic class Program {}\n")
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "Program"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	if results[0].PrimaryDoc.Path != "README.md" {
		t.Fatalf("fallback primary doc = %q", results[0].PrimaryDoc.Path)
	}
	if len(results[0].CodeEvidence) == 0 {
		t.Fatalf("expected textual code evidence in fallback, got %#v", results[0])
	}
}

func TestNavAskUsesBuiltinProfileForMinimalTechnicalDocs(t *testing.T) {
	alias := "ask-minimal-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/worker.cs", strings.Join([]string{
		"namespace Demo;",
		"public class WorkerProtocol",
		"{",
		"}",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", strings.Join([]string{
		"# 07 Baseline tecnica",
		"",
		"Worker protocol overview for daemon routing and worker protocol diagnostics.",
	}, "\n"))
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "worker protocol"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	if results[0].PrimaryDoc.Path != ".docs/wiki/07_baseline_tecnica.md" {
		t.Fatalf("primary doc = %q, want minimal technical baseline", results[0].PrimaryDoc.Path)
	}
	for _, warning := range env.Warnings {
		if warning == "no wiki match found" {
			t.Fatalf("unexpected no wiki match warning: %#v", env.Warnings)
		}
	}
}

func hasDocEvidence(items []model.AskDocEvidence, path string) bool {
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func TestNavAskNextQueriesIncludeRepoForContainerEvidence(t *testing.T) {
	alias := "ask-container-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "frontend/src/Login.tsx", "export function LoginPage() {}\n")
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", strings.Join([]string{
		"# 07. Baseline tecnica",
		"",
		"El recovery del frontend se apoya en `frontend/src/Login.tsx` y `LoginPage`.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"name = \"technical\"",
		"intent_keywords = [\"frontend\", \"recovery\", \"login\"]",
		"paths = [\".docs/wiki/07_*.md\"]",
	}, "\n"))

	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:        alias,
			Languages:   []string{"typescript"},
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

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "where is the frontend recovery flow?"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	if len(results[0].NextQueries) == 0 {
		t.Fatalf("expected next queries, got %#v", results[0])
	}
	for _, query := range results[0].NextQueries {
		if strings.Contains(query, "nav context") || strings.Contains(query, "nav related") || strings.Contains(query, "nav search") {
			if !strings.Contains(query, "--repo frontend") {
				t.Fatalf("expected repo-scoped next query, got %q", query)
			}
		}
	}
}

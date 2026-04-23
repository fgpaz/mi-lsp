package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavWikiSearchReturnsLayerFilteredDocs(t *testing.T) {
	alias := "wiki-search-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-NAV-WIKI.md", "# CT-NAV-WIKI\n\nContrato wiki search login.\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{
		{
			Path:       ".docs/wiki/03_FL/FL-AUTH-01.md",
			Title:      "FL-AUTH-01",
			DocID:      "FL-AUTH-01",
			Layer:      "03",
			Family:     "functional",
			Snippet:    "Flujo canonico de login.",
			SearchText: "flujo canonico login portal",
			IndexedAt:  1,
		},
		{
			Path:       ".docs/wiki/04_RF/RF-AUTH-001.md",
			Title:      "RF-AUTH-001 - Resolver login",
			DocID:      "RF-AUTH-001",
			Layer:      "04",
			Family:     "functional",
			Snippet:    "Resolver login desde la wiki.",
			SearchText: "resolver login auth handler RF AUTH",
			IndexedAt:  1,
		},
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-WIKI.md",
			Title:      "CT-NAV-WIKI",
			DocID:      "CT-NAV-WIKI",
			Layer:      "09",
			Family:     "technical",
			SearchText: "contrato wiki search login",
			IndexedAt:  1,
		},
	}, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "login auth", "layer": "RF", "top": 5, "include_content": true},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	results, ok := env.Items.([]model.WikiSearchResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one RF wiki search result, got %#v", env.Items)
	}
	result := results[0]
	if result.Layer != "RF" || result.DocID != "RF-AUTH-001" {
		t.Fatalf("unexpected wiki result: %#v", result)
	}
	if !strings.Contains(result.Content, "RF-AUTH-001") {
		t.Fatalf("expected included markdown content, got %q", result.Content)
	}
	joined := strings.Join(result.NextQueries, " | ")
	if !strings.Contains(joined, "nav wiki pack") || !strings.Contains(joined, "nav wiki trace RF-AUTH-001") || !strings.Contains(joined, "nav multi-read") {
		t.Fatalf("expected guided next queries, got %#v", result.NextQueries)
	}
}

func TestNavWikiSearchDocIndexEmptyReturnsDiagnostic(t *testing.T) {
	alias := "wiki-empty-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"query": "login"},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	if env.Backend != "wiki.search" || env.Hint == "" || !strings.Contains(env.Hint, "--docs-only") {
		t.Fatalf("expected docgraph diagnostic hint, got %#v", env)
	}
	results := env.Items.([]model.WikiSearchResult)
	if len(results) != 0 {
		t.Fatalf("expected no results with empty doc index, got %#v", results)
	}
}

func TestNavWikiSearchBlocksWhenGovernanceBlocked(t *testing.T) {
	alias := "wiki-gov-block-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

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
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"query": "daemon"},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}

func TestNavAskRoutePackRepoCompatWarnings(t *testing.T) {
	alias := "wiki-compat-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	app := New(root, nil)
	tests := []struct {
		name      string
		operation string
		payload   map[string]any
		wantHint  string
	}{
		{name: "ask", operation: "nav.ask", payload: map[string]any{"question": "login docs", "repo": "docs"}, wantHint: "nav wiki search"},
		{name: "route", operation: "nav.route", payload: map[string]any{"task": "login docs", "repo": "docs"}, wantHint: "nav wiki route"},
		{name: "pack", operation: "nav.pack", payload: map[string]any{"task": "login docs", "repo": "docs"}, wantHint: "nav wiki pack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := app.Execute(context.Background(), model.CommandRequest{
				Operation: tt.operation,
				Context:   model.QueryOptions{Workspace: alias},
				Payload:   tt.payload,
			})
			if err != nil {
				t.Fatalf("%s: %v", tt.operation, err)
			}
			if !strings.Contains(strings.Join(env.Warnings, " | "), "--repo") {
				t.Fatalf("expected --repo compatibility warning, got %#v", env.Warnings)
			}
			if !strings.Contains(env.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want %q", env.Hint, tt.wantHint)
			}
		})
	}
}

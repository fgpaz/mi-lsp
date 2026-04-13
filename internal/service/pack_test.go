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

func createFunctionalPackWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/auth/LoginHandler.cs", strings.Join([]string{
		"namespace Demo;",
		"public class LoginHandler",
		"{",
		"    public void Handle() { }",
		"}",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/01_alcance_funcional.md", strings.Join([]string{
		"# 1. Alcance",
		"",
		"El producto resuelve onboarding y login para usuarios del portal.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/02_arquitectura.md", strings.Join([]string{
		"# 2. Arquitectura",
		"",
		"La arquitectura distribuye el flujo de login entre CLI, auth y UI.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/03_FL/FL-AUTH-01.md", strings.Join([]string{
		"# FL-AUTH-01",
		"",
		"Flujo canonico de login del usuario final.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-AUTH-001.md", strings.Join([]string{
		"# RF-AUTH-001 - Resolver login",
		"",
		"Este RF implementa `FL-AUTH-01` y se apoya en `src/auth/LoginHandler.cs`.",
	}, "\n"))
	writeSpecBackendGovernanceFixture(t, root)
	return root
}

func TestNavPackBuildsFunctionalReadingPackInCanonicalOrder(t *testing.T) {
	alias := "pack-func-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
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
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	if env.Backend != "pack" {
		t.Fatalf("backend = %q, want pack", env.Backend)
	}
	results, ok := env.Items.([]model.PackResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", env.Items)
	}
	result := results[0]
	if result.Mode != "preview" {
		t.Fatalf("mode = %q, want preview", result.Mode)
	}
	if len(result.Docs) < 5 {
		t.Fatalf("expected canonical pack docs, got %#v", result.Docs)
	}
	got := []string{result.Docs[0].Path, result.Docs[1].Path, result.Docs[2].Path, result.Docs[3].Path, result.Docs[4].Path}
	want := []string{
		".docs/wiki/00_gobierno_documental.md",
		".docs/wiki/01_alcance_funcional.md",
		".docs/wiki/02_arquitectura.md",
		".docs/wiki/03_FL/FL-AUTH-01.md",
		".docs/wiki/04_RF/RF-AUTH-001.md",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("doc[%d] = %q, want %q (full docs: %#v)", i, got[i], want[i], result.Docs)
		}
	}
	if len(result.Docs[0].Targets) == 0 {
		t.Fatalf("expected preview targets, got %#v", result.Docs[0])
	}
}

func TestNavPackFullIncludesReadableSlices(t *testing.T) {
	alias := "pack-full-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
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
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, Full: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", env.Items)
	}
	if results[0].Mode != "full" {
		t.Fatalf("mode = %q, want full", results[0].Mode)
	}
	if len(results[0].Docs) == 0 || results[0].Docs[0].SliceText == "" {
		t.Fatalf("expected full slice text, got %#v", results[0].Docs)
	}
	if !strings.Contains(results[0].Docs[4].SliceText, "LoginHandler") {
		t.Fatalf("expected RF slice to include relevant snippet, got %#v", results[0].Docs[4])
	}
}

func TestNavPackWarnsWhenCanonicalWikiExistsButDocsAreNotIndexed(t *testing.T) {
	alias := "pack-stale-" + filepath.Base(t.TempDir())
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
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got %#v", env)
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("expected stale index warning, got %#v", env)
	}
	warnings := strings.Join(env.Warnings, " ")
	if !strings.Contains(warnings, "mi-lsp index") {
		t.Fatalf("expected re-index hint, got %v", env.Warnings)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", results)
	}
	// Tier 1 now provides canonical docs even when the index is empty/stale
	if len(results[0].Docs) == 0 {
		t.Fatalf("expected tier1 canonical docs when index is stale, got empty docs")
	}
	primaryPath := results[0].PrimaryDoc
	if !strings.Contains(primaryPath, ".docs/wiki/") {
		t.Fatalf("expected primary doc inside .docs/wiki/, got %q", primaryPath)
	}
}

func TestNavPackTreatsGenericOnlyIndexAsStaleWhenCanonicalWikiExists(t *testing.T) {
	alias := "pack-generic-stale-" + filepath.Base(t.TempDir())
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

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{{
		Path:        "README.md",
		Title:       "Generic fallback",
		Layer:       "generic",
		Family:      "generic",
		SearchText:  "generic fallback readme",
		ContentHash: "x1",
		IndexedAt:   1,
	}}, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("expected stale index warning, got %#v", env)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", results)
	}
	// Tier 1 now provides canonical docs even when only generic docs are indexed
	if len(results[0].Docs) == 0 {
		t.Fatalf("expected tier1 canonical docs when only generic docs indexed, got empty docs")
	}
	primaryPath := results[0].PrimaryDoc
	if !strings.Contains(primaryPath, ".docs/wiki/") {
		t.Fatalf("expected primary doc inside .docs/wiki/, got %q", primaryPath)
	}
}

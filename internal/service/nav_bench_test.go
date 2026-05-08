package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func BenchmarkNavSearchText(b *testing.B) {
	root, alias := setupBenchmarkWorkspace(b)
	app := New(root, nil)
	ctx := context.Background()
	request := model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"pattern": "LoginHandler", "include_content": true},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := app.Execute(ctx, request); err != nil {
			b.Fatalf("nav.search: %v", err)
		}
	}
}

func BenchmarkNavWikiSearch(b *testing.B) {
	root, alias := setupBenchmarkWorkspace(b)
	seedBenchmarkDocs(b, root)
	app := New(root, nil)
	ctx := context.Background()
	request := model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "login auth", "layer": "RF", "top": 5, "include_content": true},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := app.Execute(ctx, request); err != nil {
			b.Fatalf("nav.wiki.search: %v", err)
		}
	}
}

func BenchmarkNavPackPreview(b *testing.B) {
	root, alias := setupBenchmarkWorkspace(b)
	app := New(root, nil)
	ctx := context.Background()
	request := model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand how login works"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := app.Execute(ctx, request); err != nil {
			b.Fatalf("nav.pack: %v", err)
		}
	}
}

func setupBenchmarkWorkspace(b *testing.B) (string, string) {
	b.Helper()
	ensureWritableBenchmarkHome(b)
	root := b.TempDir()
	alias := "bench-ws-" + filepath.Base(root)

	writeBenchmarkFile(b, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeBenchmarkFile(b, root, "src/auth/LoginHandler.cs", strings.Join([]string{
		"namespace Demo;",
		"public class LoginHandler",
		"{",
		"    public void Handle() { }",
		"}",
	}, "\n"))
	writeBenchmarkFile(b, root, ".docs/wiki/01_alcance_funcional.md", "# 1. Alcance\n\nEl producto resuelve onboarding y login para usuarios del portal.\n")
	writeBenchmarkFile(b, root, ".docs/wiki/02_arquitectura.md", "# 2. Arquitectura\n\nLa arquitectura distribuye el flujo de login entre CLI, auth y UI.\n")
	writeBenchmarkFile(b, root, ".docs/wiki/03_FL/FL-AUTH-01.md", "# FL-AUTH-01\n\nFlujo canonico de login del usuario final.\n")
	writeBenchmarkFile(b, root, ".docs/wiki/04_RF/RF-AUTH-001.md", "# RF-AUTH-001 - Resolver login\n\nEste RF implementa `FL-AUTH-01` y se apoya en `src/auth/LoginHandler.cs`.\n")
	writeBenchmarkGovernanceFixture(b, root)

	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		b.Fatalf("register workspace: %v", err)
	}
	b.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	return root, alias
}

func ensureWritableBenchmarkHome(b *testing.B) {
	b.Helper()
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("USERPROFILE", home)
}

func writeBenchmarkFile(b *testing.B, root string, relativePath string, content string) {
	b.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatalf("mkdir %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatalf("write %s: %v", relativePath, err)
	}
}

func seedBenchmarkDocs(b *testing.B, root string) {
	b.Helper()
	db, err := store.Open(root)
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
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
	}, nil, nil); err != nil {
		b.Fatalf("ReplaceDocs: %v", err)
	}
}

func writeBenchmarkGovernanceFixture(b *testing.B, root string) {
	b.Helper()
	writeBenchmarkFile(b, root, ".docs/wiki/00_gobierno_documental.md", strings.Join([]string{
		"# 00. Gobierno documental",
		"",
		"## Governance Source",
		"",
		"```yaml",
		"version: 1",
		"profile: spec_backend",
		"overlays:",
		"  - spec_core",
		"  - technical",
		"numbering_recommended: true",
		"hierarchy:",
		"  - id: governance",
		"    label: Gobierno documental",
		"    layer: \"00\"",
		"    family: functional",
		"    pack_stage: governance",
		"    paths:",
		"      - .docs/wiki/00_gobierno_documental.md",
		"  - id: scope",
		"    label: Alcance",
		"    layer: \"01\"",
		"    family: functional",
		"    pack_stage: scope",
		"    paths:",
		"      - .docs/wiki/01_*.md",
		"  - id: architecture",
		"    label: Arquitectura",
		"    layer: \"02\"",
		"    family: functional",
		"    pack_stage: architecture",
		"    paths:",
		"      - .docs/wiki/02_*.md",
		"  - id: flow",
		"    label: Flujos",
		"    layer: \"03\"",
		"    family: functional",
		"    pack_stage: flow",
		"    paths:",
		"      - .docs/wiki/03_FL.md",
		"      - .docs/wiki/03_FL/*.md",
		"  - id: requirements",
		"    label: Requerimientos",
		"    layer: \"04\"",
		"    family: functional",
		"    pack_stage: requirements",
		"    paths:",
		"      - .docs/wiki/04_RF.md",
		"      - .docs/wiki/04_RF/*.md",
		"  - id: technical_baseline",
		"    label: Baseline tecnica",
		"    layer: \"07\"",
		"    family: technical",
		"    pack_stage: technical_baseline",
		"    paths:",
		"      - .docs/wiki/07_*.md",
		"      - .docs/wiki/07_tech/*.md",
		"  - id: contracts",
		"    label: Contratos tecnicos",
		"    layer: \"09\"",
		"    family: technical",
		"    pack_stage: contracts",
		"    paths:",
		"      - .docs/wiki/09_*.md",
		"      - .docs/wiki/09_contratos/*.md",
		"context_chain:",
		"  - governance",
		"  - scope",
		"  - architecture",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"closure_chain:",
		"  - governance",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"audit_chain:",
		"  - governance",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"blocking_rules:",
		"  - missing_human_governance_doc",
		"  - missing_governance_yaml",
		"  - invalid_governance_schema",
		"  - projection_out_of_sync",
		"projection:",
		"  output: .docs/wiki/_mi-lsp/read-model.toml",
		"  format: toml",
		"  auto_sync: true",
		"  versioned: true",
		"```",
	}, "\n"))

	if status := docgraph.InspectGovernance(root, true); status.Blocked {
		b.Fatalf("expected governance fixture to be valid, got blocked status: %#v", status)
	}
}

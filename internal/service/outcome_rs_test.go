package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func createOutcomeWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/01_alcance_funcional.md", "# 01. Alcance\n\nAlcance funcional.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/02_resultados_soluciones_usuario.md", "# 02. Resultados y soluciones de usuario\n\nIndice outcome.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/02_resultados/RS-TEDI-HOGAR-01.md", strings.Join([]string{
		"# RS-TEDI-HOGAR-01",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RS-TEDI-HOGAR-01 | Hogar Tedi visible y accionable |",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/02_arquitectura.md", "# 02. Arquitectura\n\nArquitectura.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/03_FL.md", "# 03. Flujos\n\nFL-OUT-01.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF.md", "# 04. RF\n\nRF-OUT-001.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/06_matriz_pruebas_RF.md", "# 06. Pruebas\n\nTP-OUT.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos_tecnicos.md", "# 09. Contratos tecnicos\n")
	writeOutcomeGovernanceFixture(t, root)
	return root
}

func writeOutcomeGovernanceFixture(t *testing.T, root string) {
	t.Helper()
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", strings.Join([]string{
		"# 00. Gobierno documental",
		"",
		"```yaml",
		"version: 1",
		"profile: spec_backend",
		"numbering_recommended: true",
		"hierarchy:",
		"  - id: governance",
		"    layer: \"00\"",
		"    family: functional",
		"    pack_stage: governance",
		"    paths:",
		"      - .docs/wiki/00_gobierno_documental.md",
		"  - id: scope",
		"    layer: \"01\"",
		"    family: functional",
		"    pack_stage: scope",
		"    paths:",
		"      - .docs/wiki/01_alcance_funcional.md",
		"  - id: outcome",
		"    layer: \"02\"",
		"    family: functional",
		"    pack_stage: outcome",
		"    paths:",
		"      - .docs/wiki/02_resultados_soluciones_usuario.md",
		"      - .docs/wiki/02_resultados/*.md",
		"  - id: architecture",
		"    layer: \"02\"",
		"    family: functional",
		"    pack_stage: architecture",
		"    paths:",
		"      - .docs/wiki/02_arquitectura.md",
		"  - id: flow",
		"    layer: \"03\"",
		"    family: functional",
		"    pack_stage: flow",
		"    paths:",
		"      - .docs/wiki/03_FL.md",
		"  - id: requirements",
		"    layer: \"04\"",
		"    family: functional",
		"    pack_stage: requirements",
		"    paths:",
		"      - .docs/wiki/04_RF.md",
		"  - id: tests",
		"    layer: \"06\"",
		"    family: functional",
		"    pack_stage: tests",
		"    paths:",
		"      - .docs/wiki/06_matriz_pruebas_RF.md",
		"  - id: technical_baseline",
		"    layer: \"07\"",
		"    family: technical",
		"    pack_stage: technical_baseline",
		"    paths:",
		"      - .docs/wiki/07_baseline_tecnica.md",
		"  - id: contracts",
		"    layer: \"09\"",
		"    family: technical",
		"    pack_stage: contracts",
		"    paths:",
		"      - .docs/wiki/09_contratos_tecnicos.md",
		"context_chain:",
		"  - governance",
		"  - scope",
		"  - outcome",
		"  - architecture",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"closure_chain:",
		"  - governance",
		"  - outcome",
		"  - requirements",
		"  - contracts",
		"  - tests",
		"audit_chain:",
		"  - governance",
		"  - outcome",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"  - tests",
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
	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("expected outcome governance fixture to be valid, got blocked status: %#v", status)
	}
}

func registerOutcomeWorkspace(t *testing.T, alias string, root string) {
	t.Helper()
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
}

func TestNavRouteExplicitRSUsesOutcomeDoc(t *testing.T) {
	alias := "route-rs-" + filepath.Base(t.TempDir())
	root := createOutcomeWorkspaceFixture(t, alias)
	registerOutcomeWorkspace(t, alias, root)

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "RS-TEDI-HOGAR-01"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	anchor := results[0].Canonical.AnchorDoc
	if anchor.Path != ".docs/wiki/02_resultados/RS-TEDI-HOGAR-01.md" {
		t.Fatalf("anchor path = %q, want RS outcome doc", anchor.Path)
	}
	if anchor.DocID != "RS-TEDI-HOGAR-01" || anchor.Layer != "RS" {
		t.Fatalf("anchor classification = %#v", anchor)
	}
}

func TestNavWikiSearchAcceptsRSLayer(t *testing.T) {
	alias := "wiki-rs-" + filepath.Base(t.TempDir())
	root := createOutcomeWorkspaceFixture(t, alias)
	registerOutcomeWorkspace(t, alias, root)

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true},
	}); err != nil {
		t.Fatalf("index.run --docs-only: %v", err)
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "RS-TEDI-HOGAR-01", "layer": "RS", "top": 5},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	results := env.Items.([]model.WikiSearchResult)
	if len(results) == 0 {
		t.Fatalf("expected RS wiki result, got none: %#v", env)
	}
	if results[0].Layer != "RS" || results[0].Stage != "outcome" {
		t.Fatalf("RS wiki classification = %#v", results[0])
	}
	if !strings.Contains(strings.Join(results[0].NextQueries, " | "), "nav wiki trace RS-TEDI-HOGAR-01") {
		t.Fatalf("expected trace next query for RS, got %#v", results[0].NextQueries)
	}
}

func TestNavPackIncludesOutcomeStage(t *testing.T) {
	alias := "pack-rs-" + filepath.Base(t.TempDir())
	root := createOutcomeWorkspaceFixture(t, alias)
	registerOutcomeWorkspace(t, alias, root)

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true},
	}); err != nil {
		t.Fatalf("index.run --docs-only: %v", err)
	}
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "RS-TEDI-HOGAR-01"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 || len(results[0].Docs) == 0 {
		t.Fatalf("expected pack docs, got %#v", env.Items)
	}
	if results[0].Docs[0].Layer != "RS" || results[0].Docs[0].Stage != "anchor" {
		t.Fatalf("pack anchor = %#v", results[0].Docs[0])
	}
}

func TestNavTraceRSTraceUsesDocIDLayerStageWithoutRF(t *testing.T) {
	alias := "trace-rs-" + filepath.Base(t.TempDir())
	root := createOutcomeWorkspaceFixture(t, alias)
	registerOutcomeWorkspace(t, alias, root)

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true},
	}); err != nil {
		t.Fatalf("index.run --docs-only: %v", err)
	}
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RS-TEDI-HOGAR-01"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results := env.Items.([]model.TraceResult)
	if len(results) != 1 {
		t.Fatalf("expected one RS trace result, got %#v", env.Items)
	}
	if results[0].DocID != "RS-TEDI-HOGAR-01" || results[0].Layer != "RS" || results[0].Stage != "outcome" {
		t.Fatalf("RS trace classification = %#v", results[0])
	}
	if results[0].RF != "" {
		t.Fatalf("RS trace should not populate RF compatibility field: %#v", results[0])
	}
}

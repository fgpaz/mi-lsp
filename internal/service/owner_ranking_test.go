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

func TestRankDocsOwnerAwarePenalizesGenericWhenCanonicalMatchExists(t *testing.T) {
	t.Setenv("MI_LSP_DOC_RANKING", "owner")

	profile := model.DocsReadProfile{}
	docs := []model.DocRecord{
		{
			Path:       "README.md",
			Title:      "Continuation memory pointer guide",
			Family:     "generic",
			Layer:      "generic",
			SearchText: "continuation memory pointer guide",
		},
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-ASK.md",
			Title:      "CT-NAV-ASK continuation memory pointer contract",
			DocID:      "CT-NAV-ASK",
			Family:     "technical",
			Layer:      "09",
			SearchText: "continuation memory pointer contract",
		},
	}

	ranked := rankDocs("how do continuation and memory pointer work?", "technical", docs, nil, profile, nil)
	if len(ranked) == 0 {
		t.Fatalf("expected ranked docs, got none")
	}
	if ranked[0].record.Path == "README.md" {
		t.Fatalf("expected canonical doc before generic README, got %#v", ranked)
	}
}

func TestRankDocsOwnerHintsBeatLexicalDefault(t *testing.T) {
	t.Setenv("MI_LSP_DOC_RANKING", "owner")

	profile := model.DocsReadProfile{
		OwnerHints: []model.DocsOwnerHint{{
			Terms:        []string{"continuation", "memory pointer"},
			PreferDocIDs: []string{"CT-NAV-ASK"},
		}},
	}
	docs := []model.DocRecord{
		{
			Path:       ".docs/wiki/04_RF/RF-QRY-010.md",
			Title:      "RF-QRY-010 continuation memory pointer continuation memory",
			DocID:      "RF-QRY-010",
			Family:     "functional",
			Layer:      "04",
			SearchText: "continuation memory pointer continuation memory",
		},
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-ASK.md",
			Title:      "CT-NAV-ASK contract",
			DocID:      "CT-NAV-ASK",
			Family:     "technical",
			Layer:      "09",
			SearchText: "ask contract",
		},
	}

	ranked := rankDocs("how do continuation and memory pointer work?", "functional", docs, nil, profile, nil)
	if len(ranked) == 0 {
		t.Fatalf("expected ranked docs, got none")
	}
	if ranked[0].record.DocID != "CT-NAV-ASK" {
		t.Fatalf("expected owner hint to lift CT-NAV-ASK, got %#v", ranked)
	}
}

func TestRankDocsRecentChangeIsWeakTieBreak(t *testing.T) {
	t.Setenv("MI_LSP_DOC_RANKING", "owner")

	docs := []model.DocRecord{
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-ASK.md",
			Title:      "continuation memory pointer contract",
			DocID:      "CT-NAV-ASK",
			Family:     "technical",
			Layer:      "09",
			SearchText: "continuation memory pointer contract",
		},
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-ROUTE.md",
			Title:      "continuation memory pointer contract",
			DocID:      "CT-NAV-ROUTE",
			Family:     "technical",
			Layer:      "09",
			SearchText: "continuation memory pointer contract",
		},
	}
	recent := []model.ReentryMemoryChange{{
		Path: ".docs/wiki/09_contratos/CT-NAV-ROUTE.md",
	}}

	ranked := rankDocs("continuation memory pointer contract", "technical", docs, nil, model.DocsReadProfile{}, recent)
	if len(ranked) < 2 {
		t.Fatalf("expected two ranked docs, got %#v", ranked)
	}
	if ranked[0].record.DocID != "CT-NAV-ROUTE" {
		t.Fatalf("expected recent doc to win tie-break, got %#v", ranked)
	}
}

func TestRankDocsLegacyOverrideIgnoresOwnerHints(t *testing.T) {
	profile := model.DocsReadProfile{
		OwnerHints: []model.DocsOwnerHint{{
			Terms:        []string{"continuation", "memory pointer"},
			PreferDocIDs: []string{"CT-NAV-ASK"},
		}},
	}
	docs := []model.DocRecord{
		{
			Path:       ".docs/wiki/04_RF/RF-QRY-010.md",
			Title:      "RF-QRY-010 continuation memory pointer continuation memory",
			DocID:      "RF-QRY-010",
			Family:     "functional",
			Layer:      "04",
			SearchText: "continuation memory pointer continuation memory",
		},
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-ASK.md",
			Title:      "CT-NAV-ASK contract",
			DocID:      "CT-NAV-ASK",
			Family:     "technical",
			Layer:      "09",
			SearchText: "ask contract",
		},
	}

	t.Setenv("MI_LSP_DOC_RANKING", "legacy")
	legacy := rankDocs("how do continuation and memory pointer work?", "functional", docs, nil, profile, nil)
	if len(legacy) == 0 || legacy[0].record.DocID != "RF-QRY-010" {
		t.Fatalf("expected legacy ranker to keep RF first, got %#v", legacy)
	}

	t.Setenv("MI_LSP_DOC_RANKING", "owner")
	owner := rankDocs("how do continuation and memory pointer work?", "functional", docs, nil, profile, nil)
	if len(owner) == 0 || owner[0].record.DocID != "CT-NAV-ASK" {
		t.Fatalf("expected owner ranker to honor hint, got %#v", owner)
	}
}

func TestRankDocsMatchesNormalizedDocIDQuery(t *testing.T) {
	docs := []model.DocRecord{
		{
			Path:       ".docs/wiki/04_RF/RF-QRY-015.md",
			Title:      "RF-QRY-015 mentions CT NAV ASK",
			DocID:      "RF-QRY-015",
			Family:     "functional",
			Layer:      "04",
			SearchText: "ct nav ask reuse route",
		},
		{
			Path:       ".docs/wiki/09_contratos/CT-NAV-ASK.md",
			Title:      "CT-NAV-ASK",
			DocID:      "CT-NAV-ASK",
			Family:     "technical",
			Layer:      "09",
			SearchText: "ct nav ask contract",
		},
	}

	ranked := rankDocs("CT-NAV-ASK", "functional", docs, nil, model.DocsReadProfile{}, nil)
	if len(ranked) == 0 || ranked[0].record.DocID != "CT-NAV-ASK" {
		t.Fatalf("expected exact doc id query to prefer CT-NAV-ASK, got %#v", ranked)
	}
}

func TestQuestionTokensKeepsCanonicalShortLayerTerms(t *testing.T) {
	tokens := docgraph.QuestionTokens("Which RF, FL, CT, TECH, DB and TP docs matter most?")
	for _, token := range []string{"rf", "fl", "ct", "tech", "db", "tp"} {
		if !containsString(tokens, token) {
			t.Fatalf("expected token %q in %#v", token, tokens)
		}
	}
}

func TestRankDocsOwnerAwarePrefersCanonicalWikiOverRawSupportArtifacts(t *testing.T) {
	t.Setenv("MI_LSP_DOC_RANKING", "owner")

	docs := []model.DocRecord{
		{
			Path:       ".docs/raw/prompts/2026-04-09-wiki-hardening-full.md",
			Title:      "MAX-PRESSURE: Wiki Full-Hardening to Code Parity",
			DocID:      "TECH-DEMO",
			Family:     "generic",
			Layer:      "20",
			SearchText: "rf fl ct tech db tp docs parity audit across all microservices",
		},
		{
			Path:       ".docs/wiki/03_FL.md",
			Title:      "03_FL",
			Family:     "functional",
			Layer:      "03",
			SearchText: "fl flow inventory docs across all microservices",
		},
		{
			Path:       ".docs/wiki/04_RF.md",
			Title:      "04_RF",
			Family:     "functional",
			Layer:      "04",
			SearchText: "rf module index docs across all microservices",
		},
		{
			Path:       ".docs/wiki/09_contratos_tecnicos.md",
			Title:      "09 Contratos Tecnicos",
			Family:     "technical",
			Layer:      "09",
			SearchText: "ct contracts api parity audit",
		},
	}

	ranked := rankDocs("Which RF, FL, CT, TECH, DB and TP docs are most relevant for a full wiki-to-code parity audit across all microservices?", "technical", docs, nil, model.DocsReadProfile{}, nil)
	if len(ranked) == 0 {
		t.Fatalf("expected ranked docs, got none")
	}
	if ranked[0].record.Path == ".docs/raw/prompts/2026-04-09-wiki-hardening-full.md" {
		t.Fatalf("expected canonical wiki doc before raw support artifact, got %#v", ranked)
	}
}

func TestNavIntentDocsModeReturnsCanonicalDocs(t *testing.T) {
	alias := "intent-docs-" + filepath.Base(t.TempDir())
	root := createOwnerAwareDocsWorkspaceFixture(t)
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
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "how do continuation and memory pointer work?"},
	})
	if err != nil {
		t.Fatalf("nav.intent: %v", err)
	}
	if env.Mode != "docs" {
		t.Fatalf("mode = %q, want docs", env.Mode)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected docs intent items, got %#v", env.Items)
	}
	if items[0]["doc_path"] == "README.md" {
		t.Fatalf("expected canonical doc before README, got %#v", items[0])
	}
	docID, _ := items[0]["doc_id"].(string)
	if !isContinuationOwnerDoc(docID) {
		t.Fatalf("expected owner doc for continuation slice, got %#v", items[0])
	}
}

func TestGovernanceProjectsOwnerHintsIntoReadModel(t *testing.T) {
	root := createOwnerAwareDocsWorkspaceFixture(t)
	profile, source, warnings := docgraph.LoadProfile(root)
	if source != "project" {
		t.Fatalf("profile source = %q, want project (warnings=%v)", source, warnings)
	}
	if len(profile.OwnerHints) == 0 {
		t.Fatalf("expected projected owner hints, got %#v", profile)
	}
	if len(profile.OwnerHints[0].Terms) == 0 || len(profile.OwnerHints[0].PreferDocIDs) == 0 {
		t.Fatalf("expected owner hint terms + doc ids, got %#v", profile.OwnerHints[0])
	}
}

func TestNavRouteOwnerAwareAvoidsREADMEForContinuationQuery(t *testing.T) {
	alias := "route-owner-" + filepath.Base(t.TempDir())
	root := createOwnerAwareDocsWorkspaceFixture(t)
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "how do continuation and memory pointer work?"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if len(results) != 1 {
		t.Fatalf("expected one route result, got %#v", env.Items)
	}
	if results[0].Canonical.AnchorDoc.Path == "README.md" {
		t.Fatalf("expected canonical anchor, got %#v", results[0])
	}
	if !isContinuationOwnerDoc(results[0].Canonical.AnchorDoc.DocID) {
		t.Fatalf("expected owner doc for continuation slice, got %#v", results[0].Canonical.AnchorDoc)
	}
}

func TestNavAskOwnerAwareAvoidsREADMEForContinuationQuery(t *testing.T) {
	alias := "ask-owner-" + filepath.Base(t.TempDir())
	root := createOwnerAwareDocsWorkspaceFixture(t)
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
		Payload:   map[string]any{"question": "how do continuation and memory pointer work?"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	if results[0].PrimaryDoc.Path == "README.md" {
		t.Fatalf("expected canonical primary doc, got %#v", results[0])
	}
	if !isContinuationOwnerDoc(results[0].PrimaryDoc.DocID) {
		t.Fatalf("expected owner doc for continuation slice, got %#v", results[0].PrimaryDoc)
	}
}

func TestNavPackOwnerAwareAvoidsREADMEForContinuationQuery(t *testing.T) {
	alias := "pack-owner-" + filepath.Base(t.TempDir())
	root := createOwnerAwareDocsWorkspaceFixture(t)
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
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 5},
		Payload:   map[string]any{"task": "how do continuation and memory pointer work?"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", env.Items)
	}
	if results[0].PrimaryDoc == "README.md" {
		t.Fatalf("expected canonical pack primary doc, got %#v", results[0])
	}
	if !hasAnyContinuationOwnerPath(results[0].PrimaryDoc) {
		t.Fatalf("expected owner doc path in pack, got %#v", results[0])
	}
}

func TestNavAskOwnerAwarePrefersCanonicalWikiOverRawSupportArtifacts(t *testing.T) {
	alias := "ask-raw-penalty-" + filepath.Base(t.TempDir())
	root := createRawSupportArtifactWorkspaceFixture(t)
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
		Payload:   map[string]any{"question": "Which RF, FL, CT, TECH, DB and TP docs are most relevant for a full wiki-to-code parity audit across all microservices?"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	if strings.HasPrefix(results[0].PrimaryDoc.Path, ".docs/raw/") {
		t.Fatalf("expected canonical wiki primary doc, got %#v", results[0].PrimaryDoc)
	}
}

func TestNavRouteOwnerAwarePrefersCanonicalWikiOverRawSupportArtifacts(t *testing.T) {
	alias := "route-raw-penalty-" + filepath.Base(t.TempDir())
	root := createRawSupportArtifactWorkspaceFixture(t)
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "Which RF, FL, CT, TECH, DB and TP docs are most relevant for a full wiki-to-code parity audit across all microservices?"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if len(results) != 1 {
		t.Fatalf("expected one route result, got %#v", env.Items)
	}
	if strings.HasPrefix(results[0].Canonical.AnchorDoc.Path, ".docs/raw/") {
		t.Fatalf("expected canonical wiki anchor doc, got %#v", results[0].Canonical.AnchorDoc)
	}
}

func TestNavPackOwnerAwarePrefersCanonicalWikiOverRawSupportArtifacts(t *testing.T) {
	alias := "pack-raw-penalty-" + filepath.Base(t.TempDir())
	root := createRawSupportArtifactWorkspaceFixture(t)
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
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 5},
		Payload:   map[string]any{"task": "Which RF, FL, CT, TECH, DB and TP docs are most relevant for a full wiki-to-code parity audit across all microservices?"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", env.Items)
	}
	if strings.HasPrefix(results[0].PrimaryDoc, ".docs/raw/") {
		t.Fatalf("expected canonical wiki pack primary doc, got %#v", results[0])
	}
}

func TestNavIntentDocsModePrefersCanonicalWikiOverRawSupportArtifacts(t *testing.T) {
	alias := "intent-raw-penalty-" + filepath.Base(t.TempDir())
	root := createRawSupportArtifactWorkspaceFixture(t)
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
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "Which RF, FL, CT, TECH, DB and TP docs are most relevant for a full wiki-to-code parity audit across all microservices?"},
	})
	if err != nil {
		t.Fatalf("nav.intent: %v", err)
	}
	if env.Mode != "docs" {
		t.Fatalf("mode = %q, want docs", env.Mode)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected docs intent items, got %#v", env.Items)
	}
	docPath, _ := items[0]["doc_path"].(string)
	if strings.HasPrefix(docPath, ".docs/raw/") {
		t.Fatalf("expected canonical wiki intent doc, got %#v", items[0])
	}
}

func TestNavIntentDocsModePrefersExactDocIDQuery(t *testing.T) {
	alias := "intent-doc-id-" + filepath.Base(t.TempDir())
	root := createOwnerAwareDocsWorkspaceFixture(t)
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
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"question": "CT-NAV-ASK"},
	})
	if err != nil {
		t.Fatalf("nav.intent: %v", err)
	}
	if env.Mode != "docs" {
		t.Fatalf("mode = %q, want docs", env.Mode)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected docs intent items, got %#v", env.Items)
	}
	if items[0]["doc_id"] != "CT-NAV-ASK" {
		t.Fatalf("expected CT-NAV-ASK to win exact doc-id query, got %#v", items[0])
	}
}

func createOwnerAwareDocsWorkspaceFixture(t *testing.T) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "README.md", strings.Join([]string{
		"# Demo workspace",
		"",
		"Continuation and memory pointer are briefly mentioned here, but this README is not the owner.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/03_FL/FL-QRY-01.md", strings.Join([]string{
		"# FL-QRY-01",
		"",
		"Flujo docs-first donde `continuation` y `memory_pointer` orientan el siguiente paso.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-QRY-010.md", strings.Join([]string{
		"# RF-QRY-010 - Query Coach y continuation",
		"",
		"`continuation` y `memory_pointer` agregan guidance corto para cada llamada.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-WKS-005.md", strings.Join([]string{
		"# RF-WKS-005 - Workspace status full stale",
		"",
		"`workspace status --full` expone memoria repo-local y marca `stale` cuando corresponde.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-NAV-ASK.md", strings.Join([]string{
		"# CT-NAV-ASK",
		"",
		"Contrato owner de `nav ask` para `continuation`, `memory_pointer` y evidence docs-first.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-NAV-ROUTE.md", strings.Join([]string{
		"# CT-NAV-ROUTE",
		"",
		"Contrato owner de `nav route` para `continuation` y `memory_pointer`.",
	}, "\n"))
	writeOwnerAwareGovernanceFixture(t, root)
	return root
}

func writeOwnerAwareGovernanceFixture(t *testing.T, root string) {
	t.Helper()
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", strings.Join([]string{
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
		"owner_hints:",
		"  - terms:",
		"      - continuation",
		"      - memory pointer",
		"      - memory_pointer",
		"    prefer_doc_ids:",
		"      - FL-QRY-01",
		"      - RF-QRY-010",
		"      - CT-NAV-ASK",
		"      - CT-NAV-ROUTE",
		"    prefer_layers:",
		"      - \"03\"",
		"      - \"04\"",
		"      - \"09\"",
		"  - terms:",
		"      - workspace status",
		"      - stale",
		"      - full",
		"    prefer_doc_ids:",
		"      - RF-WKS-005",
		"    prefer_layers:",
		"      - \"04\"",
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
		"  - id: data",
		"    label: Modelo de datos",
		"    layer: \"05\"",
		"    family: functional",
		"    pack_stage: data",
		"    paths:",
		"      - .docs/wiki/05_*.md",
		"  - id: tests",
		"    label: Pruebas",
		"    layer: \"06\"",
		"    family: functional",
		"    pack_stage: tests",
		"    paths:",
		"      - .docs/wiki/06_*.md",
		"      - .docs/wiki/06_pruebas/*.md",
		"  - id: technical_baseline",
		"    label: Baseline tecnica",
		"    layer: \"07\"",
		"    family: technical",
		"    pack_stage: technical_baseline",
		"    paths:",
		"      - .docs/wiki/07_*.md",
		"      - .docs/wiki/07_tech/*.md",
		"  - id: physical_data",
		"    label: Modelo fisico",
		"    layer: \"08\"",
		"    family: technical",
		"    pack_stage: physical_data",
		"    paths:",
		"      - .docs/wiki/08_*.md",
		"      - .docs/wiki/08_db/*.md",
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
		"  - tests",
		"audit_chain:",
		"  - governance",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - physical_data",
		"  - contracts",
		"  - tests",
		"blocking_rules:",
		"  - missing_human_governance_doc",
		"  - missing_governance_yaml",
		"  - invalid_governance_schema",
		"  - projection_out_of_sync",
		"  - workspace_index_stale",
		"projection:",
		"  output: .docs/wiki/_mi-lsp/read-model.toml",
		"  format: toml",
		"  auto_sync: true",
		"  versioned: true",
		"```",
	}, "\n"))

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("expected governance fixture to be valid, got blocked status: %#v", status)
	}
}

func createRawSupportArtifactWorkspaceFixture(t *testing.T) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/03_FL.md", strings.Join([]string{
		"# 03_FL",
		"",
		"Flow inventory and FL docs across all microservices.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF.md", strings.Join([]string{
		"# 04_RF",
		"",
		"RF module index for full parity audit.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", strings.Join([]string{
		"# 07_baseline_tecnica",
		"",
		"TECH baseline for cross-service parity audit.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/08_modelo_fisico_datos.md", strings.Join([]string{
		"# 08_modelo_fisico_datos",
		"",
		"DB baseline for runtime persistence and audit coverage.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos_tecnicos.md", strings.Join([]string{
		"# 09_contratos_tecnicos",
		"",
		"CT contract index for APIs, envelopes, and protocol parity.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/06_matriz_pruebas_RF.md", strings.Join([]string{
		"# 06_matriz_pruebas_RF",
		"",
		"TP matrix for validating RF coverage.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/raw/prompts/2026-04-09-wiki-hardening-full.md", strings.Join([]string{
		"# MAX-PRESSURE: Wiki Full-Hardening to Code Parity",
		"",
		"Prompt de soporte para hardening full wiki-to-code parity across all microservices.",
	}, "\n"))
	writeSpecBackendGovernanceFixture(t, root)
	return root
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func isContinuationOwnerDoc(docID string) bool {
	switch strings.TrimSpace(docID) {
	case "FL-QRY-01", "RF-QRY-010", "CT-NAV-ASK", "CT-NAV-ROUTE":
		return true
	default:
		return false
	}
}

func hasAnyContinuationOwnerPath(path string) bool {
	for _, marker := range []string{"FL-QRY-01", "RF-QRY-010", "CT-NAV-ASK", "CT-NAV-ROUTE"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

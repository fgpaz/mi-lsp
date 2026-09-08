package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestExtractWikiPrimaryPathPreservesOperationAuthority(t *testing.T) {
	cases := []struct {
		operation string
		items     any
		want      string
	}{
		{"nav.ask", []model.AskResult{{PrimaryDoc: model.AskDocEvidence{Path: ".docs/wiki/04_RF/RF-QRY-001.md"}}}, ".docs/wiki/04_RF/RF-QRY-001.md"},
		{"nav.route", []model.RouteResult{{Canonical: model.RouteCanonicalLane{AnchorDoc: model.RouteDoc{Path: ".docs/wiki/02_arquitectura.md"}}}}, ".docs/wiki/02_arquitectura.md"},
		{"nav.pack", []model.PackResult{{PrimaryDoc: ".docs/wiki/03_FL.md"}}, ".docs/wiki/03_FL.md"},
		{"nav.affected", []AffectedItem{{Path: ".docs/wiki/06_matriz_pruebas_RF.md"}}, ".docs/wiki/06_matriz_pruebas_RF.md"},
		{"nav.diff-context", []DiffContextResult{{ChangedSymbols: []DiffSymbol{{File: ".docs/wiki/09_contratos_tecnicos.md"}}}}, ".docs/wiki/09_contratos_tecnicos.md"},
	}
	for _, tc := range cases {
		if got := extractWikiPrimaryPath(tc.operation, tc.items); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.operation, got, tc.want)
		}
	}
}

func TestExtractWikiPrimaryPathNeverSelectsRaw(t *testing.T) {
	items := []map[string]any{{"path": ".docs/raw/task.md"}, {"path": ".docs/wiki/00_gobierno_documental.md"}}
	if got := extractWikiPrimaryPath("nav.affected", items); got != ".docs/wiki/00_gobierno_documental.md" {
		t.Fatalf("got %q, want canonical wiki path", got)
	}
}

func TestEnrichWikiCodeContextNormalIndexPreservesEnvelopeAndAuthority(t *testing.T) {
	root, app := newWikiCodeEnrichFixture(t)
	ctx := context.Background()
	cases := []struct {
		operation string
		items     any
		want      string
	}{
		{"nav.ask", []map[string]any{{"primary_doc": ".docs/wiki/04_RF/RF-GPH-007.md", "path": ".docs/raw/ignored.md"}}, ".docs/wiki/04_RF/RF-GPH-007.md"},
		{"nav.route", []map[string]any{{"canonical": ".docs/wiki/04_RF/RF-GPH-007.md", "path": ".docs/raw/ignored.md"}}, ".docs/wiki/04_RF/RF-GPH-007.md"},
		{"nav.pack", []map[string]any{{"primary_doc": ".docs/wiki/04_RF/RF-GPH-007.md"}}, ".docs/wiki/04_RF/RF-GPH-007.md"},
		{"nav.context", []map[string]any{{"path": ".docs/raw/ignored.md"}, {"path": ".docs/wiki/04_RF/RF-GPH-007.md"}}, ".docs/wiki/04_RF/RF-GPH-007.md"},
		{"nav.affected", []map[string]any{{"path": ".docs/raw/ignored.md"}, {"path": ".docs/wiki/04_RF/RF-GPH-007.md"}}, ".docs/wiki/04_RF/RF-GPH-007.md"},
		{"nav.diff-context", []map[string]any{{"file": ".docs/wiki/04_RF/RF-GPH-007.md"}}, ".docs/wiki/04_RF/RF-GPH-007.md"},
		{"nav.workspace-map", []map[string]any{{"repos": []string{"repo"}}}, ".docs/wiki/02_arquitectura.md"},
	}
	for _, tc := range cases {
		var digest string
		for iteration := 0; iteration < 30; iteration++ {
			before := model.Envelope{Ok: true, Workspace: root, Backend: "catalog", Items: tc.items}
			itemsBefore, _ := json.Marshal(before.Items)
			backendBefore := before.Backend
			got := app.enrichWikiCodeContext(ctx, model.CommandRequest{Operation: tc.operation, Context: model.QueryOptions{Workspace: root}}, before)
			itemsAfter, _ := json.Marshal(got.Items)
			if string(itemsAfter) != string(itemsBefore) || got.Backend != backendBefore {
				t.Fatalf("%s changed primary envelope: before=%s/%q after=%s/%q", tc.operation, itemsBefore, backendBefore, itemsAfter, got.Backend)
			}
			if got.WikiCodeContext == nil || got.WikiCodeContext.PrimaryDoc.Path != tc.want || len(got.WikiCodeContext.CodeEvidence) == 0 {
				t.Fatalf("%s did not attach stable code evidence: %#v", tc.operation, got.WikiCodeContext)
			}
			if iteration == 0 {
				digest = got.WikiCodeContext.DeterminismDigest
			} else if got.WikiCodeContext.DeterminismDigest != digest {
				t.Fatalf("%s digest changed at iteration %d: got %q want %q", tc.operation, iteration, got.WikiCodeContext.DeterminismDigest, digest)
			}
		}
	}

	env, err := app.Execute(ctx, model.CommandRequest{Operation: "nav.workspace-map", Context: model.QueryOptions{Workspace: root}})
	if err != nil {
		t.Fatalf("Execute workspace-map: %v", err)
	}
	if !env.Ok || env.WikiCodeContext == nil || env.WikiCodeContext.PrimaryDoc.Path != ".docs/wiki/02_arquitectura.md" || len(env.WikiCodeContext.CodeEvidence) == 0 {
		t.Fatalf("workspace-map did not enrich: %#v", env)
	}
}

func TestEnrichWikiCodeContextMatchesDirectBuilderAndRejectsUnavailablePrimaries(t *testing.T) {
	root, app := newWikiCodeEnrichFixture(t)
	ctx := context.Background()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var primary model.DocRecord
	for _, doc := range docs {
		if doc.Path == ".docs/wiki/04_RF/RF-GPH-007.md" {
			primary = doc
			break
		}
	}
	if primary.Path == "" {
		t.Fatalf("primary document not indexed: %#v", docs)
	}
	base := model.Envelope{Ok: true, Backend: "catalog", Items: []map[string]any{{"primary_doc": primary.Path}}}
	direct, err := BuildWikiCodeContext(ctx, db, primary, defaultWikiCodeTokenBudget)
	if err != nil {
		t.Fatal(err)
	}
	enriched := app.enrichWikiCodeContext(ctx, model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: root}}, base)
	if enriched.WikiCodeContext == nil || !reflect.DeepEqual(direct, *enriched.WikiCodeContext) {
		t.Fatalf("enriched context differs from direct context: direct=%#v enriched=%#v", direct, enriched.WikiCodeContext)
	}

	if err := store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, ""); err != nil {
		t.Fatal(err)
	}
	directStale, err := BuildWikiCodeContext(ctx, db, primary, defaultWikiCodeTokenBudget)
	if err != nil {
		t.Fatal(err)
	}
	enrichedStale := app.enrichWikiCodeContext(ctx, model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: root}}, base)
	if enrichedStale.WikiCodeContext == nil || !reflect.DeepEqual(directStale, *enrichedStale.WikiCodeContext) || !hasWikiOmission(enrichedStale.WikiCodeContext, "GPH_WIKI_GRAPH_STALE") {
		t.Fatalf("unavailable graph changed enriched/direct result: direct=%#v enriched=%#v", directStale, enrichedStale.WikiCodeContext)
	}

	rawPrimary := model.DocRecord{Path: ".docs/raw/task.md"}
	if _, err := BuildWikiCodeContext(ctx, db, rawPrimary, defaultWikiCodeTokenBudget); err == nil {
		t.Fatal("direct builder accepted a non-canonical primary")
	} else {
		var contextErr *model.WikiCodeContextError
		if !errors.As(err, &contextErr) || contextErr.Code != "GPH_WIKI_PRIMARY_INVALID" {
			t.Fatalf("direct rejection error=%v, want GPH_WIKI_PRIMARY_INVALID", err)
		}
	}
	raw := app.enrichWikiCodeContext(ctx, model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: root}}, model.Envelope{Ok: true, Backend: "catalog", Items: []map[string]any{{"primary_doc": rawPrimary.Path}}})
	if raw.WikiCodeContext != nil {
		t.Fatalf("enrichment attached a non-canonical primary: %#v", raw.WikiCodeContext)
	}
}

func TestEnrichWikiCodeContextStaleGraphBlockedGovernanceAndBudget(t *testing.T) {
	root, app := newWikiCodeEnrichFixture(t)
	ctx := context.Background()
	request := model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: root}}
	base := model.Envelope{Ok: true, Backend: "catalog", Items: []map[string]any{{"primary_doc": ".docs/wiki/04_RF/RF-GPH-007.md"}}}

	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, ""); err != nil {
		t.Fatal(err)
	}
	db.Close()
	stale := app.enrichWikiCodeContext(ctx, request, base)
	if stale.WikiCodeContext == nil || stale.WikiCodeContext.PrimaryDoc.Path != ".docs/wiki/04_RF/RF-GPH-007.md" || len(stale.WikiCodeContext.CodeEvidence) != 0 || !hasWikiOmission(stale.WikiCodeContext, "GPH_WIKI_GRAPH_STALE") {
		t.Fatalf("stale graph did not preserve docs authority: %#v", stale.WikiCodeContext)
	}

	invalid := app.enrichWikiCodeContext(ctx, model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: root}, Payload: map[string]any{"wiki_code_token_budget": 0}}, base)
	if invalid.WikiCodeContext == nil || !hasWarning(invalid.Warnings, "using default 4000") {
		t.Fatalf("invalid budget did not use default: %#v", invalid)
	}
	clamped := app.enrichWikiCodeContext(ctx, model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: root}, Payload: map[string]any{"wiki_code_token_budget": 999999}}, base)
	if clamped.WikiCodeContext == nil || clamped.WikiCodeContext.TokenBudget != maxWikiCodeTokenBudget || !hasWarning(clamped.Warnings, "capped at 20000") {
		t.Fatalf("clamped budget was not explicit: %#v", clamped)
	}

	blockedRoot := t.TempDir()
	if err := workspace.SaveProjectFile(blockedRoot, model.ProjectFile{Project: model.ProjectBlock{Name: "blocked", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo"}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", Languages: []string{"go"}}}}); err != nil {
		t.Fatal(err)
	}
	blocked := New(blockedRoot, nil).enrichWikiCodeContext(ctx, model.CommandRequest{Operation: "nav.pack", Context: model.QueryOptions{Workspace: blockedRoot}}, base)
	if blocked.WikiCodeContext != nil {
		t.Fatalf("governance-blocked workspace attached code evidence: %#v", blocked.WikiCodeContext)
	}
	if got := app.enrichWikiCodeContext(ctx, request, model.Envelope{Ok: false, Backend: "catalog", Items: base.Items}); got.WikiCodeContext != nil {
		t.Fatal("failed envelope was enriched")
	}
}

func newWikiCodeEnrichFixture(t *testing.T) (string, *App) {
	t.Helper()
	root := t.TempDir()
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "wiki-enrich", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/wiki-enrich", Languages: []string{"go"}}}}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeSpecBackendGovernanceFixture(t, root)
	writeWikiCodeContextFixture(t, root, "go.mod", "module example.com/wiki-enrich\n\ngo 1.23\n")
	writeWikiCodeContextFixture(t, root, "internal/demo/demo.go", "package demo\n\nfunc Value() int { return 1 }\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/02_arquitectura.md", "# Arquitectura\n\nImplementado en `internal/demo/demo.go`.\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/04_RF/RF-GPH-007.md", "# RF-GPH-007\n\nImplementado en `internal/demo/demo.go`.\n")
	if _, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "index-v1"); err != nil {
		t.Fatalf("index fixture: %v", err)
	}
	return root, New(root, nil)
}

func hasWikiOmission(ctx *model.WikiCodeContext, code string) bool {
	for _, omission := range ctx.Omissions {
		if omission.Code == code {
			return true
		}
	}
	return false
}

func hasWarning(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if contains := len(warning) >= len(fragment) && (warning == fragment || stringContains(warning, fragment)); contains {
			return true
		}
	}
	return false
}

func stringContains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func wikiSearchItems(t *testing.T, items any) []map[string]any {
	t.Helper()
	switch results := items.(type) {
	case []map[string]any:
		return results
	case []model.WikiSearchResult:
		mapped := make([]map[string]any, 0, len(results))
		for _, result := range results {
			item := map[string]any{
				"path":          result.Path,
				"doc_id":        result.DocID,
				"title":         result.Title,
				"layer":         result.Layer,
				"family":        result.Family,
				"stage":         result.Stage,
				"line":          result.Line,
				"start_line":    result.StartLine,
				"end_line":      result.EndLine,
				"score":         result.Score,
				"why":           result.Why,
				"evidence":      result.Evidence,
				"snippet":       result.Snippet,
				"content":       result.Content,
				"next_queries":  result.NextQueries,
				"lookup_status": result.LookupStatus,
			}
			mapped = append(mapped, item)
		}
		return mapped
	default:
		t.Fatalf("expected wiki search items, got %T %#v", items, items)
	}
	return nil
}

func wikiSearchString(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func wikiSearchInt(item map[string]any, key string) int {
	switch value := item[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func wikiSearchStrings(item map[string]any, key string) []string {
	switch value := item[key].(type) {
	case []string:
		return value
	case []any:
		items := make([]string, 0, len(value))
		for _, part := range value {
			items = append(items, fmt.Sprint(part))
		}
		return items
	default:
		return nil
	}
}

func prepareWikiSearchOwnerHintFixture(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read governance source: %v", err)
	}
	ownerHints := "owner_hints:\n  - terms:\n      - daemon\n    prefer_doc_ids:\n      - AE-MAINTENANCE\n"
	updated := strings.Replace(string(content), "hierarchy:\n", ownerHints+"hierarchy:\n", 1)
	if updated == string(content) {
		t.Fatal("governance source missing hierarchy marker")
	}
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", updated)
	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("refresh owner-hint governance fixture: %#v", status)
	}
	profile, source, warnings := docgraph.LoadProfile(root)
	if source != "project" || len(profile.OwnerHints) != 1 || len(warnings) != 0 {
		t.Fatalf("owner-hint projection not parsed: source=%q hints=%#v warnings=%#v", source, profile.OwnerHints, warnings)
	}
}

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
	results := wikiSearchItems(t, env.Items)
	if len(results) != 1 {
		t.Fatalf("expected one RF wiki search result, got %#v", env.Items)
	}
	result := results[0]
	if wikiSearchString(result, "layer") != "RF" || wikiSearchString(result, "doc_id") != "RF-AUTH-001" {
		t.Fatalf("unexpected wiki result: %#v", result)
	}
	if !strings.Contains(wikiSearchString(result, "content"), "RF-AUTH-001") {
		t.Fatalf("expected included markdown content, got %q", wikiSearchString(result, "content"))
	}
	if wikiSearchInt(result, "line") == 0 || wikiSearchInt(result, "start_line") == 0 || wikiSearchInt(result, "end_line") == 0 || !strings.Contains(wikiSearchString(result, "evidence"), "RF-AUTH-001") {
		t.Fatalf("expected line evidence fields, got %#v", result)
	}
	joined := strings.Join(wikiSearchStrings(result, "next_queries"), " | ")
	if !strings.Contains(joined, "nav wiki pack") || !strings.Contains(joined, "nav wiki trace RF-AUTH-001") || !strings.Contains(joined, "nav multi-read") {
		t.Fatalf("expected guided next queries, got %#v", result["next_queries"])
	}
}

func TestNavWikiSearchAdmitsTextEvidenceNotFamilyOnlyPrior(t *testing.T) {
	alias := "wiki-search-relevance-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	prepareWikiSearchOwnerHintFixture(t, root)
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
			Path:       ".docs/wiki/ae/AE-MAINTENANCE.md",
			Title:      "Maintenance baseline",
			DocID:      "AE-MAINTENANCE",
			Layer:      "AE",
			Family:     "technical",
			SearchText: "governance operational baseline",
			IndexedAt:  1,
		},
		{
			Path:       ".docs/wiki/04_RF/RF-DAEMON-001.md",
			Title:      "Daemon lifecycle",
			DocID:      "RF-DAEMON-001",
			Layer:      "04",
			Family:     "functional",
			SearchText: "daemon lifecycle restart policy",
			IndexedAt:  1,
		},
		{
			Path:       ".docs/wiki/ae/AE-DAEMON.md",
			Title:      "Maintenance owner",
			DocID:      "AE-DAEMON",
			Layer:      "AE",
			Family:     "technical",
			SearchText: "operational maintenance",
			IndexedAt:  1,
		},
	}, nil, nil); err != nil {
		_ = db.Close()
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "daemon", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	results := wikiSearchItems(t, env.Items)
	if len(results) != 2 {
		t.Fatalf("expected lexical text and doc_id evidence only: %#v", env.Items)
	}
	seen := map[string]bool{}
	for _, result := range results {
		seen[wikiSearchString(result, "doc_id")] = true
	}
	if !seen["RF-DAEMON-001"] || !seen["AE-DAEMON"] || seen["AE-MAINTENANCE"] {
		t.Fatalf("family-only or legitimate doc_id admission mismatch: %#v", env.Items)
	}
}

func TestNavWikiSearchKeepsNoIDKnowledgeDocsByPath(t *testing.T) {
	alias := "wiki-search-knowledge-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module example.com/knowledge-fixture\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/00-gobierno.md", "# 00 - Gobierno\n\nKraal-style knowledge wiki governance.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/01-alcance.md", "# 01 - Alcance\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n\n[[family]]\n  name = \"knowledge\"\n  intent_keywords = [\"knowledge\"]\n  paths = [\"wiki/\"]\n\n[generic_docs]\n  paths = [\"README.md\", \"wiki/\"]\n\n[governance]\n  source_doc = \".docs/wiki/00-gobierno.md\"\n  source_format = \"markdown\"\n  profile = \"knowledge-wiki\"\n  effective_base = \"knowledge-wiki\"\n  context_chain = [\".docs/wiki/00-gobierno.md\", \".docs/wiki/01-alcance.md\"]\n  audit_chain = [\".docs/auditoria/\"]\n")
	firstPath := "wiki/knowledge-a.md"
	secondPath := "wiki/knowledge-b.md"
	writeWorkspaceFile(t, root, firstPath, "# Shared heading\n\nFirst knowledge entry with provenance token.\n")
	writeWorkspaceFile(t, root, secondPath, "# Shared heading\n\nSecond knowledge entry with provenance token.\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{
		{Path: firstPath, Title: "Shared heading", Family: "generic", Layer: "generic", SearchText: "shared heading first knowledge entry", IndexedAt: 1},
		{Path: secondPath, Title: "Shared heading", Family: "generic", Layer: "generic", SearchText: "shared heading second knowledge entry", IndexedAt: 1},
	}, nil, nil); err != nil {
		_ = db.Close()
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "Shared heading", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search knowledge docs: %v", err)
	}
	results := wikiSearchItems(t, env.Items)
	if len(results) != 2 {
		t.Fatalf("expected both same-heading knowledge docs, got %#v", env.Items)
	}
	paths := map[string]bool{}
	for _, result := range results {
		path := wikiSearchString(result, "path")
		paths[path] = true
		if wikiSearchString(result, "doc_id") != "" || wikiSearchInt(result, "line") != 1 || !strings.Contains(wikiSearchString(result, "evidence"), "Shared heading") {
			t.Fatalf("knowledge provenance or no-ID evidence lost: %#v", result)
		}
	}
	if !paths[firstPath] || !paths[secondPath] {
		t.Fatalf("knowledge paths collapsed: %#v", paths)
	}

	noHitEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "totally-absent-term", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search no-hit: %v", err)
	}
	if noHitItems := wikiSearchItems(t, noHitEnv.Items); len(noHitItems) != 0 || noHitEnv.Hint == "" {
		t.Fatalf("natural no-hit admitted unrelated docs: items=%#v hint=%q", noHitEnv.Items, noHitEnv.Hint)
	}

	routeEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.route",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"task": "totally absent route task"},
	})
	if err != nil {
		t.Fatalf("nav.wiki.route unmatched task: %v", err)
	}
	routes, ok := routeEnv.Items.([]model.RouteResult)
	if routeEnv.Backend != "route" || !ok || len(routes) != 1 {
		t.Fatalf("Tier1 route fallback changed by search admission: %#v", routeEnv)
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
	results := wikiSearchItems(t, env.Items)
	if len(results) != 0 {
		t.Fatalf("expected no results with empty doc index, got %#v", results)
	}
}

func TestNavWikiSearchFindsExactSourceRecordID(t *testing.T) {
	alias := "wiki-source-search-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	writeWorkspaceFile(t, root, path, validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", ""))
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
	if err := store.ReplaceDocsWithSources(context.Background(), db,
		[]model.DocRecord{
			sourceDocRecord(path, "CT-SOURCE"),
			{
				Path:       ".docs/wiki/07_tech/AE-MAINTENANCE.md",
				Title:      "Contract maintenance",
				DocID:      "AE-MAINTENANCE",
				Layer:      "AE",
				Family:     "technical",
				SearchText: "contract maintenance",
				IndexedAt:  1,
			},
		},
		nil,
		nil,
		[]model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")},
		[]model.DocSourceRecord{
			sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016"),
			sourceRecord(path, "CT-SOURCE.contract", "SOURCE-ALPHA"),
		},
		nil,
	); err != nil {
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "RF-QRY-016", "top": 5},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	results := wikiSearchItems(t, env.Items)
	if len(results) == 0 {
		t.Fatalf("expected source wiki search result, got %#v", env.Items)
	}
	if wikiSearchString(results[0], "doc_id") != "CT-SOURCE" || !strings.Contains(strings.Join(wikiSearchStrings(results[0], "why"), " | "), "source_id_exact") {
		t.Fatalf("unexpected source result: %#v", results[0])
	}
	status, _ := results[0]["lookup_status"].(*model.WikiLookupStatus)
	if status == nil {
		t.Fatalf("expected lookup status on exact source result")
	}
	if status.MatchKind != "canonical_indexed_id" || status.RecordID != "RF-QRY-016" || status.BlockID != "CT-SOURCE.contract" {
		t.Fatalf("unexpected lookup status: %#v", status)
	}
	if status.TotalMatches == 0 || status.ShownMatches == 0 {
		t.Fatalf("expected match totals in lookup status: %#v", status)
	}
	if wikiSearchInt(results[0], "line") == 0 || wikiSearchString(results[0], "evidence") == "" {
		t.Fatalf("expected line evidence on exact source result, got %#v", results[0])
	}

	dottedEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "CT-SOURCE.contract", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search dotted source id: %v", err)
	}
	dottedResults := wikiSearchItems(t, dottedEnv.Items)
	if len(dottedResults) != 1 || wikiSearchString(dottedResults[0], "doc_id") != "CT-SOURCE" {
		t.Fatalf("dotted source query admitted unrelated contract text: %#v", dottedEnv.Items)
	}
	dottedStatus, _ := dottedResults[0]["lookup_status"].(*model.WikiLookupStatus)
	if dottedStatus == nil || dottedStatus.MatchKind != "canonical_indexed_id" || dottedStatus.BlockID != "CT-SOURCE.contract" || dottedStatus.RecordID != "" {
		t.Fatalf("dotted source lookup status = %#v", dottedStatus)
	}

	nonSoftwareEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "SOURCE-ALPHA", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search non-software source id: %v", err)
	}
	nonSoftwareResults := wikiSearchItems(t, nonSoftwareEnv.Items)
	if len(nonSoftwareResults) != 1 {
		t.Fatalf("non-software source query result = %#v", nonSoftwareEnv.Items)
	}
	nonSoftwareStatus, _ := nonSoftwareResults[0]["lookup_status"].(*model.WikiLookupStatus)
	if nonSoftwareStatus == nil || nonSoftwareStatus.MatchKind != "canonical_indexed_id" || nonSoftwareStatus.RecordID != "SOURCE-ALPHA" {
		t.Fatalf("non-software source lookup status = %#v", nonSoftwareStatus)
	}
}

func TestNavWikiSearchCandidatePaginationPreservesOwnerAndSourceIdentity(t *testing.T) {
	alias := "wiki-pagination-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	ownerPath := ".docs/wiki/04_RF/RF-OWNER-001.md"
	referencePath := ".docs/wiki/04_RF/RF-REFERENCE.md"
	prefixPath := ".docs/wiki/04_RF/RF-OWNER-0010.md"
	sourcePath := ".docs/wiki/09_contratos/CT-SOURCE.md"
	prefixContent := "# RF-OWNER-0010\n\nNeighboring identifier document.\n"
	writeWorkspaceFile(t, root, ownerPath, "# RF-OWNER-001\n\nCanonical owner document.\n")
	writeWorkspaceFile(t, root, referencePath, "# RF-REFERENCE\n\nThis document references RF-OWNER-001 as related context.\n")
	writeWorkspaceFile(t, root, prefixPath, prefixContent)
	writeWorkspaceFile(t, root, sourcePath, validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-OWNER-001", "llm-first", ""))
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
	docs := []model.DocRecord{
		{
			Path:       ownerPath,
			Title:      "RF-OWNER-001",
			DocID:      "RF-OWNER-001",
			Layer:      "04",
			Family:     "functional",
			SearchText: "RF-OWNER-001 canonical owner",
			IndexedAt:  1,
		},
		{
			Path:       referencePath,
			Title:      "RF-REFERENCE",
			DocID:      "RF-REFERENCE",
			Layer:      "04",
			Family:     "functional",
			SearchText: "RF-OWNER-001 related reference",
			IndexedAt:  1,
		},
		{
			Path:        prefixPath,
			Title:       "RF-OWNER-0010",
			DocID:       "RF-OWNER-0010",
			Layer:       "04",
			Family:      "functional",
			SearchText:  "RF-OWNER-001 related from a neighboring identifier",
			ContentHash: bridgeSHA1([]byte(prefixContent)),
			IndexedAt:   1,
		},
		sourceDocRecord(sourcePath, "CT-SOURCE"),
	}
	var extractedMentions []model.DocMention
	for _, path := range []string{ownerPath, referencePath, prefixPath, sourcePath} {
		content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		_, _, mentions, _, _, _, _, parseErr := docgraph.ParseSingleDoc(root, path, content, docgraph.DefaultProfile())
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		extractedMentions = append(extractedMentions, mentions...)
	}
	if err := store.ReplaceWorkspaceDocsWithReferenceSnapshot(context.Background(), db, "", docs, nil, extractedMentions,
		[]model.DocSourceBlock{sourceBlockRecord(sourcePath, "CT-SOURCE", "CT-SOURCE.contract")},
		[]model.DocSourceRecord{sourceRecord(sourcePath, "CT-SOURCE.contract", "RF-OWNER-001")}, nil, model.ReentryMemorySnapshot{}); err != nil {
		_ = db.Close()
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	search := func(offset int) model.Envelope {
		t.Helper()
		env, err := app.Execute(context.Background(), model.CommandRequest{
			Operation: "nav.wiki.search",
			Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
			Payload:   map[string]any{"query": "RF-OWNER-001", "top": 1, "offset": offset},
		})
		if err != nil {
			t.Fatalf("nav.wiki.search offset=%d: %v", offset, err)
		}
		return env
	}

	ownerEnv := search(0)
	ownerItems := wikiSearchItems(t, ownerEnv.Items)
	if len(ownerItems) != 1 || wikiSearchString(ownerItems[0], "doc_id") != "RF-OWNER-001" {
		t.Fatalf("owner page = %#v", ownerEnv.Items)
	}
	ownerStatus, _ := ownerItems[0]["lookup_status"].(*model.WikiLookupStatus)
	if ownerStatus == nil || ownerStatus.MatchKind != "canonical_indexed_id" || ownerStatus.TotalMatches != 3 || ownerStatus.ShownMatches != 1 {
		t.Fatalf("owner lookup status = %#v", ownerStatus)
	}

	sourceEnv := search(1)
	sourceItems := wikiSearchItems(t, sourceEnv.Items)
	if len(sourceItems) != 1 || wikiSearchString(sourceItems[0], "doc_id") != "CT-SOURCE" {
		t.Fatalf("source page = %#v", sourceEnv.Items)
	}
	sourceStatus, _ := sourceItems[0]["lookup_status"].(*model.WikiLookupStatus)
	if sourceStatus == nil || sourceStatus.RecordID != "RF-OWNER-001" || sourceStatus.BlockID != "CT-SOURCE.contract" || sourceStatus.TotalMatches != 3 || sourceStatus.ShownMatches != 1 {
		t.Fatalf("source lookup status = %#v", sourceStatus)
	}
	if !containsString(wikiSearchStrings(sourceItems[0], "why"), "source_id_exact") {
		t.Fatalf("source page lost exact source reason: %#v", sourceItems[0])
	}

	allEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "RF-OWNER-001", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search complete-ID evidence: %v", err)
	}
	allItems := wikiSearchItems(t, allEnv.Items)
	foundReference := false
	foundPrefixReference := false
	for _, item := range allItems {
		path := wikiSearchString(item, "path")
		if path != referencePath && path != prefixPath {
			continue
		}
		if path == referencePath {
			foundReference = true
		} else {
			foundPrefixReference = true
		}
		status, _ := item["lookup_status"].(*model.WikiLookupStatus)
		if status == nil || status.MatchKind == "canonical_indexed_id" || containsString(wikiSearchStrings(item, "why"), "doc_id=RF-OWNER-0010") {
			t.Fatalf("reference incorrectly canonicalized: %#v", item)
		}
	}
	if !foundReference || foundPrefixReference {
		t.Fatalf("complete-ID search admitted incomplete reference evidence: %#v", allEnv.Items)
	}

	unknownEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "RF-OWNER-00100", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search unknown ID: %v", err)
	}
	if unknownItems := wikiSearchItems(t, unknownEnv.Items); len(unknownItems) != 0 || unknownEnv.Hint == "" {
		t.Fatalf("unknown/prefix ID was admitted: items=%#v hint=%q", unknownEnv.Items, unknownEnv.Hint)
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

func TestAssembleWikiSearchCandidatesPrioritizesOwnerAndDeduplicatesSource(t *testing.T) {
	query := "RF-OWNER-001"
	exactDocs := []model.DocRecord{
		{Path: ".docs/wiki/04_RF/RF-OWNER-001.md", DocID: query, Layer: "04"},
		{Path: ".docs/wiki/09_contratos/CT-SOURCE.md", DocID: "CT-SOURCE", Layer: "09"},
	}
	ranked := []scoredDoc{
		{record: exactDocs[0], score: 80, reason: []string{"doc_id=RF-OWNER-001"}},
		{record: exactDocs[1], score: 60, reason: []string{"search_overlap"}},
		// Reference candidates reach the assembler after upstream literal validation.
		{record: model.DocRecord{Path: ".docs/wiki/04_RF/RF-REFERENCE.md", DocID: "RF-REFERENCE", Layer: "04", SearchText: query}, score: 70, reason: []string{"search_overlap", "mention_exact"}},
		// Normalized search text alone must not establish a literal reference.
		{record: model.DocRecord{Path: ".docs/wiki/04_RF/RF-UNVERIFIED.md", DocID: "RF-UNVERIFIED", Layer: "04", SearchText: query}, score: 90, reason: []string{"search_overlap"}},
	}

	got := assembleWikiSearchCandidates(query, exactDocs, ranked, nil)
	if len(got) != 3 {
		t.Fatalf("candidate count = %d, want 3: %#v", len(got), got)
	}
	if got[0].record.DocID != query {
		t.Fatalf("explicit owner displaced: %#v", got)
	}
	if got[1].record.DocID != "CT-SOURCE" || !containsString(got[1].reason, "source_id_exact") {
		t.Fatalf("source declaration missing or not marked exact: %#v", got[1])
	}
	if got[2].record.DocID != "RF-REFERENCE" {
		t.Fatalf("ranked reference ordering changed: %#v", got)
	}
}

func TestAssembleWikiSearchCandidatesPrioritizesSourceOnlyOwner(t *testing.T) {
	query := "RF-SOURCE-OWNER"
	exactDocs := []model.DocRecord{
		{Path: "source-owner.md", DocID: query, Layer: "04"},
		{Path: "other-source.md", DocID: "CT-SOURCE", Layer: "09"},
	}
	got := assembleWikiSearchCandidates(query, exactDocs, []scoredDoc{
		{record: exactDocs[1], score: 20, reason: []string{"search_overlap"}},
	}, nil)
	if len(got) != 2 || got[0].record.Path != "source-owner.md" || got[1].record.Path != "other-source.md" {
		t.Fatalf("source-only owner ordering = %#v", got)
	}
}

func TestWikiSearchCandidatePageAppliesOffsetAndLimitOnce(t *testing.T) {
	exactDocs := []model.DocRecord{
		{Path: "source-a.md", DocID: "SOURCE-A"},
		{Path: "source-b.md", DocID: "SOURCE-B"},
		{Path: "source-c.md", DocID: "SOURCE-C"},
	}
	got := assembleWikiSearchCandidates("record-1", exactDocs, []scoredDoc{
		{record: exactDocs[0], score: 3},
		// Pagination consumes the already validated reference candidate.
		{record: model.DocRecord{Path: "ranked-d.md", DocID: "RANKED-D", SearchText: "record-1"}, score: 2, reason: []string{"search_overlap", "mention_exact"}},
	}, nil)
	if len(got) != 4 {
		t.Fatalf("eligible count = %d, want 4: %#v", len(got), got)
	}
	page := wikiSearchCandidatePage(got, 1, 1)
	if len(page) != 1 || page[0].record.Path != "source-b.md" {
		t.Fatalf("offset/limit page = %#v, want source-b.md only", page)
	}
	if page := wikiSearchCandidatePage(got, 0, 1); len(page) != 1 {
		t.Fatalf("top=1 overflowed page: %#v", page)
	}
	if page := wikiSearchCandidatePage(got, 99, 1); len(page) != 0 {
		t.Fatalf("out-of-range offset page = %#v, want empty", page)
	}
	maxInt := int(^uint(0) >> 1)
	if page := wikiSearchCandidatePage(got, 1, maxInt); len(page) != 3 {
		t.Fatalf("extreme top page = %#v, want remaining candidates without overflow", page)
	}
}

func TestAssembleWikiSearchCandidatesWithoutIDPreservesRankedOrder(t *testing.T) {
	ranked := []scoredDoc{
		{record: model.DocRecord{Path: "first.md", Title: "First", SearchText: "normal"}, score: 10, reason: []string{"title_overlap"}},
		{record: model.DocRecord{Path: "second.md", Title: "Second", SearchText: "query"}, score: 9, reason: []string{"search_overlap"}},
	}
	got := assembleWikiSearchCandidates("normal query", nil, ranked, nil)
	if len(got) != 2 || got[0].record.Path != "first.md" || got[1].record.Path != "second.md" {
		t.Fatalf("no-ID ranked order = %#v, want existing order", got)
	}
}

func TestWikiSearchIdentifierMatcherPreservesLiteralBoundaries(t *testing.T) {
	matcher := newWikiSearchIdentifierMatcher("RF-X-1")
	for _, test := range []struct {
		name string
		text string
		want bool
	}{
		{name: "sentence punctuation", text: "Ver RF-X-1.", want: true},
		{name: "comma", text: "RF-X-1,", want: true},
		{name: "colon", text: "RF-X-1:", want: true},
		{name: "semicolon", text: "RF-X-1;", want: true},
		{name: "closing parenthesis", text: "RF-X-1)", want: true},
		{name: "closing bracket", text: "RF-X-1]", want: true},
		{name: "wikilink", text: "[[RF-X-1]]", want: true},
		{name: "markdown link", text: "[owner](RF-X-1.md)", want: true},
		{name: "id markdown extension continuation", text: "RF-X-1.md.EXTRA", want: false},
		{name: "hyphen continuation", text: "RF-X-1-EXTRA", want: false},
		{name: "underscore continuation", text: "RF-X-1_EXTRA", want: false},
		{name: "dot continuation", text: "RF-X-1.extra", want: false},
		{name: "numeric prefix collision", text: "RF-X-10", want: false},
		{name: "preceding prefix", text: "XRF-X-1", want: false},
		{name: "preceding unicode letter", text: "éRF-X-1", want: false},
		{name: "following unicode letter", text: "RF-X-1é", want: false},
		{name: "separated words", text: "RF X 1", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := matcher.matches(test.text); got != test.want {
				t.Fatalf("matches(%q)=%v, want %v", test.text, got, test.want)
			}
		})
	}
}

func TestWikiSearchIdentifierMatcherPreservesUnicodeIdentity(t *testing.T) {
	matcher := newWikiSearchIdentifierMatcher("ID-ñ")
	if !matcher.matches("ID-ñ") {
		t.Fatal("exact Unicode identifier must match")
	}
	if matcher.matches("ID-n") {
		t.Fatal("literal identifier matching must not fold diacritics")
	}
}

func TestWikiSearchNaturalEvidenceFoldsDiacriticsAndPrefersBody(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md"
	bodyLine := "- Result cache LRU (256 entradas, TTL 10 min): cachea read-only ops del daemon."
	content := strings.Join([]string{
		"---",
		"doc_id: TECH-DAEMON-GOBERNANZA",
		"title: Daemon, gobernanza local y runtime compartido",
		"layer: TECH",
		"---",
		"",
		"# TECH-DAEMON-GOBERNANZA",
		"",
		"```yaml",
		"harness_protocol: SDD-HARNESS-v1",
		"id: \"TECH-DAEMON-GOBERNANZA\"",
		"imports:",
		"  - '[[00_gobierno_documental]]'",
		"  - '[[TECH-DAEMON-GOBERNANZA]]'",
		"```",
		"",
		"Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).",
		"",
		"### Acceso compartido y caching",
		"",
		bodyLine,
		"",
		"```toon",
		"block_id: TECH-DAEMON-GOBERNANZA.result-cache-v1",
		"enabled: true",
		"```",
	}, "\n") + "\n"
	writeWorkspaceFile(t, root, path, content)
	bodyLineNo := strings.Count(content[:strings.Index(content, bodyLine)], "\n") + 1

	doc := model.DocRecord{
		Path:       path,
		Title:      "Daemon, gobernanza local y runtime compartido",
		DocID:      "TECH-DAEMON-GOBERNANZA",
		Snippet:    bodyLine,
		SearchText: content,
	}
	for _, query := range []string{"caché daemon", "cache daemon"} {
		t.Run(query, func(t *testing.T) {
			evidence, ok := wikiSearchLineEvidence(root, path, query)
			if !ok {
				t.Fatalf("natural evidence unavailable for %q", query)
			}
			if evidence.line != bodyLineNo || evidence.startLine <= 5 || !strings.Contains(evidence.text, bodyLine) {
				t.Fatalf("natural evidence for %q = %#v, want body line %d", query, evidence, bodyLineNo)
			}
			if strings.Contains(evidence.text, "doc_id:") || strings.Contains(evidence.text, "harness_protocol:") || strings.Contains(evidence.text, "imports:") {
				t.Fatalf("metadata leaked into preferred evidence for %q: %#v", query, evidence)
			}
			score, reasons := wikiSearchNaturalEvidenceScore(doc, docgraph.QuestionTokens(query), query)
			if score <= 0 || !containsString(reasons, "natural_coverage=2/2") {
				t.Fatalf("natural score for %q = %d, %v", query, score, reasons)
			}
		})
	}

	metadataPath := ".docs/wiki/07_tech/TECH-METADATA-ONLY.md"
	writeWorkspaceFile(t, root, metadataPath, "---\ndoc_id: TECH-METADATA-ONLY\ntitle: Daemon metadata only\nlayer: TECH\n---\n\n# Unrelated heading\n\nNo matching body passage.\n")
	fallback, ok := wikiSearchLineEvidence(root, metadataPath, "daemon")
	if !ok || fallback.line != 3 || !strings.Contains(fallback.text, "title: Daemon metadata only") {
		t.Fatalf("metadata-only fallback = %#v, ok=%v", fallback, ok)
	}
}

func TestAssembleWikiSearchCandidatesExactIDRequiresCompleteReference(t *testing.T) {
	query := "RF-X-1"
	ranked := []scoredDoc{
		{record: model.DocRecord{Path: "owner.md", DocID: query}, score: 90, reason: []string{"doc_id=" + query}},
		{record: model.DocRecord{Path: "reference.md", DocID: "RF-REFERENCE", SearchText: "related [[RF-X-1]] and [owner](RF-X-1.md) context"}, score: 20, reason: []string{"mention_exact"}},
		{record: model.DocRecord{Path: "prefix.md", DocID: "RF-X-10", SearchText: "related RF-X-10 context"}, score: 80, reason: []string{"doc_id=RF-X-10", "search_overlap"}},
		{record: model.DocRecord{Path: "suffix.md", DocID: "RF-SUFFIX", SearchText: "related RF-X-1-EXTRA context"}, score: 70, reason: []string{"search_overlap"}},
		{record: model.DocRecord{Path: "underscore.md", DocID: "RF-UNDERSCORE", SearchText: "related RF-X-1_EXTRA context"}, score: 60, reason: []string{"search_overlap"}},
		{record: model.DocRecord{Path: "dotted.md", DocID: "RF-DOTTED", SearchText: "related RF-X-1.extra context"}, score: 50, reason: []string{"search_overlap"}},
		{record: model.DocRecord{Path: "extension.md", DocID: "RF-EXTENSION", SearchText: "related RF-X-1.md.md context"}, score: 45, reason: []string{"search_overlap"}},
		{record: model.DocRecord{Path: "phrase.md", DocID: "RF-PHRASE", SearchText: "related RF X 1 context"}, score: 40, reason: []string{"search_overlap"}},
	}

	got := assembleWikiSearchCandidates(query, nil, ranked, nil)
	if len(got) != 2 || got[0].record.DocID != query || got[1].record.DocID != "RF-REFERENCE" {
		t.Fatalf("exact-ID admission = %#v", got)
	}
	if containsString(got[1].reason, "doc_id=RF-X-10") {
		t.Fatalf("partial doc ID reason leaked into search result: %#v", got[1])
	}
}

func TestAssembleWikiSearchCandidatesAdmitsDeclaredNonHyphenIDs(t *testing.T) {
	for _, query := range []string{"DOC_UNDERSCORE", "DOC.dotted", "ID-ñ"} {
		t.Run(query, func(t *testing.T) {
			got := assembleWikiSearchCandidates(query, nil, []scoredDoc{{
				record: model.DocRecord{Path: "declared.md", DocID: query},
				score:  60,
				reason: []string{"doc_id=" + query},
			}}, nil)
			if len(got) != 1 || !strings.EqualFold(got[0].record.DocID, query) {
				t.Fatalf("declared non-hyphen ID was not admitted: %#v", got)
			}
		})
	}
}

func TestAssembleWikiSearchCandidatesNaturalQueryRejectsRoutingOnlyCandidates(t *testing.T) {
	ranked := []scoredDoc{
		{record: model.DocRecord{Path: "hint-only.md", DocID: "AE-MAINTENANCE", Title: "Maintenance baseline", SearchText: "governance operational baseline"}, score: 90, reason: []string{"family=technical", "owner_hint=AE-MAINTENANCE"}},
		{record: model.DocRecord{Path: "doc-id.md", DocID: "AE-DAEMON", Title: "Maintenance owner", SearchText: "operational maintenance"}, score: 30, reason: []string{"family=technical"}},
		{record: model.DocRecord{Path: "text.md", DocID: "RF-TEXT", SearchText: "daemon lifecycle"}, score: 20, reason: []string{"family=technical", "search_overlap"}},
	}

	got := assembleWikiSearchCandidates("daemon", nil, ranked, nil)
	if len(got) != 2 || got[0].record.Path != "doc-id.md" || got[1].record.Path != "text.md" {
		t.Fatalf("natural-query admission = %#v", got)
	}
}

func TestNavWikiSearchNaturalRankingPrefersCoverageAndUsefulEvidence(t *testing.T) {
	alias := "wiki-search-natural-ranking-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	prepareWikiSearchOwnerHintFixture(t, root)
	partialPath := ".docs/wiki/ae/AE-MAINTENANCE.md"
	fullPath := ".docs/wiki/04_RF/RF-DAEMON-LIFECYCLE.md"
	accentPath := ".docs/wiki/04_RF/RF-AUTENTICACION.md"
	writeWorkspaceFile(t, root, partialPath, "# Daemon maintenance\n\nMaintenance notes for the daemon.\n")
	writeWorkspaceFile(t, root, fullPath, "---\ndoc_id: RF-DAEMON-LIFECYCLE\nimports: []\n---\n# Daemon lifecycle\n\nThe daemon lifecycle documents restart policy and recovery.\n")
	writeWorkspaceFile(t, root, accentPath, "# Autenticación\n\nLa autenticación requiere evidencia de identidad.\n")
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
		{Path: partialPath, Title: "Daemon maintenance", DocID: "AE-MAINTENANCE", Layer: "AE", Family: "technical", Snippet: "Maintenance notes for the daemon.", SearchText: "daemon maintenance", IndexedAt: 1},
		{Path: fullPath, Title: "Daemon lifecycle", DocID: "RF-DAEMON-LIFECYCLE", Layer: "04", Family: "functional", Snippet: "The daemon lifecycle documents restart policy and recovery.", SearchText: "daemon lifecycle restart policy recovery", IndexedAt: 1},
		{Path: accentPath, Title: "Autenticación", DocID: "RF-AUTENTICACION", Layer: "04", Family: "functional", Snippet: "La autenticación requiere evidencia de identidad.", SearchText: "autenticación evidencia identidad", IndexedAt: 1},
	}, nil, nil); err != nil {
		_ = db.Close()
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "daemon lifecycle", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	results := wikiSearchItems(t, env.Items)
	if len(results) != 2 || wikiSearchString(results[0], "path") != fullPath {
		t.Fatalf("natural evidence order = %#v", env.Items)
	}
	if wikiSearchInt(results[0], "score") <= wikiSearchInt(results[1], "score") {
		t.Fatalf("full query evidence did not produce a coherent score order: %#v", env.Items)
	}
	if !strings.Contains(wikiSearchString(results[0], "evidence"), "daemon lifecycle") ||
		strings.Contains(wikiSearchString(results[0], "evidence"), "doc_id:") ||
		wikiSearchInt(results[0], "start_line") >= wikiSearchInt(results[0], "end_line") {
		t.Fatalf("useful source excerpt/range missing: %#v", results[0])
	}
	if !containsString(wikiSearchStrings(results[0], "why"), "natural_coverage=2/2") {
		t.Fatalf("natural coverage reason missing: %#v", results[0])
	}

	accentEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "autenticación", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search diacritics: %v", err)
	}
	accentResults := wikiSearchItems(t, accentEnv.Items)
	if len(accentResults) != 1 || wikiSearchString(accentResults[0], "path") != accentPath ||
		wikiSearchInt(accentResults[0], "line") != 3 || wikiSearchInt(accentResults[0], "start_line") != 2 ||
		wikiSearchInt(accentResults[0], "end_line") != 4 ||
		!strings.Contains(wikiSearchString(accentResults[0], "evidence"), "La autenticación requiere evidencia de identidad.") {
		t.Fatalf("diacritic evidence mismatch: %#v", accentResults)
	}

	if err := os.Remove(filepath.Join(root, filepath.FromSlash(fullPath))); err != nil {
		t.Fatalf("remove stale source: %v", err)
	}
	staleEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "daemon lifecycle", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search stale source: %v", err)
	}
	staleResults := wikiSearchItems(t, staleEnv.Items)
	if len(staleResults) != 2 || wikiSearchInt(staleResults[0], "line") != 0 || wikiSearchString(staleResults[0], "evidence") != "" || wikiSearchString(staleResults[0], "snippet") == "" || !containsString(wikiSearchStrings(staleResults[0], "why"), "source_evidence_unavailable=indexed_snippet_fallback") {
		t.Fatalf("stale source fallback was not transparent: %#v", staleResults)
	}
}

func TestAssembleWikiSearchCandidatesKeepsAmbiguousOwnersVisible(t *testing.T) {
	query := "RF-DUPLICATE"
	ranked := []scoredDoc{
		{record: model.DocRecord{Path: "a.md", DocID: query}, score: 10},
		{record: model.DocRecord{Path: "b.md", DocID: query}, score: 9},
	}
	got := assembleWikiSearchCandidates(query, nil, ranked, nil)
	if len(got) != 2 || got[0].record.Path != "a.md" || got[1].record.Path != "b.md" {
		t.Fatalf("ambiguous owner candidates = %#v, want both paths without collapsing", got)
	}
	warnings := wikiSearchAmbiguityWarnings(got)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "a.md") || !strings.Contains(warnings[0], "b.md") {
		t.Fatalf("ambiguous owner warning = %#v", warnings)
	}
}

func TestAssembleWikiSearchCandidatesAppliesLayerFilterBeforePagination(t *testing.T) {
	ranked := []scoredDoc{
		{record: model.DocRecord{Path: "rf.md", DocID: "RF-1", Layer: "04", SearchText: "normal"}, score: 10, reason: []string{"search_overlap"}},
		{record: model.DocRecord{Path: "tech.md", DocID: "TECH-1", Layer: "07", SearchText: "normal"}, score: 9, reason: []string{"search_overlap"}},
		{record: model.DocRecord{Path: "rf-2.md", DocID: "RF-2", Layer: "04", SearchText: "normal"}, score: 8, reason: []string{"search_overlap"}},
	}
	got := assembleWikiSearchCandidates("normal query", nil, ranked, map[string]struct{}{"RF": {}})
	if len(got) != 2 || got[0].record.DocID != "RF-1" || got[1].record.DocID != "RF-2" {
		t.Fatalf("filtered candidates = %#v, want both RF docs in ranked order", got)
	}
	page := wikiSearchCandidatePage(got, 1, 1)
	if len(page) != 1 || page[0].record.DocID != "RF-2" {
		t.Fatalf("filtered page = %#v, want RF-2", page)
	}
}

func TestAppExecuteWikiSearchUsesFreshDocgraphOwnerAndImporterReference(t *testing.T) {
	alias := "wiki-identity-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	ownerPath := ".docs/wiki/09_contratos/CT-OWNER-INTEGRATION-001.md"
	importerPath := ".docs/wiki/09_contratos/CT-IMPORTER-INTEGRATION-001.md"
	writeWorkspaceFile(t, root, ownerPath, "---\ndoc_id: CT-OWNER-INTEGRATION-001\n---\n# Owner\n\nCanonical owner.\n")
	writeWorkspaceFile(t, root, importerPath, "# Importer\n\n```yaml\nharness_protocol: SDD-HARNESS-v1\nid: CT-IMPORTER-INTEGRATION-001\nimports:\n  - '[[CT-OWNER-INTEGRATION-001]]'\n```\n")
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
	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		db.Close()
		t.Fatalf("LoadIgnoreMatcher: %v", err)
	}
	docs, edges, mentions, blocks, records, bindings, _, err := docgraph.IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(context.Background(), root, matcher, nil, nil)
	if err != nil {
		db.Close()
		t.Fatalf("fresh docgraph extraction: %v", err)
	}
	if err := store.ReplaceWorkspaceDocsWithReferenceSnapshot(context.Background(), db, "", docs, edges, mentions, blocks, records, bindings, model.ReentryMemorySnapshot{}); err != nil {
		db.Close()
		t.Fatalf("publish fresh docgraph: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "CT-OWNER-INTEGRATION-001", "top": 10},
	})
	if err != nil {
		t.Fatalf("nav.wiki.search: %v", err)
	}
	results := wikiSearchItems(t, env.Items)
	ownerCount, importerCount := 0, 0
	for _, result := range results {
		switch wikiSearchString(result, "path") {
		case ownerPath:
			ownerCount++
			if wikiSearchString(result, "doc_id") != "CT-OWNER-INTEGRATION-001" {
				t.Fatalf("owner result has wrong DocID: %#v", result)
			}
		case importerPath:
			importerCount++
			if wikiSearchString(result, "doc_id") == "CT-OWNER-INTEGRATION-001" {
				t.Fatalf("importer became canonical owner: %#v", result)
			}
		}
	}
	if ownerCount != 1 || importerCount != 1 {
		t.Fatalf("owner/importer results = %d/%d, results=%#v", ownerCount, importerCount, results)
	}
}

func TestWikiSearchCanonicalReadsAllowDottedNamesAndRejectSymlinkEscapes(t *testing.T) {
	root := t.TempDir()
	relPath := "docs/v1..draft.md"
	content := "# Dotted source\n\nCanonical dotted evidence.\n"
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relPath)), []byte(content), 0o644); err != nil {
		t.Fatalf("write dotted source: %v", err)
	}
	if evidence, ok := wikiSearchLineEvidence(root, relPath, "canonical dotted"); !ok || !strings.Contains(evidence.text, "Canonical dotted evidence") {
		t.Fatalf("dotted source evidence = %#v, want canonical content", evidence)
	}
	if got := readWikiSearchContent(root, relPath, 0); got != content {
		t.Fatalf("dotted source content = %q, want %q", got, content)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside workspace"), 0o644); err != nil {
		t.Fatalf("write outside source: %v", err)
	}
	linkPath := filepath.Join(root, "docs", "escape.md")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, ok := wikiSearchLineEvidence(root, "docs/escape.md", "outside"); ok {
		t.Fatalf("symlink escape returned source evidence")
	}
	if got := readWikiSearchContent(root, "docs/escape.md", 0); got != "" {
		t.Fatalf("symlink escape returned content %q", got)
	}
}

func TestWikiSearchLegacySourceWarningsExposeFailureCauses(t *testing.T) {
	warnings := wikiSearchLegacySourceWarnings(map[wikiSearchSourceFailure]int{
		wikiSearchSourceFailureInvalidPath:   1,
		wikiSearchSourceFailureMissing:       2,
		wikiSearchSourceFailureOversize:      3,
		wikiSearchSourceFailureStaleHash:     4,
		wikiSearchSourceFailureMissingHash:   5,
		wikiSearchSourceFailureSymlinkEscape: 6,
	})
	if len(warnings) != 1 {
		t.Fatalf("legacy source warnings = %#v, want one aggregate warning", warnings)
	}
	for _, expected := range []string{"invalid_path=1", "missing_source=2", "oversize_source=3", "stale_hash=4", "missing_hash=5", "symlink_escape=6"} {
		if !strings.Contains(warnings[0], expected) {
			t.Fatalf("legacy source warning %q missing %q", warnings[0], expected)
		}
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

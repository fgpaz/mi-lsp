package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/livecontext"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const (
	wikiCodeVerticalRFPath        = ".docs/wiki/04_RF/RF-DEMO-001.md"
	wikiCodeVerticalServicePath   = "src/demo/service.mjs"
	wikiCodeVerticalServiceSymbol = "runDemo"
	wikiCodeVerticalTestPath      = "test/demo/service.test.mjs"
)

var wikiCodeVerticalAcceptanceCaseInventory = []string{
	"full_index_fixture_baseline", "reverse_lookup", "supporting_only", "graph_stale", "edit_binding_overlay",
	"remove_binding_tombstone", "add_binding_reverse", "delete_rename_target", "fail_closed_inputs", "raw_audit_decoys",
	"unmapped_changed_code", "modern_js_extensions", "lost_watcher_event", "direct_daemon_parity", "no_query_writes",
	"same_tick_rewrite", "unknown_state_omission", "stable_digest_30", "cost_counters", "graph_v1_alias_normalization",
	"planned_binding", "retired_exclusion_redirect", "duplicate_doc_ids", "active_retired_transition", "governance_imports_self_export",
	"graph_v1_phase_ordering", "legacy_advisory", "mjs_find_related", "wiki_to_code_direct_precision", "code_to_wiki_reverse_recall",
	"false_direct_implementation_edges", "raw_audit_primary_results", "warm_mixed_neighbors_p95", "warm_direct_binding_lookup_p95",
	"stable_digest_runs", "dirty_single_file_overlay_target", "latency_campaign_complete",
}

func TestWikiCodeVerticalAcceptanceCaseInventoryIsClosed(t *testing.T) {
	if len(wikiCodeVerticalAcceptanceCaseInventory) != 37 {
		t.Fatalf("acceptance case inventory length=%d, want 37", len(wikiCodeVerticalAcceptanceCaseInventory))
	}
	seen := make(map[string]struct{}, len(wikiCodeVerticalAcceptanceCaseInventory))
	for _, id := range wikiCodeVerticalAcceptanceCaseInventory {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate acceptance case %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestWikiCodeVerticalFixtureBaselineReverseAndModernJavaScript(t *testing.T) {
	root, app := newWikiCodeVerticalFixtureWithGovernedSource(t)
	ctx := context.Background()

	forward := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{
		Kind:     model.ScopeExactWiki,
		DocPaths: []string{wikiCodeVerticalRFPath},
	})
	if forward.PrimaryDoc.Path != wikiCodeVerticalRFPath {
		t.Fatalf("primary document=%q, want %q", forward.PrimaryDoc.Path, wikiCodeVerticalRFPath)
	}
	implementation := wikiCodeVerticalImplementations(forward.DirectCode)
	if len(implementation) != 1 || implementation[0].Path != wikiCodeVerticalServicePath || implementation[0].Symbol != wikiCodeVerticalServiceSymbol || implementation[0].Status != model.WikiCodeStatusResolvedSymbol {
		t.Fatalf("implementation direct_code=%#v, want the resolved service symbol only", forward.DirectCode)
	}
	if len(forward.Tests) != 1 || forward.Tests[0].Path != wikiCodeVerticalTestPath || forward.Tests[0].Relation != model.RelationTests {
		t.Fatalf("tests=%#v, want the fixture test binding", forward.Tests)
	}
	if containsWikiCodePath(forward.DirectCode, "src/demo/helper.mjs") || containsWikiCodePath(forward.DirectCode, ".docs/raw/decoy.md") || containsWikiCodePath(forward.DirectCode, ".docs/auditoria/decoy.md") {
		t.Fatalf("supporting/raw/audit evidence was promoted to direct_code=%#v", forward.DirectCode)
	}
	if forward.Cost.MetadataChecked < 0 || forward.Cost.FilesHashed < 0 || forward.Cost.FilesParsed < 0 || forward.Cost.CatalogQueries < 0 || forward.Cost.GraphNodesVisited < 0 {
		t.Fatalf("negative cost counters=%#v", forward.Cost)
	}
	if forward.Provenance.QueryOnly != true {
		t.Fatalf("query-only provenance=%#v", forward.Provenance)
	}
	traceEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.wiki.trace",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 32, TokenBudget: 20_000},
		Payload:   map[string]any{"rf": "RF-DEMO-001"},
	})
	if err != nil || traceEnv.WikiCodeContext == nil || len(wikiCodeVerticalImplementations(traceEnv.WikiCodeContext.DirectCode)) != 1 {
		t.Fatalf("integrated nav.wiki.trace context=%#v err=%v envelope=%s", traceEnv.WikiCodeContext, err, wikiCodeVerticalEnvelopeDiagnostics(traceEnv))
	}

	routeEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 32, TokenBudget: 20_000},
		Payload:   map[string]any{"task": "RF-DEMO-001"},
	})
	if err != nil || routeEnv.WikiCodeContext == nil || len(wikiCodeVerticalImplementations(routeEnv.WikiCodeContext.DirectCode)) != 1 {
		t.Fatalf("integrated nav.route context=%#v err=%v envelope=%s", routeEnv.WikiCodeContext, err, wikiCodeVerticalEnvelopeDiagnostics(routeEnv))
	}

	askEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 32, TokenBudget: 20_000},
		Payload:   map[string]any{"question": "RF-DEMO-001"},
	})
	if err != nil || askEnv.WikiCodeContext == nil || len(wikiCodeVerticalImplementations(askEnv.WikiCodeContext.DirectCode)) != 1 {
		t.Fatalf("integrated nav.ask context=%#v err=%v envelope=%s", askEnv.WikiCodeContext, err, wikiCodeVerticalEnvelopeDiagnostics(askEnv))
	}

	contextEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.context",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 32, TokenBudget: 20_000, BackendHint: "catalog"},
		Payload:   map[string]any{"file": wikiCodeVerticalServicePath, "line": 1},
	})
	if err != nil || contextEnv.WikiCodeContext == nil || !containsWikiCodeDocID(contextEnv.WikiCodeContext.WikiContext, "RF-DEMO-001") {
		t.Fatalf("integrated nav.context reverse owner=%#v err=%v envelope=%s", contextEnv.WikiCodeContext, err, wikiCodeVerticalEnvelopeDiagnostics(contextEnv))
	}
	contextItems, ok := contextEnv.Items.([]map[string]any)
	if !ok || len(contextItems) != 1 || contextItems[0]["qualified_name"] != wikiCodeVerticalServicePath+"::"+wikiCodeVerticalServiceSymbol {
		t.Fatalf("integrated nav.context catalog identity=%#v, want %s::%s", contextEnv.Items, wikiCodeVerticalServicePath, wikiCodeVerticalServiceSymbol)
	}

	db, err := store.OpenReadOnlyExisting(root, store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bindings, err := store.ListDocArtifactBindings(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRFBindings := 0
	for _, binding := range bindings {
		if binding.DocID == "RF-DEMO-001" {
			canonicalRFBindings++
			if binding.AuthoringOrigin != model.AuthoringOriginCanonical {
				t.Fatalf("RF-DEMO-001 binding origin=%q, want canonical: %#v", binding.AuthoringOrigin, binding)
			}
		}
	}
	if canonicalRFBindings != 3 {
		t.Fatalf("RF-DEMO-001 binding count=%d, want 3", canonicalRFBindings)
	}
	var fileLanguage string
	if err := db.QueryRowContext(ctx, "SELECT language FROM files WHERE file_path = ?", wikiCodeVerticalServicePath).Scan(&fileLanguage); err != nil {
		t.Fatal(err)
	}
	if fileLanguage != "javascript" {
		t.Fatalf("service.mjs catalog language=%q, want javascript", fileLanguage)
	}

	reverse := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{
		Kind:         model.ScopeReverseCode,
		TargetPath:   wikiCodeVerticalServicePath,
		TargetSymbol: wikiCodeVerticalServiceSymbol,
	})
	var rf *model.WikiCodeWikiContextItem
	for index := range reverse.WikiContext {
		item := &reverse.WikiContext[index]
		if item.DocID == "RF-DEMO-001" && item.Status != "redirect" {
			rf = item
			break
		}
	}
	if rf == nil {
		t.Fatalf("reverse wiki context=%#v, want RF-DEMO-001", reverse.WikiContext)
	}
	if len(rf.Parents) != 1 || rf.Parents[0].DocID != "FL-DEMO-001" {
		t.Fatalf("reverse parents=%#v, want linked FL-DEMO-001", rf.Parents)
	}

	findEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 20},
		Payload:   map[string]any{"pattern": wikiCodeVerticalServiceSymbol, "exact": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	findItems, ok := findEnv.Items.([]model.SymbolRecord)
	if !ok || len(findItems) == 0 {
		t.Fatalf("nav.find items=%T %#v, want runDemo", findEnv.Items, findEnv.Items)
	}
	foundMJS := false
	for _, item := range findItems {
		if item.FilePath == wikiCodeVerticalServicePath && item.Name == wikiCodeVerticalServiceSymbol {
			foundMJS = true
			if item.Language != "javascript" {
				t.Fatalf("mjs symbol language=%q, want catalog extractor language javascript", item.Language)
			}
		}
	}
	if !foundMJS {
		t.Fatalf("nav.find symbols=%#v, want %s::%s", findItems, wikiCodeVerticalServicePath, wikiCodeVerticalServiceSymbol)
	}

	relatedEnv, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.related",
		Context:   model.QueryOptions{Workspace: root, MaxItems: 20},
		Payload:   map[string]any{"symbol": wikiCodeVerticalServiceSymbol, "depth": "definition"},
	})
	if err != nil {
		t.Fatal(err)
	}
	relatedItems, ok := relatedEnv.Items.([]symbolNeighborhood)
	if !ok || len(relatedItems) != 1 || relatedItems[0].Definition == nil || relatedItems[0].Definition.File != wikiCodeVerticalServicePath {
		t.Fatalf("nav.related items=%T %#v, want .mjs definition", relatedEnv.Items, relatedEnv.Items)
	}
	if relatedEnv.WikiCodeContext == nil || !containsWikiCodeDocID(relatedEnv.WikiCodeContext.WikiContext, "RF-DEMO-001") {
		t.Fatalf("nav.related wiki context=%#v, want RF-DEMO-001", relatedEnv.WikiCodeContext)
	}
}

func TestWikiCodeVerticalColdReverseOwnerBeyondBudgetUsesOverlayAndResolver(t *testing.T) {
	root := newWikiCodeVerticalColdReverseFixture(t)
	targetPath := "src/cold/owner.mjs"
	targetSymbol := targetPath + "::runCold"
	ownerPath := ".docs/wiki/04_RF/RF-ZZZ-COLD-OWNER.md"
	ownerFile := filepath.Join(root, filepath.FromSlash(ownerPath))
	updated := strings.Join([]string{
		"# Cold owner",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"id: RF-ZZZ-COLD-OWNER",
		"",
		"```toon",
		"block_id: RF-ZZZ-COLD-OWNER.bindings",
		"artifact_bindings:",
		"  - relation: implements",
		"    target_kind: symbol",
		"    target_path: " + targetPath,
		"    target_symbol: " + targetSymbol,
		"```",
		"",
	}, "\n")
	writeWikiCodeVerticalFile(t, ownerFile, updated)

	db := mustOpenVerticalReadOnlyDB(t, root)
	defer db.Close()
	scope := livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: targetPath, TargetSymbol: targetSymbol}
	overlay, err := livecontext.BuildOverlay(context.Background(), livecontext.OverlayRequest{
		WorkspaceRoot: root,
		DB:            db,
		Scope:         scope,
		MaxDocuments:  50,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundAddition := false
	var ownerBinding model.DocArtifactBinding
	for _, binding := range overlay.Additions {
		if binding.DocPath == ownerPath && binding.TargetPath == targetPath && binding.TargetSymbol == targetSymbol {
			foundAddition = true
			ownerBinding = binding
		}
	}
	if !foundAddition {
		t.Fatalf("cold owner was not promoted into bounded overlay additions=%#v omissions=%#v", overlay.Additions, overlay.Omissions)
	}
	resolved, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{
		Direction: model.WikiCodeDirectionCodeToWiki, WorkspaceRoot: root,
		TargetPath: targetPath, TargetSymbol: targetSymbol, Limit: 20,
	}, overlay)
	if err != nil {
		t.Fatal(err)
	}
	ownerMatches := 0
	for _, item := range resolved.WikiContext {
		if item.DocID != "RF-ZZZ-COLD-OWNER" {
			continue
		}
		ownerMatches++
		if ownerBinding.BindingRef == "" || item.BindingRef != ownerBinding.BindingRef || item.Path != ownerPath || item.Status != model.WikiCodeStatusResolvedSymbol {
			t.Fatalf("cold owner resolution=%#v, want fixture binding ref %q, path %q and resolved_symbol status", item, ownerBinding.BindingRef, ownerPath)
		}
	}
	if ownerMatches != 1 {
		t.Fatalf("cold owner matches=%d, want exactly one owner after overlay=%#v", ownerMatches, resolved)
	}
}

func TestWikiCodeVerticalNewCanonicalOwnerBeyondBudgetRemainsTypedUnknown(t *testing.T) {
	root := newWikiCodeVerticalColdReverseFixture(t)
	targetPath := "src/cold/owner.mjs"
	targetSymbol := targetPath + "::runCold"
	ownerPath := ".docs/wiki/04_RF/RF-ZZZ-COLD-NEW.md"
	writeWikiCodeVerticalFile(t, filepath.Join(root, filepath.FromSlash(".docs/wiki/04_RF/RF-ZZZ-COLD-WRONG.md")), strings.Join([]string{
		"# Wrong cold owner",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"id: RF-ZZZ-COLD-WRONG",
		"",
		"```toon",
		"block_id: RF-ZZZ-COLD-WRONG.bindings",
		"artifact_bindings:",
		"  - relation: implements",
		"    target_kind: symbol",
		"    target_path: src/cold/other.mjs",
		"    target_symbol: " + targetSymbol,
		"```",
		"",
	}, "\n"))
	writeWikiCodeVerticalFile(t, filepath.Join(root, filepath.FromSlash(ownerPath)), strings.Join([]string{
		"# New cold owner",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"id: RF-ZZZ-COLD-NEW",
		"",
		"```toon",
		"block_id: RF-ZZZ-COLD-NEW.bindings",
		"artifact_bindings:",
		"  - relation: implements",
		"    target_kind: symbol",
		"    target_path: " + targetPath,
		"    target_symbol: " + targetSymbol,
		"```",
		"",
	}, "\n"))
	db := mustOpenVerticalReadOnlyDB(t, root)
	defer db.Close()
	scope := livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: targetPath, TargetSymbol: targetSymbol}
	overlay, err := livecontext.BuildOverlay(context.Background(), livecontext.OverlayRequest{WorkspaceRoot: root, DB: db, Scope: scope, MaxDocuments: 50})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, binding := range overlay.Additions {
		if binding.DocPath == ownerPath && binding.TargetPath == targetPath && binding.TargetSymbol == targetSymbol {
			found = true
		}
	}
	if found {
		t.Fatalf("unknown new owner was promoted without bounded proof additions=%#v", overlay.Additions)
	}
	unknownOwner := false
	for _, omission := range overlay.Omissions {
		if omission.Path == ownerPath && omission.Code == model.OmissionUnknownDocument {
			unknownOwner = true
		}
	}
	if !unknownOwner {
		t.Fatalf("new owner was not reported as typed unknown omissions=%#v", overlay.Omissions)
	}
	if overlay.Cost.FilesParsed > 50 || overlay.Cost.BytesRead > livecontext.DefaultOverlayMaxBytes {
		t.Fatalf("metadata candidate lane exceeded bounds cost=%#v", overlay.Cost)
	}
	resolved, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionCodeToWiki, WorkspaceRoot: root, TargetPath: targetPath, TargetSymbol: targetSymbol, Limit: 20}, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if containsWikiCodeDocID(resolved.WikiContext, "RF-ZZZ-COLD-NEW") || containsWikiCodeDocID(resolved.WikiContext, "RF-ZZZ-COLD-WRONG") {
		t.Fatalf("unknown/wrong-target owners=%#v", resolved.WikiContext)
	}
}

func newWikiCodeVerticalColdReverseFixture(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	source := filepath.Join(repoRoot, "testdata", "wiki-code-bidirectional")
	root := t.TempDir()
	if err := copyWikiCodeVerticalTree(source, root); err != nil {
		t.Fatalf("copy T3 fixture: %v", err)
	}
	writeWikiCodeVerticalFile(t, filepath.Join(root, filepath.FromSlash("src/cold/owner.mjs")), "export function runCold() { return 1; }\n")
	for index := 0; index < 60; index++ {
		path := filepath.Join(root, filepath.FromSlash(fmt.Sprintf(".docs/wiki/04_RF/RF-%03d-COLD-PREFIX.md", index)))
		writeWikiCodeVerticalFile(t, path, fmt.Sprintf("# Prefix %03d\nwiki_source_protocol: SDD-WIKI-SOURCE-v1\nid: RF-%03d-COLD-PREFIX\n\n", index, index))
	}
	writeWikiCodeVerticalFile(t, filepath.Join(root, filepath.FromSlash(".docs/wiki/04_RF/RF-ZZZ-COLD-OWNER.md")), "# Cold owner\nwiki_source_protocol: SDD-WIKI-SOURCE-v1\nid: RF-ZZZ-COLD-OWNER\n\nReferenced target src/cold/owner.mjs for the cold owner.\n")
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "wiki-code-cold-reverse", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"javascript", "typescript"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/wiki-code-cold-reverse", Languages: []string{"javascript", "typescript"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	if _, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "wiki-code-cold-reverse-baseline"); err != nil {
		t.Fatalf("index cold reverse fixture: %v", err)
	}
	return root
}

func TestWikiCodeVerticalOverlayReadYourWritesAndFailClosedInputs(t *testing.T) {
	t.Run("edit_binding_without_reindex", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		path := filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath))
		content := readWikiCodeVerticalFile(t, path)
		content = strings.Replace(content, "target_path: src/demo/service.mjs", "target_path: src/demo/unmapped.mjs", 1)
		content = strings.Replace(content, "target_symbol: runDemo", "target_symbol: standaloneFeature", 1)
		writeWikiCodeVerticalFile(t, path, content)

		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		implementation := wikiCodeVerticalImplementations(got.DirectCode)
		if len(implementation) != 1 || implementation[0].Path != "src/demo/unmapped.mjs" || implementation[0].Symbol != "standaloneFeature" {
			t.Fatalf("edited direct_code=%#v, want overlay target", got.DirectCode)
		}
		for _, item := range got.DirectCode {
			if item.Relation == model.RelationImplements && item.Path == wikiCodeVerticalServicePath && item.Symbol == wikiCodeVerticalServiceSymbol {
				t.Fatalf("old binding survived edit overlay=%#v", got.DirectCode)
			}
		}
		if got.Freshness.Bindings != model.FreshnessOverlay {
			t.Fatalf("edited binding freshness=%q, want overlay", got.Freshness.Bindings)
		}
	})

	t.Run("remove_binding_tombstone", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		path := filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath))
		content := strings.Replace(readWikiCodeVerticalFile(t, path), "artifact_bindings:", "bindings_removed:", 1)
		writeWikiCodeVerticalFile(t, path, content)

		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		if len(got.DirectCode) != 0 || len(got.Tests) != 0 {
			t.Fatalf("removed binding remained visible: direct=%#v tests=%#v", got.DirectCode, got.Tests)
		}
		if got.Classification != model.WikiCodeClassificationUnmappedChangedCode {
			t.Fatalf("removed binding classification=%q omissions=%#v, want unmapped", got.Classification, got.Omissions)
		}
	})

	t.Run("add_binding_reverse_lookup", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		newPath := filepath.Join(root, filepath.FromSlash(".docs/wiki/04_RF/RF-DEMO-NEW.md"))
		writeWikiCodeVerticalFile(t, newPath, wikiCodeVerticalBindingDoc("RF-DEMO-NEW", "src/demo/unmapped.mjs", "standaloneFeature", model.RelationImplements, model.TargetKindSymbol))

		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: "src/demo/unmapped.mjs", TargetSymbol: "standaloneFeature"})
		if !containsWikiCodeDocID(got.WikiContext, "RF-DEMO-NEW") {
			t.Fatalf("new reverse context=%#v, want RF-DEMO-NEW", got.WikiContext)
		}
	})

	t.Run("lost_watcher_event", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		path := filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath))
		content := strings.Replace(readWikiCodeVerticalFile(t, path), "target_path: src/demo/service.mjs", "target_path: src/demo/unmapped.mjs", 1)
		content = strings.Replace(content, "target_symbol: runDemo", "target_symbol: standaloneFeature", 1)
		writeWikiCodeVerticalFile(t, path, content)
		// Deliberately do not call IncrementalIndexWithPaths or the watcher. The
		// query overlay is the correctness path for a lost watcher event.
		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		implementation := wikiCodeVerticalImplementations(got.DirectCode)
		if len(implementation) != 1 || implementation[0].Path != "src/demo/unmapped.mjs" || implementation[0].Symbol != "standaloneFeature" {
			t.Fatalf("lost-event overlay direct_code=%#v", got.DirectCode)
		}
	})

	t.Run("delete_and_rename_target", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		oldPath := filepath.Join(root, filepath.FromSlash(wikiCodeVerticalServicePath))
		renamedPath := filepath.Join(root, filepath.FromSlash("src/demo/service-renamed.mjs"))
		if err := os.Rename(oldPath, renamedPath); err != nil {
			t.Fatal(err)
		}
		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		if containsWikiCodePath(got.DirectCode, wikiCodeVerticalServicePath) || !containsWikiCodeOmission(got.Omissions, model.WikiCodeStatusMissingPath) {
			t.Fatalf("renamed target was presented as current: direct=%#v omissions=%#v", got.DirectCode, got.Omissions)
		}
		if err := os.WriteFile(renamedPath, []byte("export function runDemoRenamed() { return 3; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath))
		content := readWikiCodeVerticalFile(t, path)
		content = strings.Replace(content, "target_path: src/demo/service.mjs", "target_path: src/demo/service-renamed.mjs", 1)
		content = strings.Replace(content, "target_symbol: runDemo", "target_symbol: runDemoRenamed", 1)
		writeWikiCodeVerticalFile(t, path, content)
		got = wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		if containsWikiCodePath(got.DirectCode, wikiCodeVerticalServicePath) || containsWikiCodeSymbol(got.DirectCode, wikiCodeVerticalServiceSymbol) {
			t.Fatalf("old renamed fact remained visible: %#v", got.DirectCode)
		}
		if !containsWikiCodeOmission(got.Omissions, model.WikiCodeStatusMissingSymbol) {
			t.Fatalf("renamed symbol did not become typed omission: %#v", got.Omissions)
		}
	})

	t.Run("unmapped_changed_code", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		unmappedPath := filepath.Join(root, filepath.FromSlash("src/demo/unmapped.mjs"))
		if err := os.WriteFile(unmappedPath, []byte("export function standaloneFeature() { return 'changed'; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: "src/demo/unmapped.mjs", TargetSymbol: "standaloneFeature"})
		if got.Classification != model.WikiCodeClassificationUnmappedChangedCode || len(got.NextQueries) == 0 {
			t.Fatalf("changed unmapped result=%#v, want typed classification and next queries", got)
		}
	})

	t.Run("missing_ambiguous_unsafe_and_concurrent", func(t *testing.T) {
		root, app := newWikiCodeVerticalFixture(t)
		missing := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocIDs: []string{"RF-DEMO-AMBIGUOUS"}})
		if !containsWikiCodeOmission(missing.Omissions, model.WikiCodeStatusMissingPath) {
			t.Fatalf("missing target omissions=%#v", missing.Omissions)
		}

		findEnv, err := app.Execute(context.Background(), model.CommandRequest{
			Operation: "nav.find",
			Context:   model.QueryOptions{Workspace: root, MaxItems: 10},
			Payload:   map[string]any{"pattern": "resolveConflict"},
		})
		if err != nil {
			t.Fatal(err)
		}
		findItems, ok := findEnv.Items.([]model.SymbolRecord)
		if !ok || len(findItems) != 2 {
			t.Fatalf("ambiguous candidates=%T %#v, want two bounded catalog candidates", findEnv.Items, findEnv.Items)
		}
		paths := []string{findItems[0].FilePath, findItems[1].FilePath}
		sort.Strings(paths)
		if !reflectStringSetEqual(paths, []string{"src/demo/ambiguous-a.mjs", "src/demo/ambiguous-b.mjs"}) {
			t.Fatalf("ambiguous candidate paths=%v", paths)
		}

		outside := filepath.Join(t.TempDir(), "outside.mjs")
		if err := os.WriteFile(outside, []byte("export function escape() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, filepath.FromSlash("src/demo/escape.mjs"))
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlink unavailable for unsafe-target oracle: %v", err)
		}
		unsafeContent := strings.Replace(readWikiCodeVerticalFile(t, filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath))), "target_path: src/demo/service.mjs", "target_path: src/demo/escape.mjs", 1)
		unsafeContent = strings.Replace(unsafeContent, "target_symbol: runDemo", "target_symbol: escape", 1)
		writeWikiCodeVerticalFile(t, filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath)), unsafeContent)
		unsafe := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		if !containsWikiCodeOmission(unsafe.Omissions, model.WikiCodeStatusUnsafeTarget) || containsWikiCodePath(unsafe.DirectCode, "src/demo/escape.mjs") {
			t.Fatalf("unsafe symlink result=%#v", unsafe)
		}

		concurrentOverlay := model.WikiCodeOverlay{
			Scope:      model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}},
			Tombstones: []model.BindingTombstone{{DocPath: wikiCodeVerticalRFPath, Reason: model.OmissionConcurrentChange}},
			Omissions:  []model.WikiCodeOmission{{Code: model.OmissionConcurrentChange, Path: wikiCodeVerticalRFPath, Reason: "test concurrent-change proof"}},
		}
		concurrentDB := mustOpenVerticalReadOnlyDB(t, root)
		concurrent, err := ResolveWikiCodeContext(context.Background(), root, concurrentDB, WikiCodeResolveRequest{
			Direction:     model.WikiCodeDirectionWikiToCode,
			WorkspaceRoot: root,
			DocPaths:      []string{wikiCodeVerticalRFPath},
		}, concurrentOverlay)
		concurrentDB.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(wikiCodeVerticalImplementations(concurrent.DirectCode)) != 0 || !containsWikiCodeOmission(concurrent.Omissions, model.OmissionConcurrentChange) {
			t.Fatalf("concurrent result was not fail-closed: %#v", concurrent)
		}
	})
}

func TestWikiCodeVerticalNoQueryWritesAndThirtyStableDigests(t *testing.T) {
	root, app := newWikiCodeVerticalFixture(t)
	before := wikiCodeVerticalSQLiteSnapshot(t, root)
	var first model.WikiCodeContext
	var canonical string
	for run := 0; run < 30; run++ {
		got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
		if run == 0 {
			first = got
			canonical = wikiCodeVerticalCollectionsDigest(got)
		} else {
			if got.DeterminismDigest != first.DeterminismDigest {
				t.Fatalf("run %d determinism digest=%q, want %q", run, got.DeterminismDigest, first.DeterminismDigest)
			}
			if wikiCodeVerticalCollectionsDigest(got) != canonical {
				t.Fatalf("run %d canonical order changed", run)
			}
		}
	}
	after := wikiCodeVerticalSQLiteSnapshot(t, root)
	if !reflectStringMapEqual(before, after) {
		t.Fatalf("query mutated SQLite bytes/state: before=%#v after=%#v", before, after)
	}
	for name, value := range map[string]int{
		"metadata_checked": first.Cost.MetadataChecked,
		"files_hashed":     first.Cost.FilesHashed,
		"files_parsed":     first.Cost.FilesParsed,
		"catalog_queries":  first.Cost.CatalogQueries,
		"graph_nodes":      first.Cost.GraphNodesVisited,
		"graph_edges":      first.Cost.GraphEdgesVisited,
	} {
		if value < 0 {
			t.Fatalf("cost counter %s=%d", name, value)
		}
	}
}

func TestWikiCodeVerticalGraphV1CompatibilityAndMetrics(t *testing.T) {
	canonical := wikisource.Parse(".docs/wiki/04_RF/RF-CANONICAL.md", wikiCodeVerticalBindingDoc("RF-CANONICAL", "src/demo/service.mjs", "runDemo", model.RelationImplements, model.TargetKindSymbol), 1)
	canonicalBindings := wikisource.SourceBindings(canonical, 1)
	if len(canonicalBindings) != 1 || canonicalBindings[0].AuthoringOrigin != model.AuthoringOriginCanonical || canonicalBindings[0].Relation != model.RelationImplements {
		t.Fatalf("canonical artifact_bindings=%#v", canonicalBindings)
	}
	for index, item := range []struct {
		field    string
		target   string
		relation string
	}{
		{field: "implementation_anchors", target: "src/demo/service.mjs", relation: model.RelationImplements},
		{field: "code_links", target: "src/demo/helper.mjs", relation: model.RelationOperates},
		{field: "test_links", target: "test/demo/service.test.mjs", relation: model.RelationTests},
	} {
		legacyContent := strings.Join([]string{
			"# Legacy advisory",
			"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
			fmt.Sprintf("id: RF-LEGACY-ADVISORY-%d", index),
			"",
			"```toon",
			fmt.Sprintf("block_id: RF-LEGACY-ADVISORY-%d.core", index),
			item.field + ":",
			"  - " + item.target,
			"```",
		}, "\n")
		legacy := wikisource.Parse(fmt.Sprintf(".docs/wiki/04_RF/RF-LEGACY-ADVISORY-%d.md", index), legacyContent, 1)
		legacyBindings := wikisource.SourceBindings(legacy, 1)
		found := false
		for _, binding := range legacyBindings {
			if binding.TargetPath != item.target {
				continue
			}
			found = true
			if binding.AuthoringOrigin != model.AuthoringOriginLegacy || binding.Relation != item.relation {
				t.Fatalf("legacy %s normalization=%#v, want legacy/advisory %s", item.field, binding, item.relation)
			}
		}
		if !found {
			t.Fatalf("legacy %s produced no binding: %#v", item.field, legacyBindings)
		}
	}

	root, app := newWikiCodeVerticalFixture(t)
	planned := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocIDs: []string{"RF-DEMO-PLANNED"}})
	if len(planned.DirectCode) != 0 || !containsWikiCodeOmission(planned.Omissions, "planned_binding_excluded") {
		t.Fatalf("planned binding result=%#v, want stored but non-navigable", planned)
	}
	plannedDB := mustOpenVerticalReadOnlyDB(t, root)
	plannedBindings, err := store.ListDocArtifactBindings(context.Background(), plannedDB)
	plannedDB.Close()
	if err != nil {
		t.Fatal(err)
	}
	plannedStored := false
	for _, binding := range plannedBindings {
		if binding.DocID == "RF-DEMO-PLANNED" && binding.BindingStatus == model.BindingStatusPlanned {
			plannedStored = true
			break
		}
	}
	if !plannedStored || len(planned.GraphPaths) != 0 {
		t.Fatalf("planned binding storage/edges stored=%v graph_paths=%#v", plannedStored, planned.GraphPaths)
	}

	retired := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: wikiCodeVerticalServicePath, TargetSymbol: "legacyRun"})
	if containsWikiCodeDocID(retired.WikiContext, "RF-DEMO-LEGACY") {
		t.Fatalf("retired document appeared without explicit historical selector: %#v", retired.WikiContext)
	}
	db := mustOpenVerticalReadOnlyDB(t, root)
	historical, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{
		Direction:    model.WikiCodeDirectionCodeToWiki,
		TargetPath:   wikiCodeVerticalServicePath,
		TargetSymbol: "legacyRun",
		DocSelectors: []string{"RF-DEMO-LEGACY"},
	}, model.WikiCodeOverlay{})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	foundRedirect := false
	for _, item := range historical.WikiContext {
		if item.DocID == "RF-DEMO-LEGACY" && item.Status == "redirect" && item.SupersededBy == "RF-DEMO-001" {
			foundRedirect = true
		}
	}
	if !foundRedirect {
		t.Fatalf("historical redirect=%#v", historical.WikiContext)
	}

	// Active-to-retired transitions are accepted only as a non-navigable
	// historical view; they may never remain direct implementation evidence.
	baseDB := mustOpenVerticalReadOnlyDB(t, root)
	baseBindings, err := store.ListDocArtifactBindings(context.Background(), baseDB)
	baseDB.Close()
	if err != nil {
		t.Fatal(err)
	}
	var active model.DocArtifactBinding
	for _, binding := range baseBindings {
		if binding.DocID == "RF-DEMO-001" && binding.TargetPath == wikiCodeVerticalServicePath && binding.TargetSymbol == wikiCodeVerticalServiceSymbol {
			active = binding
			break
		}
	}
	if active.BindingRef == "" {
		t.Fatal("active RF binding not found")
	}
	active.DocLifecycle = model.DocLifecycleRetired
	transitionDB := mustOpenVerticalReadOnlyDB(t, root)
	transition, err := ResolveWikiCodeContext(context.Background(), root, transitionDB, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{wikiCodeVerticalRFPath}}, model.WikiCodeOverlay{
		Scope:      model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}},
		Tombstones: []model.BindingTombstone{{BindingRef: active.BindingRef}},
		Additions:  []model.DocArtifactBinding{active},
	})
	transitionDB.Close()
	if err != nil {
		t.Fatal(err)
	}
	if containsWikiCodeSymbol(transition.DirectCode, wikiCodeVerticalServiceSymbol) || !containsWikiCodeOmission(transition.Omissions, "retired_binding_rejected") {
		t.Fatalf("active-retired transition result=%#v", transition)
	}

	// Graph v1's semantic order is relation-driven, never folder-number-driven.
	ordered := model.WikiCodeContext{DirectCode: []model.WikiCodeEvidence{
		{Path: "src/operate.mjs", Relation: model.RelationOperates},
		{Path: "test/service.test.mjs", Relation: model.RelationTests},
		{Path: "src/service.mjs", Relation: model.RelationImplements},
	}}
	model.SortWikiCodeContext(&ordered)
	if got := []string{ordered.DirectCode[0].Relation, ordered.DirectCode[1].Relation, ordered.DirectCode[2].Relation}; !reflectStringSetEqual(got, []string{model.RelationImplements, model.RelationTests, model.RelationOperates}) || ordered.DirectCode[0].Relation != model.RelationImplements {
		t.Fatalf("semantic relation order=%v", got)
	}

	precision := 1.0
	if len(wikiCodeVerticalImplementations(forwardMetrics(t, app, root).DirectCode)) != 1 {
		precision = 0
	}
	reverseMetrics := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: wikiCodeVerticalServicePath, TargetSymbol: wikiCodeVerticalServiceSymbol})
	recall := 0.0
	if containsWikiCodeDocID(reverseMetrics.WikiContext, "RF-DEMO-001") {
		recall = 1
	}
	if precision != 1.0 || recall != 1.0 {
		t.Fatalf("metrics precision=%v recall=%v, want 1.0/1.0", precision, recall)
	}
	if containsWikiCodePath(forwardMetrics(t, app, root).DirectCode, ".docs/raw/decoy.md") || containsWikiCodePath(forwardMetrics(t, app, root).DirectCode, ".docs/auditoria/decoy.md") {
		t.Fatal("raw/audit result became primary direct evidence")
	}
}

func TestWikiCodeVerticalStaleGraphPreservesDirectEvidenceAndVisibleOmission(t *testing.T) {
	root, app := newWikiCodeVerticalFixture(t)
	writer, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetGraphRuntimeState(context.Background(), writer, store.GraphRuntimeStale, ""); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
	implementation := wikiCodeVerticalImplementations(got.DirectCode)
	if len(implementation) != 1 || implementation[0].Path != wikiCodeVerticalServicePath {
		t.Fatalf("stale graph direct_code=%#v, want direct RF binding", got.DirectCode)
	}
	if len(got.SupportingCode) != 0 || !containsWikiCodeOmission(got.Omissions, model.WikiCodeStatusGraphStale) {
		t.Fatalf("stale graph support=%#v omissions=%#v", got.SupportingCode, got.Omissions)
	}

	unknownBefore := wikiCodeVerticalSQLiteSnapshot(t, root)
	unknownDB := mustOpenVerticalReadOnlyDB(t, root)
	unknown, err := ResolveWikiCodeContext(context.Background(), root, unknownDB, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{wikiCodeVerticalRFPath}, GraphFreshness: model.GraphFreshness{State: model.GraphFreshnessUnknown}}, model.WikiCodeOverlay{})
	unknownDB.Close()
	unknownAfter := wikiCodeVerticalSQLiteSnapshot(t, root)
	if !reflectStringMapEqual(unknownBefore, unknownAfter) {
		t.Fatalf("unknown-state query mutated SQLite: before=%#v after=%#v", unknownBefore, unknownAfter)
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(wikiCodeVerticalImplementations(unknown.DirectCode)) != 1 || !containsWikiCodeOmission(unknown.Omissions, model.WikiCodeStatusGraphUnavailable) {
		t.Fatalf("unknown graph state=%#v, want direct evidence plus typed omission", unknown)
	}
}

func TestWikiCodeVerticalSameSizeSameTickRewriteIsCurrent(t *testing.T) {
	root, app := newWikiCodeVerticalFixture(t)
	path := filepath.Join(root, filepath.FromSlash(wikiCodeVerticalRFPath))
	content := readWikiCodeVerticalFile(t, path)
	if !strings.Contains(content, "target_symbol: runDemo") {
		t.Fatal("fixture binding symbol not found")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	rewritten := strings.Replace(content, "target_symbol: runDemo", "target_symbol: runDeme", 1)
	if len(rewritten) != len(content) {
		t.Fatal("same-size rewrite setup drifted")
	}
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	got := wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
	if containsWikiCodeSymbol(wikiCodeVerticalImplementations(got.DirectCode), wikiCodeVerticalServiceSymbol) || !containsWikiCodeOmission(got.Omissions, model.WikiCodeStatusMissingSymbol) {
		t.Fatalf("same-tick rewrite result=%#v, want missing_symbol and no stale runDemo", got)
	}
}

func TestWikiCodeVerticalDuplicateDocIDsAndGovernanceSelfEdgesFailClosed(t *testing.T) {
	root, app := newWikiCodeVerticalFixture(t)
	writer, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := store.ListDocRecords(context.Background(), writer)
	if err != nil {
		writer.Close()
		t.Fatal(err)
	}
	docs = append(docs, model.DocRecord{Path: ".docs/wiki/04_RF/RF-DEMO-001-copy.md", DocID: "RF-DEMO-001", Layer: "04", Family: "functional", ContentHash: "duplicate"})
	if err := store.ReplaceDocs(context.Background(), writer, docs, nil, nil); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = app.ResolveWikiCodeContext(context.Background(), WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, WorkspaceRoot: root, DocSelectors: []string{wikiCodeVerticalRFPath}}, model.WikiCodeOverlay{})
	var contextErr *model.WikiCodeContextError
	if !errors.As(err, &contextErr) || contextErr.Code != "GPH_WIKI_DUPLICATE_DOC_ID" {
		t.Fatalf("duplicate doc ID error=%v", err)
	}

	root, _ = newWikiCodeVerticalFixture(t)
	db := mustOpenVerticalReadOnlyDB(t, root)
	edges, err := store.ListDocEdges(context.Background(), db)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range edges {
		if edge.FromPath != "" && edge.FromPath == edge.ToPath {
			t.Fatalf("governance/materialization self-edge=%#v", edge)
		}
	}
}

func newWikiCodeVerticalFixture(t *testing.T) (string, *App) {
	return newWikiCodeVerticalFixtureWithGovernance(t, false)
}

func newWikiCodeVerticalFixtureWithGovernedSource(t *testing.T) (string, *App) {
	return newWikiCodeVerticalFixtureWithGovernance(t, true)
}

func newWikiCodeVerticalFixtureWithGovernance(t *testing.T, governedSource bool) (string, *App) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	source := filepath.Join(repoRoot, "testdata", "wiki-code-bidirectional")
	root := t.TempDir()
	if err := copyWikiCodeVerticalTree(source, root); err != nil {
		t.Fatalf("copy T3 fixture: %v", err)
	}
	if governedSource {
		// Reuse the canonical test governance fixture only for the integrated
		// command path; the baseline fixture intentionally remains invalid so
		// negative governance cases keep their original behavior.
		writeSpecBackendGovernanceFixture(t, root)
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "wiki-code-vertical", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"javascript", "typescript"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/wiki-code-vertical", Languages: []string{"javascript", "typescript"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	if _, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "wiki-code-vertical-baseline"); err != nil {
		t.Fatalf("index T3 fixture: %v", err)
	}
	return root, New(root, nil)
}

func wikiCodeVerticalEnvelopeDiagnostics(env model.Envelope) string {
	warnings := append([]string(nil), env.Warnings...)
	if len(warnings) > 4 {
		warnings = append(warnings[:4], "<more>")
	}
	return fmt.Sprintf("ok=%t backend=%q hint=%q warnings=%q", env.Ok, env.Backend, env.Hint, warnings)
}

func copyWikiCodeVerticalTree(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(source, entry.Name())
		dstPath := filepath.Join(destination, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture contains unsupported symlink %s", entry.Name())
		}
		if entry.IsDir() {
			if err := copyWikiCodeVerticalTree(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		content, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func wikiCodeVerticalContext(t *testing.T, app *App, root string, scope livecontext.WikiCodeScope) model.WikiCodeContext {
	t.Helper()
	got, err := app.buildLiveWikiCodeContext(context.Background(), "nav.trace", scope, model.QueryOptions{Workspace: root, TokenBudget: 20_000, MaxItems: 128})
	if err != nil {
		t.Fatalf("live wiki-code context: %v", err)
	}
	return got
}

func forwardMetrics(t *testing.T, app *App, root string) model.WikiCodeContext {
	t.Helper()
	return wikiCodeVerticalContext(t, app, root, livecontext.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{wikiCodeVerticalRFPath}})
}

func wikiCodeVerticalBindingDoc(docID, targetPath, targetSymbol, relation, targetKind string) string {
	return fmt.Sprintf("---\nid: %s\ntitle: %s\n---\n\n```toon\nwiki_source_protocol: SDD-WIKI-SOURCE-v1\nid: \"%s\"\nblock_id: \"%s.requirement_core\"\nkind: \"req\"\nartifact_bindings:\n  - target_kind: %s\n    relation: %s\n    role: implementation\n    target_path: %s\n    target_symbol: %s\n```\n", docID, docID, docID, docID, targetKind, relation, targetPath, targetSymbol)
}

func readWikiCodeVerticalFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeWikiCodeVerticalFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustOpenVerticalReadOnlyDB(t *testing.T, root string) *sql.DB {
	t.Helper()
	db, err := store.OpenReadOnlyExisting(root, store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func wikiCodeVerticalSQLiteSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	paths := []string{store.WorkspaceDBPath(root), store.WorkspaceDBPath(root) + "-wal", store.WorkspaceDBPath(root) + "-shm"}
	result := make(map[string]string, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			result[filepath.Base(path)] = "<absent>"
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		result[filepath.Base(path)] = hex.EncodeToString(sum[:]) + fmt.Sprintf(":%d", len(content))
	}
	return result
}

func wikiCodeVerticalCollectionsDigest(got model.WikiCodeContext) string {
	value := struct {
		Direct      []model.WikiCodeEvidence        `json:"direct"`
		Tests       []model.WikiCodeEvidence        `json:"tests"`
		Supporting  []model.WikiCodeEvidence        `json:"supporting"`
		WikiContext []model.WikiCodeWikiContextItem `json:"wiki_context"`
	}{got.DirectCode, got.Tests, got.SupportingCode, got.WikiContext}
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func wikiCodeVerticalImplementations(items []model.WikiCodeEvidence) []model.WikiCodeEvidence {
	result := make([]model.WikiCodeEvidence, 0, len(items))
	for _, item := range items {
		if item.Relation == model.RelationImplements && item.Status != model.WikiCodeStatusMissingSymbol && item.Status != model.WikiCodeStatusMissingPath && item.Status != model.WikiCodeStatusAmbiguousSymbol {
			result = append(result, item)
		}
	}
	return result
}

func containsWikiCodePath(items []model.WikiCodeEvidence, path string) bool {
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func containsWikiCodeSymbol(items []model.WikiCodeEvidence, symbol string) bool {
	for _, item := range items {
		if item.Symbol == symbol {
			return true
		}
	}
	return false
}

func containsWikiCodeDocID(items []model.WikiCodeWikiContextItem, docID string) bool {
	for _, item := range items {
		if item.DocID == docID {
			return true
		}
	}
	return false
}

func containsWikiCodeOmission(items []model.WikiCodeContextOmission, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func reflectStringSetEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}

func reflectStringMapEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

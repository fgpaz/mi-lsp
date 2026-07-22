package service

import (
	"context"
	"os"
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

func TestNavPackPreviewUsesRouteCoreAnchorAndShortPack(t *testing.T) {
	alias := "pack-func-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	waitForIndexingComplete(t, initEnv)

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
	if len(result.Docs) == 0 || len(result.Docs) > 3 {
		t.Fatalf("expected short preview pack, got %#v", result.Docs)
	}
	if result.Docs[0].Stage != "anchor" {
		t.Fatalf("expected first preview doc to be anchor, got %#v", result.Docs[0])
	}
	if result.PrimaryDoc == "" || result.PrimaryDoc != result.Docs[0].Path {
		t.Fatalf("primary doc = %q, want first preview anchor %#v", result.PrimaryDoc, result.Docs)
	}
	if len(result.Docs[0].Targets) == 0 {
		t.Fatalf("expected preview targets, got %#v", result.Docs[0])
	}
	if env.MemoryPointer == nil {
		t.Fatalf("expected memory pointer, got %#v", env)
	}
	if env.Continuation == nil || env.Continuation.Reason != "expand_preview" || env.Continuation.Next.Op != "nav.pack" || !env.Continuation.Next.Full {
		t.Fatalf("expected pack expansion continuation, got %#v", env.Continuation)
	}
}

func TestNavPackFullIncludesReadableSlices(t *testing.T) {
	alias := "pack-full-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	waitForIndexingComplete(t, initEnv)

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
	if len(results[0].Docs) == 0 || results[0].Docs[0].Stage != "anchor" || results[0].Docs[0].SliceText == "" {
		t.Fatalf("expected full slice text, got %#v", results[0].Docs)
	}
	if !strings.Contains(results[0].Docs[0].SliceText, "LoginHandler") {
		t.Fatalf("expected anchor RF slice to include relevant snippet, got %#v", results[0].Docs[0])
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

func TestNavPackNextQueriesArePopulated(t *testing.T) {
	alias := "pack-nq-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	waitForIndexingComplete(t, initEnv)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) == 0 {
		t.Fatalf("expected at least one pack result, got none")
	}
	if len(results[0].NextQueries) == 0 {
		t.Fatalf("expected next_queries to be populated, got empty")
	}
	if !strings.HasPrefix(results[0].NextQueries[0], "mi-lsp") {
		t.Fatalf("expected next query to start with mi-lsp, got %q", results[0].NextQueries[0])
	}
}

func TestNavPackExplicitRFAnchorWinsOverRouteCore(t *testing.T) {
	alias := "pack-rf-anchor-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	waitForIndexingComplete(t, initEnv)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
		Payload:   map[string]any{"task": "understand login", "rf": "RF-AUTH-001"},
	})
	if err != nil {
		t.Fatalf("nav.pack with rf anchor: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) == 0 {
		t.Fatalf("expected at least one pack result, got none")
	}
	wantPrimary := ".docs/wiki/04_RF/RF-AUTH-001.md"
	if results[0].PrimaryDoc != wantPrimary {
		t.Fatalf("primary_doc = %q, want %q (explicit --rf anchor must win over route core)", results[0].PrimaryDoc, wantPrimary)
	}
	if len(results[0].Docs) == 0 || results[0].Docs[0].Path != wantPrimary || results[0].Docs[0].Stage != "anchor" {
		t.Fatalf("expected explicit RF anchor first, got %#v", results[0].Docs)
	}
}

func TestNavPackFullExplicitDocIsAnchorFirstAndPreserved(t *testing.T) {
	alias := "pack-doc-anchor-full-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	waitForIndexingComplete(t, initEnv)

	wantPrimary := ".docs/wiki/04_RF/RF-AUTH-001.md"
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, Full: true, MaxItems: 2},
		Payload:   map[string]any{"task": "understand login", "doc": wantPrimary},
	})
	if err != nil {
		t.Fatalf("nav.pack with explicit doc: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) != 1 {
		t.Fatalf("expected one pack result, got %#v", env.Items)
	}
	if results[0].PrimaryDoc != wantPrimary {
		t.Fatalf("primary_doc = %q, want %q", results[0].PrimaryDoc, wantPrimary)
	}
	if len(results[0].Docs) == 0 || results[0].Docs[0].Path != wantPrimary || results[0].Docs[0].Stage != "anchor" {
		t.Fatalf("expected explicit doc anchor first and preserved, got %#v", results[0].Docs)
	}
	if results[0].Docs[0].SliceText == "" || results[0].Docs[0].SliceStart == 0 || results[0].Docs[0].SliceEnd == 0 {
		t.Fatalf("expected full anchor slice evidence, got %#v", results[0].Docs[0])
	}
}

func TestNavPackLookupStatusUsesPrimaryDocForExactRF(t *testing.T) {
	alias := "pack-lookup-rf-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	waitForIndexingComplete(t, initEnv)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, Full: true, MaxItems: 8},
		Payload:   map[string]any{"task": "RF-AUTH-001"},
	})
	if err != nil {
		t.Fatalf("nav.pack exact rf: %v", err)
	}
	results := env.Items.([]model.PackResult)
	if len(results) == 0 {
		t.Fatalf("expected at least one pack result, got none")
	}
	status := results[0].LookupStatus
	if status == nil {
		t.Fatalf("expected lookup status, got nil")
	}
	if status.DocID != "RF-AUTH-001" || status.Path != ".docs/wiki/04_RF/RF-AUTH-001.md" || status.MatchKind != "canonical_indexed_id" {
		t.Fatalf("unexpected pack lookup status: %#v", status)
	}
}

func TestPackOperationContractsDoNotMix(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation string
		cliPrefix string
	}{
		{name: "legacy", operation: "nav.pack", cliPrefix: "mi-lsp nav pack"},
		{name: "wiki", operation: "nav.wiki.pack", cliPrefix: "mi-lsp nav wiki pack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := model.PackResult{PrimaryDoc: ".docs/wiki/04_RF/RF-AUTH-001.md"}
			continuation := buildPackContinuation(tc.operation, "understand login", result, model.QueryOptions{}, nil)
			if continuation == nil || continuation.Next.Op != tc.operation {
				t.Fatalf("continuation = %#v, want operation %q", continuation, tc.operation)
			}

			queries := buildPackNextQueries(tc.operation, "workspace", "understand login", false, []model.PackDoc{{DocID: "RF-AUTH-001"}})
			if len(queries) == 0 || queries[0] != tc.cliPrefix+` "understand login" --workspace workspace --full` {
				t.Fatalf("next_queries = %#v, want %q", queries, tc.cliPrefix)
			}
			wrongPrefix := "mi-lsp nav pack"
			if tc.operation == "nav.pack" {
				wrongPrefix = "mi-lsp nav wiki pack"
			}
			if strings.HasPrefix(queries[0], wrongPrefix) {
				t.Fatalf("next_queries mixed contracts: %#v", queries)
			}
		})
	}
}

func TestPackReentryMemoryPreservesOperationContract(t *testing.T) {
	for _, operation := range []string{"nav.pack", "nav.wiki.pack"} {
		t.Run(operation, func(t *testing.T) {
			snapshot := model.ReentryMemorySnapshot{
				Handoff: "pack-handoff",
				BestReentry: model.ContinuationTarget{
					Op:    operation,
					Query: "understand login",
				},
			}
			memory := &loadedReentryMemory{Snapshot: snapshot}
			pointer := buildMemoryPointer(snapshot, false)
			if pointer == nil || pointer.ReentryOp != operation {
				t.Fatalf("memory pointer = %#v, want reentry_op %q", pointer, operation)
			}
			continuation := buildMemoryFallbackContinuation(memory, true)
			if continuation == nil || continuation.Next.Op != operation {
				t.Fatalf("memory continuation = %#v, want operation %q", continuation, operation)
			}
		})
	}
}

func TestPackGovernanceGatePreservesOperationID(t *testing.T) {
	for _, operation := range []string{"nav.pack", "nav.wiki.pack"} {
		t.Run(operation, func(t *testing.T) {
			alias := "pack-governance-" + filepath.Base(t.TempDir())
			root := createFunctionalPackWorkspaceFixture(t, alias)
			if err := os.Remove(filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")); err != nil {
				t.Fatalf("remove governance source: %v", err)
			}
			if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
				Name: alias,
				Root: root,
				Kind: model.WorkspaceKindSingle,
			}); err != nil {
				t.Fatalf("register workspace: %v", err)
			}
			defer func() { _ = workspace.RemoveWorkspace(alias) }()

			env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
				Operation: operation,
				Context:   model.QueryOptions{Workspace: alias},
				Payload:   map[string]any{"task": "understand login"},
			})
			if err != nil {
				t.Fatalf("%s: %v", operation, err)
			}
			if !strings.Contains(env.Hint, operation) {
				t.Fatalf("governance hint = %q, want operation %q", env.Hint, operation)
			}
		})
	}
}

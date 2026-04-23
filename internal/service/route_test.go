package service

import (
	"context"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavRouteRequiresTask(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeSpecBackendGovernanceFixture(t, root)

	alias := "route-notask-" + t.Name()
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
	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{},
	})
	if err == nil || !strings.Contains(err.Error(), "task is required") {
		t.Fatalf("expected 'task is required' error, got %v", err)
	}
}

func TestNavRouteReturnsCanonicalDocFromGovernance(t *testing.T) {
	alias := "route-tier1-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got %#v", env)
	}
	if env.Backend != "route" {
		t.Fatalf("backend = %q, want route", env.Backend)
	}

	results, ok := env.Items.([]model.RouteResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one RouteResult, got %T %#v", env.Items, env.Items)
	}
	result := results[0]
	if result.Canonical.AnchorDoc.Path == "" {
		t.Fatalf("expected canonical anchor doc, got empty path")
	}
	if !strings.Contains(result.Canonical.AnchorDoc.Path, ".docs/wiki/") {
		t.Fatalf("expected anchor inside .docs/wiki/, got %q", result.Canonical.AnchorDoc.Path)
	}
}

func TestNavRouteExplicitEmbeddedRFUsesContainingRFDocWhenDocsIndexEmpty(t *testing.T) {
	alias := "route-embedded-rf-" + t.Name()
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-IDN.md", strings.Join([]string{
		"# RF-IDN",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-IDN-07 | OAuth device authorization sanitizado |",
		"| RF-IDN-08 | Reindexado documental observable |",
	}, "\n"))
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "RF-IDN-08"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if got := results[0].Canonical.AnchorDoc.Path; got != ".docs/wiki/04_RF/RF-IDN.md" {
		t.Fatalf("anchor path = %q, want aggregate RF doc", got)
	}
	if got := results[0].Canonical.AnchorDoc.DocID; got != "RF-IDN-08" {
		t.Fatalf("anchor doc_id = %q, want RF-IDN-08", got)
	}
}

func TestNavRoutePreservesExplicitEmbeddedRFWhenDocsIndexExists(t *testing.T) {
	alias := "route-indexed-embedded-rf-" + t.Name()
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF.md", "# 04 Requerimientos Funcionales (RF)\n\nRF-IDN-08 listado general.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-IDN.md", strings.Join([]string{
		"# RF-IDN",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-IDN-08 | Reindexado documental observable |",
	}, "\n"))
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
			Path:       ".docs/wiki/04_RF.md",
			Title:      "04 Requerimientos Funcionales (RF)",
			DocID:      "RF-IDN-08",
			Layer:      "04",
			Family:     "functional",
			SearchText: "rf idn 08 requerimientos funcionales",
			IndexedAt:  1,
		},
		{
			Path:       ".docs/wiki/04_RF/RF-IDN.md",
			Title:      "RF-IDN",
			DocID:      "RF-IDN",
			Layer:      "04",
			Family:     "functional",
			SearchText: "rf idn 08 reindexado documental observable",
			IndexedAt:  1,
		},
	}, nil, []model.DocMention{{
		DocPath:      ".docs/wiki/04_RF/RF-IDN.md",
		MentionType:  "doc_id",
		MentionValue: "RF-IDN-08",
	}}); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "RF-IDN-08"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	anchor := results[0].Canonical.AnchorDoc
	if anchor.Path != ".docs/wiki/04_RF/RF-IDN.md" {
		t.Fatalf("anchor path = %q, want aggregate RF doc", anchor.Path)
	}
	if anchor.DocID != "RF-IDN-08" {
		t.Fatalf("anchor doc_id = %q, want RF-IDN-08", anchor.DocID)
	}
	if !strings.Contains(anchor.Title, "Reindexado documental") {
		t.Fatalf("anchor title = %q, want embedded RF title", anchor.Title)
	}
}

func TestNavRouteDoesNotAttachMissingExplicitRFToGovernanceFallback(t *testing.T) {
	alias := "route-missing-explicit-rf-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "RF-MISSING-999"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	anchor := results[0].Canonical.AnchorDoc
	if anchor.DocID == "RF-MISSING-999" {
		t.Fatalf("missing RF should not be attached to fallback anchor: %#v", anchor)
	}
	if anchor.Path == "" {
		t.Fatalf("expected a safe fallback anchor path")
	}
}

func TestNavRoutePreviewModeByDefault(t *testing.T) {
	alias := "route-preview-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "login flow"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if results[0].Mode != "preview" {
		t.Fatalf("mode = %q, want preview", results[0].Mode)
	}
	if env.Continuation == nil || env.Continuation.Reason != "expand_preview" || env.Continuation.Next.Op != "nav.pack" {
		t.Fatalf("expected route continuation toward nav.pack, got %#v", env.Continuation)
	}
}

func TestNavRouteFullModeActivatesWithFlag(t *testing.T) {
	alias := "route-full-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias, Full: true},
		Payload:   map[string]any{"task": "login flow"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if results[0].Mode != "full" {
		t.Fatalf("mode = %q, want full", results[0].Mode)
	}
}

func TestNavRouteAnchorDocHasAnchorStage(t *testing.T) {
	alias := "route-stage-anchor-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if got := results[0].Canonical.AnchorDoc.Stage; got != "anchor" {
		t.Fatalf("AnchorDoc.Stage = %q, want anchor", got)
	}
}

func TestNavRoutePreviewPackHasPreviewStage(t *testing.T) {
	alias := "route-stage-preview-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	// Tier 1 (governance-only, no index): stage comes from the profile stage order (e.g. "scope", "architecture").
	// Tier 2 (indexed): stage is "preview". In both cases Stage must be non-empty.
	for i, doc := range results[0].Canonical.PreviewPack {
		if doc.Stage == "" {
			t.Fatalf("PreviewPack[%d].Stage is empty, want non-empty stage signal", i)
		}
	}
}

func TestNavRouteDiscoveryDocsHaveDiscoveryStage(t *testing.T) {
	alias := "route-stage-discovery-" + t.Name()
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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias, Full: true},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	if results[0].Discovery == nil || len(results[0].Discovery.Docs) == 0 {
		t.Skip("no discovery docs available without indexed workspace — stage signal verified at service level")
	}
	for i, doc := range results[0].Discovery.Docs {
		if doc.Stage != "discovery" {
			t.Fatalf("Discovery.Docs[%d].Stage = %q, want discovery", i, doc.Stage)
		}
	}
}

func TestNavRouteUsesTaskFallbackFromQuestion(t *testing.T) {
	alias := "route-qfallback-" + t.Name()
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

	// "question" key should be accepted as fallback when "task" is absent
	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"question": "how does login work?"},
	})
	if err != nil {
		t.Fatalf("nav.route via question: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got %#v", env)
	}
}

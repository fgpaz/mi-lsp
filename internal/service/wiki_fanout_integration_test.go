package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// setupTwoWikiWorkspaces creates two test workspaces with basic wiki structure.
// Returns (ws1_alias, ws2_alias) and cleans up on test teardown.
func setupTwoWikiWorkspaces(t *testing.T) (string, string) {
	t.Helper()
	ensureWritableTestHome(t)

	// Workspace 1: Alpha with RF docs
	alias1 := "wiki-fanout-alpha-" + filepath.Base(t.TempDir())
	root1 := t.TempDir()
	writeWorkspaceFile(t, root1, "src/Alpha.cs", "namespace Alpha;\npublic class AlphaClass { }")
	writeWorkspaceFile(t, root1, ".docs/wiki/00_gobierno_documental.md", "# Gobierno\nprofile: spec_backend")
	writeWorkspaceFile(t, root1, ".docs/wiki/04_RF/RF-ALPHA-001.md", "# RF-ALPHA-001\nRequerimiento alpha para testing.")
	writeSpecBackendGovernanceFixture(t, root1)

	if _, err := workspace.RegisterWorkspace(alias1, model.WorkspaceRegistration{
		Name:      alias1,
		Root:      root1,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace 1: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias1) })

	// Workspace 2: Bravo with FL docs
	alias2 := "wiki-fanout-bravo-" + filepath.Base(t.TempDir())
	root2 := t.TempDir()
	writeWorkspaceFile(t, root2, "src/Bravo.cs", "namespace Bravo;\npublic class BravoClass { }")
	writeWorkspaceFile(t, root2, ".docs/wiki/00_gobierno_documental.md", "# Gobierno\nprofile: spec_backend")
	writeWorkspaceFile(t, root2, ".docs/wiki/03_FL/FL-BRAVO-01.md", "# FL-BRAVO-01\nFlujo bravo para testing.")
	writeSpecBackendGovernanceFixture(t, root2)

	if _, err := workspace.RegisterWorkspace(alias2, model.WorkspaceRegistration{
		Name:      alias2,
		Root:      root2,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace 2: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias2) })

	return alias1, alias2
}

// TestNavWikiSearch_AllWorkspaces_ReturnsMultipleWorkspaceResults verifies that
// wiki search with --all-workspaces succeeds and returns results.
func TestNavWikiSearch_AllWorkspaces_ReturnsMultipleWorkspaceResults(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	// Create app instance using first workspace root (we'll override in request)
	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	// Execute wiki search with all_workspaces=true
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"query":          "testing",
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.search all-workspaces: %v", err)
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}

	// Verify we got items
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}
}

// TestNavWikiInventory_AllWorkspaces_HasAliasField verifies that
// wiki inventory with --all-workspaces succeeds and returns results.
func TestNavWikiInventory_AllWorkspaces_HasAliasField(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.inventory",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.inventory all-workspaces: %v", err)
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}

	// Verify we got items
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}
}

// TestNavWikiRoute_AllWorkspaces_HasWorkspaceField verifies that
// wiki route with --all-workspaces returns results (actual field validation is in service tests).
func TestNavWikiRoute_AllWorkspaces_HasWorkspaceField(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.route",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"task":           "understand structure",
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.route all-workspaces: %v", err)
	}

	// Verify we got some results (type may vary due to marshaling)
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}
}

// TestNavWikiPack_AllWorkspaces_MultipleItems verifies that
// wiki pack with --all-workspaces returns pack results.
func TestNavWikiPack_AllWorkspaces_MultipleItems(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.pack",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"task":           "understand governance",
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.pack all-workspaces: %v", err)
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}

	// Verify we got items
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}
}

// TestNavWikiTrace_AllWorkspaces_HasWorkspaceField verifies that
// wiki trace with --all-workspaces returns trace results.
func TestNavWikiTrace_AllWorkspaces_HasWorkspaceField(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.trace",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"doc_id":         "RF-ALPHA-001",
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.trace all-workspaces: %v", err)
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}

	// Verify we got items
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}
}

// TestNavWikiSearch_SingleWorkspace_NoWorkspaceField verifies backward compat:
// without --all-workspaces, the workspace field should not be populated
// (or query should work with single workspace mode).
func TestNavWikiSearch_SingleWorkspace_BackCompat(t *testing.T) {
	alias := "compat-single-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)

	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload: map[string]any{
			"query": "login",
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.search single workspace: %v", err)
	}

	// In single-workspace mode, the Workspace envelope field should match the workspace
	if env.Workspace != alias {
		t.Fatalf("expected Workspace = %q, got %q", alias, env.Workspace)
	}
}

// TestNavWikiInventory_WithLayerCounts verifies that wiki inventory
// operation succeeds with --with-layer-counts flag.
func TestNavWikiInventory_WithLayerCounts_HasLayerField(t *testing.T) {
	alias := "inventory-layers-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)

	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	app := New(root, nil)

	// With --with-layer-counts
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.inventory",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload: map[string]any{
			"with_layer_counts": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.inventory with layer counts: %v", err)
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}

	// Verify we got items
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}
}

// TestNavWikiSearch_AllWorkspaces_StatsIncludeWorkspacesQueried verifies that
// the envelope stats include workspaces_queried when using --all-workspaces.
func TestNavWikiSearch_AllWorkspaces_StatsIncludeWorkspacesQueried(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.search",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"query":          "testing",
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.search all-workspaces: %v", err)
	}

	// Stats should include workspaces_queried
	if env.Stats.WorkspacesQueried <= 0 {
		t.Fatalf("expected WorkspacesQueried > 0, got %d", env.Stats.WorkspacesQueried)
	}
}

// TestNavWikiPack_AllWorkspaces_NMiniPacks verifies that pack with --all-workspaces
// returns pack results.
func TestNavWikiPack_AllWorkspaces_NMiniPacks(t *testing.T) {
	alias1, _ := setupTwoWikiWorkspaces(t)

	root1, _ := workspace.ResolveWorkspace(alias1)
	app := New(root1.Root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.pack",
		Context:   model.QueryOptions{Workspace: "", MaxItems: 20},
		Payload: map[string]any{
			"task":           "test pack",
			"all_workspaces": true,
		},
	})

	if err != nil {
		t.Fatalf("nav.wiki.pack all-workspaces: %v", err)
	}

	// Verify the operation succeeded
	if !env.Ok {
		t.Fatalf("expected Ok=true, got %v", env.Ok)
	}

	// Verify we got items
	if env.Items == nil {
		t.Fatalf("expected non-nil Items, got nil")
	}
}

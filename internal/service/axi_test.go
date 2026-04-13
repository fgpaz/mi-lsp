package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestWorkspaceInitAXIModePrefersAXINextSteps(t *testing.T) {
	alias := "axi-init-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{AXI: true},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	items := env.Items.([]map[string]any)
	nextSteps := items[0]["next_steps"].([]string)
	if len(nextSteps) < 5 {
		t.Fatalf("expected five AXI next steps, got %#v", nextSteps)
	}
	if strings.Contains(nextSteps[0], "--axi") {
		t.Fatalf("expected workspace status step to rely on default AXI, got %q", nextSteps[0])
	}
	if strings.Contains(nextSteps[1], "--axi") {
		t.Fatalf("expected governance step to stay direct, got %q", nextSteps[1])
	}
	// nextSteps[2] is now nav route (preview-first orientation)
	if !strings.Contains(nextSteps[2], "nav route") {
		t.Fatalf("expected nav route as step 2, got %q", nextSteps[2])
	}
	// nextSteps[3] is nav ask (orientation fallback)
	if strings.Contains(nextSteps[3], "--axi") {
		t.Fatalf("expected orientation ask step to rely on default AXI, got %q", nextSteps[3])
	}
	// nextSteps[4] is the full expansion step (workspace-map)
	if !strings.Contains(nextSteps[4], "--axi") || !strings.Contains(nextSteps[4], "--full") {
		t.Fatalf("expected expansion step to include explicit --axi --full, got %q", nextSteps[4])
	}
}

func TestWorkspaceStatusAXIPreviewAddsGuidance(t *testing.T) {
	alias := "axi-status-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
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
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias, AXI: true},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}

	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["view"] != "preview" {
		t.Fatalf("view = %#v, want preview", item["view"])
	}
	if _, ok := item["next_steps"].([]string); !ok {
		t.Fatalf("expected next_steps in preview item, got %#v", item)
	}
	if _, exists := item["repos"]; exists {
		t.Fatalf("preview should not include repos list, got %#v", item["repos"])
	}
}

func TestWorkspaceStatusAXIFullKeepsExpandedDetails(t *testing.T) {
	alias := "axi-status-full-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
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
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, Full: true},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}

	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["view"] != "full" {
		t.Fatalf("view = %#v, want full", item["view"])
	}
	if _, ok := item["repos"]; !ok {
		t.Fatalf("expected repos list in full mode, got %#v", item)
	}
}

func TestNavAskAXIPreviewSuggestsFullAndKeepsUsefulNextQueries(t *testing.T) {
	alias := "axi-ask-" + filepath.Base(t.TempDir())
	root := createLinkedDocsWorkspaceFixture(t, alias)
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
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 5},
		Payload:   map[string]any{"question": "RF-QRY-010"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}

	if env.NextHint == nil || !strings.Contains(*env.NextHint, "--full") {
		t.Fatalf("expected full expansion next_hint, got %#v", env.NextHint)
	}
	results := env.Items.([]model.AskResult)
	if len(results) != 1 {
		t.Fatalf("expected one ask result, got %#v", env.Items)
	}
	if len(results[0].DocEvidence) > 2 {
		t.Fatalf("preview should trim doc evidence, got %#v", results[0].DocEvidence)
	}
	if len(results[0].NextQueries) == 0 {
		t.Fatalf("expected next queries, got %#v", results[0])
	}
	if !strings.Contains(results[0].NextQueries[0], "nav search") {
		t.Fatalf("expected first next query to stay discovery-friendly, got %q", results[0].NextQueries[0])
	}
	for _, query := range results[0].NextQueries {
		if strings.Contains(query, "--classic") {
			t.Fatalf("did not expect classic override in next query, got %q", query)
		}
	}
}

func TestWorkspaceMapAXIPreviewSkipsExpansionHintWhenNothingWasTrimmed(t *testing.T) {
	alias := "axi-map-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
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
		Operation: "nav.workspace-map",
		Context:   model.QueryOptions{Workspace: alias, AXI: true},
	})
	if err != nil {
		t.Fatalf("nav.workspace-map: %v", err)
	}

	if env.Hint != "" {
		t.Fatalf("expected no preview hint when preview did not trim, got %q", env.Hint)
	}
	if env.NextHint != nil {
		t.Fatalf("expected no preview next_hint when preview did not trim, got %#v", env.NextHint)
	}
}

func TestSearchAXIPreviewSuggestsFullExpansion(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 5},
		Payload:   map[string]any{"pattern": "HelloWorld"},
	})
	if err != nil {
		t.Fatalf("nav.search: %v", err)
	}

	if env.NextHint == nil || !strings.Contains(*env.NextHint, "--full") {
		t.Fatalf("expected full expansion next_hint, got %#v", env.NextHint)
	}
}

func TestIntentAXIPreviewSuggestsFullExpansion(t *testing.T) {
	alias := "axi-intent-" + filepath.Base(t.TempDir())
	root := createContainerWorkspaceFixture(t, alias)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: root, AXI: true, MaxItems: 5},
		Payload:   map[string]any{"question": "password reset backend", "top": 5, "repo": "backend"},
	})
	if err != nil {
		t.Fatalf("nav.intent: %v", err)
	}

	if env.NextHint == nil || !strings.Contains(*env.NextHint, "--full") {
		t.Fatalf("expected full expansion next_hint, got %#v", env.NextHint)
	}
}

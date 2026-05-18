package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavAffectedFromGitDiffIncludesUntrackedGoFileTestAndDocs(t *testing.T) {
	alias := "affected-ws-" + filepath.Base(t.TempDir())
	root := createDiffWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	writeWorkspaceFile(t, root, "internal/service/affected.go", "package service\n\nfunc ChangedImpactSelector() {}\n")

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"from_git_diff": true,
			"changed_ref":   "HEAD",
			"include_tests": true,
			"include_docs":  true,
		},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}

	items := affectedItemsFromEnvelope(t, env)
	assertAffectedItem(t, items, "code", "internal/service/affected.go", "")
	assertAffectedItem(t, items, "test", "internal/service", "go test ./internal/service")
	assertAffectedItem(t, items, "doc", ".docs/wiki/04_RF/RF-QRY-017.md", "")
	if !containsWarning(env.Warnings, affectedHeuristicWarning) {
		t.Fatalf("expected heuristic warning, got %#v", env.Warnings)
	}
}

func TestNavAffectedParsesStdinAndUsesOverrideTestCommand(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"stdin":         `["internal/store/schema.go","internal/store/schema.go","internal/cli/nav.go"]`,
			"include_tests": true,
			"include_docs":  true,
			"test_command":  "go test ./internal/store -run TestStore",
		},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}

	items := affectedItemsFromEnvelope(t, env)
	assertAffectedItem(t, items, "code", "internal/store/schema.go", "")
	assertAffectedItem(t, items, "code", "internal/cli/nav.go", "")
	assertAffectedItem(t, items, "test", "internal/store", "go test ./internal/store -run TestStore")
	assertAffectedItem(t, items, "doc", ".docs/wiki/08_modelo_fisico_datos.md", "")
	assertAffectedItem(t, items, "doc", ".docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md", "")
	if len(items) == 0 {
		t.Fatal("expected affected items")
	}
}

func TestNavAffectedNoChangesQuietHasNoHint(t *testing.T) {
	alias := "affected-clean-ws-" + filepath.Base(t.TempDir())
	root := createDiffWorkspaceFixture(t, alias)
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
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"from_git_diff": true,
			"changed_ref":   "HEAD",
			"quiet":         true,
		},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}
	items := affectedItemsFromEnvelope(t, env)
	if len(items) != 0 {
		t.Fatalf("expected no affected items, got %#v", items)
	}
	if env.Hint != "" {
		t.Fatalf("quiet no-change hint = %q, want empty", env.Hint)
	}
	if !containsWarning(env.Warnings, "no affected paths detected") {
		t.Fatalf("expected no-change warning, got %#v", env.Warnings)
	}
}

func affectedItemsFromEnvelope(t *testing.T, env model.Envelope) []AffectedItem {
	t.Helper()
	items, ok := env.Items.([]AffectedItem)
	if !ok {
		t.Fatalf("expected []AffectedItem, got %#v", env.Items)
	}
	return items
}

func assertAffectedItem(t *testing.T, items []AffectedItem, kind string, path string, suggestedCommand string) {
	t.Helper()
	for _, item := range items {
		if item.Kind != kind || item.Path != path {
			continue
		}
		if suggestedCommand != "" && item.SuggestedCommand != suggestedCommand {
			t.Fatalf("item %s/%s command = %q, want %q", kind, path, item.SuggestedCommand, suggestedCommand)
		}
		if item.Reason == "" || item.Confidence <= 0 {
			t.Fatalf("item %s/%s missing stable reason/confidence: %#v", kind, path, item)
		}
		return
	}
	t.Fatalf("missing affected item kind=%s path=%s command=%s in %#v", kind, path, suggestedCommand, items)
}

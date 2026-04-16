package cli

import (
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestRecordOperationPersistsResolvedWorkspaceWhenSelectorIsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	if _, err := workspace.RegisterWorkspace("interbancarizacion_coelsa", model.WorkspaceRegistration{
		Name:      "interbancarizacion_coelsa",
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() {
		_ = workspace.RemoveWorkspace("interbancarizacion_coelsa")
	})

	telemetry := NewCLITelemetry("test-cli", "session-empty-selector", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	request := model.CommandRequest{
		Operation: "nav.ask",
		Context: model.QueryOptions{
			Workspace: "",
			Format:    "toon",
		},
	}
	envelope := model.Envelope{
		Ok:        true,
		Workspace: "interbancarizacion_coelsa",
		Backend:   "ask",
		Items:     []map[string]any{{"summary": "ok"}},
	}

	telemetry.RecordOperation(request, envelope, nil, 15*time.Millisecond, "direct")

	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		t.Fatalf("OpenTelemetryStore: %v", err)
	}
	defer store.Close()

	events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{SessionID: "session-empty-selector", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	got := events[0]
	if got.WorkspaceInput != "" {
		t.Fatalf("workspace_input = %q, want empty string", got.WorkspaceInput)
	}
	if got.Workspace != "interbancarizacion_coelsa" {
		t.Fatalf("workspace = %q, want interbancarizacion_coelsa", got.Workspace)
	}
	if got.WorkspaceAlias != "interbancarizacion_coelsa" {
		t.Fatalf("workspace_alias = %q, want interbancarizacion_coelsa", got.WorkspaceAlias)
	}
	if got.WorkspaceRoot != root {
		t.Fatalf("workspace_root = %q, want %q", got.WorkspaceRoot, root)
	}
	if got.RuntimeKey != "ask::interbancarizacion_coelsa::default" {
		t.Fatalf("runtime_key = %q, want ask::interbancarizacion_coelsa::default", got.RuntimeKey)
	}
}

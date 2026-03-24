package cli

import (
	"errors"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRecordOperationInfersBackendForFailedContextRequests(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	telemetry := NewCLITelemetry("test-cli", "session-1", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	telemetry.RecordOperation(model.CommandRequest{
		Operation: "nav.context",
		Context:   model.QueryOptions{Workspace: "multi-tedi"},
		Payload:   map[string]any{"file": "src/frontend/web/app/page.tsx", "line": 43},
	}, model.Envelope{}, errors.New("boom"), 42*time.Millisecond)

	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		t.Fatalf("OpenTelemetryStore: %v", err)
	}
	defer store.Close()

	events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{Workspace: "multi-tedi", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Backend != "tsserver" {
		t.Fatalf("backend = %q, want tsserver", events[0].Backend)
	}
}

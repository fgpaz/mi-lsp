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
	}, model.Envelope{}, errors.New("boom"), 42*time.Millisecond, "direct")

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

func TestRecordOperationPersistsRouteAndQueryBudgetMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	telemetry := NewCLITelemetry("test-cli", "session-2", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	request := model.CommandRequest{
		Operation: "nav.search",
		Context: model.QueryOptions{
			Workspace:   "multi-tedi",
			Format:      "toon",
			TokenBudget: 1234,
			MaxItems:    7,
			MaxChars:    456,
			Compress:    true,
		},
		Payload: map[string]any{"pattern": "handler"},
	}
	envelope := model.Envelope{
		Ok:        true,
		Backend:   "text",
		Items:     []map[string]any{{"file": "src/handler.go", "line": 10}},
		Truncated: true,
	}

	telemetry.RecordOperation(request, envelope, nil, 25*time.Millisecond, "direct_fallback")

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

	got := events[0]
	if got.Route != "direct_fallback" {
		t.Fatalf("route = %q, want direct_fallback", got.Route)
	}
	if got.Format != "toon" {
		t.Fatalf("format = %q, want toon", got.Format)
	}
	if got.TokenBudget != 1234 {
		t.Fatalf("token_budget = %d, want 1234", got.TokenBudget)
	}
	if got.MaxItems != 7 {
		t.Fatalf("max_items = %d, want 7", got.MaxItems)
	}
	if got.MaxChars != 456 {
		t.Fatalf("max_chars = %d, want 456", got.MaxChars)
	}
	if !got.Compress {
		t.Fatal("compress = false, want true")
	}
	if !got.Truncated {
		t.Fatal("truncated = false, want true")
	}
	if got.ResultCount != 1 {
		t.Fatalf("result_count = %d, want 1", got.ResultCount)
	}
}

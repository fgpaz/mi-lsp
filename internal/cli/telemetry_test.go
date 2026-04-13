package cli

import (
	"encoding/json"
	"errors"
	"strings"
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

func TestRecordOperationCapturesSearchRoutingDiagnostics(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	telemetry := NewCLITelemetry("test-cli", "session-3", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	request := model.CommandRequest{
		Operation: "nav.search",
		Context: model.QueryOptions{
			Workspace:   "multi-tedi",
			Format:      "toon",
			TokenBudget: 256,
			MaxItems:    1,
			MaxChars:    512,
			Compress:    true,
		},
		Payload: map[string]any{"pattern": "billing retry", "repo": "frontend"},
	}
	envelope := model.Envelope{
		Ok:        true,
		Backend:   "text",
		Items:     []map[string]any{{"file": "frontend/src/billing.ts", "line": 10}},
		Truncated: true,
	}

	telemetry.RecordOperation(request, envelope, nil, 25*time.Millisecond, "direct")

	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		t.Fatalf("OpenTelemetryStore: %v", err)
	}
	defer store.Close()

	events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{SessionID: "session-3", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	got := events[0]
	if got.WarningCount != 0 {
		t.Fatalf("warning_count = %d, want 0", got.WarningCount)
	}
	if got.PatternMode != "literal" {
		t.Fatalf("pattern_mode = %q, want literal", got.PatternMode)
	}
	if got.RoutingOutcome != "narrowed_repo" {
		t.Fatalf("routing_outcome = %q, want narrowed_repo", got.RoutingOutcome)
	}
	if got.FailureStage != "none" {
		t.Fatalf("failure_stage = %q, want none", got.FailureStage)
	}
	if got.TruncationReason != "max_items" {
		t.Fatalf("truncation_reason = %q, want max_items", got.TruncationReason)
	}
	if strings.Contains(got.DecisionJSON, "billing retry") {
		t.Fatalf("decision_json leaked raw pattern: %s", got.DecisionJSON)
	}

	var decision map[string]any
	if err := json.Unmarshal([]byte(got.DecisionJSON), &decision); err != nil {
		t.Fatalf("Unmarshal(decision_json): %v", err)
	}
	if decision["pattern_len"] != float64(len("billing retry")) {
		t.Fatalf("pattern_len = %#v, want %d", decision["pattern_len"], len("billing retry"))
	}
	if decision["pattern_has_spaces"] != true {
		t.Fatalf("pattern_has_spaces = %#v, want true", decision["pattern_has_spaces"])
	}
	if decision["selector_present"] != true {
		t.Fatalf("selector_present = %#v, want true", decision["selector_present"])
	}
	if decision["repo_selector_valid"] != true {
		t.Fatalf("repo_selector_valid = %#v, want true", decision["repo_selector_valid"])
	}
}

func TestRecordOperationCapturesRouterHintDiagnostics(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	telemetry := NewCLITelemetry("test-cli", "session-4", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	nextHint := "rerun with --repo <name>"
	request := model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: "multi-tedi"},
		Payload:   map[string]any{"pattern": "PasswordResetService", "exact": true, "repo": "missing"},
	}
	envelope := model.Envelope{
		Ok:       false,
		Backend:  "router",
		Warnings: []string{`unknown repo selector "missing"`},
		NextHint: &nextHint,
	}

	telemetry.RecordOperation(request, envelope, nil, 10*time.Millisecond, "direct")

	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		t.Fatalf("OpenTelemetryStore: %v", err)
	}
	defer store.Close()

	events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{SessionID: "session-4", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	got := events[0]
	if got.WarningCount != 1 {
		t.Fatalf("warning_count = %d, want 1", got.WarningCount)
	}
	if got.PatternMode != "none" {
		t.Fatalf("pattern_mode = %q, want none", got.PatternMode)
	}
	if got.RoutingOutcome != "router_error" {
		t.Fatalf("routing_outcome = %q, want router_error", got.RoutingOutcome)
	}
	if got.FailureStage != "selector_validation" {
		t.Fatalf("failure_stage = %q, want selector_validation", got.FailureStage)
	}
	if got.HintCode != "repo_selector_invalid" {
		t.Fatalf("hint_code = %q, want repo_selector_invalid", got.HintCode)
	}
	if strings.Contains(got.DecisionJSON, "PasswordResetService") || strings.Contains(got.DecisionJSON, "missing") {
		t.Fatalf("decision_json leaked raw selector/pattern: %s", got.DecisionJSON)
	}
}

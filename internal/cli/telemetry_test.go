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

func TestRecordOperationCapturesIntentModeAndDocRanker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MI_LSP_DOC_RANKING", "legacy")

	telemetry := NewCLITelemetry("test-cli", "session-intent-docs", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	request := model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: "mi-lsp", MaxItems: 5},
		Payload:   map[string]any{"question": "how do continuation and memory pointer work?"},
	}
	envelope := model.Envelope{
		Ok:        true,
		Backend:   "intent",
		Mode:      "docs",
		Items:     []map[string]any{{"doc_path": ".docs/wiki/09_contratos/CT-NAV-ASK.md", "doc_id": "CT-NAV-ASK"}},
		Warnings:  []string{"repo selector applies only to code mode; ignored after docs classification"},
		Truncated: false,
	}

	telemetry.RecordOperation(request, envelope, nil, 12*time.Millisecond, "direct")

	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		t.Fatalf("OpenTelemetryStore: %v", err)
	}
	defer store.Close()

	events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{SessionID: "session-intent-docs", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	var decision map[string]any
	if err := json.Unmarshal([]byte(events[0].DecisionJSON), &decision); err != nil {
		t.Fatalf("Unmarshal(decision_json): %v", err)
	}
	if decision["doc_ranker"] != "legacy" {
		t.Fatalf("doc_ranker = %#v, want legacy", decision["doc_ranker"])
	}
	if decision["intent_mode"] != "docs" {
		t.Fatalf("intent_mode = %#v, want docs", decision["intent_mode"])
	}
}

func TestRecordOperationCapturesCoachMetadataWithoutLeakingCoachContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	telemetry := NewCLITelemetry("test-cli", "session-coach", false)
	if telemetry == nil {
		t.Fatal("expected telemetry instance")
	}
	defer telemetry.Close()

	request := model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: "mi-lsp"},
		Payload:   map[string]any{"question": "how does daemon routing work?"},
	}
	envelope := model.Envelope{
		Ok:      true,
		Backend: "ask",
		Items: []model.AskResult{{
			Summary:    "Thin answer",
			PrimaryDoc: model.AskDocEvidence{Path: ".docs/wiki/07_baseline_tecnica.md"},
		}},
		Coach: &model.Coach{
			Trigger:    "low_confidence",
			Message:    "The answer is usable, but the supporting evidence is still thin.",
			Confidence: "low",
			Actions: []model.CoachAction{{
				Kind:    "refine",
				Label:   "Search supporting code",
				Command: `mi-lsp nav search "daemon" --workspace mi-lsp --include-content`,
			}},
		},
		Continuation: &model.Continuation{
			Reason: "low_evidence",
			Next: model.ContinuationTarget{
				Op:    "nav.search",
				Query: "daemon",
				DocID: "TECH-DAEMON",
			},
		},
		MemoryPointer: &model.MemoryPointer{
			DocID:     "TECH-DAEMON",
			Why:       "Daemon contract changed recently.",
			ReentryOp: "nav.search",
			Handoff:   "plans/2026-04-16-reentry-wave",
			Stale:     true,
		},
	}

	telemetry.RecordOperation(request, envelope, nil, 18*time.Millisecond, "direct")

	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		t.Fatalf("OpenTelemetryStore: %v", err)
	}
	defer store.Close()

	events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{SessionID: "session-coach", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	got := events[0]
	if got.HintCode != "low_confidence" {
		t.Fatalf("hint_code = %q, want low_confidence", got.HintCode)
	}
	if strings.Contains(got.DecisionJSON, "supporting evidence is still thin") || strings.Contains(got.DecisionJSON, "nav search") || strings.Contains(got.DecisionJSON, "Daemon contract changed recently") {
		t.Fatalf("decision_json leaked coach content: %s", got.DecisionJSON)
	}

	var decision map[string]any
	if err := json.Unmarshal([]byte(got.DecisionJSON), &decision); err != nil {
		t.Fatalf("Unmarshal(decision_json): %v", err)
	}
	if decision["coach_present"] != true {
		t.Fatalf("coach_present = %#v, want true", decision["coach_present"])
	}
	if decision["coach_trigger"] != "low_confidence" {
		t.Fatalf("coach_trigger = %#v, want low_confidence", decision["coach_trigger"])
	}
	if decision["coach_action_count"] != float64(1) {
		t.Fatalf("coach_action_count = %#v, want 1", decision["coach_action_count"])
	}
	if decision["continuation_present"] != true {
		t.Fatalf("continuation_present = %#v, want true", decision["continuation_present"])
	}
	if decision["continuation_reason"] != "low_evidence" {
		t.Fatalf("continuation_reason = %#v, want low_evidence", decision["continuation_reason"])
	}
	if decision["continuation_op"] != "nav.search" {
		t.Fatalf("continuation_op = %#v, want nav.search", decision["continuation_op"])
	}
	if decision["memory_pointer_present"] != true {
		t.Fatalf("memory_pointer_present = %#v, want true", decision["memory_pointer_present"])
	}
	if decision["memory_stale"] != true {
		t.Fatalf("memory_stale = %#v, want true", decision["memory_stale"])
	}
}

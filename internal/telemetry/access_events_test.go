package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestResolveWorkspaceIdentity_PrefersRootAndPreservesInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	if _, err := workspace.RegisterWorkspace("multi-tedi", model.WorkspaceRegistration{Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace(alias): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace("multi-tedi") })

	identity := ResolveWorkspaceIdentity(root)
	if identity.Input != root {
		t.Fatalf("Input = %q, want %q", identity.Input, root)
	}
	if identity.Root != root {
		t.Fatalf("Root = %q, want %q", identity.Root, root)
	}
	if identity.Alias != "multi-tedi" {
		t.Fatalf("Alias = %q, want multi-tedi", identity.Alias)
	}
	if identity.Display != "multi-tedi" {
		t.Fatalf("Display = %q, want multi-tedi", identity.Display)
	}
}

func TestClassifyErrorInfo_DetectsGlobalJSONMismatch(t *testing.T) {
	info := ClassifyErrorInfo("roslyn", strings.Join([]string{
		"A compatible .NET SDK was not found.",
		"Requested SDK version: 10.0.201",
		"global.json file: C:\\repos\\mios\\gastos\\backend\\global.json",
	}, "\n"), nil)

	if info.Kind != "sdk" {
		t.Fatalf("Kind = %q, want sdk", info.Kind)
	}
	if info.Code != "dotnet_global_json_mismatch" {
		t.Fatalf("Code = %q, want dotnet_global_json_mismatch", info.Code)
	}
}

func TestRuntimeKeyForOperationUsesResolvedWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	if _, err := workspace.RegisterWorkspace("alias-one", model.WorkspaceRegistration{Name: "alias-one", Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace(alias-one): %v", err)
	}
	if _, err := workspace.RegisterWorkspace("alias-two", model.WorkspaceRegistration{Name: "alias-two", Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace(alias-two): %v", err)
	}

	left := RuntimeKeyForOperation(
		model.CommandRequest{Operation: "nav.refs", Context: model.QueryOptions{Workspace: "alias-one"}, Payload: map[string]any{"entrypoint": "default"}},
		model.Envelope{Workspace: "alias-one", Backend: "roslyn"},
	)
	right := RuntimeKeyForOperation(
		model.CommandRequest{Operation: "nav.refs", Context: model.QueryOptions{Workspace: "alias-two"}, Payload: map[string]any{"entrypoint": "default"}},
		model.Envelope{Workspace: "alias-two", Backend: "roslyn"},
	)
	if left != right {
		t.Fatalf("runtime key should collapse aliases for same root: %q vs %q", left, right)
	}
	if !strings.Contains(left, root) {
		t.Fatalf("runtime key = %q, want root %q", left, root)
	}
}

func TestEnrichAccessEventUsesEnvelopeErrorTyping(t *testing.T) {
	event := EnrichAccessEvent(model.AccessEvent{
		OccurredAt: time.Now(),
		Operation:  "nav.search",
		Backend:    "text",
		Route:      "direct",
	}, model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{MaxItems: 50},
		Payload:   map[string]any{"pattern": "missing"},
	}, model.Envelope{
		Ok:      false,
		Backend: "text",
		Items:   []map[string]any{},
		Error: &model.EnvelopeError{
			Kind:     "workspace",
			Code:     "workspace_resolution_failed",
			Message:  "workspace missing is not registered",
			Stage:    "selector_validation",
			HintCode: "workspace_resolution_failed",
		},
	}, nil)

	if event.ErrorKind != "workspace" || event.ErrorCode != "workspace_resolution_failed" {
		t.Fatalf("error typing = %q/%q, want workspace/workspace_resolution_failed", event.ErrorKind, event.ErrorCode)
	}
	if event.FailureStage != "selector_validation" {
		t.Fatalf("failure_stage = %q, want selector_validation", event.FailureStage)
	}
	if event.HintCode != "workspace_resolution_failed" {
		t.Fatalf("hint_code = %q, want workspace_resolution_failed", event.HintCode)
	}
}

func TestClassifyErrorInfo_DetectsDotnetSDKMissing(t *testing.T) {
	info := ClassifyErrorInfo("roslyn", "It was not possible to find any installed .NET SDKs", nil)

	if info.Kind != "sdk" {
		t.Fatalf("Kind = %q, want sdk", info.Kind)
	}
	if info.Code != "dotnet_sdk_missing" {
		t.Fatalf("Code = %q, want dotnet_sdk_missing", info.Code)
	}
}

func TestClassifyErrorInfo_DetectsProcessSpawnAccessDenied(t *testing.T) {
	info := ClassifyErrorInfo("roslyn", "CreateProcess C:\\tools\\dotnet.exe: Access is denied", nil)

	if info.Kind != "backend_runtime" {
		t.Fatalf("Kind = %q, want backend_runtime", info.Kind)
	}
	if info.Code != "process_spawn_access_denied" {
		t.Fatalf("Code = %q, want process_spawn_access_denied", info.Code)
	}
}

func TestClassifyErrorInfo_DetectsEditPlanErrors(t *testing.T) {
	tests := []struct {
		name    string
		message string
		code    string
	}{
		{name: "invalid packet", message: "invalid edit-plan packet JSON: unexpected EOF", code: "qry_edit_plan_invalid_packet"},
		{name: "unsafe path", message: "target target-main: path denied by .git/**", code: "qry_edit_plan_unsafe_path"},
		{name: "hash mismatch", message: "target target-main: expected_hash mismatch", code: "qry_edit_plan_hash_mismatch"},
		{name: "overlap", message: "operation op-b overlaps target range already used by operation op-a", code: "qry_edit_plan_overlap"},
		{name: "language not supported", message: `operation op: language_not_supported: AST backend for language "python" is not implemented`, code: "qry_edit_plan_language_not_supported"},
		{name: "experimental", message: "--apply requires --experimental-apply", code: "qry_edit_plan_apply_requires_experimental"},
		{name: "dirty git", message: "apply requires a clean git workspace; commit or stash changes first", code: "qry_edit_plan_dirty_git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ClassifyErrorInfo("edit-plan", tt.message, nil)
			if info.Kind != "validation" || info.Code != tt.code {
				t.Fatalf("ClassifyErrorInfo = %q/%q, want validation/%s", info.Kind, info.Code, tt.code)
			}
		})
	}
}

func TestResolveWindowPresetRecent(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	window, err := ResolveWindow("recent", now)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}
	if window.Name != "recent" {
		t.Fatalf("Name = %q, want recent", window.Name)
	}
	if got := now.Sub(window.Since); got != 24*time.Hour {
		t.Fatalf("duration = %s, want 24h", got)
	}
}

func TestEnrichAccessEventRedactsRawTelemetryInputs(t *testing.T) {
	secret := "F10_SECRET_QUERY_PROMPT_ARGV_SNIPPET_PATH_SELECTOR"
	event := EnrichAccessEvent(model.AccessEvent{
		Operation: "nav.search",
		Backend:   "text",
		Route:     "direct",
		Repo:      secret,
		Warnings:  []string{secret, secret},
		Error:     secret,
	}, model.CommandRequest{
		Operation: "nav.search",
		Payload: map[string]any{
			"pattern": secret,
			"query":   secret,
			"prompt":  secret,
			"argv":    secret,
			"snippet": secret,
			"path":    secret,
			"repo":    secret,
		},
	}, model.Envelope{
		Ok:      false,
		Backend: "text",
		Items:   []map[string]any{{"snippet": secret, "path": secret}},
		Warnings: []string{
			secret,
			secret,
		},
		Error: &model.EnvelopeError{
			Kind:     "backend_runtime",
			Code:     "search_failed",
			Message:  secret,
			Stage:    "backend",
			HintCode: "search_failed",
		},
	}, nil)

	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal(normalized event): %v", err)
	}
	if strings.Contains(string(body), secret) || strings.Contains(event.DecisionJSON, secret) {
		t.Fatalf("raw telemetry input leaked: event=%s decision=%s", body, event.DecisionJSON)
	}
	if event.Repo != "selected" {
		t.Fatalf("repo = %q, want selected", event.Repo)
	}
	if event.WarningCount != 2 {
		t.Fatalf("warning_count = %d, want original count 2", event.WarningCount)
	}
	if len(event.Warnings) != 1 || event.Warnings[0] != "search_failed" {
		t.Fatalf("warnings = %#v, want deduplicated search_failed", event.Warnings)
	}
	if event.Error != "search_failed" || event.ErrorCode != "search_failed" {
		t.Fatalf("error fields = %q/%q, want stable search_failed", event.Error, event.ErrorCode)
	}
	if event.ErrorKind != "backend_runtime" || event.FailureStage != "backend" || event.HintCode != "search_failed" {
		t.Fatalf("typed diagnostics lost: kind=%q stage=%q hint=%q", event.ErrorKind, event.FailureStage, event.HintCode)
	}
	var decision map[string]any
	if err := json.Unmarshal([]byte(event.DecisionJSON), &decision); err != nil {
		t.Fatalf("Unmarshal(decision_json): %v", err)
	}
	if decision["pattern_len"] != float64(len(secret)) || decision["selector_present"] != true {
		t.Fatalf("typed decision fields lost: %#v", decision)
	}
}

func TestNormalizeAccessEventDropsRawDecisionJSONFields(t *testing.T) {
	secret := "F10_RAW_PROMPT_PATH_SNIPPET"
	event := NormalizeAccessEvent(model.AccessEvent{
		Operation:    "nav.search",
		Backend:      "text",
		DecisionJSON: `{"pattern_len":7,"used_regex":false,"result_source":"` + secret + `","raw_query":"` + secret + `"}`,
	})
	if strings.Contains(event.DecisionJSON, secret) || strings.Contains(event.DecisionJSON, "raw_query") {
		t.Fatalf("raw decision JSON leaked: %s", event.DecisionJSON)
	}
	if event.DecisionJSON != `{"pattern_len":7,"used_regex":false}` {
		t.Fatalf("typed decision JSON = %q, want allowlisted fields", event.DecisionJSON)
	}
}

func TestStableTelemetryCodeUsesClosedAllowlistAndLengthLimit(t *testing.T) {
	for _, code := range []string{
		"search_failed",
		"process_spawn_access_denied",
		"repo_selector_invalid",
		"GPH_QUERY_CURSOR_STALE",
		"scope_narrowing_required",
	} {
		if got := stableTelemetryCode(code); got != code {
			t.Errorf("stableTelemetryCode(%q) = %q, want code to survive", code, got)
		}
	}
	for _, code := range []string{
		"user-selector",
		"secret_query",
		"warning_code_from_prompt",
		strings.Repeat("a", maxStableTelemetryCodeLength+1),
	} {
		if got := stableTelemetryCode(code); got != "" {
			t.Errorf("stableTelemetryCode(%q) = %q, want empty", code, got)
		}
	}
}

func TestNormalizeAccessEventRejectsArbitraryTelemetryCodesEverywhere(t *testing.T) {
	const (
		userSelector = "user-selector"
		secretQuery  = "secret_query"
	)
	event := NormalizeAccessEvent(model.AccessEvent{
		Operation:    "nav.search",
		Backend:      "text",
		Error:        "backend failed",
		ErrorKind:    "backend_runtime",
		ErrorCode:    userSelector,
		HintCode:     secretQuery,
		Warnings:     []string{userSelector, secretQuery, userSelector},
		DecisionJSON: `{"runtime_error_code":"` + userSelector + `","coach_trigger":"` + secretQuery + `","continuation_reason":"` + userSelector + `","planner_outcome":"` + secretQuery + `","safe_degrade_reason":"` + userSelector + `","guardrail_trigger":"` + secretQuery + `","result_source":"` + userSelector + `","pattern_len":4}`,
	})

	if event.Error != "operation_error" || event.ErrorCode != "" || event.HintCode != "" {
		t.Fatalf("arbitrary error codes survived: error=%q error_code=%q hint_code=%q", event.Error, event.ErrorCode, event.HintCode)
	}
	if len(event.Warnings) != 1 || event.Warnings[0] != "warning_present" {
		t.Fatalf("arbitrary warning codes survived: %#v", event.Warnings)
	}
	for _, leaked := range []string{userSelector, secretQuery} {
		if strings.Contains(event.DecisionJSON, leaked) {
			t.Fatalf("arbitrary code %q survived in decision_json: %s", leaked, event.DecisionJSON)
		}
	}
	if !strings.Contains(event.DecisionJSON, `"pattern_len":4`) {
		t.Fatalf("typed decision field was lost: %s", event.DecisionJSON)
	}
}

func TestNormalizeAccessEventPreservesKnownTelemetryCodes(t *testing.T) {
	event := NormalizeAccessEvent(model.AccessEvent{
		Operation:    "nav.context",
		Backend:      "catalog",
		Error:        "worker fallback",
		ErrorKind:    "backend_runtime",
		ErrorCode:    "process_spawn_access_denied",
		HintCode:     "repo_selector_invalid",
		DecisionJSON: `{"runtime_error_code":"process_spawn_access_denied","coach_trigger":"low_confidence","continuation_reason":"low_evidence","planner_outcome":"scope_preview","safe_degrade_reason":"scope_narrowing_required","guardrail_trigger":"scope_narrowing_required"}`,
	})

	if event.Error != "process_spawn_access_denied" || event.ErrorCode != "process_spawn_access_denied" {
		t.Fatalf("known error code was not preserved: error=%q error_code=%q", event.Error, event.ErrorCode)
	}
	if event.HintCode != "repo_selector_invalid" {
		t.Fatalf("known hint code was not preserved: %q", event.HintCode)
	}
	for _, code := range []string{"process_spawn_access_denied", "low_confidence", "low_evidence", "scope_preview", "scope_narrowing_required"} {
		if !strings.Contains(event.DecisionJSON, code) {
			t.Fatalf("known decision code %q was not preserved: %s", code, event.DecisionJSON)
		}
	}
}

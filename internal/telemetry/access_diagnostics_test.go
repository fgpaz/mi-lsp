package telemetry

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestEnrichAccessEventAssignsTypedQFailureCodes(t *testing.T) {
	tests := []struct {
		stage string
		code  string
	}{
		{stage: "parse", code: "q_parse_failed"},
		{stage: "execute", code: "q_execute_failed"},
		{stage: "workspace", code: "q_workspace_failed"},
		{stage: "projection", code: "q_projection_failed"},
	}
	for _, test := range tests {
		t.Run(test.stage, func(t *testing.T) {
			request := model.CommandRequest{Operation: "q", Payload: map[string]any{"q": "text private query"}}
			envelope := model.Envelope{
				Ok:      false,
				Backend: "q",
				Error:   &model.EnvelopeError{Kind: "q", Code: "stage_failed", Message: "stage_failed", Stage: test.stage},
			}
			event := EnrichAccessEvent(model.AccessEvent{Operation: "q", Backend: "q", Success: false, Intent: "text private query"}, request, envelope, nil)
			if event.ErrorCode != test.code || event.FailureStage != test.stage {
				t.Fatalf("typed telemetry = (%q, %q), want (%q, %q)", event.ErrorCode, event.FailureStage, test.code, test.stage)
			}
			if event.Error != test.code {
				t.Fatalf("persisted error contains an unexpected value: %q", event.Error)
			}
			if strings.Contains(event.Intent, "private query") || strings.Contains(event.Error, "private query") {
				t.Fatalf("telemetry persisted raw query text: intent=%q error=%q", event.Intent, event.Error)
			}
			if envelope.Error.Code != "stage_failed" {
				t.Fatalf("q-v1 envelope error code changed: %q", envelope.Error.Code)
			}
		})
	}
}

func TestEnrichAccessEventTypesNavFindCatalogFallback(t *testing.T) {
	tests := []struct {
		code string
	}{
		{code: "nav_find_index_absent"},
		{code: "nav_find_index_unreadable"},
		{code: "nav_find_index_broken"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			request := model.CommandRequest{Operation: "nav.find"}
			envelope := model.Envelope{
				Ok:       true,
				Backend:  "text",
				Warnings: []string{"catalog unavailable; telemetry_code=" + test.code},
			}
			event := EnrichAccessEvent(model.AccessEvent{Operation: "nav.find", Backend: "text", Success: true, Warnings: envelope.Warnings}, request, envelope, nil)
			if event.ErrorKind != "catalog" || event.ErrorCode != test.code || event.HintCode != test.code || event.FailureStage != "catalog" {
				t.Fatalf("typed catalog telemetry = (%q, %q, %q, %q), want catalog/%q/catalog", event.ErrorKind, event.ErrorCode, event.HintCode, event.FailureStage, test.code)
			}
			if len(event.Warnings) != 1 || event.Warnings[0] != test.code {
				t.Fatalf("persisted warnings = %v, want only %q", event.Warnings, test.code)
			}
		})
	}
}

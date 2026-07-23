package telemetry

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestIntentFromRequestEnvelopeHandlesTypedAndSerializedPlans(t *testing.T) {
	request := model.CommandRequest{Operation: "nav.intent", Payload: map[string]any{"intent": "not-allowlisted"}}
	typed := model.Envelope{Items: []model.IntentPlan{{Intent: "callers"}}}
	if got := IntentFromRequestEnvelope(request, typed); got != "callers" {
		t.Fatalf("typed intent=%q", got)
	}
	serialized := model.Envelope{Items: []any{map[string]any{"intent": "callees", "question": "raw must not be persisted"}}}
	if got := IntentFromRequestEnvelope(model.CommandRequest{Operation: "nav.ask"}, serialized); got != "callees" {
		t.Fatalf("serialized intent=%q", got)
	}
	if got := IntentFromRequestEnvelope(model.CommandRequest{Operation: "nav.ask"}, model.Envelope{Items: []any{map[string]any{"intent": "raw-prompt"}}}); got != "unknown" {
		t.Fatalf("invalid serialized intent=%q", got)
	}
}

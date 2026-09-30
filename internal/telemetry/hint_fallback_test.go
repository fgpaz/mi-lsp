package telemetry

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestFailureWithoutHintPersistsErrorCodeOrExplicitIncomplete(t *testing.T) {
	withCode := EnrichAccessEvent(model.AccessEvent{Success: false}, model.CommandRequest{}, model.Envelope{
		Ok:    false,
		Error: &model.EnvelopeError{Code: "nav_generic", Message: "backend failed"},
	}, nil)
	if withCode.HintCode != "nav_generic" {
		t.Fatalf("hint = %q, want nav_generic", withCode.HintCode)
	}
	without := EnrichAccessEvent(model.AccessEvent{Success: false}, model.CommandRequest{}, model.Envelope{Ok: false}, nil)
	if without.HintCode != "explicit_incomplete" {
		t.Fatalf("hint = %q, want explicit_incomplete", without.HintCode)
	}
	ok := EnrichAccessEvent(model.AccessEvent{Success: true}, model.CommandRequest{}, model.Envelope{Ok: true, Items: []string{}}, nil)
	if strings.TrimSpace(ok.HintCode) != "" && ok.HintCode != "none" {
		t.Fatalf("success hint = %q, want empty", ok.HintCode)
	}
}

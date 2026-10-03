package daemon

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDaemonErrorEnvelopeTypesGraphObservationError(t *testing.T) {
	cause := fmt.Errorf("wrapped: %w", &model.GraphObservationError{Code: "GPH_IDENTITY_UNAVAILABLE", Field: "repository_identity", Message: "repository identity could not be resolved"})
	env := daemonErrorEnvelope(model.CommandRequest{Operation: "index.start"}, cause, "daemon")
	if env.Error == nil {
		t.Fatal("missing error")
	}
	got := env.Error
	if got.Kind != "backend_runtime" || got.Code != "gph_identity_unavailable" || got.Stage != "graph_observation" || got.ReasonCode != "explicit_incomplete" || !strings.Contains(got.Detail, "repository_identity") {
		t.Fatalf("error = %+v", *got)
	}
}

func TestDaemonErrorEnvelopeNeverLeavesIndexGeneric(t *testing.T) {
	env := daemonErrorEnvelope(model.CommandRequest{Operation: "index.start"}, errors.New("something exploded while indexing"), "daemon")
	if env.Error == nil {
		t.Fatal("missing error")
	}
	code := env.Error.Code
	if code != "index_failed" || strings.HasSuffix(code, "_generic") {
		t.Fatalf("code = %q, want index_failed", code)
	}
	if !strings.Contains(env.Error.Detail, "something exploded") {
		t.Fatalf("detail = %q", env.Error.Detail)
	}
}

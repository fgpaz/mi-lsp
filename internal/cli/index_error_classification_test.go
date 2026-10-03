package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestBuildCLIErrorEnvelopeTypesGraphObservationError(t *testing.T) {
	cause := fmt.Errorf("index failed: %w", &model.GraphObservationError{Code: "GPH_IDENTITY_UNAVAILABLE", Field: "repository_identity", Message: "repository identity could not be resolved"})
	env := buildCLIErrorEnvelope(model.CommandRequest{Operation: "index.start", Context: model.QueryOptions{Workspace: "demo"}}, "direct", cause)

	if env.Error == nil {
		t.Fatal("missing error")
	}
	got := env.Error
	if got.Kind != "backend_runtime" || got.Code != "gph_identity_unavailable" || got.Stage != "graph_observation" || got.HintCode != "gph_identity_unavailable" {
		t.Fatalf("error = %+v", *got)
	}
	if got.ReasonCode != "explicit_incomplete" || !strings.Contains(got.Detail, "repository_identity") {
		t.Fatalf("reason/detail = %q / %q", got.ReasonCode, got.Detail)
	}
}

func TestBuildCLIErrorEnvelopeNeverLeavesIndexGeneric(t *testing.T) {
	for _, cause := range []error{
		errors.New("something exploded while indexing"),
		errors.New("publishing the snapshot ran out of luck"),
	} {
		env := buildCLIErrorEnvelope(model.CommandRequest{Operation: "index.start", Context: model.QueryOptions{Workspace: "demo"}}, "direct", cause)
		if env.Error == nil {
			t.Fatal("missing error")
		}
		code := env.Error.Code
		if code == "" || strings.HasSuffix(code, "_generic") || strings.EqualFold(code, "unknown") {
			t.Fatalf("index error code = %q for %v, want a typed code", code, cause)
		}
		if code != "index_failed" {
			t.Fatalf("index error code = %q, want index_failed", code)
		}
		if !strings.Contains(env.Error.Detail, strings.SplitN(cause.Error(), ":", 2)[0]) {
			t.Fatalf("detail = %q, want the sanitized cause", env.Error.Detail)
		}
	}
}

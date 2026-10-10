package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDaemonStateForOutputRedactsWithoutMutatingInternalState(t *testing.T) {
	internal := model.DaemonState{AdminToken: "test-admin-token"}
	shown := daemonStateForOutput(internal)
	if shown.AdminToken != redactedAdminToken {
		t.Fatalf("output token = %q, want redacted marker", shown.AdminToken)
	}
	if internal.AdminToken != "test-admin-token" {
		t.Fatal("redaction changed the internal daemon token")
	}
}

func TestRedactDaemonStateOutputMasksDecodedStatusEnvelope(t *testing.T) {
	const token = "test-admin-token"
	envelope := model.Envelope{Items: []any{
		map[string]any{"state": map[string]any{"admin_token": token}},
	}}

	body, err := json.Marshal(redactDaemonStateOutput(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), token) {
		t.Fatal("status output contains the raw admin token")
	}
	if !strings.Contains(string(body), redactedAdminToken) {
		t.Fatalf("status output does not contain redaction marker: %s", body)
	}
}

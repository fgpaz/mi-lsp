package telemetry

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestRuntimeKeyForOperationUsesCanonicalRootAcrossAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	for _, alias := range []string{"alias-one", "alias-two"} {
		if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root}); err != nil {
			t.Fatalf("RegisterWorkspace(%s): %v", alias, err)
		}
	}

	first := RuntimeKeyForOperation(
		model.CommandRequest{Context: model.QueryOptions{Workspace: "alias-one"}, Payload: map[string]any{"entrypoint": "main::app"}},
		model.Envelope{Ok: true, Workspace: "alias-one", Backend: "roslyn"},
	)
	second := RuntimeKeyForOperation(
		model.CommandRequest{Context: model.QueryOptions{Workspace: "alias-two"}, Payload: map[string]any{"entrypoint": "main::app"}},
		model.Envelope{Ok: true, Workspace: "alias-two", Backend: "roslyn"},
	)

	if first != second {
		t.Fatalf("runtime keys differ for duplicate aliases: %q vs %q", first, second)
	}
}

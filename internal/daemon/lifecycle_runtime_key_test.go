package daemon

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRuntimeKeyCanonicalizesDuplicateAliasRoot(t *testing.T) {
	root := t.TempDir()
	request := model.WorkerRequest{BackendType: "roslyn", EntrypointID: "app::main"}

	first := runtimeKey(model.WorkspaceRegistration{Name: "alias-one", Root: root}, request)
	second := runtimeKey(model.WorkspaceRegistration{Name: "alias-two", Root: root}, request)

	if first != second {
		t.Fatalf("runtime keys differ for duplicate aliases: %q vs %q", first, second)
	}
}

func TestRuntimeKeyKeepsDistinctEntrypointsSeparate(t *testing.T) {
	root := t.TempDir()
	workspace := model.WorkspaceRegistration{Name: "alias-one", Root: root}

	first := runtimeKey(workspace, model.WorkerRequest{BackendType: "roslyn", EntrypointID: "app::main"})
	second := runtimeKey(workspace, model.WorkerRequest{BackendType: "roslyn", EntrypointID: "app::worker"})

	if first == second {
		t.Fatalf("runtime keys collapsed distinct entrypoints: %q", first)
	}
}

func TestRuntimeKeyUsesRootSentinelWhenRootMissing(t *testing.T) {
	key := runtimeKey(model.WorkspaceRegistration{Name: "alias-one"}, model.WorkerRequest{BackendType: "roslyn"})
	if key != "roslyn::-::." {
		t.Fatalf("runtime key = %q, want root sentinel fallback", key)
	}
}

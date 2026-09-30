package workspace

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestUnknownAliasKeepsFailureAndNamesCWDWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	if err := SaveProjectFile(root, model.ProjectFile{Project: model.ProjectBlock{Name: "caller-workspace", Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}}); err != nil {
		t.Fatal(err)
	}
	registry := model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{
		"caller-workspace": {Root: root, Kind: model.WorkspaceKindSingle},
	}}
	if err := SaveRegistry(registry); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveWorkspaceSelection("demo-local", filepath.Join(root, "src"))
	resolution, ok := AsWorkspaceResolutionError(err)
	if !ok || !resolution.FallbackAvailable || resolution.Fallback.Registration.Name != "caller-workspace" {
		t.Fatalf("resolution = %+v ok=%v err=%v", resolution, ok, err)
	}
	var selector *WorkspaceSelectorError
	if !errors.As(err, &selector) || selector.Code != WorkspaceSelectorNotFound {
		t.Fatalf("unwrap = %v", err)
	}
	continuation, detail, ready := ContinuationForUnresolvedSelector("workspace.status", err)
	if !ready || continuation.Next.Workspace != "caller-workspace" || continuation.Next.Op != "workspace.status" {
		t.Fatalf("continuation = %+v detail=%q", continuation, detail)
	}
}

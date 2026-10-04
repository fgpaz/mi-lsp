package workspace

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRegisteredMainWorktreePrefersLastWorkspaceThenLexicalAlias(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "-C", root, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	output, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").CombinedOutput()
	if err != nil {
		t.Fatalf("git common dir: %v: %s", err, output)
	}
	commonDir := filepath.Clean(strings.TrimSpace(string(output)))
	registry := model.RegistryFile{
		Workspaces: map[string]model.WorkspaceRegistration{
			"z-main": {Root: root},
			"a-main": {Root: root},
		},
	}

	t.Run("prefers LastWorkspace among duplicate aliases", func(t *testing.T) {
		registry.Defaults.LastWorkspace = "z-main"
		registration, found := registeredMainWorktree(commonDir, registry)
		if !found || registration.Name != "z-main" {
			t.Fatalf("registeredMainWorktree() = (%+v, %v), want alias z-main", registration, found)
		}
	})

	t.Run("falls back to lexicographically smallest alias", func(t *testing.T) {
		registry.Defaults.LastWorkspace = "missing"
		registration, found := registeredMainWorktree(commonDir, registry)
		if !found || registration.Name != "a-main" {
			t.Fatalf("registeredMainWorktree() = (%+v, %v), want alias a-main", registration, found)
		}
	})
}

package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// NeedsLightRegistration performs a read-only precheck; the writer rechecks under lock.
func NeedsLightRegistration(selector, cwd string) (bool, error) {
	if AutoRegisterDisabledByEnv() {
		return false, nil
	}
	root, ok := autoRegisterCandidateRoot(selector, cwd)
	if !ok {
		return false, nil
	}
	if linked, _, valid := linkedWorktree(root); valid && linked {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(AutoRegisterModeEnvVar)), "force") && !autoRegisterDefaultRootEligible(root) {
		return false, nil
	}
	registry, err := LoadRegistryReadOnly()
	if err != nil {
		return false, err
	}
	_, _, found := registeredWorkspaceForRoot(root, registry)
	return !found, nil
}

// RegisterGitRootLight only updates the registry. It never creates project files or indexes.
func RegisterGitRootLight(selector, cwd string) (bool, error) {
	if AutoRegisterDisabledByEnv() {
		return false, nil
	}
	root, ok := autoRegisterCandidateRoot(selector, cwd)
	if !ok {
		return false, nil
	}
	if linked, _, valid := linkedWorktree(root); valid && linked {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(AutoRegisterModeEnvVar)), "force") && !autoRegisterDefaultRootEligible(root) {
		return false, nil
	}
	registered := false
	err := WithRegistryLock(func() error {
		registry, err := LoadRegistry()
		if err != nil {
			return err
		}
		if _, _, found := registeredWorkspaceForRoot(root, registry); found {
			return nil
		}
		if registry.Workspaces == nil {
			registry.Workspaces = map[string]model.WorkspaceRegistration{}
		}
		alias := autoRegisterAlias(root, registry)
		registry.Workspaces[alias] = model.WorkspaceRegistration{Name: alias, Root: filepath.Clean(root), Kind: model.WorkspaceKindSingle}
		if err := SaveRegistry(registry); err != nil {
			return err
		}
		registered = true
		return nil
	})
	return registered, err
}

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// AutoRegisterEnvVar disables implicit workspace registration when set to a
// truthy value ("1", "true", "yes", "on").
const AutoRegisterEnvVar = "MI_LSP_NO_AUTO_REGISTER"

// AutoRegisterResult describes what AutoRegisterWorkspace did. A zero value
// means nothing was registered (opted out, no candidate, or already known).
type AutoRegisterResult struct {
	Registered bool
	Alias      string
	Root       string
}

// AutoRegisterDisabledByEnv reports whether the environment opts out of
// implicit registration.
func AutoRegisterDisabledByEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(AutoRegisterEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// AutoRegisterWorkspace persists a registry entry for the Git root that
// contains the requested selector path (or the caller cwd when selector is
// empty), if that root is not registered yet. It never registers $HOME, the
// filesystem root, or directories outside a Git repository, never overwrites
// an existing alias, and never touches an existing project.toml.
func AutoRegisterWorkspace(selector string, callerCWD string) (AutoRegisterResult, error) {
	root, ok := autoRegisterCandidateRoot(selector, callerCWD)
	if !ok {
		return AutoRegisterResult{}, nil
	}
	// Cheap read-only precheck so the common (already registered) path takes no lock.
	if registry, err := LoadRegistryReadOnly(); err == nil {
		if _, _, found := registeredWorkspaceForRoot(root, registry); found {
			return AutoRegisterResult{}, nil
		}
		if strings.TrimSpace(selector) != "" {
			if _, isAlias := registry.Workspaces[strings.TrimSpace(selector)]; isAlias {
				return AutoRegisterResult{}, nil
			}
		}
	}

	var result AutoRegisterResult
	err := WithRegistryLock(func() error {
		registry, err := LoadRegistry()
		if err != nil {
			return err
		}
		if _, _, found := registeredWorkspaceForRoot(root, registry); found {
			return nil // another process won the race, or the root has another alias: reuse it
		}
		alias := autoRegisterAlias(root, registry)
		registration, project, err := DetectWorkspaceLayout(root, alias)
		if err != nil {
			return err
		}
		registration.Name = alias
		project.Project.Name = alias
		registration = ApplyProjectTopology(registration, project)
		registry.Workspaces[alias] = registration
		if err := SaveRegistry(registry); err != nil {
			return err
		}
		// Same persisted state as `init`, but an existing project.toml is canon: keep it.
		if _, statErr := os.Stat(ProjectConfigPath(registration.Root)); os.IsNotExist(statErr) {
			if err := SaveProjectFile(registration.Root, project); err != nil {
				return err
			}
		}
		result = AutoRegisterResult{Registered: true, Alias: alias, Root: registration.Root}
		return nil
	})
	return result, err
}

func autoRegisterCandidateRoot(selector string, callerCWD string) (string, bool) {
	selector = strings.TrimSpace(selector)
	start := strings.TrimSpace(callerCWD)
	if selector != "" {
		path, ok := resolveSelectorPath(selector, callerCWD)
		if !ok {
			return "", false
		}
		start = path
	}
	if start == "" {
		return "", false
	}
	if info, err := os.Stat(start); err != nil {
		return "", false
	} else if !info.IsDir() {
		start = filepath.Dir(start)
	}
	root, ok := gitTopLevel(start)
	if !ok {
		return "", false
	}
	if autoRegisterForbiddenRoot(root) {
		return "", false
	}
	return root, true
}

func autoRegisterForbiddenRoot(root string) bool {
	clean := filepath.Clean(root)
	if clean == filepath.Dir(clean) { // "/" or a volume root
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		homeClean := filepath.Clean(home)
		if evaluated, evalErr := filepath.EvalSymlinks(homeClean); evalErr == nil {
			homeClean = evaluated
		}
		if sameFilePath(clean, homeClean) {
			return true
		}
	}
	return false
}

func sameFilePath(left string, right string) bool {
	if left == right {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

// autoRegisterAlias picks basename(root) or, when that alias belongs to a
// different root, a deterministic "<base>-<hash>" suffix derived from the root
// path. Existing aliases are never reused for a different root.
func autoRegisterAlias(root string, registry model.RegistryFile) string {
	base := strings.TrimSpace(filepath.Base(root))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "workspace"
	}
	if _, taken := registry.Workspaces[base]; !taken {
		return base
	}
	identity := root
	if canonical, ok := normalizeComparablePath(root); ok {
		identity = canonical
	}
	sum := sha256.Sum256([]byte(identity))
	digest := hex.EncodeToString(sum[:])
	for length := 6; length <= len(digest); length += 2 {
		candidate := base + "-" + digest[:length]
		if _, taken := registry.Workspaces[candidate]; !taken {
			return candidate
		}
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%s-%d", base, digest, n)
		if _, taken := registry.Workspaces[candidate]; !taken {
			return candidate
		}
	}
}

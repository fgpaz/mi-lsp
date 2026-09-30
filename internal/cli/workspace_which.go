package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type workspaceWhichResult struct {
	Name       string
	Root       string
	Executable string
	Source     string
}

func newWorkspaceWhichCommand(state *rootState) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "which",
		Short: "Show the resolved workspace name, root, and executable",
		Long: `Resolve a workspace from the registry only.

Precedence matches the rest of the CLI: explicit --workspace, then the registered root that contains --cwd (or the process cwd), then last_workspace. This command does not start the daemon, open an index, or use the network, and it does not change the registry.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := resolveWorkspaceWhich(state.workspace, cwd)
			if err != nil {
				return err
			}
			opts := state.queryOptions(cmd, "workspace.which", nil)
			env := model.Envelope{
				Ok:        true,
				Workspace: resolved.Name,
				Backend:   "registry",
				Items: []map[string]any{{
					"name":       resolved.Name,
					"root":       resolved.Root,
					"executable": resolved.Executable,
					"source":     resolved.Source,
				}},
			}
			env = output.ApplyEnvelopeLimits(env, opts)
			return state.printPreparedEnvelope(env, opts)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "Directory to resolve when --workspace is omitted")
	return command
}

func resolveWorkspaceWhich(explicit string, cwd string) (workspaceWhichResult, error) {
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		return workspaceWhichResult{}, err
	}
	explicit = literalWorkspacePath(explicit)
	if explicit != "" {
		registration, ok := matchExplicitWorkspace(registry, explicit, cwd)
		if !ok {
			return workspaceWhichResult{}, fmt.Errorf("workspace %q is not registered", explicit)
		}
		return workspaceWhichFromRegistration(registration, string(workspace.ResolutionSourceExplicit)), nil
	}
	lookupCWD := literalWorkspacePath(cwd)
	if lookupCWD == "" {
		if processCWD, cwdErr := os.Getwd(); cwdErr == nil {
			lookupCWD = literalWorkspacePath(processCWD)
		}
	}
	if registration, ok := matchWorkspaceCWD(registry, lookupCWD); ok {
		return workspaceWhichFromRegistration(registration, string(workspace.ResolutionSourceCallerCWD)), nil
	}
	last := strings.TrimSpace(registry.Defaults.LastWorkspace)
	if last != "" {
		if registration, ok := registry.Workspaces[last]; ok {
			registration.Name = last
			registration.Root = literalWorkspacePath(registration.Root)
			return workspaceWhichFromRegistration(registration, string(workspace.ResolutionSourceLastWorkspace)), nil
		}
	}
	return workspaceWhichResult{}, fmt.Errorf("no workspace specified and no default workspace configured")
}

func workspaceWhichFromRegistration(registration model.WorkspaceRegistration, source string) workspaceWhichResult {
	return workspaceWhichResult{
		Name:       strings.TrimSpace(registration.Name),
		Root:       literalWorkspacePath(registration.Root),
		Executable: currentExecutablePath(),
		Source:     source,
	}
}

// literalWorkspacePath keeps a Windows path unchanged.
// strconv.Unquote and fmt-built JSON treat \r as a carriage return and drop the backslash before \m and \p.
func literalWorkspacePath(path string) string {
	return strings.TrimSpace(path)
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Clean(path)
}

func matchExplicitWorkspace(registry model.RegistryFile, selector string, cwd string) (model.WorkspaceRegistration, bool) {
	if registration, ok := registry.Workspaces[selector]; ok {
		registration.Name = selector
		registration.Root = literalWorkspacePath(registration.Root)
		return registration, true
	}
	candidate := selector
	if !whichPathIsAbsolute(selector) {
		base := literalWorkspacePath(cwd)
		if base == "" {
			if processCWD, err := os.Getwd(); err == nil {
				base = literalWorkspacePath(processCWD)
			}
		}
		candidate = joinWorkspacePath(base, selector)
	}
	return matchWorkspaceRoot(registry, candidate)
}

func matchWorkspaceCWD(registry model.RegistryFile, cwd string) (model.WorkspaceRegistration, bool) {
	if cwd == "" {
		return model.WorkspaceRegistration{}, false
	}
	bestLen := -1
	best := make([]model.WorkspaceRegistration, 0, 1)
	for _, registration := range registryRegistrations(registry) {
		if !cwdWithinWorkspaceRoot(cwd, registration.Root) {
			continue
		}
		length := len(workspaceMatchKey(registration.Root))
		if length > bestLen {
			best = []model.WorkspaceRegistration{registration}
			bestLen = length
			continue
		}
		if length == bestLen {
			best = append(best, registration)
		}
	}
	if bestLen < 0 {
		return model.WorkspaceRegistration{}, false
	}
	return chooseWorkspaceRegistration(best, registry.Defaults.LastWorkspace), true
}

func matchWorkspaceRoot(registry model.RegistryFile, candidate string) (model.WorkspaceRegistration, bool) {
	key := workspaceMatchKey(candidate)
	if key == "" {
		return model.WorkspaceRegistration{}, false
	}
	matches := make([]model.WorkspaceRegistration, 0, 1)
	for _, registration := range registryRegistrations(registry) {
		if workspaceMatchKey(registration.Root) == key {
			matches = append(matches, registration)
		}
	}
	if len(matches) == 0 {
		return model.WorkspaceRegistration{}, false
	}
	return chooseWorkspaceRegistration(matches, registry.Defaults.LastWorkspace), true
}

func registryRegistrations(registry model.RegistryFile) []model.WorkspaceRegistration {
	names := make([]string, 0, len(registry.Workspaces))
	for name := range registry.Workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]model.WorkspaceRegistration, 0, len(names))
	for _, name := range names {
		registration := registry.Workspaces[name]
		registration.Name = name
		registration.Root = literalWorkspacePath(registration.Root)
		items = append(items, registration)
	}
	return items
}

func chooseWorkspaceRegistration(matches []model.WorkspaceRegistration, lastWorkspace string) model.WorkspaceRegistration {
	sorted := append([]model.WorkspaceRegistration(nil), matches...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	lastWorkspace = strings.TrimSpace(lastWorkspace)
	if lastWorkspace != "" {
		for _, registration := range sorted {
			if registration.Name == lastWorkspace {
				return registration
			}
		}
	}
	return sorted[0]
}

func cwdWithinWorkspaceRoot(cwd string, root string) bool {
	cwdKey := workspaceMatchKey(cwd)
	rootKey := workspaceMatchKey(root)
	if cwdKey == "" || rootKey == "" {
		return false
	}
	return cwdKey == rootKey || strings.HasPrefix(cwdKey, rootKey+"/")
}

func workspaceMatchKey(path string) string {
	trimmed := literalWorkspacePath(path)
	if trimmed == "" {
		return ""
	}
	slash := strings.ReplaceAll(trimmed, `\`, "/")
	slash = strings.TrimRight(slash, "/")
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		slash = strings.ToLower(slash)
	}
	return slash
}

func whichPathIsAbsolute(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	return strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `//`)
}

func joinWorkspacePath(base string, selector string) string {
	selector = literalWorkspacePath(selector)
	if selector == "" || selector == "." {
		return base
	}
	separator := "/"
	if strings.Contains(base, `\`) && !strings.Contains(base, "/") {
		separator = `\`
	}
	base = strings.TrimRight(base, `/\`)
	selector = strings.TrimLeft(selector, `/\`)
	if base == "" {
		return selector
	}
	if selector == "" {
		return base
	}
	return base + separator + selector
}

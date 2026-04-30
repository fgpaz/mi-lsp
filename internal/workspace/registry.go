package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const registryDirName = ".mi-lsp"

type ResolutionSource string

const (
	ResolutionSourceExplicit      ResolutionSource = "explicit"
	ResolutionSourcePath          ResolutionSource = "path"
	ResolutionSourceCallerCWD     ResolutionSource = "caller_cwd"
	ResolutionSourceLastWorkspace ResolutionSource = "last_workspace"
)

type WorkspaceResolution struct {
	Registration model.WorkspaceRegistration
	Source       ResolutionSource
	Warnings     []string
}

type RootGroup struct {
	Root            string
	AliasCount      int
	Aliases         []string
	CanonicalAlias  string
	SelectionReason string
	Kind            string
	Registrations   []model.WorkspaceRegistration
	Warnings        []string
}

func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, registryDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func RegistryPath() (string, error) {
	dir, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "registry.toml"), nil
}

func LoadRegistry() (model.RegistryFile, error) {
	registry := model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}
	path, err := RegistryPath()
	if err != nil {
		return registry, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return registry, nil
	} else if err != nil {
		return registry, err
	}
	if _, err := toml.DecodeFile(path, &registry); err != nil {
		return registry, err
	}
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	for name, ws := range registry.Workspaces {
		ws.Name = name
		registry.Workspaces[name] = ws
	}
	return registry, nil
}

func SaveRegistry(registry model.RegistryFile) error {
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	path, err := RegistryPath()
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(registry)
}

func RegisterWorkspace(name string, registration model.WorkspaceRegistration) (model.RegistryFile, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return registry, err
	}
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	registration.Name = name
	registry.Workspaces[name] = registration
	registry.Defaults.LastWorkspace = name
	return registry, SaveRegistry(registry)
}

func RemoveWorkspace(name string) error {
	registry, err := LoadRegistry()
	if err != nil {
		return err
	}
	if _, ok := registry.Workspaces[name]; !ok {
		return fmt.Errorf("workspace %q is not registered", name)
	}
	delete(registry.Workspaces, name)
	if registry.Defaults.LastWorkspace == name {
		registry.Defaults.LastWorkspace = ""
	}
	return SaveRegistry(registry)
}

func ResolveWorkspace(nameOrPath string) (model.WorkspaceRegistration, error) {
	resolution, err := ResolveWorkspaceSelection(nameOrPath, "")
	if err != nil {
		return model.WorkspaceRegistration{}, err
	}
	return resolution.Registration, nil
}

func ResolveWorkspaceSelection(nameOrPath string, callerCWD string) (WorkspaceResolution, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return WorkspaceResolution{}, err
	}

	selector := strings.TrimSpace(nameOrPath)
	if selector != "" {
		if ws, ok := registry.Workspaces[selector]; ok {
			ws.Name = selector
			return WorkspaceResolution{Registration: ws, Source: ResolutionSourceExplicit}, nil
		}
		if resolvedPath, ok := resolveSelectorPath(selector, callerCWD); ok {
			registration, err := DetectWorkspace(resolvedPath)
			if err != nil {
				return WorkspaceResolution{}, err
			}
			return WorkspaceResolution{Registration: registration, Source: ResolutionSourcePath}, nil
		}
		return WorkspaceResolution{}, errors.New("workspace not found in registry and path does not exist")
	}

	if resolution, ok := resolveWorkspaceFromCallerCWD(callerCWD, registry); ok {
		return resolution, nil
	}

	if registry.Defaults.LastWorkspace != "" {
		if ws, ok := registry.Workspaces[registry.Defaults.LastWorkspace]; ok {
			ws.Name = registry.Defaults.LastWorkspace
			warnings := []string{
				fmt.Sprintf("workspace omitted; no registered workspace matched caller cwd %q; falling back to last_workspace=%q", strings.TrimSpace(callerCWD), ws.Name),
			}
			return WorkspaceResolution{
				Registration: ws,
				Source:       ResolutionSourceLastWorkspace,
				Warnings:     warnings,
			}, nil
		}
	}

	return WorkspaceResolution{}, errors.New("no workspace specified and no default workspace configured")
}

func ListWorkspaces() ([]model.WorkspaceRegistration, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	items := make([]model.WorkspaceRegistration, 0, len(registry.Workspaces))
	for name, ws := range registry.Workspaces {
		ws.Name = name
		items = append(items, ws)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func GroupWorkspacesByRoot() ([]RootGroup, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	grouped := map[string][]model.WorkspaceRegistration{}
	displayRoots := map[string]string{}
	for alias, ws := range registry.Workspaces {
		ws.Name = alias
		key, ok := normalizeComparablePath(ws.Root)
		if !ok {
			key = strings.ToLower(strings.TrimSpace(ws.Root))
		}
		if key == "" {
			key = "<missing-root>"
		}
		grouped[key] = append(grouped[key], ws)
		if displayRoots[key] == "" {
			displayRoots[key] = cleanDisplayRoot(ws.Root)
		}
	}

	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	groups := make([]RootGroup, 0, len(keys))
	for _, key := range keys {
		registrations := grouped[key]
		sort.Slice(registrations, func(i, j int) bool {
			return strings.ToLower(registrations[i].Name) < strings.ToLower(registrations[j].Name)
		})
		selection := selectAliasForRoot(registrations, registry.Defaults.LastWorkspace)
		aliases := make([]string, 0, len(registrations))
		kinds := map[string]struct{}{}
		warnings := []string{}
		for _, registration := range registrations {
			aliases = append(aliases, registration.Name)
			if strings.TrimSpace(string(registration.Kind)) != "" {
				kinds[string(registration.Kind)] = struct{}{}
			}
		}
		root := displayRoots[key]
		if root == "" {
			root = registrations[0].Root
		}
		if _, err := os.Stat(root); err != nil {
			warnings = append(warnings, fmt.Sprintf("root path is not accessible: %v", err))
		}
		if len(registrations) > 1 {
			warnings = append(warnings, "multiple aliases share this root; aliases are preserved")
		}
		groups = append(groups, RootGroup{
			Root:            root,
			AliasCount:      len(registrations),
			Aliases:         aliases,
			CanonicalAlias:  selection.Registration.Name,
			SelectionReason: selection.Reason,
			Kind:            summarizeKinds(kinds),
			Registrations:   registrations,
			Warnings:        warnings,
		})
	}
	return groups, nil
}

func WorkspaceStateDir(root string) string {
	return filepath.Join(root, registryDirName)
}

func ProjectConfigPath(root string) string {
	return filepath.Join(WorkspaceStateDir(root), "project.toml")
}

func LoadProjectFile(root string) (model.ProjectFile, error) {
	project := model.ProjectFile{}
	path := ProjectConfigPath(root)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return project, nil
	} else if err != nil {
		return project, err
	}
	_, err := toml.DecodeFile(path, &project)
	return project, err
}

func SaveProjectFile(root string, project model.ProjectFile) error {
	stateDir := WorkspaceStateDir(root)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	file, err := os.Create(ProjectConfigPath(root))
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(project)
}

func resolveWorkspaceFromCallerCWD(callerCWD string, registry model.RegistryFile) (WorkspaceResolution, bool) {
	normalizedCWD, ok := normalizeComparablePath(callerCWD)
	if !ok {
		return WorkspaceResolution{}, false
	}

	grouped := map[string][]model.WorkspaceRegistration{}
	longestRootLen := -1
	for alias, registration := range registry.Workspaces {
		registration.Name = alias
		normalizedRoot, ok := normalizeComparablePath(registration.Root)
		if !ok || !pathContains(normalizedCWD, normalizedRoot) {
			continue
		}
		grouped[normalizedRoot] = append(grouped[normalizedRoot], registration)
		if len(normalizedRoot) > longestRootLen {
			longestRootLen = len(normalizedRoot)
		}
	}
	if longestRootLen < 0 {
		return WorkspaceResolution{}, false
	}

	bestRoots := make([]string, 0, len(grouped))
	for root := range grouped {
		if len(root) == longestRootLen {
			bestRoots = append(bestRoots, root)
		}
	}
	sort.Strings(bestRoots)
	bestRoot := bestRoots[0]
	selection := selectAliasForRoot(grouped[bestRoot], registry.Defaults.LastWorkspace)
	return WorkspaceResolution{
		Registration: selection.Registration,
		Source:       ResolutionSourceCallerCWD,
		Warnings:     selection.Warnings,
	}, true
}

type aliasSelection struct {
	Registration model.WorkspaceRegistration
	Warnings     []string
	Reason       string
}

func selectAliasForRoot(registrations []model.WorkspaceRegistration, lastWorkspace string) aliasSelection {
	sorted := append([]model.WorkspaceRegistration(nil), registrations...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	if len(sorted) == 1 {
		return aliasSelection{Registration: sorted[0], Reason: "single alias"}
	}

	root := sorted[0].Root
	reason := "lexicographic fallback"
	chosen := sorted[0]

	if projectName := loadProjectName(root); projectName != "" {
		if candidate, ok := findRegistrationByAlias(sorted, projectName); ok {
			chosen = candidate
			reason = "project.name"
		}
	}

	if reason == "lexicographic fallback" {
		if candidate, ok := findRegistrationByAlias(sorted, filepath.Base(root)); ok {
			chosen = candidate
			reason = "root basename"
		}
	}

	if reason == "lexicographic fallback" && strings.TrimSpace(lastWorkspace) != "" {
		if candidate, ok := findRegistrationByAlias(sorted, lastWorkspace); ok {
			chosen = candidate
			reason = "same-root last_workspace"
		}
	}

	warnings := []string{
		fmt.Sprintf("workspace omitted; multiple registry aliases share root %q; selected %q using %s", root, chosen.Name, reason),
	}
	return aliasSelection{Registration: chosen, Warnings: warnings, Reason: reason}
}

func findRegistrationByAlias(registrations []model.WorkspaceRegistration, alias string) (model.WorkspaceRegistration, bool) {
	for _, registration := range registrations {
		if strings.EqualFold(strings.TrimSpace(registration.Name), strings.TrimSpace(alias)) {
			return registration, true
		}
	}
	return model.WorkspaceRegistration{}, false
}

func loadProjectName(root string) string {
	project, err := LoadProjectFile(root)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(project.Project.Name)
}

func resolveSelectorPath(selector string, callerCWD string) (string, bool) {
	candidates := []string{selector}
	if strings.TrimSpace(callerCWD) != "" && !filepath.IsAbs(selector) {
		candidates = append([]string{filepath.Join(callerCWD, selector)}, candidates...)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		return absolute, true
	}
	return "", false
}

func normalizeComparablePath(path string) (string, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", false
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", false
	}
	return strings.ToLower(filepath.Clean(absolute)), true
}

func pathContains(cwd string, root string) bool {
	return cwd == root || strings.HasPrefix(cwd, root+string(os.PathSeparator))
}

func cleanDisplayRoot(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	return filepath.Clean(trimmed)
}

func summarizeKinds(kinds map[string]struct{}) string {
	if len(kinds) == 0 {
		return ""
	}
	items := make([]string, 0, len(kinds))
	for kind := range kinds {
		items = append(items, kind)
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

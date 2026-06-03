package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

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

const workspaceResolutionNotFoundMessage = "workspace not found in registry and path does not exist"

type WorkspaceResolutionError struct {
	Selector          string
	CallerCWD         string
	Fallback          WorkspaceResolution
	FallbackAvailable bool
	Warnings          []string
}

func (e *WorkspaceResolutionError) Error() string {
	selector := strings.TrimSpace(e.Selector)
	if selector == "" {
		return workspaceResolutionNotFoundMessage
	}
	message := fmt.Sprintf("workspace %q is not registered and path does not exist", selector)
	if e.FallbackAvailable {
		alias := strings.TrimSpace(e.Fallback.Registration.Name)
		root := strings.TrimSpace(e.Fallback.Registration.Root)
		if alias != "" && root != "" {
			return fmt.Sprintf("%s; caller cwd %q resolves to workspace %q at %q for diagnostics only; rerun with --workspace %s if intended, or run `mi-lsp workspace hygiene --apply-safe` after review", message, strings.TrimSpace(e.CallerCWD), alias, root, alias)
		}
	}
	if strings.TrimSpace(e.CallerCWD) != "" {
		return fmt.Sprintf("%s; caller cwd %q did not resolve to a registered workspace; try --workspace . from the repo root or run `mi-lsp init . --name <alias>`", message, strings.TrimSpace(e.CallerCWD))
	}
	return message + "; run `mi-lsp workspace list --group-by-root` or `mi-lsp workspace doctor`"
}

func AsWorkspaceResolutionError(err error) (*WorkspaceResolutionError, bool) {
	var resolutionErr *WorkspaceResolutionError
	if errors.As(err, &resolutionErr) {
		return resolutionErr, true
	}
	return nil, false
}

type WorkspaceRootGroup struct {
	Root            string   `json:"root"`
	AliasCount      int      `json:"alias_count"`
	Aliases         []string `json:"aliases"`
	CanonicalAlias  string   `json:"canonical_alias"`
	SelectionReason string   `json:"selection_reason"`
	Kind            string   `json:"kind,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

type WorkspaceDoctorReport struct {
	AliasesSharingRoot []WorkspaceRootGroup      `json:"aliases_sharing_root,omitempty"`
	WorktreeFamilies   []WorkspaceWorktreeFamily `json:"worktree_families,omitempty"`
	GitCaseCollisions  []GitCaseCollision        `json:"git_case_collisions,omitempty"`
	StalePaths         []WorkspaceStalePath      `json:"stale_paths,omitempty"`
	BinaryShadowing    []BinaryCandidate         `json:"binary_shadowing,omitempty"`
	Health             string                    `json:"health,omitempty"`
	NextActions        []WorkspaceDoctorAction   `json:"next_actions,omitempty"`
	Suggestions        []string                  `json:"suggestions,omitempty"`
}

type WorkspacePruneReport struct {
	DryRun       bool                 `json:"dry_run"`
	Registry     string               `json:"registry,omitempty"`
	Candidates   []WorkspaceStalePath `json:"candidates,omitempty"`
	Removed      []WorkspaceStalePath `json:"removed,omitempty"`
	Skipped      []WorkspaceStalePath `json:"skipped,omitempty"`
	RemovedCount int                  `json:"removed_count"`
}

type WorkspaceWorktreeFamily struct {
	GitCommonDir string   `json:"git_common_dir"`
	Roots        []string `json:"roots"`
	Aliases      []string `json:"aliases"`
	Warnings     []string `json:"warnings,omitempty"`
}

type GitCaseCollision struct {
	Root         string   `json:"root"`
	Aliases      []string `json:"aliases"`
	GitCommonDir string   `json:"git_common_dir,omitempty"`
	Paths        []string `json:"paths"`
	Warnings     []string `json:"warnings,omitempty"`
}

type WorkspaceStalePath struct {
	Alias string `json:"alias"`
	Root  string `json:"root"`
	Error string `json:"error"`
}

type BinaryCandidate struct {
	Path     string `json:"path"`
	Active   bool   `json:"active,omitempty"`
	Revision string `json:"revision,omitempty"`
	Modified string `json:"modified,omitempty"`
	GOOS     string `json:"goos,omitempty"`
	GOARCH   string `json:"goarch,omitempty"`
	Warning  string `json:"warning,omitempty"`
}

type WorkspaceDoctorAction struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Command  string `json:"command"`
	Reason   string `json:"reason"`
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

func PruneStaleWorkspaces(apply bool) (WorkspacePruneReport, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return WorkspacePruneReport{}, err
	}
	registryPath, err := RegistryPath()
	if err != nil {
		return WorkspacePruneReport{}, err
	}
	report := WorkspacePruneReport{
		DryRun:   !apply,
		Registry: filepath.Clean(registryPath),
	}
	if registry.Workspaces == nil {
		return report, nil
	}
	names := make([]string, 0, len(registry.Workspaces))
	for name := range registry.Workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ws := registry.Workspaces[name]
		if strings.TrimSpace(ws.Root) == "" {
			report.Skipped = append(report.Skipped, WorkspaceStalePath{Alias: name, Root: ws.Root, Error: "empty root; skipped"})
			continue
		}
		if _, statErr := os.Stat(ws.Root); statErr == nil {
			continue
		} else if errors.Is(statErr, os.ErrNotExist) {
			candidate := WorkspaceStalePath{Alias: name, Root: ws.Root, Error: statErr.Error()}
			report.Candidates = append(report.Candidates, candidate)
			if apply {
				delete(registry.Workspaces, name)
				report.Removed = append(report.Removed, candidate)
			}
		} else {
			report.Skipped = append(report.Skipped, WorkspaceStalePath{Alias: name, Root: ws.Root, Error: statErr.Error()})
		}
	}
	report.RemovedCount = len(report.Removed)
	if apply && len(report.Removed) > 0 {
		if registry.Defaults.LastWorkspace != "" {
			for _, removed := range report.Removed {
				if removed.Alias == registry.Defaults.LastWorkspace {
					registry.Defaults.LastWorkspace = ""
					break
				}
			}
		}
		if err := SaveRegistry(registry); err != nil {
			return WorkspacePruneReport{}, err
		}
	}
	return report, nil
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
		return WorkspaceResolution{}, newWorkspaceResolutionError(selector, callerCWD, registry)
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

func newWorkspaceResolutionError(selector string, callerCWD string, registry model.RegistryFile) error {
	resolutionErr := &WorkspaceResolutionError{
		Selector:  strings.TrimSpace(selector),
		CallerCWD: strings.TrimSpace(callerCWD),
	}
	if fallback, ok := resolveWorkspaceFromCallerCWD(callerCWD, registry); ok {
		resolutionErr.Fallback = fallback
		resolutionErr.FallbackAvailable = true
		resolutionErr.Warnings = append(resolutionErr.Warnings, fallback.Warnings...)
	}
	return resolutionErr
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

func GroupWorkspacesByRoot() ([]WorkspaceRootGroup, error) {
	workspaces, err := ListWorkspaces()
	if err != nil {
		return nil, err
	}
	grouped := map[string][]model.WorkspaceRegistration{}
	displayRoot := map[string]string{}
	for _, ws := range workspaces {
		key, ok := normalizeComparablePath(ws.Root)
		if !ok {
			key = strings.ToLower(strings.TrimSpace(ws.Root))
		}
		grouped[key] = append(grouped[key], ws)
		if displayRoot[key] == "" {
			displayRoot[key] = ws.Root
		}
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]WorkspaceRootGroup, 0, len(keys))
	for _, key := range keys {
		registrations := grouped[key]
		selection := selectAliasForRoot(registrations, "")
		aliases := make([]string, 0, len(registrations))
		kind := ""
		for _, registration := range registrations {
			aliases = append(aliases, registration.Name)
			if kind == "" {
				kind = registration.Kind
			} else if registration.Kind != "" && registration.Kind != kind {
				kind = "mixed"
			}
		}
		sort.Strings(aliases)
		warnings := append([]string{}, selection.Warnings...)
		if len(aliases) > 1 {
			warnings = appendStringIfMissing(warnings, "multiple aliases share the same workspace root; keep aliases explicit in agent sessions")
		}
		groups = append(groups, WorkspaceRootGroup{
			Root:            displayRoot[key],
			AliasCount:      len(aliases),
			Aliases:         aliases,
			CanonicalAlias:  selection.Registration.Name,
			SelectionReason: selection.Reason,
			Kind:            kind,
			Warnings:        warnings,
		})
	}
	return groups, nil
}

func DoctorWorkspaces() (WorkspaceDoctorReport, error) {
	workspaces, err := ListWorkspaces()
	if err != nil {
		return WorkspaceDoctorReport{}, err
	}
	groups, err := GroupWorkspacesByRoot()
	if err != nil {
		return WorkspaceDoctorReport{}, err
	}
	report := WorkspaceDoctorReport{}
	for _, group := range groups {
		if group.AliasCount > 1 {
			report.AliasesSharingRoot = append(report.AliasesSharingRoot, group)
		}
	}

	commonDirGroups := map[string][]model.WorkspaceRegistration{}
	commonDirDisplay := map[string]string{}
	registrationsByRoot := map[string][]model.WorkspaceRegistration{}
	rootDisplay := map[string]string{}
	for _, ws := range workspaces {
		if _, err := os.Stat(ws.Root); err != nil {
			report.StalePaths = append(report.StalePaths, WorkspaceStalePath{Alias: ws.Name, Root: ws.Root, Error: err.Error()})
			continue
		}
		rootKey, ok := normalizeComparablePath(ws.Root)
		if !ok {
			rootKey = strings.ToLower(strings.TrimSpace(ws.Root))
		}
		registrationsByRoot[rootKey] = append(registrationsByRoot[rootKey], ws)
		if rootDisplay[rootKey] == "" {
			rootDisplay[rootKey] = ws.Root
		}

		commonDir, ok := gitCommonDir(ws.Root)
		if !ok {
			continue
		}
		key, ok := normalizeComparablePath(commonDir)
		if !ok {
			key = strings.ToLower(strings.TrimSpace(commonDir))
		}
		commonDirGroups[key] = append(commonDirGroups[key], ws)
		if commonDirDisplay[key] == "" {
			commonDirDisplay[key] = commonDir
		}
	}
	commonKeys := make([]string, 0, len(commonDirGroups))
	for key := range commonDirGroups {
		commonKeys = append(commonKeys, key)
	}
	sort.Strings(commonKeys)
	for _, key := range commonKeys {
		registrations := commonDirGroups[key]
		rootSet := map[string]bool{}
		aliasSet := map[string]bool{}
		for _, registration := range registrations {
			rootSet[filepath.Clean(registration.Root)] = true
			aliasSet[registration.Name] = true
		}
		if len(rootSet) <= 1 {
			continue
		}
		roots := sortedKeys(rootSet)
		aliases := sortedKeys(aliasSet)
		report.WorktreeFamilies = append(report.WorktreeFamilies, WorkspaceWorktreeFamily{
			GitCommonDir: commonDirDisplay[key],
			Roots:        roots,
			Aliases:      aliases,
			Warnings:     []string{"registered worktrees share a git common dir but must keep separate aliases, indexes, watchers, and runtimes"},
		})
	}

	rootKeys := make([]string, 0, len(registrationsByRoot))
	for key := range registrationsByRoot {
		rootKeys = append(rootKeys, key)
	}
	sort.Strings(rootKeys)
	for _, key := range rootKeys {
		root := rootDisplay[key]
		collisions := gitTreeCaseCollisions(root)
		if len(collisions) == 0 {
			continue
		}
		aliases := make([]string, 0, len(registrationsByRoot[key]))
		for _, registration := range registrationsByRoot[key] {
			aliases = append(aliases, registration.Name)
		}
		sort.Strings(aliases)
		commonDir, _ := gitCommonDir(root)
		for _, paths := range collisions {
			report.GitCaseCollisions = append(report.GitCaseCollisions, GitCaseCollision{
				Root:         root,
				Aliases:      aliases,
				GitCommonDir: commonDir,
				Paths:        paths,
				Warnings:     []string{"git tree contains paths that differ only by casing; Windows checkouts may materialize one path and hide or overwrite the other"},
			})
		}
	}

	report.BinaryShadowing = inspectBinaryShadowing()
	report.Health = workspaceDoctorHealth(report)
	report.NextActions = workspaceDoctorNextActions(report)
	report.Suggestions = append(report.Suggestions,
		"Use one explicit alias per worktree: mi-lsp init . --name <alias>",
		"Run queries with the active worktree alias: mi-lsp nav search <pattern> --workspace <alias>",
		"Use mi-lsp workspace list --group-by-root to inspect duplicate aliases without mutating registry",
	)
	return report, nil
}

func workspaceDoctorHealth(report WorkspaceDoctorReport) string {
	switch {
	case len(report.StalePaths) > 0 || len(report.GitCaseCollisions) > 0:
		return "action_required"
	case len(report.BinaryShadowing) > 1 || hasBinaryVersionDrift(report.BinaryShadowing) || len(report.WorktreeFamilies) > 0 || len(report.AliasesSharingRoot) > 0:
		return "attention"
	default:
		return "ok"
	}
}

func workspaceDoctorNextActions(report WorkspaceDoctorReport) []WorkspaceDoctorAction {
	var actions []WorkspaceDoctorAction
	if len(report.StalePaths) > 0 {
		actions = append(actions, WorkspaceDoctorAction{
			ID:       "prune_stale_aliases",
			Severity: "high",
			Command:  "mi-lsp workspace prune --stale --dry-run",
			Reason:   "registry contains aliases whose roots no longer exist; dry-run lists the cleanup plan without deleting files",
		})
	}
	if len(report.GitCaseCollisions) > 0 {
		actions = append(actions, WorkspaceDoctorAction{
			ID:       "fix_git_case_collisions",
			Severity: "high",
			Command:  "git -C <root> ls-tree -r HEAD --name-only",
			Reason:   "one or more registered Git trees contain paths that differ only by casing; repair the repo tree with a dedicated git rm/mv commit before using Windows worktrees",
		})
	}
	if len(report.WorktreeFamilies) > 0 {
		actions = append(actions, WorkspaceDoctorAction{
			ID:       "verify_worktree_aliases",
			Severity: "medium",
			Command:  "mi-lsp workspace list --group-by-root",
			Reason:   "registered worktrees share git common dirs; agents should use the physical worktree alias they are operating in",
		})
	}
	if len(report.AliasesSharingRoot) > 0 {
		actions = append(actions, WorkspaceDoctorAction{
			ID:       "review_duplicate_root_aliases",
			Severity: "low",
			Command:  "mi-lsp workspace list --group-by-root",
			Reason:   "multiple aliases point at the same root; this is allowed but can confuse handoffs unless aliases are explicit",
		})
	}
	if len(report.BinaryShadowing) > 1 {
		actions = append(actions, WorkspaceDoctorAction{
			ID:       "review_binary_shadowing",
			Severity: "medium",
			Command:  binaryShadowingLookupCommand(),
			Reason:   "multiple mi-lsp binaries are visible on PATH; agents may execute a different build than expected",
		})
	}
	if hasBinaryVersionDrift(report.BinaryShadowing) {
		actions = append(actions, WorkspaceDoctorAction{
			ID:       "review_binary_version_drift",
			Severity: "medium",
			Command:  binaryShadowingLookupCommand(),
			Reason:   "one or more visible mi-lsp binaries report a different revision than the active binary",
		})
	}
	return actions
}

func gitTreeCaseCollisions(root string) [][]string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", root, "ls-tree", "-r", "HEAD", "--name-only")
	output, err := command.Output()
	if err != nil {
		return nil
	}
	byFoldedPath := map[string]map[string]bool{}
	for _, raw := range strings.Split(string(output), "\n") {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
		if byFoldedPath[key] == nil {
			byFoldedPath[key] = map[string]bool{}
		}
		byFoldedPath[key][path] = true
	}
	keys := make([]string, 0, len(byFoldedPath))
	for key, paths := range byFoldedPath {
		if len(paths) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	collisions := make([][]string, 0, len(keys))
	for _, key := range keys {
		collisions = append(collisions, sortedKeys(byFoldedPath[key]))
	}
	return collisions
}

func binaryShadowingLookupCommand() string {
	if runtime.GOOS == "windows" {
		return "where.exe mi-lsp"
	}
	return "which -a mi-lsp"
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

func ExplicitWorkspaceCWDWarnings(selector string, callerCWD string) []string {
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.TrimSpace(callerCWD) == "" {
		return nil
	}
	selected, err := ResolveWorkspaceSelection(selector, callerCWD)
	if err != nil {
		return nil
	}
	cwdResolution, ok := resolveWorkspaceFromCallerCWD(callerCWD, mustLoadRegistry())
	if !ok {
		return nil
	}
	selectedRoot, selectedOK := normalizeComparablePath(selected.Registration.Root)
	cwdRoot, cwdOK := normalizeComparablePath(cwdResolution.Registration.Root)
	if !selectedOK || !cwdOK || selectedRoot == cwdRoot {
		return nil
	}
	return []string{
		fmt.Sprintf("explicit workspace %q resolves to root %q, but caller cwd %q is inside registered workspace %q at %q; explicit workspace wins", selector, selected.Registration.Root, callerCWD, cwdResolution.Registration.Name, cwdResolution.Registration.Root),
	}
}

func mustLoadRegistry() model.RegistryFile {
	registry, err := LoadRegistry()
	if err != nil {
		return model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}
	}
	return registry
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

func gitCommonDir(root string) (string, bool) {
	ctxRoot := strings.TrimSpace(root)
	if ctxRoot == "" {
		return "", false
	}
	command := exec.Command("git", "-C", ctxRoot, "rev-parse", "--git-common-dir")
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	common := strings.TrimSpace(string(output))
	if common == "" {
		return "", false
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(ctxRoot, common)
	}
	if evaluated, err := filepath.EvalSymlinks(common); err == nil {
		common = evaluated
	}
	return filepath.Clean(common), true
}

func inspectBinaryShadowing() []BinaryCandidate {
	pathValue := os.Getenv("PATH")
	if pathValue == "" {
		return nil
	}
	active, _ := os.Executable()
	active = filepath.Clean(active)
	seen := map[string]bool{}
	candidates := make([]BinaryCandidate, 0)
	for _, dir := range filepath.SplitList(pathValue) {
		for _, name := range binaryNames() {
			candidate := filepath.Join(dir, name)
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			cleaned := filepath.Clean(candidate)
			key := cleaned
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			item := BinaryCandidate{Path: cleaned}
			if samePath(cleaned, active) {
				item.Active = true
			}
			applyBinaryMetadata(&item)
			candidates = append(candidates, item)
		}
	}
	if active != "." && active != "" {
		key := active
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			item := BinaryCandidate{Path: active, Active: true}
			applyBinaryMetadata(&item)
			candidates = append([]BinaryCandidate{item}, candidates...)
		}
	}
	if len(candidates) > 1 {
		activeRevision := activeBinaryRevision(candidates)
		for i := range candidates {
			if !candidates[i].Active {
				candidates[i].Warning = appendBinaryWarning(candidates[i].Warning, "another mi-lsp binary is visible on PATH; verify which binary your shell resolves")
			}
			if activeRevision != "" && candidates[i].Revision != "" && candidates[i].Revision != activeRevision {
				candidates[i].Warning = appendBinaryWarning(candidates[i].Warning, "binary revision differs from active mi-lsp binary")
			}
		}
	}
	return candidates
}

func applyBinaryMetadata(item *BinaryCandidate) {
	if item == nil || strings.TrimSpace(item.Path) == "" {
		return
	}
	output, err := exec.Command("go", "version", "-m", item.Path).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		item.Warning = appendBinaryWarning(item.Warning, "binary metadata unavailable: "+detail)
		return
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "build\tGOOS="):
			item.GOOS = strings.TrimPrefix(line, "build\tGOOS=")
		case strings.HasPrefix(line, "build\tGOARCH="):
			item.GOARCH = strings.TrimPrefix(line, "build\tGOARCH=")
		case strings.HasPrefix(line, "build\tvcs.revision="):
			item.Revision = strings.TrimPrefix(line, "build\tvcs.revision=")
		case strings.HasPrefix(line, "build\tvcs.modified="):
			item.Modified = strings.TrimPrefix(line, "build\tvcs.modified=")
		}
	}
}

func activeBinaryRevision(candidates []BinaryCandidate) string {
	for _, candidate := range candidates {
		if candidate.Active && strings.TrimSpace(candidate.Revision) != "" {
			return candidate.Revision
		}
	}
	return ""
}

func hasBinaryVersionDrift(candidates []BinaryCandidate) bool {
	activeRevision := activeBinaryRevision(candidates)
	if activeRevision == "" {
		return false
	}
	for _, candidate := range candidates {
		if candidate.Revision != "" && candidate.Revision != activeRevision {
			return true
		}
	}
	return false
}

func appendBinaryWarning(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	if strings.Contains(existing, next) {
		return existing
	}
	return existing + "; " + next
}

func binaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"mi-lsp.exe", "mi-lsp"}
	}
	return []string{"mi-lsp"}
}

func samePath(left string, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func sortedKeys(items map[string]bool) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendStringIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// AutoRegisterEnvVar disables implicit workspace registration when set to a
// truthy value ("1", "true", "yes", "on").
const AutoRegisterEnvVar = "MI_LSP_NO_AUTO_REGISTER"

// AutoRegisterModeEnvVar explicitly permits implicit registration outside the
// default scope when set to "force".
const AutoRegisterModeEnvVar = "MI_LSP_AUTOREGISTER"

// AutoRegisterResult describes what AutoRegisterWorkspace did. A zero value
// means nothing was registered (opted out, no candidate, or already known).
type AutoRegisterResult struct {
	Registered bool
	Alias      string
	Root       string
	// HasCommits is false for a repo without HEAD, which cannot be indexed yet.
	HasCommits bool
	Forced     bool
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
// empty), if that root is not registered yet. By default it only registers roots
// inside $HOME and outside the temp directory, and resolves linked worktrees
// through a registered main root.
// MI_LSP_AUTOREGISTER=force explicitly bypasses those scope restrictions. It
// never registers $HOME or the filesystem root, never overwrites an existing
// alias, and never touches an existing project.toml.
func AutoRegisterWorkspace(selector string, callerCWD string) (AutoRegisterResult, error) {
	root, ok := autoRegisterCandidateRoot(selector, callerCWD)
	if !ok {
		return AutoRegisterResult{}, nil
	}
	force := strings.EqualFold(strings.TrimSpace(os.Getenv(AutoRegisterModeEnvVar)), "force")
	if isLinkedWorktree, _, valid := linkedWorktree(root); valid && isLinkedWorktree {
		// Linked worktrees are resolved as physical read-only overlays. Never
		// register or silently alias one to the main worktree's content/index.
		return AutoRegisterResult{}, nil
	}
	if !force && !autoRegisterDefaultRootEligible(root) {
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
		// Persist the project file first so a failure cannot leave a registry entry
		// that makes later auto-registration calls skip the missing file.
		if _, statErr := os.Stat(ProjectConfigPath(registration.Root)); statErr != nil {
			if !os.IsNotExist(statErr) {
				return statErr
			}
			if err := saveAutoRegisterProjectFile(registration.Root, project); err != nil {
				return err
			}
		}
		registry.Workspaces[alias] = registration
		if err := SaveRegistry(registry); err != nil {
			return err
		}
		result = AutoRegisterResult{Registered: true, Alias: alias, Root: registration.Root, HasCommits: gitHasHead(registration.Root), Forced: force}
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

func autoRegisterDefaultRootEligible(root string) bool {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return false
	}
	rootPath, rootOK := normalizeComparablePath(root)
	homePath, homeOK := normalizeComparablePath(home)
	if !rootOK || !homeOK {
		return false
	}
	tempRoots := []string{os.TempDir()}
	if runtime.GOOS != "windows" {
		tempRoots = append(tempRoots, string(filepath.Separator)+"tmp")
	}
	for _, tempRoot := range tempRoots {
		if tempPath, ok := normalizeComparablePath(tempRoot); ok && pathContains(rootPath, tempPath) {
			return false
		}
	}
	return rootPath == homePath || pathContains(rootPath, homePath)
}

func linkedWorktree(root string) (bool, string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return false, "", false
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return false, "", false
	}
	gitPath := func(args ...string) (string, bool) {
		commandArgs := append([]string{"-C", root, "rev-parse"}, args...)
		out, err := processutil.Command("git", commandArgs...).Output()
		if err != nil {
			return "", false
		}
		path := strings.TrimSpace(string(out))
		if path == "" {
			return "", false
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return "", false
		}
		if evaluated, evalErr := filepath.EvalSymlinks(path); evalErr == nil {
			path = evaluated
		}
		return filepath.Clean(path), true
	}
	gitDir, ok := gitPath("--absolute-git-dir")
	if !ok {
		return false, "", false
	}
	commonDir, ok := gitPath("--path-format=absolute", "--git-common-dir")
	if !ok {
		// --path-format was added after older supported Git versions; the
		// legacy relative result is rooted at the worktree path above.
		commonDir, ok = gitPath("--git-common-dir")
	}
	if !ok {
		return false, "", false
	}
	gitDir, gitDirOK := normalizeComparablePath(gitDir)
	commonDir, commonDirOK := normalizeComparablePath(commonDir)
	if !gitDirOK || !commonDirOK {
		return false, "", false
	}
	return gitDir != commonDir, commonDir, true
}

// worktreeControlBacklink is a cheap preflight, not evidence of identity by itself.
// Git verification below is still required before any provenance is recorded.
func worktreeControlBacklink(root string) bool {
	control := filepath.Join(root, ".git")
	info, err := os.Stat(control)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return false
	}
	data, err := os.ReadFile(control)
	if err != nil {
		return false
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return false
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
	if gitDir == "" || strings.ContainsAny(gitDir, "\r\n") {
		return false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	gitDir, err = filepath.EvalSymlinks(gitDir)
	if err != nil {
		return false
	}
	backlinkFile := filepath.Join(gitDir, "gitdir")
	backlinkInfo, err := os.Stat(backlinkFile)
	if err != nil || !backlinkInfo.Mode().IsRegular() || backlinkInfo.Size() > 16384 {
		return false
	}
	backlink, err := os.ReadFile(backlinkFile)
	if err != nil || len(backlink) > 16384 {
		return false
	}
	backlinkPath := strings.TrimSpace(string(backlink))
	if !filepath.IsAbs(backlinkPath) {
		backlinkPath = filepath.Join(gitDir, backlinkPath)
	}
	return sameFilePath(backlinkPath, control)
}

// verifiedWorktreeIdentity records provenance only while both physical trees exist.
func verifiedWorktreeIdentity(root string) *model.WorktreeIdentity {
	physical, err := filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(physical) {
		return nil
	}
	physical = filepath.Clean(physical)
	if !worktreeControlBacklink(physical) {
		return nil
	}
	top, ok := gitTopLevel(physical)
	if !ok || !sameFilePath(top, physical) {
		return nil
	}
	linked, common, ok := linkedWorktree(physical)
	if !ok || !linked {
		return nil
	}
	main := filepath.Dir(common)
	mainTop, ok := gitTopLevel(main)
	if !ok || !sameFilePath(mainTop, main) {
		return nil
	}
	mainLinked, mainCommon, ok := linkedWorktree(main)
	if !ok || mainLinked || !sameFilePath(mainCommon, common) || sameFilePath(main, physical) {
		return nil
	}
	out, err := processutil.Command("git", "-C", physical, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return nil
	}
	gitDir, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil || !filepath.IsAbs(gitDir) || sameFilePath(gitDir, common) {
		return nil
	}
	backlink, err := os.ReadFile(filepath.Join(gitDir, "gitdir"))
	if err != nil || len(backlink) > 16384 {
		return nil
	}
	backlinkPath := strings.TrimSpace(string(backlink))
	if !filepath.IsAbs(backlinkPath) {
		backlinkPath = filepath.Join(gitDir, backlinkPath)
	}
	if !sameFilePath(backlinkPath, filepath.Join(physical, ".git")) {
		return nil
	}
	return &model.WorktreeIdentity{
		Version: 1, Verification: "verified", PhysicalRoot: physical, MainRoot: filepath.Clean(main),
		CommonDir: filepath.Clean(common), GitDir: filepath.Clean(gitDir),
	}
}

// refreshedWorktreeIdentity distinguishes positive non-WT proof from an
// indeterminate verification. Uncertainty preserves provenance but disqualifies
// it for automatic GC until a living tree can be verified again.
func refreshedWorktreeIdentity(root string, previous *model.WorktreeIdentity) *model.WorktreeIdentity {
	if previous != nil && previous.Version != 1 {
		identity := *previous
		return &identity
	}
	if identity := verifiedWorktreeIdentity(root); identity != nil {
		return identity
	}
	if previous == nil {
		return nil
	}
	if marker, err := os.Lstat(filepath.Join(root, ".git")); err == nil && marker.IsDir() {
		// Directory shape alone is not positive Git proof: permissions,
		// corruption or a transient command failure must retain provenance.
		top, topOK := gitTopLevel(root)
		linked, common, commonOK := linkedWorktree(root)
		if topOK && commonOK && !linked && sameFilePath(top, root) && sameFilePath(common, filepath.Join(root, ".git")) {
			return nil
		}
	}
	if previous != nil {
		identity := *previous
		identity.Verification = "indeterminate"
		return &identity
	}
	return nil
}

// validRecordedWorktreeIdentity never attempts to infer a missing tree's identity.
func validRecordedWorktreeIdentity(root string, id *model.WorktreeIdentity) bool {
	if id == nil || id.Version != 1 || id.Verification != "verified" {
		return false
	}
	for _, path := range []string{root, id.PhysicalRoot, id.MainRoot, id.CommonDir, id.GitDir} {
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return false
		}
	}
	return sameFilePath(root, id.PhysicalRoot) &&
		!sameFilePath(id.PhysicalRoot, id.MainRoot) &&
		!sameFilePath(id.GitDir, id.CommonDir) &&
		!sameFilePath(id.MainRoot, id.CommonDir) &&
		!sameFilePath(id.PhysicalRoot, id.CommonDir)
}

func registeredMainWorktree(commonDir string, registry model.RegistryFile) (model.WorkspaceRegistration, bool) {
	lastWorkspace := registry.Defaults.LastWorkspace
	var selected model.WorkspaceRegistration
	selectedAlias := ""
	for alias, registration := range registry.Workspaces {
		linked, registeredCommon, ok := linkedWorktree(registration.Root)
		if !ok || linked || registeredCommon != commonDir {
			continue
		}
		registration.Name = alias
		if alias == lastWorkspace {
			return registration, true
		}
		if selectedAlias == "" || alias < selectedAlias {
			selected = registration
			selectedAlias = alias
		}
	}
	if selectedAlias != "" {
		return selected, true
	}
	return model.WorkspaceRegistration{}, false
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

func saveAutoRegisterProjectFile(root string, project model.ProjectFile) error {
	stateDir := WorkspaceStateDir(root)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	path := ProjectConfigPath(root)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("project config path is not a regular file: %s", path)
		}
		return nil
	}
	if err := toml.NewEncoder(file).Encode(project); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func gitHasHead(root string) bool {
	return processutil.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "HEAD").Run() == nil
}

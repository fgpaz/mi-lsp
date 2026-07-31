package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestResolveWorkspaceSelectionRejectsStaleLastWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	caller := t.TempDir()
	staleRoot := filepath.Join(t.TempDir(), "missing-workspace")
	registerTestWorkspace(t, "stale", staleRoot)
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	registry.Defaults.LastWorkspace = "stale"
	if err := SaveRegistry(registry); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	for _, resolve := range []struct {
		name string
		fn   func(string, string) (WorkspaceResolution, error)
	}{
		{name: "normal", fn: ResolveWorkspaceSelection},
		{name: "read-only", fn: ResolveWorkspaceSelectionReadOnly},
	} {
		t.Run(resolve.name, func(t *testing.T) {
			_, err := resolve.fn("", caller)
			if err == nil {
				t.Fatal("stale last_workspace resolved successfully")
			}
			var selectorErr *WorkspaceSelectorError
			if !errors.As(err, &selectorErr) || selectorErr.Code != WorkspaceSelectorStale {
				t.Fatalf("error = %v, want %s", err, WorkspaceSelectorStale)
			}
		})
	}
}

func TestResolveWorkspaceSelectionPrefersCallerCWDOverLastWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	otherRoot := t.TempDir()
	callerRoot := t.TempDir()
	mustCreateDir(t, filepath.Join(callerRoot, "src", "backend"))

	if err := SaveProjectFile(callerRoot, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "interbancarizacion_coelsa",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(callerRoot): %v", err)
	}

	registerTestWorkspace(t, "interbancarizacion_coelsa", callerRoot)
	registerTestWorkspace(t, "mis-cals", otherRoot)

	resolution, err := ResolveWorkspaceSelection("", filepath.Join(callerRoot, "src", "backend"))
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if resolution.Registration.Name != "interbancarizacion_coelsa" {
		t.Fatalf("Registration.Name = %q, want interbancarizacion_coelsa", resolution.Registration.Name)
	}
	if resolution.Source != ResolutionSourceCallerCWD {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceCallerCWD)
	}
	if len(resolution.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", resolution.Warnings)
	}
}

func TestResolveWorkspaceSelectionUnknownExplicitAliasFailsClosedInBothModes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	callerCWD := filepath.Join(root, "src")
	mustCreateDir(t, callerCWD)
	registerTestWorkspace(t, "live", root)

	for _, tc := range []struct {
		name    string
		resolve func(string, string) (WorkspaceResolution, error)
	}{
		{name: "normal", resolve: ResolveWorkspaceSelection},
		{name: "read-only", resolve: ResolveWorkspaceSelectionReadOnly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.resolve("missing", callerCWD)
			if err == nil {
				t.Fatal("unknown explicit alias resolved successfully")
			}
			var selectorErr *WorkspaceSelectorError
			if !errors.As(err, &selectorErr) || selectorErr.Code != WorkspaceSelectorNotFound {
				t.Fatalf("err = %v, want %s", err, WorkspaceSelectorNotFound)
			}
		})
	}
}

func TestResolveWorkspaceSelectionReadOnlyPreservesNonGitContainment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	callerCWD := filepath.Join(root, "src", "backend")
	mustCreateDir(t, callerCWD)
	registerTestWorkspace(t, "plain", root)

	resolution, err := ResolveWorkspaceSelectionReadOnly("", callerCWD)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelectionReadOnly: %v", err)
	}
	if resolution.Source != ResolutionSourceCallerCWD {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceCallerCWD)
	}
	if resolution.Registration.Name != "plain" || resolution.Registration.Root != root {
		t.Fatalf("registration = %#v, want plain root %q", resolution.Registration, root)
	}
}

func TestResolveWorkspaceSelectionReadOnlyUsesGitTopLevelIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	mustRunGit(t, container, "init", "parent")
	mustRunGit(t, parent, "config", "user.email", "test@example.com")
	mustRunGit(t, parent, "config", "user.name", "Test User")
	writeRegistryTestFile(t, filepath.Join(parent, "parent.txt"), "parent")
	mustRunGit(t, parent, "add", ".")
	mustRunGit(t, parent, "commit", "-m", "parent")

	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	mustCreateDir(t, filepath.Dir(worktreeRoot))
	mustRunGit(t, parent, "worktree", "add", "--detach", worktreeRoot)
	t.Cleanup(func() {
		_ = runGitBestEffort(parent, "worktree", "remove", "--force", worktreeRoot)
		_ = runGitBestEffort(parent, "worktree", "prune")
	})

	registerTestWorkspace(t, "parent", parent)
	resolution, err := ResolveWorkspaceSelectionReadOnly("", worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelectionReadOnly(unregistered worktree): %v", err)
	}
	if resolution.Source != ResolutionSourceGitTopLevel {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceGitTopLevel)
	}
	worktreeIdentity, err := InspectWorkspaceIdentity(worktreeRoot)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity(worktree): %v", err)
	}
	resolvedIdentity, err := InspectWorkspaceIdentity(resolution.Registration.Root)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity(resolved): %v", err)
	}
	if resolvedIdentity.ComparableRoot != worktreeIdentity.ComparableRoot {
		t.Fatalf("resolved root = %q, want worktree root %q", resolvedIdentity.ComparableRoot, worktreeIdentity.ComparableRoot)
	}
	if parentIdentity, _ := InspectWorkspaceIdentity(parent); parentIdentity.ComparableRoot == resolvedIdentity.ComparableRoot {
		t.Fatal("unregistered Git worktree collapsed to lexical parent")
	}

	if !hasGitDir(worktreeRoot) {
		t.Fatal("valid .git worktree file was not detected")
	}

	registerTestWorkspace(t, "feature", worktreeRoot)
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	registry.Defaults.LastWorkspace = "parent"
	if err := SaveRegistry(registry); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	registered, err := ResolveWorkspaceSelectionReadOnly("", worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelectionReadOnly(registered worktree): %v", err)
	}
	if registered.Source != ResolutionSourceGitTopLevel {
		t.Fatalf("Source = %q, want %q", registered.Source, ResolutionSourceGitTopLevel)
	}
	if registered.Registration.Name != "feature" {
		t.Fatalf("Registration.Name = %q, want feature", registered.Registration.Name)
	}
	if registered.Registration.Root != worktreeRoot {
		t.Fatalf("Registration.Root = %q, want %q", registered.Registration.Root, worktreeRoot)
	}
	commonParent, commonParentOK := gitCommonDir(parent)
	commonWorktree, commonWorktreeOK := gitCommonDir(worktreeRoot)
	if commonParentOK && commonWorktreeOK && commonParent != commonWorktree {
		t.Fatalf("common dirs differ: parent=%q worktree=%q", commonParent, commonWorktree)
	}
	if store.OperationalWorkspaceDBPath(parent) == store.OperationalWorkspaceDBPath(worktreeRoot) {
		t.Fatal("worktree and parent share operational DB path")
	}
}

func TestResolveWorkspaceSelectionReadOnlyFailsClosedWhenGitRevParseFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	mustCreateDir(t, worktreeRoot)
	writeRegistryTestFile(t, filepath.Join(worktreeRoot, ".git"), "gitdir: ../../.git/worktrees/feature\n")
	registerTestWorkspace(t, "parent", parent)
	installFailingGit(t)

	resolution, err := ResolveWorkspaceSelectionReadOnly("", worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelectionReadOnly: %v", err)
	}
	if resolution.Source != ResolutionSourceGitTopLevel {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceGitTopLevel)
	}
	if filepath.Clean(resolution.Registration.Root) != filepath.Clean(worktreeRoot) {
		t.Fatalf("resolved root = %q, want synthetic marker root %q", resolution.Registration.Root, worktreeRoot)
	}
	if strings.Contains(strings.Join(resolution.Warnings, " "), filepath.Clean(parent)) {
		t.Fatalf("Warnings = %v, unexpectedly mention lexical parent", resolution.Warnings)
	}
	normal, err := ResolveWorkspaceSelection("", worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if filepath.Clean(normal.Registration.Root) != filepath.Clean(worktreeRoot) {
		t.Fatalf("normal resolved root = %q, want %q", normal.Registration.Root, worktreeRoot)
	}
}

func TestResolveWorkspaceSelectionReadOnlyFailsClosedForMalformedGitMarkers(t *testing.T) {
	cases := []struct {
		name       string
		markerFile string
		markerDir  bool
		wantStatus string
	}{
		{name: "malformed", markerFile: "not a git marker\n", wantStatus: "malformed marker file"},
		{name: "unparseable", markerFile: "gitdir:\n", wantStatus: "malformed marker file"},
		{name: "target inaccessible", markerFile: "gitdir: missing-worktree\n", wantStatus: "marker target inaccessible"},
		{name: "directory marker", markerDir: true, wantStatus: "directory marker"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)

			container := t.TempDir()
			parent := filepath.Join(container, "parent")
			worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
			mustCreateDir(t, worktreeRoot)
			if tc.markerDir {
				mustCreateDir(t, filepath.Join(worktreeRoot, ".git"))
			} else {
				writeRegistryTestFile(t, filepath.Join(worktreeRoot, ".git"), tc.markerFile)
			}
			registerTestWorkspace(t, "parent", parent)
			installFailingGit(t)

			marker, ok := inspectGitMarker(worktreeRoot)
			if !ok || marker.Root != worktreeRoot || marker.Status != tc.wantStatus {
				t.Fatalf("inspectGitMarker = %#v, %v; want root %q and status %q", marker, ok, worktreeRoot, tc.wantStatus)
			}

			resolution, err := ResolveWorkspaceSelectionReadOnly("", worktreeRoot)
			if err != nil {
				t.Fatalf("ResolveWorkspaceSelectionReadOnly: %v", err)
			}
			if resolution.Source != ResolutionSourceGitTopLevel {
				t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceGitTopLevel)
			}
			if filepath.Clean(resolution.Registration.Root) != filepath.Clean(worktreeRoot) {
				t.Fatalf("resolved root = %q, want %q", resolution.Registration.Root, worktreeRoot)
			}
			if filepath.Clean(resolution.Registration.Root) == filepath.Clean(parent) {
				t.Fatal("Git marker failure selected lexical parent")
			}
			if strings.Contains(strings.Join(resolution.Warnings, " "), filepath.Clean(parent)) {
				t.Fatalf("Warnings = %v, unexpectedly mention lexical parent", resolution.Warnings)
			}
			normal, err := ResolveWorkspaceSelection("", worktreeRoot)
			if err != nil {
				t.Fatalf("ResolveWorkspaceSelection: %v", err)
			}
			if filepath.Clean(normal.Registration.Root) != filepath.Clean(worktreeRoot) {
				t.Fatalf("normal resolved root = %q, want %q", normal.Registration.Root, worktreeRoot)
			}
		})
	}
}

func TestInspectGitMarkerUnsupportedMarkerFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	mustCreateDir(t, worktreeRoot)
	markerTarget := t.TempDir()
	if err := os.Symlink(markerTarget, filepath.Join(worktreeRoot, ".git")); err != nil {
		t.Skipf("symlink marker unavailable on %s: %v", runtime.GOOS, err)
	}
	registerTestWorkspace(t, "parent", parent)
	installFailingGit(t)

	marker, ok := inspectGitMarker(worktreeRoot)
	if !ok || marker.Root != worktreeRoot || marker.Status != "unsupported marker" {
		t.Fatalf("inspectGitMarker = %#v, %v; want unsupported marker at %q", marker, ok, worktreeRoot)
	}
	for _, resolve := range []struct {
		name string
		fn   func(string, string) (WorkspaceResolution, error)
	}{
		{name: "normal", fn: ResolveWorkspaceSelection},
		{name: "read-only", fn: ResolveWorkspaceSelectionReadOnly},
	} {
		t.Run(resolve.name, func(t *testing.T) {
			resolution, err := resolve.fn("", worktreeRoot)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if filepath.Clean(resolution.Registration.Root) != filepath.Clean(worktreeRoot) {
				t.Fatalf("resolved root = %q, want %q", resolution.Registration.Root, worktreeRoot)
			}
		})
	}
}

func TestInspectGitMarkerUnreadableMarkerFailsClosedWhenHostEnforcesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL setup is host-specific; skip when ACL support is unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	mustCreateDir(t, worktreeRoot)
	markerPath := filepath.Join(worktreeRoot, ".git")
	writeRegistryTestFile(t, markerPath, "gitdir: missing-worktree\n")
	originalMode := os.FileMode(0o644)
	if err := os.Chmod(markerPath, 0); err != nil {
		t.Skipf("marker permissions unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(markerPath, originalMode) })
	if _, err := os.ReadFile(markerPath); err == nil {
		t.Skip("host does not enforce unreadable file permissions for this test user")
	}
	registerTestWorkspace(t, "parent", parent)
	installFailingGit(t)

	marker, ok := inspectGitMarker(worktreeRoot)
	if !ok || marker.Root != worktreeRoot || marker.Status != "inaccessible marker file" {
		t.Fatalf("inspectGitMarker = %#v, %v; want inaccessible marker at %q", marker, ok, worktreeRoot)
	}
	resolution, err := ResolveWorkspaceSelection("", worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if filepath.Clean(resolution.Registration.Root) != filepath.Clean(worktreeRoot) {
		t.Fatalf("resolved root = %q, want %q", resolution.Registration.Root, worktreeRoot)
	}
}

func TestResolveWorkspaceSelectionUsesProjectNameForSameRootAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, "src"))

	if err := SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "interbancarizacion_coelsa",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(root): %v", err)
	}

	registerTestWorkspace(t, "coelsa", root)
	registerTestWorkspace(t, "interbanc-parent", root)
	registerTestWorkspace(t, "interbancarizacion_coelsa", root)

	resolution, err := ResolveWorkspaceSelection("", filepath.Join(root, "src"))
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if resolution.Registration.Name != "interbancarizacion_coelsa" {
		t.Fatalf("Registration.Name = %q, want interbancarizacion_coelsa", resolution.Registration.Name)
	}
	if resolution.Source != ResolutionSourceCallerCWD {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceCallerCWD)
	}
	if len(resolution.Warnings) == 0 {
		t.Fatal("expected ambiguity warning for same-root aliases")
	}
	if !strings.Contains(strings.Join(resolution.Warnings, " "), "multiple registry aliases") {
		t.Fatalf("Warnings = %v, want multiple registry aliases message", resolution.Warnings)
	}
}

func TestResolveWorkspaceSelectionFallsBackToLastWorkspaceWhenCWDDoesNotMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	unmatched := t.TempDir()
	registerTestWorkspace(t, "mis-cals", root)

	resolution, err := ResolveWorkspaceSelection("", unmatched)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if resolution.Registration.Name != "mis-cals" {
		t.Fatalf("Registration.Name = %q, want mis-cals", resolution.Registration.Name)
	}
	if resolution.Source != ResolutionSourceLastWorkspace {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceLastWorkspace)
	}
	if len(resolution.Warnings) == 0 {
		t.Fatal("expected last_workspace fallback warning")
	}
	if !strings.Contains(strings.Join(resolution.Warnings, " "), "last_workspace") {
		t.Fatalf("Warnings = %v, want last_workspace message", resolution.Warnings)
	}
}

func TestDoctorWorkspacesReportsWorktreeFamiliesWithoutCollapsingAliases(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	parent := t.TempDir()
	mainRoot := filepath.Join(parent, "repo")
	worktreeRoot := filepath.Join(parent, "repo-feature")
	mustRunGit(t, parent, "init", "repo")
	mustRunGit(t, mainRoot, "config", "user.email", "test@example.com")
	mustRunGit(t, mainRoot, "config", "user.name", "Test User")
	mustCreateDir(t, filepath.Join(mainRoot, "src"))
	writeRegistryTestFile(t, filepath.Join(mainRoot, "src", "main.txt"), "main")
	mustRunGit(t, mainRoot, "add", ".")
	mustRunGit(t, mainRoot, "commit", "-m", "init")
	mustRunGit(t, mainRoot, "worktree", "add", worktreeRoot, "-b", "feature")
	t.Cleanup(func() {
		_ = runGitBestEffort(mainRoot, "worktree", "remove", "--force", worktreeRoot)
		_ = runGitBestEffort(mainRoot, "worktree", "prune")
	})

	registerTestWorkspace(t, "mi-lsp-main", mainRoot)
	registerTestWorkspace(t, "mi-lsp-feature", worktreeRoot)

	report, err := DoctorWorkspaces()
	if err != nil {
		t.Fatalf("DoctorWorkspaces: %v", err)
	}
	if len(report.WorktreeFamilies) != 1 {
		t.Fatalf("WorktreeFamilies = %#v, want one family", report.WorktreeFamilies)
	}
	family := report.WorktreeFamilies[0]
	if len(family.Roots) != 2 {
		t.Fatalf("family.Roots = %#v, want two roots", family.Roots)
	}
	if !containsString(family.Aliases, "mi-lsp-main") || !containsString(family.Aliases, "mi-lsp-feature") {
		t.Fatalf("family.Aliases = %#v, want both worktree aliases", family.Aliases)
	}
	if report.Health != "attention" {
		t.Fatalf("Health = %q, want attention", report.Health)
	}
	if !doctorActionsContain(report.NextActions, "verify_worktree_aliases") {
		t.Fatalf("NextActions = %#v, want verify_worktree_aliases", report.NextActions)
	}
}

func TestDoctorWorkspacesReportsGitCaseCollisions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	mustRunGit(t, parent, "init", "repo")
	mustRunGit(t, root, "config", "user.email", "test@example.com")
	mustRunGit(t, root, "config", "user.name", "Test User")
	blob := mustRunGitInput(t, root, "case collision\n", "hash-object", "-w", "--stdin")
	mustRunGit(t, root, "update-index", "--add", "--cacheinfo", "100644,"+blob+",Docs/API.md")
	mustRunGit(t, root, "update-index", "--add", "--cacheinfo", "100644,"+blob+",docs/api.md")
	mustRunGit(t, root, "commit", "-m", "case collision")

	registerTestWorkspace(t, "case-collision", root)

	report, err := DoctorWorkspaces()
	if err != nil {
		t.Fatalf("DoctorWorkspaces: %v", err)
	}
	if report.Health != "action_required" {
		t.Fatalf("Health = %q, want action_required", report.Health)
	}
	if len(report.GitCaseCollisions) != 1 {
		t.Fatalf("GitCaseCollisions = %#v, want one collision", report.GitCaseCollisions)
	}
	paths := report.GitCaseCollisions[0].Paths
	if !containsString(paths, "Docs/API.md") || !containsString(paths, "docs/api.md") {
		t.Fatalf("collision paths = %#v, want both casings", paths)
	}
	if !doctorActionsContain(report.NextActions, "fix_git_case_collisions") {
		t.Fatalf("NextActions = %#v, want fix_git_case_collisions", report.NextActions)
	}
}

func TestPruneStaleWorkspacesDryRunAndApply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	liveRoot := t.TempDir()
	missingRoot := filepath.Join(t.TempDir(), "missing-worktree")
	registerTestWorkspace(t, "live", liveRoot)
	registerTestWorkspace(t, "stale", missingRoot)
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	registry.Workspaces["empty"] = model.WorkspaceRegistration{Name: "empty", Root: "", Kind: "mixed"}
	if err := SaveRegistry(registry); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	dryRun, err := PruneStaleWorkspaces(false)
	if err != nil {
		t.Fatalf("PruneStaleWorkspaces(false): %v", err)
	}
	if !dryRun.DryRun {
		t.Fatal("dry run report should have DryRun=true")
	}
	if len(dryRun.Candidates) != 2 || dryRun.Candidates[0].Alias != "empty" || dryRun.Candidates[1].Alias != "stale" {
		t.Fatalf("dry run candidates = %#v, want empty + stale", dryRun.Candidates)
	}
	if _, err := ResolveWorkspace("stale"); err != nil {
		t.Fatalf("dry run removed stale alias: %v", err)
	}
	if _, err := ResolveWorkspaceSelectionReadOnly("stale", ""); err == nil {
		t.Fatal("read-only explicit stale selector should fail closed")
	}

	applied, err := PruneStaleWorkspaces(true)
	if err != nil {
		t.Fatalf("PruneStaleWorkspaces(true): %v", err)
	}
	if applied.DryRun {
		t.Fatal("apply report should have DryRun=false")
	}
	if applied.RemovedCount != 2 || len(applied.Removed) != 2 {
		t.Fatalf("applied removed = %#v count=%d, want empty + stale", applied.Removed, applied.RemovedCount)
	}
	if _, err := ResolveWorkspace("stale"); err == nil {
		t.Fatal("stale alias should be removed after apply")
	}
	if _, err := ResolveWorkspace("empty"); err == nil {
		t.Fatal("empty-root alias should be removed after apply")
	}
	if _, err := ResolveWorkspace("live"); err != nil {
		t.Fatalf("live alias should remain: %v", err)
	}
}

func TestDoctorWorkspacesReportsActionRequiredForStaleAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	liveRoot := t.TempDir()
	missingRoot := filepath.Join(t.TempDir(), "missing-worktree")
	registerTestWorkspace(t, "live", liveRoot)
	registerTestWorkspace(t, "stale", missingRoot)

	report, err := DoctorWorkspaces()
	if err != nil {
		t.Fatalf("DoctorWorkspaces: %v", err)
	}
	if report.Health != "action_required" {
		t.Fatalf("Health = %q, want action_required", report.Health)
	}
	if !doctorActionsContain(report.NextActions, "prune_stale_aliases") {
		t.Fatalf("NextActions = %#v, want prune_stale_aliases", report.NextActions)
	}
}

func TestDoctorWorkspacesBinaryShadowingActionUsesHostCommand(t *testing.T) {
	report := WorkspaceDoctorReport{
		BinaryShadowing: []BinaryCandidate{
			{Path: "/tmp/mi-lsp", Active: true},
			{Path: "/usr/bin/mi-lsp"},
		},
	}

	actions := workspaceDoctorNextActions(report)
	var command string
	for _, action := range actions {
		if action.ID == "review_binary_shadowing" {
			command = action.Command
			break
		}
	}
	if command == "" {
		t.Fatalf("NextActions = %#v, want review_binary_shadowing", actions)
	}
	want := "which -a mi-lsp"
	if runtime.GOOS == "windows" {
		want = "where.exe mi-lsp"
	}
	if command != want {
		t.Fatalf("binary shadowing command = %q, want %q", command, want)
	}
}

func TestDoctorWorkspacesBinaryRevisionDriftAction(t *testing.T) {
	report := WorkspaceDoctorReport{
		BinaryShadowing: []BinaryCandidate{
			{Path: "/tmp/mi-lsp", Active: true, Revision: "aaa"},
			{Path: "/usr/bin/mi-lsp", Revision: "bbb"},
		},
	}

	actions := workspaceDoctorNextActions(report)
	if !doctorActionsContain(actions, "review_binary_version_drift") {
		t.Fatalf("NextActions = %#v, want review_binary_version_drift", actions)
	}
}

func registerTestWorkspace(t *testing.T, alias string, root string) {
	t.Helper()
	if _, err := RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(%s): %v", alias, err)
	}
	t.Cleanup(func() {
		_ = RemoveWorkspace(alias)
	})
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func installFailingGit(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	name := "git"
	contents := "#!/bin/sh\nexit 127\n"
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		name = "git.cmd"
		contents = "@echo off\r\nexit /b 127\r\n"
		mode = 0o644
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), mode); err != nil {
		t.Fatalf("WriteFile(failing git): %v", err)
	}
	t.Setenv("PATH", dir)
}

func runGitBestEffort(dir string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = dir
	return command.Run()
}

func mustRunGitInput(t *testing.T, dir string, input string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return strings.TrimSpace(string(output))
}

func writeRegistryTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func doctorActionsContain(actions []WorkspaceDoctorAction, id string) bool {
	for _, action := range actions {
		if action.ID == id {
			return true
		}
	}
	return false
}

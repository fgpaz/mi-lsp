package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

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

func TestPruneStaleWorkspacesDryRunAndApply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	liveRoot := t.TempDir()
	missingRoot := filepath.Join(t.TempDir(), "missing-worktree")
	registerTestWorkspace(t, "live", liveRoot)
	registerTestWorkspace(t, "stale", missingRoot)

	dryRun, err := PruneStaleWorkspaces(false)
	if err != nil {
		t.Fatalf("PruneStaleWorkspaces(false): %v", err)
	}
	if !dryRun.DryRun {
		t.Fatal("dry run report should have DryRun=true")
	}
	if len(dryRun.Candidates) != 1 || dryRun.Candidates[0].Alias != "stale" {
		t.Fatalf("dry run candidates = %#v, want stale", dryRun.Candidates)
	}
	if _, err := ResolveWorkspace("stale"); err != nil {
		t.Fatalf("dry run removed stale alias: %v", err)
	}

	applied, err := PruneStaleWorkspaces(true)
	if err != nil {
		t.Fatalf("PruneStaleWorkspaces(true): %v", err)
	}
	if applied.DryRun {
		t.Fatal("apply report should have DryRun=false")
	}
	if applied.RemovedCount != 1 || len(applied.Removed) != 1 || applied.Removed[0].Alias != "stale" {
		t.Fatalf("applied removed = %#v count=%d, want stale", applied.Removed, applied.RemovedCount)
	}
	if _, err := ResolveWorkspace("stale"); err == nil {
		t.Fatal("stale alias should be removed after apply")
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

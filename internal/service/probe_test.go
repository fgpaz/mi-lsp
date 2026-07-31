package service

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type probeSnapshotEntry struct {
	Mode   os.FileMode
	Size   int64
	Mtime  int64
	SHA256 string
	IsDir  bool
}

func TestProbeWithoutDatabaseReportsPartialAndPreservesDeepSnapshots(t *testing.T) {
	home, root := prepareProbeWorkspace(t, true)
	beforeRoot := deepProbeSnapshot(t, root)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.Status != model.ProbeStatusPartial {
		t.Fatalf("status = %q, want partial", report.Status)
	}
	if report.SideEffects {
		t.Fatal("probe reported side effects")
	}
	if report.State.Operational != "missing" {
		t.Fatalf("operational = %q, want missing", report.State.Operational)
	}
	if report.Evidence.DBMode != "not_opened" {
		t.Fatalf("db mode = %q, want not_opened", report.Evidence.DBMode)
	}
	assertDeepProbeSnapshotEqual(t, beforeRoot, deepProbeSnapshot(t, root))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeNestedGitWorktreeDoesNotReadLexicalParentState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	source := filepath.Join(container, "source")
	mustProbeGit(t, container, "init", "parent")
	mustProbeGit(t, parent, "config", "user.email", "test@example.com")
	mustProbeGit(t, parent, "config", "user.name", "Test User")
	if err := workspace.SaveProjectFile(parent, model.ProjectFile{Project: model.ProjectBlock{Name: "parent", Kind: model.WorkspaceKindSingle}}); err != nil {
		t.Fatalf("SaveProjectFile(parent): %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "parent.go"), []byte("package parent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent): %v", err)
	}
	mustProbeGit(t, parent, "add", ".")
	mustProbeGit(t, parent, "commit", "-m", "parent")
	parentDB, err := store.Open(parent)
	if err != nil {
		t.Fatalf("Open parent DB: %v", err)
	}
	if err := parentDB.Close(); err != nil {
		t.Fatalf("Close parent DB: %v", err)
	}

	mustProbeGit(t, container, "init", "source")
	mustProbeGit(t, source, "config", "user.email", "test@example.com")
	mustProbeGit(t, source, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(source, "source.go"), []byte("package source\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(source): %v", err)
	}
	mustProbeGit(t, source, "add", ".")
	mustProbeGit(t, source, "commit", "-m", "source")

	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	if err := os.MkdirAll(filepath.Dir(worktreeRoot), 0o755); err != nil {
		t.Fatalf("MkdirAll worktree parent: %v", err)
	}
	mustProbeGit(t, source, "worktree", "add", "--detach", worktreeRoot)
	t.Cleanup(func() {
		_ = runProbeGitBestEffort(source, "worktree", "remove", "--force", worktreeRoot)
		_ = runProbeGitBestEffort(source, "worktree", "prune")
	})

	if _, err := workspace.RegisterWorkspace("parent", model.WorkspaceRegistration{Name: "parent", Root: parent}); err != nil {
		t.Fatalf("RegisterWorkspace(parent): %v", err)
	}
	beforeParent := deepProbeSnapshot(t, parent)
	beforeWorktree := deepProbeSnapshot(t, worktreeRoot)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{CallerCWD: worktreeRoot})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	worktreeIdentity, err := workspace.InspectWorkspaceIdentity(worktreeRoot)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity(worktree): %v", err)
	}
	if report.Workspace.CanonicalRoot != worktreeIdentity.CanonicalRoot {
		t.Fatalf("canonical root = %q, want %q", report.Workspace.CanonicalRoot, worktreeIdentity.CanonicalRoot)
	}
	if report.Workspace.CanonicalRoot == (mustProbeIdentity(t, parent)).CanonicalRoot {
		t.Fatal("probe selected lexical parent Git root")
	}
	if report.Workspace.ResolutionSource != string(workspace.ResolutionSourceGitTopLevel) {
		t.Fatalf("resolution source = %q, want %q", report.Workspace.ResolutionSource, workspace.ResolutionSourceGitTopLevel)
	}
	if report.SideEffects {
		t.Fatal("probe reported side effects")
	}
	if report.State.PortablePath == workspace.ProjectConfigPath(parent) || report.State.LegacyPath == store.WorkspaceDBPath(parent) {
		t.Fatal("probe inspected parent state paths")
	}
	if report.State.Config != "missing" || report.State.Operational != "missing" || report.Status != model.ProbeStatusAbsent {
		t.Fatalf("state=%#v status=%q, want missing/missing/absent", report.State, report.Status)
	}
	assertDeepProbeSnapshotEqual(t, beforeParent, deepProbeSnapshot(t, parent))
	assertDeepProbeSnapshotEqual(t, beforeWorktree, deepProbeSnapshot(t, worktreeRoot))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeGitRevParseFailureDoesNotReadLexicalParentState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	mustProbeGit(t, container, "init", "parent")
	mustProbeGit(t, parent, "config", "user.email", "test@example.com")
	mustProbeGit(t, parent, "config", "user.name", "Test User")
	if err := workspace.SaveProjectFile(parent, model.ProjectFile{Project: model.ProjectBlock{Name: "parent", Kind: model.WorkspaceKindSingle}}); err != nil {
		t.Fatalf("SaveProjectFile(parent): %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "parent.go"), []byte("package parent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent): %v", err)
	}
	mustProbeGit(t, parent, "add", ".")
	mustProbeGit(t, parent, "commit", "-m", "parent")
	parentDB, err := store.Open(parent)
	if err != nil {
		t.Fatalf("Open parent DB: %v", err)
	}
	if err := parentDB.Close(); err != nil {
		t.Fatalf("Close parent DB: %v", err)
	}

	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	if err := os.MkdirAll(filepath.Dir(worktreeRoot), 0o755); err != nil {
		t.Fatalf("MkdirAll worktree parent: %v", err)
	}
	mustProbeGit(t, parent, "worktree", "add", "--detach", worktreeRoot)
	t.Cleanup(func() {
		_ = runProbeGitBestEffort(parent, "worktree", "remove", "--force", worktreeRoot)
		_ = runProbeGitBestEffort(parent, "worktree", "prune")
	})
	if err := workspace.SaveProjectFile(worktreeRoot, model.ProjectFile{Project: model.ProjectBlock{Name: "feature", Kind: model.WorkspaceKindSingle}}); err != nil {
		t.Fatalf("SaveProjectFile(worktree): %v", err)
	}
	worktreeDB, err := store.Open(worktreeRoot)
	if err != nil {
		t.Fatalf("Open worktree DB: %v", err)
	}
	if err := worktreeDB.Close(); err != nil {
		t.Fatalf("Close worktree DB: %v", err)
	}

	if _, err := workspace.RegisterWorkspace("parent", model.WorkspaceRegistration{Name: "parent", Root: parent}); err != nil {
		t.Fatalf("RegisterWorkspace(parent): %v", err)
	}
	beforeParent := deepProbeSnapshot(t, parent)
	beforeWorktree := deepProbeSnapshot(t, worktreeRoot)
	beforeHome := deepProbeSnapshot(t, home)
	installFailingProbeGit(t)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{CallerCWD: worktreeRoot})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	worktreeIdentity, err := workspace.InspectWorkspaceIdentity(worktreeRoot)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity(worktree): %v", err)
	}
	if report.Workspace.CanonicalRoot != worktreeIdentity.CanonicalRoot {
		t.Fatalf("canonical root = %q, want %q", report.Workspace.CanonicalRoot, worktreeIdentity.CanonicalRoot)
	}
	if report.Workspace.CanonicalRoot == mustProbeIdentity(t, parent).CanonicalRoot {
		t.Fatal("probe selected lexical parent Git root")
	}
	if report.Workspace.ResolutionSource != string(workspace.ResolutionSourceGitTopLevel) {
		t.Fatalf("resolution source = %q, want %q", report.Workspace.ResolutionSource, workspace.ResolutionSourceGitTopLevel)
	}
	if report.State.PortablePath != workspace.ProjectConfigPath(worktreeRoot) || report.State.LegacyPath != store.WorkspaceDBPath(worktreeRoot) {
		t.Fatalf("state paths = %#v, want worktree paths", report.State)
	}
	if report.State.PortablePath == workspace.ProjectConfigPath(parent) || report.State.LegacyPath == store.WorkspaceDBPath(parent) || report.State.OperationalPath == store.OperationalWorkspaceDBPath(parent) {
		t.Fatal("probe selected parent state paths")
	}
	if report.State.Config != "portable" || report.State.Database != "present_ro" || report.Status != model.ProbeStatusCurrent {
		t.Fatalf("state=%#v status=%q, want portable/present_ro/current", report.State, report.Status)
	}
	for _, path := range report.Evidence.Files {
		if path == workspace.ProjectConfigPath(parent) || path == store.WorkspaceDBPath(parent) {
			t.Fatalf("evidence inspected parent path %q", path)
		}
	}
	assertDeepProbeSnapshotEqual(t, beforeParent, deepProbeSnapshot(t, parent))
	assertDeepProbeSnapshotEqual(t, beforeWorktree, deepProbeSnapshot(t, worktreeRoot))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeMissingExplicitAliasFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	if _, err := workspace.RegisterWorkspace("live", model.WorkspaceRegistration{Name: "live", Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if _, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "missing", CallerCWD: root}); err == nil {
		t.Fatal("missing explicit alias should fail closed")
	}
}

func TestProbeOmittedStaleLastWorkspaceFailsTypedWithoutReadingRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))

	caller := t.TempDir()
	staleRoot := filepath.Join(t.TempDir(), "missing-workspace")
	if _, err := workspace.RegisterWorkspace("stale", model.WorkspaceRegistration{Name: "stale", Root: staleRoot}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	beforeCaller := deepProbeSnapshot(t, caller)
	beforeHome := deepProbeSnapshot(t, home)

	_, err := ProbeWorkspace(t.Context(), model.ProbeOptions{CallerCWD: caller})
	if err == nil {
		t.Fatal("stale last_workspace should fail typed")
	}
	var selectorErr *workspace.WorkspaceSelectorError
	if !errors.As(err, &selectorErr) || selectorErr.Code != workspace.WorkspaceSelectorStale {
		t.Fatalf("error = %v, want %s", err, workspace.WorkspaceSelectorStale)
	}
	assertDeepProbeSnapshotEqual(t, beforeCaller, deepProbeSnapshot(t, caller))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeStatePathReadErrorIsUnknownAndSanitized(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))

	root := t.TempDir()
	if err := os.WriteFile(workspace.WorkspaceStateDir(root), []byte("state path blocker"), 0o644); err != nil {
		t.Fatalf("WriteFile(state dir blocker): %v", err)
	}
	if _, err := workspace.RegisterWorkspace("blocked-state", model.WorkspaceRegistration{Name: "blocked-state", Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "blocked-state", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.Status != model.ProbeStatusUnknown || report.SideEffects {
		t.Fatalf("status=%q side_effects=%v, want unknown/false", report.Status, report.SideEffects)
	}
	if report.State.Config != "unknown" || report.State.Operational != "unknown" || report.State.Database != "not_checked" {
		t.Fatalf("state=%#v, want unknown/unknown/not_checked", report.State)
	}
	warnings := strings.Join(report.Warnings, " ")
	if !strings.Contains(warnings, "evidence_unavailable_or_unreadable") || !strings.Contains(warnings, "not a directory") {
		t.Fatalf("warnings=%v, want sanitized unreadable-state reason", report.Warnings)
	}
	if strings.Contains(warnings, root) {
		t.Fatalf("warnings=%v, leaked state root path", report.Warnings)
	}
}

func TestProbeOperationalStatePathUnreadableReportsUnknownViaStatSeam(t *testing.T) {
	_, root := prepareProbeWorkspace(t, true)
	target := store.OperationalWorkspaceDBPath(root)
	if target == "" {
		t.Skip("operational state path unavailable on this host")
	}
	originalStat := probeStat
	t.Cleanup(func() { probeStat = originalStat })
	probeStat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(target) {
			return nil, &os.PathError{Op: "stat", Path: path, Err: os.ErrPermission}
		}
		return originalStat(path)
	}

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.Status != model.ProbeStatusUnknown || report.State.Operational != "unknown" || report.State.Database != "not_checked" {
		t.Fatalf("status=%q state=%#v, want unknown/unknown/not_checked", report.Status, report.State)
	}
	warnings := strings.Join(report.Warnings, " ")
	if !strings.Contains(warnings, "operational state: evidence_unavailable_or_unreadable") || !strings.Contains(warnings, "permission denied") {
		t.Fatalf("warnings=%v, want sanitized operational unreadable classification", report.Warnings)
	}
	if strings.Contains(warnings, root) {
		t.Fatalf("warnings=%v, leaked workspace root", report.Warnings)
	}
}

func TestProbeLegacyStatePathUnreadableReportsUnknownViaStatSeam(t *testing.T) {
	_, root := prepareProbeWorkspace(t, true)
	target := store.WorkspaceDBPath(root)
	originalStat := probeStat
	t.Cleanup(func() { probeStat = originalStat })
	probeStat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(target) {
			return nil, &os.PathError{Op: "stat", Path: path, Err: os.ErrPermission}
		}
		return originalStat(path)
	}

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.Status != model.ProbeStatusUnknown || report.State.Operational != "unknown" || report.State.Database != "not_checked" {
		t.Fatalf("status=%q state=%#v, want unknown/unknown/not_checked", report.Status, report.State)
	}
	warnings := strings.Join(report.Warnings, " ")
	if !strings.Contains(warnings, "legacy state: evidence_unavailable_or_unreadable") || !strings.Contains(warnings, "permission denied") {
		t.Fatalf("warnings=%v, want sanitized legacy unreadable classification", report.Warnings)
	}
	if strings.Contains(warnings, root) {
		t.Fatalf("warnings=%v, leaked workspace root", report.Warnings)
	}
}

func TestProbeExplicitFileRootFailsTyped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	file := filepath.Join(root, "not-a-workspace-root")
	if err := os.WriteFile(file, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := workspace.RegisterWorkspace("file-root", model.WorkspaceRegistration{Name: "file-root", Root: file}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	_, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "file-root", CallerCWD: root})
	if err == nil {
		t.Fatal("file root should fail")
	}
	var selectorErr *workspace.WorkspaceSelectorError
	if !errors.As(err, &selectorErr) || selectorErr.Code != workspace.WorkspaceSelectorNotDirectory {
		t.Fatalf("error = %v, want %s", err, workspace.WorkspaceSelectorNotDirectory)
	}
}

func TestProbeBothDatabasesPrefersLocalOperationalAndReportsLegacy(t *testing.T) {
	home, root := prepareProbeWorkspace(t, true)
	legacy, err := store.Open(root)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("Close legacy: %v", err)
	}
	createOperationalDB(t, root)
	writeProbeSidecars(t, store.WorkspaceDBPath(root), "legacy")
	writeProbeSidecars(t, store.OperationalWorkspaceDBPath(root), "operational")
	beforeRoot := deepProbeSnapshot(t, root)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.State.Operational != "local" {
		t.Fatalf("operational = %q, want local", report.State.Operational)
	}
	if report.State.MigrationStatus != "legacy_present" {
		t.Fatalf("migration status = %q, want legacy_present", report.State.MigrationStatus)
	}
	if report.State.Database != "present_ro" || report.Evidence.DBMode != "ro" {
		t.Fatalf("database=%q db_mode=%q, want present_ro/ro", report.State.Database, report.Evidence.DBMode)
	}
	if report.Status != model.ProbeStatusCurrent {
		t.Fatalf("status = %q, want current", report.Status)
	}
	assertDeepProbeSnapshotEqual(t, beforeRoot, deepProbeSnapshot(t, root))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeDatabaseWithoutConfigIsPartialAndPreservesSnapshots(t *testing.T) {
	home, root := prepareProbeWorkspace(t, false)
	legacy, err := store.Open(root)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("Close legacy: %v", err)
	}
	beforeRoot := deepProbeSnapshot(t, root)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.State.Config != "missing" || report.State.Operational != "local" {
		t.Fatalf("config=%q operational=%q, want missing/local", report.State.Config, report.State.Operational)
	}
	if report.Status != model.ProbeStatusPartial {
		t.Fatalf("status = %q, want partial", report.Status)
	}
	assertDeepProbeSnapshotEqual(t, beforeRoot, deepProbeSnapshot(t, root))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeDatabaseWithUnreadableConfigIsUnknown(t *testing.T) {
	home, root := prepareProbeWorkspace(t, false)
	legacy, err := store.Open(root)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("Close legacy: %v", err)
	}
	configPath := workspace.ProjectConfigPath(root)
	if err := os.WriteFile(configPath, []byte("[project\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	beforeRoot := deepProbeSnapshot(t, root)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.State.Config != "unknown" || report.Status == model.ProbeStatusCurrent {
		t.Fatalf("config=%q status=%q, want unknown and not current", report.State.Config, report.Status)
	}
	assertDeepProbeSnapshotEqual(t, beforeRoot, deepProbeSnapshot(t, root))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeDatabaseWithDirectoryConfigIsUnknown(t *testing.T) {
	home, root := prepareProbeWorkspace(t, false)
	legacy, err := store.Open(root)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("Close legacy: %v", err)
	}
	if err := os.Mkdir(workspace.ProjectConfigPath(root), 0o755); err != nil {
		t.Fatalf("Mkdir config: %v", err)
	}
	beforeRoot := deepProbeSnapshot(t, root)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{Selector: "probe-fixture", CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.State.Config != "unknown" || report.Status == model.ProbeStatusCurrent {
		t.Fatalf("config=%q status=%q, want unknown and not current", report.State.Config, report.Status)
	}
	assertDeepProbeSnapshotEqual(t, beforeRoot, deepProbeSnapshot(t, root))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func TestProbeNoRegistryOrDatabaseDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	beforeRoot := deepProbeSnapshot(t, root)
	beforeHome := deepProbeSnapshot(t, home)

	report, err := ProbeWorkspace(t.Context(), model.ProbeOptions{CallerCWD: root})
	if err != nil {
		t.Fatalf("ProbeWorkspace: %v", err)
	}
	if report.Status != model.ProbeStatusAbsent {
		t.Fatalf("status = %q, want absent", report.Status)
	}
	assertDeepProbeSnapshotEqual(t, beforeRoot, deepProbeSnapshot(t, root))
	assertDeepProbeSnapshotEqual(t, beforeHome, deepProbeSnapshot(t, home))
}

func prepareProbeWorkspace(t *testing.T, withConfig bool) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))
	root := t.TempDir()
	if withConfig {
		if err := workspace.SaveProjectFile(root, model.ProjectFile{Project: model.ProjectBlock{Name: "probe-fixture", Kind: model.WorkspaceKindSingle}}); err != nil {
			t.Fatalf("SaveProjectFile: %v", err)
		}
	}
	if _, err := workspace.RegisterWorkspace("probe-fixture", model.WorkspaceRegistration{Name: "probe-fixture", Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	return home, root
}

func createOperationalDB(t *testing.T, root string) {
	t.Helper()
	path := store.OperationalWorkspaceDBPath(root)
	if path == "" {
		t.Fatal("operational path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll operational state: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open operational: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE probe_fixture(id INTEGER)"); err != nil {
		db.Close()
		t.Fatalf("create operational fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close operational fixture: %v", err)
	}
}

func mustProbeGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func runProbeGitBestEffort(dir string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = dir
	return command.Run()
}

func installFailingProbeGit(t *testing.T) {
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

func mustProbeIdentity(t *testing.T, root string) workspace.WorkspaceIdentity {
	t.Helper()
	identity, err := workspace.InspectWorkspaceIdentity(root)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity(%s): %v", root, err)
	}
	return identity
}

func writeProbeSidecars(t *testing.T, dbPath string, label string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		path := dbPath + suffix
		if err := os.WriteFile(path, []byte("preexisting "+label+suffix), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}
}

func deepProbeSnapshot(t *testing.T, root string) map[string]probeSnapshotEntry {
	t.Helper()
	result := map[string]probeSnapshotEntry{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := probeSnapshotEntry{Mode: info.Mode(), Size: info.Size(), Mtime: info.ModTime().UnixNano(), IsDir: info.IsDir()}
		if !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			digest := sha256.Sum256(data)
			entry.SHA256 = hex.EncodeToString(digest[:])
		}
		result[filepath.ToSlash(rel)] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("deep snapshot %s: %v", root, err)
	}
	return result
}

func assertDeepProbeSnapshotEqual(t *testing.T, before, after map[string]probeSnapshotEntry) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("snapshot length changed: before=%d after=%d", len(before), len(after))
	}
	beforeKeys := make([]string, 0, len(before))
	for key := range before {
		beforeKeys = append(beforeKeys, key)
	}
	sort.Strings(beforeKeys)
	for _, key := range beforeKeys {
		left, ok := before[key]
		right, exists := after[key]
		if !ok || !exists || left != right {
			t.Fatalf("snapshot changed at %q: before=%#v after=%#v", key, left, right)
		}
	}
}

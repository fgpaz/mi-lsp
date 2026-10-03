package workspace

import (
	"errors"
	"github.com/fgpaz/mi-lsp/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func autoRegisterHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(AutoRegisterEnvVar, "")
	return home
}

func autoRegisterGitRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir
}

func TestAutoRegisterFirstQueryInUnregisteredGitRepo(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "work", "demo"))
	sub := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := AutoRegisterWorkspace("", sub)
	if err != nil {
		t.Fatalf("auto register: %v", err)
	}
	if !result.Registered || result.Alias != "demo" || result.Root != repo {
		t.Fatalf("result = %+v, want demo at %s", result, repo)
	}
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Workspaces["demo"].Root != repo {
		t.Fatalf("registry = %+v", registry.Workspaces)
	}
	if registry.Defaults.LastWorkspace != "" {
		t.Fatalf("auto register must not change last_workspace, got %q", registry.Defaults.LastWorkspace)
	}
	if _, err := os.Stat(ProjectConfigPath(repo)); err != nil {
		t.Fatalf("project.toml not persisted: %v", err)
	}

	again, err := AutoRegisterWorkspace("", sub)
	if err != nil || again.Registered {
		t.Fatalf("second call = %+v, %v; want no-op", again, err)
	}
}

func TestAutoRegisterByRequestedPath(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "other"))
	elsewhere := t.TempDir()

	result, err := AutoRegisterWorkspace(repo, elsewhere)
	if err != nil || !result.Registered || result.Alias != "other" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	// A selector that is neither an alias nor a path is not registered implicitly.
	result, err = AutoRegisterWorkspace("no-such-alias", repo)
	if err != nil || result.Registered {
		t.Fatalf("unknown selector = %+v, %v", result, err)
	}
}

func TestAutoRegisterNeverRegistersHomeOrRoot(t *testing.T) {
	home := autoRegisterHome(t)
	autoRegisterGitRepo(t, home) // $HOME itself is a git repo (dotfiles)
	if result, err := AutoRegisterWorkspace("", home); err != nil || result.Registered {
		t.Fatalf("home = %+v, %v", result, err)
	}
	sub := filepath.Join(home, "notes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if result, err := AutoRegisterWorkspace("", sub); err != nil || result.Registered {
		t.Fatalf("dir inside home-rooted repo = %+v, %v", result, err)
	}
	if !autoRegisterForbiddenRoot(string(filepath.Separator)) {
		t.Fatal("filesystem root must be forbidden")
	}
	if result, err := AutoRegisterWorkspace("", string(filepath.Separator)); err != nil || result.Registered {
		t.Fatalf("root = %+v, %v", result, err)
	}
}

func TestAutoRegisterSkipsNonGitDirectory(t *testing.T) {
	home := autoRegisterHome(t)
	plain := filepath.Join(home, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", home)
	result, err := AutoRegisterWorkspace("", plain)
	if err != nil || result.Registered {
		t.Fatalf("non-git = %+v, %v", result, err)
	}
	registry, _ := LoadRegistryReadOnly()
	if len(registry.Workspaces) != 0 {
		t.Fatalf("registry must stay empty: %+v", registry.Workspaces)
	}
}

func TestAutoRegisterNameCollisionUsesDeterministicSuffix(t *testing.T) {
	home := autoRegisterHome(t)
	first := autoRegisterGitRepo(t, filepath.Join(home, "a", "app"))
	second := autoRegisterGitRepo(t, filepath.Join(home, "b", "app"))

	r1, err := AutoRegisterWorkspace("", first)
	if err != nil || r1.Alias != "app" {
		t.Fatalf("first = %+v, %v", r1, err)
	}
	r2, err := AutoRegisterWorkspace("", second)
	if err != nil || !r2.Registered || r2.Alias == "app" || !strings.HasPrefix(r2.Alias, "app-") {
		t.Fatalf("second = %+v, %v", r2, err)
	}
	registry, _ := LoadRegistry()
	if registry.Workspaces["app"].Root != first {
		t.Fatalf("existing alias was overwritten: %+v", registry.Workspaces["app"])
	}
	if want := autoRegisterAlias(second, registryWith(map[string]string{"app": first})); want != r2.Alias {
		t.Fatalf("suffix not deterministic: %s vs %s", want, r2.Alias)
	}
}

func TestAutoRegisterReusesExistingAliasForSameRoot(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "proj"))
	if _, err := RegisterWorkspace("custom-alias", registrationFor("custom-alias", repo)); err != nil {
		t.Fatal(err)
	}
	result, err := AutoRegisterWorkspace("", repo)
	if err != nil || result.Registered {
		t.Fatalf("result = %+v, %v; want reuse", result, err)
	}
	registry, _ := LoadRegistry()
	if len(registry.Workspaces) != 1 {
		t.Fatalf("a second alias was created: %+v", registry.Workspaces)
	}
}

func TestAutoRegisterRetriesAfterProjectCreationFailure(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "partial"))
	if err := os.WriteFile(WorkspaceStateDir(repo), []byte("blocks directory creation"), 0o600); err != nil {
		t.Fatal(err)
	}

	if result, err := AutoRegisterWorkspace("", repo); err == nil || result.Registered {
		t.Fatalf("first call = %+v, %v; want project file creation failure", result, err)
	}
	registry, err := LoadRegistry()
	if err != nil || len(registry.Workspaces) != 0 {
		t.Fatalf("failed project creation left a registry entry: %+v, %v", registry.Workspaces, err)
	}
	if err := os.Remove(WorkspaceStateDir(repo)); err != nil {
		t.Fatal(err)
	}

	result, err := AutoRegisterWorkspace("", repo)
	if err != nil || !result.Registered || result.Alias != "partial" {
		t.Fatalf("retry = %+v, %v; want recovered registration", result, err)
	}
	if _, err := os.Stat(ProjectConfigPath(repo)); err != nil {
		t.Fatalf("project.toml not recovered: %v", err)
	}
	registry, err = LoadRegistry()
	if err != nil || len(registry.Workspaces) != 1 || registry.Workspaces["partial"].Root != repo {
		t.Fatalf("recovered registry = %+v, %v", registry.Workspaces, err)
	}
}

func TestAutoRegisterKeepsExistingProjectToml(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "canon"))
	if err := os.MkdirAll(WorkspaceStateDir(repo), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# hand written canon\n[project]\nname = \"canon\"\nkind = \"single\"\nlanguages = [\"go\"]\n"
	if err := os.WriteFile(ProjectConfigPath(repo), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := AutoRegisterWorkspace("", repo)
	if err != nil || !result.Registered {
		t.Fatalf("result = %+v, %v", result, err)
	}
	got, _ := os.ReadFile(ProjectConfigPath(repo))
	if string(got) != original {
		t.Fatalf("project.toml was modified:\n%s", got)
	}
}

func TestAutoRegisterOptOutEnv(t *testing.T) {
	for _, value := range []string{"1", "true", "ON"} {
		t.Setenv(AutoRegisterEnvVar, value)
		if !AutoRegisterDisabledByEnv() {
			t.Fatalf("%q should disable auto register", value)
		}
	}
	t.Setenv(AutoRegisterEnvVar, "0")
	if AutoRegisterDisabledByEnv() {
		t.Fatal("0 must not disable auto register")
	}
}

func TestAutoRegisterConcurrentGoroutinesRegisterOnce(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "race"))
	var wg sync.WaitGroup
	results := make([]AutoRegisterResult, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = AutoRegisterWorkspace("", repo)
		}(i)
	}
	wg.Wait()
	registered := 0
	for _, r := range results {
		if r.Registered {
			registered++
		}
	}
	registry, _ := LoadRegistry()
	if registered != 1 || len(registry.Workspaces) != 1 {
		t.Fatalf("registered=%d registry=%+v", registered, registry.Workspaces)
	}
}

// TestAutoRegisterTwoProcesses races two real processes registering two
// different repos with the same basename; neither entry may be lost.
func TestAutoRegisterTwoProcesses(t *testing.T) {
	home := autoRegisterHome(t)
	repoA := autoRegisterGitRepo(t, filepath.Join(home, "a", "svc"))
	repoB := autoRegisterGitRepo(t, filepath.Join(home, "b", "svc"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outputs := make([]string, 2)
	for i, repo := range []string{repoA, repoB} {
		wg.Add(1)
		go func(i int, repo string) {
			defer wg.Done()
			cmd := exec.Command(exe, "-test.run=TestAutoRegisterHelperProcess")
			cmd.Env = append(os.Environ(), "MILSP_AUTOREG_HELPER="+repo, "HOME="+home, "USERPROFILE="+home)
			out, _ := cmd.CombinedOutput()
			outputs[i] = string(out)
		}(i, repo)
	}
	wg.Wait()
	for _, out := range outputs {
		if !strings.Contains(out, "AUTOREG_OK") {
			t.Fatalf("helper failed: %s", out)
		}
	}
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]bool{}
	for _, ws := range registry.Workspaces {
		roots[ws.Root] = true
	}
	if len(registry.Workspaces) != 2 || !roots[repoA] || !roots[repoB] {
		t.Fatalf("registry lost an entry: %+v", registry.Workspaces)
	}
}

func TestAutoRegisterHelperProcess(t *testing.T) {
	repo := os.Getenv("MILSP_AUTOREG_HELPER")
	if repo == "" {
		t.Skip("helper process only")
	}
	if _, err := AutoRegisterWorkspace("", repo); err != nil {
		t.Fatalf("helper: %v", err)
	}
	os.Stdout.WriteString("AUTOREG_OK\n")
}

func registrationFor(name string, root string) model.WorkspaceRegistration {
	return model.WorkspaceRegistration{Name: name, Root: root, Kind: model.WorkspaceKindSingle}
}

func registryWith(aliases map[string]string) model.RegistryFile {
	registry := model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}
	for alias, root := range aliases {
		registry.Workspaces[alias] = registrationFor(alias, root)
	}
	return registry
}

func TestAutoRegisterReportsMissingHead(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "empty"))
	result, err := AutoRegisterWorkspace("", repo)
	if err != nil || !result.Registered || result.HasCommits {
		t.Fatalf("result = %+v, %v; want registered without commits", result, err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if !gitHasHead(repo) {
		t.Fatal("gitHasHead must be true after the first commit")
	}
}

func TestAutoRegisterLockTimeoutWithLiveHolder(t *testing.T) {
	home := autoRegisterHome(t)
	repo := autoRegisterGitRepo(t, filepath.Join(home, "locked"))
	old := registryLockTimeout
	registryLockTimeout = 150 * time.Millisecond
	t.Cleanup(func() { registryLockTimeout = old })

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithRegistryLock(func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	_, err := AutoRegisterWorkspace("", repo)
	var timeoutErr *RegistryLockTimeoutError
	if !errors.As(err, &timeoutErr) || timeoutErr.Code() != "registry_lock_timeout" {
		t.Fatalf("err = %v, want registry_lock_timeout", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if result, err := AutoRegisterWorkspace("", repo); err != nil || !result.Registered {
		t.Fatalf("after release = %+v, %v", result, err)
	}
}

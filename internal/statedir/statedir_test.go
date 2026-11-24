package statedir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirDefaultsToRepoLocal(t *testing.T) {
	t.Setenv(Env, "")
	root := t.TempDir()
	if got, want := Dir(root), filepath.Join(root, ".mi-lsp"); got != want {
		t.Fatalf("default Dir = %q, want %q", got, want)
	}
	t.Setenv(Env, "repo")
	if got, want := Dir(root), filepath.Join(root, ".mi-lsp"); got != want {
		t.Fatalf("repo Dir = %q, want %q", got, want)
	}
}

func TestValidateRejectsUnknownValueWithoutRepoFallback(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(Env, "machnie")
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "valid values: repo, machine") {
		t.Fatalf("Validate must reject a mistyped value, got %v", err)
	}
	root := t.TempDir()
	if dir := Dir(root); strings.HasPrefix(dir, root) {
		t.Fatalf("a mistyped value must never resolve under the repo: %q", dir)
	}
}

// Two repos, and a linked worktree with its parent, never share state, and
// none of them resolves under its own root.
func TestMachineDirIsolatesReposAndWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv(Env, LocationMachine)
	if err := Validate(); err != nil {
		t.Fatal(err)
	}

	base := t.TempDir()
	repoA := filepath.Join(base, "a")
	repoB := filepath.Join(base, "b")
	worktree := filepath.Join(base, "a-wt")
	for _, repo := range []string{repoA, repoB} {
		gitRun(t, "", "init", "--quiet", repo)
		if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, repo, "add", ".")
		gitRun(t, repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--quiet", "-m", "init")
	}
	gitRun(t, repoA, "worktree", "add", "--quiet", worktree)

	seen := map[string]string{}
	for _, root := range []string{repoA, repoB, worktree} {
		dir := Dir(root)
		if dir == "" || !strings.HasPrefix(dir, cache) {
			t.Fatalf("machine Dir(%q) = %q, want a path under the cache %q", root, dir, cache)
		}
		if rel, err := filepath.Rel(root, dir); err == nil && !strings.HasPrefix(rel, "..") {
			t.Fatalf("machine Dir(%q) = %q resolves under the repo", root, dir)
		}
		if other, ok := seen[dir]; ok {
			t.Fatalf("%q and %q share state dir %q", other, root, dir)
		}
		seen[dir] = root
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

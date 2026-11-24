package cli

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/statedir"
)

// RF-WKS-007-B04: with MI_LSP_STATE_LOCATION=machine, init + nav intent + a
// symbol query on a clean repo leave `git status --ignored` empty and keep the
// index in the machine-local state dir.
func TestMachineStateLocationLeavesCleanRepoUntouched(t *testing.T) {
	repo, cache := newMachineStateRepo(t)

	runStateLocationRoot(t, "init", repo, "--name", "machine-state")
	runStateLocationRoot(t, "--workspace", "machine-state", "nav", "intent", "hello machine")
	found := runStateLocationRoot(t, "--workspace", "machine-state", "nav", "find", "HelloMachine")
	if !strings.Contains(found, "HelloMachine") {
		t.Fatalf("symbol query did not answer from the machine-local index: %s", found)
	}

	if status := stateGitOutput(t, repo, "status", "--porcelain", "--ignored", "--untracked-files=all"); status != "" {
		t.Fatalf("machine state wrote under the repo:\n%s", status)
	}
	if _, err := os.Stat(filepath.Join(repo, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("repo-local .mi-lsp must not exist in machine mode: %v", err)
	}
	stateDir := statedir.Dir(repo)
	if !strings.HasPrefix(stateDir, cache) {
		t.Fatalf("state dir %q is not under the user cache %q", stateDir, cache)
	}
	for _, name := range []string{"index.db", "project.toml"} {
		if _, err := os.Stat(filepath.Join(stateDir, name)); err != nil {
			t.Fatalf("machine state %s missing: %v", name, err)
		}
	}
}

// RF-WKS-007-B04: in machine mode a nav read answered from an index built at
// another HEAD of the same path reports index_stale_revision with both SHAs;
// at the indexed HEAD it reports nothing. The read never reindexes.
func TestMachineStateLocationReportsIndexFromAnotherRevision(t *testing.T) {
	repo, _ := newMachineStateRepo(t)
	runStateLocationRoot(t, "init", repo, "--name", "machine-revision")
	indexed := stateGitOutput(t, repo, "rev-parse", "HEAD")
	if out := runStateLocationRoot(t, "--workspace", "machine-revision", "nav", "find", "HelloMachine"); strings.Contains(out, "index_stale_revision") {
		t.Fatalf("an index at HEAD must not be reported stale:\n%s", out)
	}

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc HelloRevision() string { return \"hi\" }\n\nfunc main() { _ = HelloRevision() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateGit(t, repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--quiet", "-am", "rev2")
	head := stateGitOutput(t, repo, "rev-parse", "HEAD")

	out := runStateLocationRoot(t, "--workspace", "machine-revision", "nav", "find", "HelloRevision")
	if !strings.Contains(out, "index_stale_revision") || !strings.Contains(out, indexed) || !strings.Contains(out, head) {
		t.Fatalf("an index from %s read at HEAD %s must carry index_stale_revision with both SHAs:\n%s", indexed, head, out)
	}
	if status := stateGitOutput(t, repo, "status", "--porcelain", "--ignored", "--untracked-files=all"); status != "" {
		t.Fatalf("machine state wrote under the repo:\n%s", status)
	}
}

// newMachineStateRepo isolates HOME and the user cache, selects the machine
// state location without updates, autoindex or auto-register, and returns a
// committed Go repo whose root refuses new entries, so a stray .mi-lsp/ fails
// loudly. The test runs from inside the repo.
func newMachineStateRepo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	home := t.TempDir()
	cache := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv(statedir.Env, statedir.LocationMachine)
	t.Setenv("MI_LSP_AUTO_UPDATE", "0")
	t.Setenv("MI_LSP_AUTOINDEX", "0")
	t.Setenv("MI_LSP_NO_AUTO_REGISTER", "1")
	t.Setenv("MI_LSP_CLIENT_NAME", "state-location-test")

	repo := filepath.Join(t.TempDir(), "repo")
	stateGit(t, "", "init", "--quiet", repo)
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc HelloMachine() string { return \"hi\" }\n\nfunc main() { _ = HelloMachine() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateGit(t, repo, "add", ".")
	stateGit(t, repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--quiet", "-m", "init")
	// The working tree root refuses new entries, so a stray .mi-lsp/ fails loudly.
	if err := os.Chmod(repo, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(repo, 0o755) })
	t.Chdir(repo)
	return repo, cache
}

func TestInvalidStateLocationFailsBeforeAnyCommand(t *testing.T) {
	t.Setenv(statedir.Env, "machnie")
	root := NewRootCommand()
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), statedir.Env) {
		t.Fatalf("an invalid %s must fail explicitly, got %v", statedir.Env, err)
	}
}

func runStateLocationRoot(t *testing.T, args ...string) string {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	old := os.Stdout
	os.Stdout = out
	root := NewRootCommand()
	root.SetArgs(append([]string{"--format", "json"}, args...))
	runErr := root.Execute()
	os.Stdout = old
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(out)
	if runErr != nil {
		t.Fatalf("mi-lsp %v: %v\n%s", args, runErr, data)
	}
	var envelope struct {
		Ok bool `json:"ok"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !envelope.Ok {
		t.Fatalf("mi-lsp %v did not answer ok: %v\n%s", args, err, data)
	}
	return string(data)
}

func stateGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func stateGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

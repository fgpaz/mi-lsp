package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeOverlayFingerprintsIgnoredGoSourceCreationAndModification(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	parent := t.TempDir()
	base := filepath.Join(parent, "main")
	linked := filepath.Join(parent, "linked")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	runOverlayGit(t, base, "init", "--quiet")
	runOverlayGit(t, base, "config", "user.email", "overlay-test@example.invalid")
	runOverlayGit(t, base, "config", "user.name", "overlay-test")
	for name, content := range map[string]string{
		"go.mod":     "module example.invalid/overlay\n\ngo 1.23\n",
		"main.go":    "package overlay\nfunc Tracked() {}\n",
		".gitignore": "generated.go\n",
	} {
		if err := os.WriteFile(filepath.Join(base, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runOverlayGit(t, base, "add", "go.mod", "main.go", ".gitignore")
	runOverlayGit(t, base, "commit", "--quiet", "-m", "fixture")
	runOverlayGit(t, base, "worktree", "add", "--quiet", "--detach", linked, "HEAD")

	clean, err := ResolveWorktreeOverlay(linked)
	if err != nil {
		t.Fatal(err)
	}
	if len(clean.Changed) != 0 || len(clean.Deleted) != 0 {
		t.Fatalf("clean worktree reported delta: %+v", clean)
	}
	ignored := filepath.Join(linked, "generated.go")
	if err := os.WriteFile(ignored, []byte("package overlay\nfunc Generated() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := ResolveWorktreeOverlay(linked)
	if err != nil {
		t.Fatal(err)
	}
	if !containsOverlayPath(created.Changed, "generated.go") || created.Fingerprint == clean.Fingerprint {
		t.Fatalf("ignored source creation was not fingerprinted: clean=%s created=%+v", clean.Fingerprint, created)
	}

	if err := os.WriteFile(ignored, []byte("package overlay\nfunc Generated() {}\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	modified, err := ResolveWorktreeOverlay(linked)
	if err != nil {
		t.Fatal(err)
	}
	if !containsOverlayPath(modified.Changed, "generated.go") || modified.Fingerprint == created.Fingerprint {
		t.Fatalf("ignored source modification did not invalidate fingerprint: created=%s modified=%+v", created.Fingerprint, modified)
	}
}

func runOverlayGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func containsOverlayPath(paths []string, want string) bool {
	for _, path := range paths {
		if strings.EqualFold(filepath.ToSlash(path), want) {
			return true
		}
	}
	return false
}

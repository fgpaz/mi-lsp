package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCalculateGitFrontierTracksCommitsAndWorkingTree(t *testing.T) {
	root := t.TempDir()
	gitTestCommand(t, root, "init", "-q")
	gitTestCommand(t, root, "config", "user.email", "frontier@example.invalid")
	gitTestCommand(t, root, "config", "user.name", "frontier-test")
	tracked := filepath.Join(root, "tracked.ts")
	if err := os.WriteFile(tracked, []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, root, "add", "tracked.ts")
	gitTestCommand(t, root, "commit", "-qm", "base")
	base := gitTestOutput(t, root, "rev-parse", "HEAD")

	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(tracked, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("export const value = 2;\n")
	gitTestCommand(t, root, "add", "tracked.ts")
	gitTestCommand(t, root, "commit", "-qm", "tracked change")
	head := gitTestOutput(t, root, "rev-parse", "HEAD")
	committed, err := calculateGitFrontier(context.Background(), root, base, head)
	if err != nil {
		t.Fatalf("calculate committed frontier: %v", err)
	}
	repeat, err := calculateGitFrontier(context.Background(), root, base, head)
	if err != nil || repeat != committed {
		t.Fatalf("frontier not deterministic: %q, %v", repeat, err)
	}

	write("export const value = 3;\n")
	working, err := calculateGitFrontier(context.Background(), root, base, head)
	if err != nil {
		t.Fatalf("calculate working frontier: %v", err)
	}
	if working == committed {
		t.Fatal("tracked working-tree change did not affect frontier")
	}
	if err := os.WriteFile(filepath.Join(root, "new file.ts"), []byte("export const fresh = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked, err := calculateGitFrontier(context.Background(), root, base, head)
	if err != nil {
		t.Fatalf("calculate untracked frontier: %v", err)
	}
	if untracked == working {
		t.Fatal("untracked file did not affect frontier")
	}
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "external-link.ts")); err == nil {
		if _, err := calculateGitFrontier(context.Background(), root, base, head); err != nil {
			t.Fatalf("symlink frontier must hash its link target without following it: %v", err)
		}
	}
}

func gitTestCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func gitTestOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(bytesTrimSpace(output))
}

func bytesTrimSpace(data []byte) []byte {
	start, end := 0, len(data)
	for start < end && (data[start] == ' ' || data[start] == '\n' || data[start] == '\r' || data[start] == '\t') {
		start++
	}
	for end > start && (data[end-1] == ' ' || data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == '\t') {
		end--
	}
	return data[start:end]
}

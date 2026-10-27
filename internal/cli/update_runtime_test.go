package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// This uses the real root, rather than a fabricated update command. It remains
// a behavioral regression while the updater package does not yet exist.
func TestUpdateIsRegisteredOnRealRoot(t *testing.T) {
	root := NewRootCommand()
	cmd, remaining, err := root.Find([]string{"update"})
	if err != nil || cmd == root || cmd.Name() != "update" || len(remaining) != 0 {
		t.Fatalf("update must be a real root command: command=%v remaining=%v err=%v", cmd, remaining, err)
	}
}

func TestUpdateCheckWithoutStateDoesNotInventStatusOrWriteHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MI_LSP_AUTO_UPDATE", "0")
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"update", "check", "--format", "json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("check without readable state must return an error, not a fabricated status")
	}
	if strings.Contains(strings.ToLower(err.Error()), "unknown command") {
		t.Fatalf("update check must exist: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("check without state emitted a fabricated status: %q", stdout.String())
	}
	entries, readErr := os.ReadDir(home)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("read-only check wrote to home: %v", entries)
	}
}

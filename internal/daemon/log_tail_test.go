package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestFilterBenignDaemonLogNoiseDropsClosedConnectionHelpBlock(t *testing.T) {
	lines := []LogTailLine{
		{Line: 1, Text: "2026/04/30 daemon ready"},
		{Line: 2, Text: "Error: accept tcp 127.0.0.1:1234: use of closed network connection"},
		{Line: 3, Text: "Usage:"},
		{Line: 4, Text: "  mi-lsp daemon serve [flags]"},
		{Line: 5, Text: "Flags:"},
		{Line: 6, Text: "  -h, --help"},
		{Line: 7, Text: "2026/04/30 watcher refreshed"},
	}

	filtered := FilterBenignDaemonLogNoise(lines)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2: %#v", len(filtered), filtered)
	}
	if filtered[0].Line != 1 || filtered[1].Line != 7 {
		t.Fatalf("filtered lines = %#v, want original line 1 and 7", filtered)
	}
}

func TestResolveDaemonLogPathUsesPersistedDaemonRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	daemonRoot := t.TempDir()
	fallbackRoot := t.TempDir()
	logPath := filepath.Join(daemonRoot, ".mi-lsp", "daemon.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("daemon log\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := saveDaemonState(model.DaemonState{RepoRoot: daemonRoot}); err != nil {
		t.Fatalf("saveDaemonState: %v", err)
	}

	got := ResolveDaemonLogPath(fallbackRoot)
	if got != logPath {
		t.Fatalf("ResolveDaemonLogPath = %q, want %q", got, logPath)
	}
}

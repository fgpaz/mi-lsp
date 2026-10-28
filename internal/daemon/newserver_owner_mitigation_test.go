package daemon

import (
	"fmt"
	"strings"
	"os"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// A legacy daemon state whose owner is demonstrably alive but whose endpoint
// cannot be reached must fail closed before this process binds or opens SQLite.
func TestNewServerWithOptionsRejectsUnreachableLiveLegacyOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-owner-mitigation-test-%d", os.Getpid()))

	state := model.DaemonState{
		PID:      os.Getpid(),
		Endpoint: "aorph-unreachable-legacy-owner-test-endpoint",
		StartedAt: time.Now().Add(-time.Hour),
	}
	if err := saveDaemonState(state); err != nil {
		t.Fatal(err)
	}
	dbPath, err := daemonDatabasePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("fixture database must not exist before construction: %v", err)
	}

	server, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, DefaultStartOptions())
	if server != nil {
		server.Shutdown()
		t.Fatal("constructor returned a server while a live legacy owner was unreachable")
	}
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "owner") {
		t.Fatalf("expected an owner-diagnostic error for the unreachable live legacy daemon, got %v", err)
	}
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Fatalf("constructor touched or created SQLite before rejecting the owner: %v", statErr)
	}
	got, loadErr := loadDaemonState()
	if loadErr != nil {
		t.Fatalf("legacy state was not preserved: %v", loadErr)
	}
	if got.PID != state.PID || got.Endpoint != state.Endpoint {
		t.Fatalf("legacy state mutated: got pid=%d endpoint=%q", got.PID, got.Endpoint)
	}
}

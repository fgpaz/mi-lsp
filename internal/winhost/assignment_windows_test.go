//go:build windows

package winhost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestAssignmentHelper would write the marker if the suspended process resumed.
// Its private directory and environment are local test preconditions, not a
// sandbox or an authority guarantee for arbitrary payloads.
func TestAssignmentHelper(t *testing.T) {
	marker := os.Getenv("WINHOST_ASSIGNMENT_MARKER")
	if marker == "" {
		return
	}
	if err := os.WriteFile(marker, []byte("resumed"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAssignmentFailureDoesNotResume(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "resumed")
	control := filepath.Join(dir, "control")
	if err := os.WriteFile(control, nil, 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINHOST_ASSIGNMENT_MARKER", marker)
	original := assignOwnedProcess
	defer func() { assignOwnedProcess = original }()
	called := false
	assignOwnedProcess = func(job, process uintptr) error {
		called = true
		return syscall.ERROR_ACCESS_DENIED
	}
	transport := execute([]string{"--control-file", control, "--", exe, "-test.run=^TestAssignmentHelper$"})
	if !called {
		t.Fatal("assignment seam was not reached")
	}
	if transport != 0 {
		t.Fatalf("result transport failed: %d", transport)
	}
	data, err := os.ReadFile(control + ".result")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "\n")
	if len(parts) != 3 || parts[0] != "execsafe-host/v1 infra_error 125" || parts[2] != "" {
		t.Fatalf("invalid result framing: %q", data)
	}
	var got outcome
	if err := json.Unmarshal([]byte(parts[1]), &got); err != nil {
		t.Fatal(err)
	}
	if got.Protocol != "execsafe-host/v1" || got.Kind != "infra_error" || got.ExitCode != 125 || got.Error == nil || got.Error.Operation != "assign_job" || got.Error.Win32 != uint32(syscall.ERROR_ACCESS_DENIED) {
		t.Fatalf("unexpected assignment result: %+v", got)
	}
	if len(got.Cleanup) != 0 {
		t.Fatalf("suspended process cleanup failed: %+v", got.Cleanup)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker must be absent, stat error: %v", err)
	}
}

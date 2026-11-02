//go:build windows

package winhost

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func coreArgs(t *testing.T) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	control := filepath.Join(dir, "control")
	if err := os.WriteFile(control, nil, 0600); err != nil { t.Fatal(err) }
	exe, err := os.Executable()
	if err != nil { t.Fatal(err) }
	marker := filepath.Join(dir, "marker")
	t.Setenv("WINHOST_ASSIGNMENT_MARKER", marker)
	return []string{"--control-file", control, "--", exe, "-test.run=^TestAssignmentHelper$"}, marker
}

func coreStreams() [3]*os.File { return [3]*os.File{os.Stdin, os.Stdout, os.Stderr} }

func TestCoreBeforeCreateStopsChild(t *testing.T) {
	args, marker := coreArgs(t)
	calls := 0
	got := runWithStdio(args, time.Minute, coreStreams(), ownedRunHooks{BeforeCreate: func() error { calls++; return syscall.ERROR_ACCESS_DENIED }})
	if calls != 1 || got.Kind != "infra_error" || got.ExitCode != 125 || got.Error == nil || got.Error.Operation != "before_create" { t.Fatalf("calls=%d outcome=%+v", calls, got) }
	if _, err := os.Stat(marker); !os.IsNotExist(err) { t.Fatalf("child ran: %v", err) }
}

func TestCoreBeforeResumeOwnedAndCleanup(t *testing.T) {
	args, marker := coreArgs(t)
	order := ""
	got := runWithStdio(args, time.Minute, coreStreams(), ownedRunHooks{
		BeforeCreate: func() error { order += "c"; return nil },
		BeforeResume: func(root syscall.Handle, job uintptr) error {
			order += "r"
			var info jobAccounting
			ok, _, err := queryJob.Call(job, 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
			if root == 0 || ok == 0 || info.activeProcesses != 1 { t.Errorf("invalid ownership root=%v active=%d err=%v", root, info.activeProcesses, err) }
			state, err := syscall.WaitForSingleObject(root, 0)
			if err != nil || state != waitTimeout { t.Errorf("root already terminated or wait failed: state=%d err=%v", state, err) }
			// This bounded observation does not prove thread suspension.
			if _, err := os.Stat(marker); !os.IsNotExist(err) { t.Errorf("marker present before callback returned: %v", err) }
			return syscall.ERROR_ACCESS_DENIED
		},
	})
	if order != "cr" || got.Kind != "infra_error" || got.ExitCode != 125 || got.Error == nil || got.Error.Operation != "before_resume" || len(got.Cleanup) != 0 { t.Fatalf("order=%q outcome=%+v", order, got) }
	if _, err := os.Stat(marker); !os.IsNotExist(err) { t.Fatalf("child ran: %v", err) }
}

func TestCoreInvalidStdioAndInvocationSkipHooks(t *testing.T) {
	args, _ := coreArgs(t)
	calls := 0
	hooks := ownedRunHooks{BeforeCreate: func() error { calls++; return nil }, BeforeResume: func(syscall.Handle, uintptr) error { calls++; return nil }}
	streams := coreStreams()
	streams[1] = nil
	got := runWithStdio(args, time.Minute, streams, hooks)
	if got.Error == nil || got.Error.Operation != "missing_stdio" || calls != 0 { t.Fatalf("outcome=%+v calls=%d", got, calls) }
	got = runWithStdio(nil, time.Minute, coreStreams(), hooks)
	if got.Error == nil || got.Error.Operation != "invalid_invocation" || calls != 0 { t.Fatalf("outcome=%+v calls=%d", got, calls) }
}

func TestCoreAssignmentFailureSkipsBeforeResume(t *testing.T) {
	args, marker := coreArgs(t)
	original := assignOwnedProcess
	defer func() { assignOwnedProcess = original }()
	assignOwnedProcess = func(job, process uintptr) error { return syscall.ERROR_ACCESS_DENIED }
	called := false
	got := runWithStdio(args, time.Minute, coreStreams(), ownedRunHooks{BeforeResume: func(syscall.Handle, uintptr) error { called = true; return nil }})
	if called || got.Error == nil || got.Error.Operation != "assign_job" || len(got.Cleanup) != 0 { t.Fatalf("called=%v outcome=%+v", called, got) }
	if _, err := os.Stat(marker); !os.IsNotExist(err) { t.Fatalf("child ran: %v", err) }
}

// This helper only performs I/O when explicitly selected by the parent test.
func TestCoreStdioHelper(t *testing.T) {
	if os.Getenv("WINHOST_CORE_STDIO_HELPER") != "1" { return }
	data, err := io.ReadAll(os.Stdin)
	if err != nil || string(data) != "core-private-input-731" { t.Fatalf("input=%q err=%v", data, err) }
	if _, err := io.WriteString(os.Stdout, "core-private-stdout-731"); err != nil { t.Fatal(err) }
	if _, err := io.WriteString(os.Stderr, "core-private-stderr-731"); err != nil { t.Fatal(err) }
}

func TestCoreClosedStdioSkipsHooks(t *testing.T) {
	args, marker := coreArgs(t)
	f, err := os.Create(filepath.Join(t.TempDir(), "closed"))
	if err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	calls := 0
	streams := coreStreams()
	streams[1] = f
	got := runWithStdio(args, time.Minute, streams, ownedRunHooks{BeforeCreate: func() error { calls++; return nil }, BeforeResume: func(syscall.Handle, uintptr) error { calls++; return nil }})
	if got.Kind != "infra_error" || got.ExitCode != 125 || got.Error == nil || got.Error.Operation != "duplicate_stdio" || calls != 0 { t.Fatalf("outcome=%+v calls=%d", got, calls) }
	if _, err := os.Stat(marker); !os.IsNotExist(err) { t.Fatalf("child marker present: %v", err) }
}

func TestCoreExplicitStdioAndNilHooks(t *testing.T) {
	args, _ := coreArgs(t)
	args[4] = "-test.run=^TestCoreStdioHelper$"
	t.Setenv("WINHOST_CORE_STDIO_HELPER", "1")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input")
	if err := os.WriteFile(inputPath, []byte("core-private-input-731"), 0600); err != nil { t.Fatal(err) }
	in, err := os.Open(inputPath)
	if err != nil { t.Fatal(err) }
	defer in.Close()
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil { t.Fatal(err) }
	defer out.Close()
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil { t.Fatal(err) }
	defer stderr.Close()
	got := runWithStdio(args, time.Minute, [3]*os.File{in, out, stderr}, ownedRunHooks{})
	if got.Kind != "root_exit" || got.ExitCode != 0 || len(got.Cleanup) != 0 { t.Fatalf("outcome=%+v", got) }
	stdoutBytes, err := os.ReadFile(out.Name())
	if err != nil { t.Fatal(err) }
	stderrBytes, err := os.ReadFile(stderr.Name())
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(stdoutBytes), "core-private-stdout-731") || strings.Contains(string(stdoutBytes), "core-private-stderr-731") || !strings.Contains(string(stderrBytes), "core-private-stderr-731") || strings.Contains(string(stderrBytes), "core-private-stdout-731") { t.Fatalf("routing stdout=%q stderr=%q", stdoutBytes, stderrBytes) }
	if _, err := in.Seek(0, 0); err != nil { t.Fatalf("input unusable: %v", err) }
	data, err := io.ReadAll(in)
	if err != nil || string(data) != "core-private-input-731" { t.Fatalf("input after run=%q err=%v", data, err) }
	if _, err := out.WriteString("parent-stdout"); err != nil { t.Fatalf("stdout unusable: %v", err) }
	if _, err := stderr.WriteString("parent-stderr"); err != nil { t.Fatalf("stderr unusable: %v", err) }
}

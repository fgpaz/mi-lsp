package runtimeinfo

import (
	"context"
	"fmt"
	"os/exec"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLeaseCancelBusyAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, err := AcquireFileLeaseContext(ctx, path)
	if err != nil { t.Fatal(err) }
	// Cancellation of the acquisition context after success cannot release it.
	cancel()
	busyCtx, busyCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer busyCancel()
	if got, err := AcquireFileLeaseContext(busyCtx, path); err == nil || got != nil { t.Fatal("busy acquisition succeeded") }
	if err := first.Release(); err != nil { t.Fatal(err) }
	if err := first.Release(); err != nil { t.Fatal(err) }
	second, err := AcquireFileLeaseContext(context.Background(), path)
	if err != nil { t.Fatal(err) }
	if err := second.Release(); err != nil { t.Fatal(err) }
	info, err := os.Lstat(path)
	if err != nil { t.Fatal(err) }
	if !info.Mode().IsRegular() { t.Fatal("guard is not regular") }
}

func TestFileLeaseChildExitsWithoutRelease(t *testing.T) {
	path := os.Getenv("MI_FILE_LEASE_CHILD_PATH")
	if path != "" {
		lease, err := AcquireFileLeaseContext(context.Background(), path)
		if err != nil { fmt.Fprintln(os.Stdout, "acquire-error"); os.Exit(3) }
		// Intentionally leave Release uncalled; process exit must release the
		// kernel handle. The parent joins this child before reacquiring.
		_ = lease
		fmt.Fprintln(os.Stdout, "lease-held")
		os.Exit(0)
	}
	path = filepath.Join(t.TempDir(), "child-guard")
	cmd := exec.Command(os.Args[0], "-test.run=^TestFileLeaseChildExitsWithoutRelease$")
	cmd.Env = append(os.Environ(), "MI_FILE_LEASE_CHILD_PATH="+path)
	output, err := cmd.CombinedOutput() // waits/reaps the actual child process
	if err != nil { t.Fatalf("controlled child failed: %v (%s)", err, output) }
	if string(output) == "" { t.Fatal("child produced no ownership handshake") }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := AcquireFileLeaseContext(ctx, path)
	if err != nil { t.Fatalf("child exit did not release OS handle: %v", err) }
	if err := lease.Release(); err != nil { t.Fatal(err) }
}

func TestFileLeaseRejectsRelativeAndSymlink(t *testing.T) {
	if _, err := AcquireFileLeaseContext(context.Background(), "relative-guard"); err == nil { t.Fatal("relative path accepted") }
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil { t.Fatal(err) }
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil { t.Skipf("symlink unavailable: %v", err) }
	if _, err := AcquireFileLeaseContext(context.Background(), link); err == nil { t.Fatal("symlink accepted") }
}

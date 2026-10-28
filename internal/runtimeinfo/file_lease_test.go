package runtimeinfo

import (
	"context"
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

func TestFileLeaseRejectsRelativeAndSymlink(t *testing.T) {
	if _, err := AcquireFileLeaseContext(context.Background(), "relative-guard"); err == nil { t.Fatal("relative path accepted") }
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil { t.Fatal(err) }
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil { t.Skipf("symlink unavailable: %v", err) }
	if _, err := AcquireFileLeaseContext(context.Background(), link); err == nil { t.Fatal("symlink accepted") }
}

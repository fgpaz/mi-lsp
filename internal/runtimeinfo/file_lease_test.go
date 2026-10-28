package runtimeinfo

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

func TestFileLeaseCancelBusyAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, err := AcquireFileLeaseContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Cancellation of the acquisition context after success cannot release it.
	cancel()
	busyCtx, busyCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer busyCancel()
	got, busyErr := AcquireFileLeaseContext(busyCtx, path)
	if got != nil {
		_ = got.Release()
	}
	if !errors.Is(busyErr, context.DeadlineExceeded) {
		t.Fatalf("busy acquisition error = %v, want context deadline", busyErr)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireFileLeaseContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("guard is not regular")
	}
}

func TestFileLeaseChildExitsWithoutRelease(t *testing.T) {
	const key = "MI_FILE_LEASE_CHILD_PATH="
	for _, env := range os.Environ() {
		if len(env) >= len(key) && env[:len(key)] == key {
			path := env[len(key):]
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			lease, err := AcquireFileLeaseContext(ctx, path)
			cancel()
			if err != nil {
				fmt.Fprintln(os.Stdout, "acquire-error")
				os.Exit(3)
			}
			// Exact readiness line means acquisition has completed; stay alive
			// holding the lease until the parent closes stdin.
			fmt.Fprintln(os.Stdout, "FILELEASE-READY")
			_, _ = bufio.NewReader(os.Stdin).ReadString(0)
			runtime.KeepAlive(lease)
			os.Exit(0) // intentionally no Release: process exit releases the handle
		}
	}
	path := filepath.Join(t.TempDir(), "child-guard")
	cmd := processutil.Command(os.Args[0], "-test.run=^TestFileLeaseChildExitsWithoutRelease$")
	cmd.Env = make([]string, 0, len(os.Environ())+1)
	for _, env := range os.Environ() {
		if len(env) < len(key) || env[:len(key)] != key {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, key+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	joined := false
	join := func() error {
		if joined {
			return nil
		}
		joined = true
		return cmd.Wait()
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if !joined {
			_ = join()
		}
	})
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "FILELEASE-READY\n" {
			_ = stdin.Close()
			_ = join()
			t.Fatalf("unexpected child handshake %q", line)
		}
	case <-time.After(6 * time.Second):
		_ = stdin.Close()
		_ = join()
		t.Fatal("child readiness deadline")
	}
	// Contender must time out while the live child still owns the lease.
	contenderCtx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	contender, err := AcquireFileLeaseContext(contenderCtx, path)
	cancel()
	if contender != nil {
		_ = contender.Release()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		_ = stdin.Close()
		_ = join()
		t.Fatalf("live-child contender error = %v, want context deadline", err)
	}
	if err := stdin.Close(); err != nil {
		_ = join()
		t.Fatal(err)
	}
	if err := join(); err != nil {
		t.Fatalf("controlled child exit: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := AcquireFileLeaseContext(ctx, path)
	if err != nil {
		t.Fatalf("child exit did not release OS handle: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestFileLeaseRejectsRelativeAndSymlink(t *testing.T) {
	if _, err := AcquireFileLeaseContext(context.Background(), "relative-guard"); err == nil {
		t.Fatal("relative path accepted")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := AcquireFileLeaseContext(context.Background(), link); err == nil {
		t.Fatal("symlink accepted")
	}
}

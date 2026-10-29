package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutostartBudgetClampsCallerDeadline(t *testing.T) {
	before := time.Now()
	ctx, cancel := daemonAutoStartContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	after := time.Now()
	if daemonAutoStartBudget != 3*time.Second || !ok || deadline.Before(before.Add(3*time.Second)) || deadline.After(after.Add(3*time.Second)) {
		t.Fatalf("missing startup cap: %v, %v", deadline, ok)
	}
	parent, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	child, closeChild := daemonAutoStartContext(parent)
	defer closeChild()
	parentDeadline, _ := parent.Deadline()
	childDeadline, _ := child.Deadline()
	if !childDeadline.Equal(parentDeadline) {
		t.Fatalf("caller deadline reset: %v != %v", childDeadline, parentDeadline)
	}
}

func TestAutostartAlreadyCanceledDoesNotBeginAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := EnsureDaemonForContext(ctx, "unused", "test", "nav.find"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ensure canceled: %v", err)
	}
	_, started, err := SpawnBackgroundWithOptionsContext(ctx, "unused", 1, time.Second, DefaultStartOptions())
	if started || !errors.Is(err, context.Canceled) {
		t.Fatalf("spawn began after cancellation: started=%v err=%v", started, err)
	}
	path := filepath.Join(t.TempDir(), "start.lock")
	if _, err := acquireStartReservationAtContext(ctx, path, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("reservation canceled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "start.guard")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled acquisition touched guard: %v", err)
	}
}

func TestStartReservationCanceledGuardRetainsOwnerAndRetriesSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	owner, err := acquireStartReservationAtContext(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := os.OpenFile(owner.guardPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := tryLockStartGuard(guard)
	if err != nil || !locked {
		_ = guard.Close()
		t.Fatalf("hold private guard: locked=%v err=%v", locked, err)
	}
	held := true
	defer func() {
		if held {
			_ = unlockStartGuard(guard)
		}
		_ = guard.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := closeStartReservationContext(ctx, owner); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cleanup ignored caller deadline: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("unacquired guard released or rewrote reservation: %v", err)
	}
	// This attempt did not consume startLock.Close's once-only cleanup. After
	// releasing the competing guard, the same owner can synchronously retry.
	if err := unlockStartGuard(guard); err != nil {
		t.Fatal(err)
	}
	held = false
	if err := closeStartReservationContext(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner cleanup did not finish: %v", err)
	}
}

func TestStartReservationContextNeverUnlinksDifferentNonce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	owner, err := acquireStartReservationAtContext(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	otherNonce := strings.Repeat("0", startLockNonceBytes*2)
	if otherNonce == owner.nonce {
		otherNonce = strings.Repeat("1", startLockNonceBytes*2)
	}
	other := &startLock{path: owner.path, guardPath: owner.guardPath, pid: owner.pid, nonce: otherNonce}
	if err := closeStartReservationContext(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	metadata, known := parseStartLockMetadata(body)
	if err != nil || !known || metadata.nonce != owner.nonce {
		t.Fatalf("different nonce removed owner: %v", err)
	}
}

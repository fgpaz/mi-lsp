package daemon

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

type injectedMetadataError struct{}

func (injectedMetadataError) Error() string { return "injected StopRun error" }

// Controlled driver ownership, not a native SQLite or Windows lifetime proof.
func TestServerShutdownCancelsHeldTelemetryBeforeMetadataAndJoins(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS != "windows" {
		var err error
		home, err = os.MkdirTemp("/tmp", "aorph-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(home) })
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	gate := &drainConnector{path: filepath.Join(home, "held.db"), entered: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
	db := sql.OpenDB(gate)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{db: db, writeQueue: make(chan telemetryWriteRequest, 8), done: make(chan struct{}), walMaxBytes: telemetryWALMaxBytes}
	if err := store.configureConnection(); err != nil {
		t.Fatal(err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	runID, err := store.StartRun(model.DaemonState{PID: os.Getpid(), StartedAt: time.Now()})
	if err != nil || runID == 0 {
		t.Fatalf("start nonzero run: id=%d err=%v", runID, err)
	}
	leasePath := filepath.Join(home, "owner.lock")
	lease, err := acquireStartLockAt(leasePath, time.Second, time.Now, processExists)
	if err != nil {
		t.Fatal(err)
	}
	store.wg.Add(1)
	go store.asyncWriteWorker()
	lifeLeasePath := filepath.Join(home, "instance.life.guard")
	lifeLease, err := runtimeinfo.AcquireFileLeaseContext(context.Background(), lifeLeasePath)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{telemetry: store, state: model.DaemonState{RunID: runID}, instanceLease: lease, osInstanceLease: lifeLease, manager: NewManager(home, 1, time.Minute)}
	fenceReleased := make(chan struct{})
	var releaseOnce sync.Once
	var shutdownStarted bool
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(gate.release) })
		if !shutdownStarted {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_ = server.shutdownWithFence(ctx, func() { close(fenceReleased) }, true)
		}
		<-server.shutdownDone
		_ = store.WaitClosed(context.Background())
	})
	if err := store.RecordAccess(runID, model.AccessEvent{SessionID: "held", Operation: "held", OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not enter real SQLite connector query")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	shutdownStarted = true
	err = server.shutdownWithFence(ctx, func() { close(fenceReleased) }, true)
	if !errors.Is(err, ErrShutdownIncomplete) {
		t.Fatalf("bounded shutdown: %v", err)
	}
	select {
	case <-gate.canceled:
	case <-time.After(telemetryCloseDrainTimeout + time.Second):
		t.Fatal("shutdown did not cancel held driver context before metadata join")
	}
	select {
	case <-server.shutdownDone:
		t.Fatal("shutdown completed before driver release")
	default:
	}
	select {
	case <-fenceReleased:
		t.Fatal("fence released before driver release")
	default:
	}
	if _, err := os.Stat(leasePath); err != nil {
		t.Fatalf("cooperative lease not held: %v", err)
	}
	contenderCtx, contenderCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer contenderCancel()
	if contender, err := runtimeinfo.AcquireFileLeaseContext(contenderCtx, lifeLeasePath); err == nil {
		_ = contender.Release()
		t.Fatal("OS lifetime guard admitted a second owner before the database joined")
	}
	releaseOnce.Do(func() { close(gate.release) })
	select {
	case <-server.shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not join after driver release")
	}
	if got := store.WALHealth().FailedWrites; got != 1 {
		t.Errorf("canceled held write not counted exactly once: %d", got)
	}
	if err := store.WaitClosed(context.Background()); err != nil {
		t.Errorf("physical close: %v", err)
	}
	select {
	case <-fenceReleased:
	default:
		t.Error("fence not released after join")
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cooperative lease not cleaned: %v", err)
	}
	availableCtx, availableCancel := context.WithTimeout(context.Background(), time.Second)
	defer availableCancel()
	contender, err := runtimeinfo.AcquireFileLeaseContext(availableCtx, lifeLeasePath)
	if err != nil {
		t.Errorf("OS lifetime guard not released after database join: %v", err)
	} else {
		_ = contender.Release()
	}
}

func TestServerShutdownReportsStopRunExecErrorAndCloses(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS != "windows" {
		var err error
		home, err = os.MkdirTemp("/tmp", "aorph-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(home) })
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	injected := &injectedMetadataError{}
	gate := &drainConnector{path: filepath.Join(home, "metadata.db"), stopRunErr: injected}
	db := sql.OpenDB(gate)
	db.SetMaxOpenConns(1)
	store := &TelemetryStore{db: db, writeQueue: make(chan telemetryWriteRequest, 8), done: make(chan struct{}), walMaxBytes: telemetryWALMaxBytes}
	if err := store.configureConnection(); err != nil {
		t.Fatal(err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	runID, err := store.StartRun(model.DaemonState{PID: os.Getpid(), StartedAt: time.Now()})
	if err != nil || runID == 0 {
		t.Fatalf("start run: %d %v", runID, err)
	}
	leasePath := filepath.Join(home, "owner.lock")
	lease, err := acquireStartLockAt(leasePath, time.Second, time.Now, processExists)
	if err != nil {
		t.Fatal(err)
	}
	store.wg.Add(1)
	go store.asyncWriteWorker()
	server := &Server{telemetry: store, state: model.DaemonState{RunID: runID}, instanceLease: lease}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.ShutdownContext(ctx); !errors.Is(err, injected) {
		t.Errorf("shutdown did not report StopRun error: %v", err)
	}
	if err := store.WaitClosed(context.Background()); err != nil {
		t.Errorf("physical close: %v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lease not cleaned: %v", err)
	}
}

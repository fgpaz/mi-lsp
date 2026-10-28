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
)

// Controlled driver ownership, not a native SQLite or Windows lifetime proof.
func TestServerShutdownCancelsHeldTelemetryBeforeMetadataAndJoins(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS != "windows" {
		var err error
		home, err = os.MkdirTemp("/tmp", "aorph-")
		if err != nil { t.Fatal(err) }
		t.Cleanup(func() { _ = os.RemoveAll(home) })
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	gate := &drainConnector{path: filepath.Join(home, "held.db"), entered: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
	db := sql.OpenDB(gate)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{db: db, writeQueue: make(chan telemetryWriteRequest, 8), done: make(chan struct{}), walMaxBytes: telemetryWALMaxBytes}
	if err := store.configureConnection(); err != nil { t.Fatal(err) }
	if err := store.initSchema(); err != nil { t.Fatal(err) }
	runID, err := store.StartRun(model.DaemonState{PID: os.Getpid(), StartedAt: time.Now()})
	if err != nil || runID == 0 { t.Fatalf("start nonzero run: id=%d err=%v", runID, err) }
	leasePath := filepath.Join(home, "owner.lock")
	lease, err := acquireStartLockAt(leasePath, time.Second, time.Now, processExists)
	if err != nil { t.Fatal(err) }
	store.wg.Add(1)
	go store.asyncWriteWorker()
	server := &Server{telemetry: store, state: model.DaemonState{RunID: runID}, instanceLease: lease, manager: NewManager(home, 1, time.Minute)}
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
	if err := store.RecordAccess(runID, model.AccessEvent{SessionID: "held", Operation: "held", OccurredAt: time.Now()}); err != nil { t.Fatal(err) }
	select {
	case <-gate.entered:
	case <-time.After(time.Second): t.Fatal("writer did not enter real SQLite connector query")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	shutdownStarted = true
	err = server.shutdownWithFence(ctx, func() { close(fenceReleased) }, true)
	if !errors.Is(err, ErrShutdownIncomplete) { t.Fatalf("bounded shutdown: %v", err) }
	select {
	case <-gate.canceled:
	case <-time.After(telemetryCloseDrainTimeout + time.Second): t.Fatal("shutdown did not cancel held driver context before metadata join")
	}
	select { case <-server.shutdownDone: t.Fatal("shutdown completed before driver release"); default: }
	select { case <-fenceReleased: t.Fatal("fence released before driver release"); default: }
	if _, err := os.Stat(leasePath); err != nil { t.Fatalf("lease not held: %v", err) }
	releaseOnce.Do(func() { close(gate.release) })
	select {
	case <-server.shutdownDone:
	case <-time.After(5 * time.Second): t.Fatal("shutdown did not join after driver release")
	}
	if server.shutdownErr == nil { t.Error("canceled metadata was reported as successful cleanup") }
	select { case <-fenceReleased: default: t.Error("fence not released after join") }
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) { t.Errorf("lease not cleaned: %v", err) }
}

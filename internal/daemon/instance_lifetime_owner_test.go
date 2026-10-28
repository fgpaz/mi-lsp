package daemon

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func TestShutdownReleasesFenceAndBothLeasesOnlyAfterDatabaseJoin(t *testing.T) {
	home := privateLifetimeTestHome(t)
	gate := &drainConnector{
		path: filepath.Join(home, "shutdown.db"), entered: make(chan struct{}),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	db := sql.OpenDB(gate)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{
		db: db, writeQueue: make(chan telemetryWriteRequest, 8),
		done: make(chan struct{}), walMaxBytes: telemetryWALMaxBytes,
	}
	if err := store.configureConnection(); err != nil {
		t.Fatal(err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	store.wg.Add(1)
	go store.asyncWriteWorker()
	lifePath, err := daemonInstanceLifeGuardPath()
	if err != nil {
		t.Fatal(err)
	}
	lifeLease, err := runtimeinfo.AcquireFileLeaseContext(context.Background(), lifePath)
	if err != nil {
		t.Fatal(err)
	}
	coopPath, err := daemonInstanceLockPath()
	if err != nil {
		_ = lifeLease.Release()
		t.Fatal(err)
	}
	coopLease, err := acquireStartLockAt(coopPath, time.Second, time.Now, processExists)
	if err != nil {
		_ = lifeLease.Release()
		t.Fatal(err)
	}
	server := &Server{
		telemetry: store, instanceLease: coopLease, osInstanceLease: lifeLease,
		shutdownDone: make(chan struct{}), shutdownRetry: make(chan struct{}, 1),
	}
	var fenceOnce sync.Once
	fenceReleased := make(chan struct{})
	fence := func() { fenceOnce.Do(func() { close(fenceReleased) }) }
	shutdownStarted := false
	defer func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
		if !shutdownStarted {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.shutdownWithFence(ctx, fence, true); err != nil && !errors.Is(err, ErrShutdownIncomplete) {
				t.Errorf("cleanup shutdown: %v", err)
			}
		}
		select {
		case <-server.shutdownDone:
		case <-time.After(5 * time.Second):
			t.Error("shutdown owner did not join during cleanup")
		}
	}()
	if err := store.RecordAccess(0, model.AccessEvent{SessionID: "shutdown", Operation: "held", OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("DB operation did not enter controlled driver")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	shutdownStarted = true
	err = server.shutdownWithFence(ctx, fence, true)
	cancel()
	if !errors.Is(err, ErrShutdownIncomplete) {
		t.Fatalf("bounded caller result: %v", err)
	}
	select {
	case <-gate.canceled:
	case <-time.After(telemetryCloseDrainTimeout + time.Second):
		t.Fatal("DB operation did not observe cancellation")
	}
	// Hold beyond the 2s Close caller wait. The owner must wait for the later
	// physical result, not retain the transient timeout as shutdownErr.
	closeWaitElapsed := time.NewTimer(telemetryCloseDrainTimeout + 250*time.Millisecond)
	select {
	case <-server.shutdownDone:
		closeWaitElapsed.Stop()
		t.Fatal("shutdown completed before the DB driver barrier was released")
	case <-closeWaitElapsed.C:
	}
	select {
	case <-fenceReleased:
		t.Fatal("shutdown fence released before DB join")
	default:
	}
	lifeCtx, lifeCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	if contender, acquireErr := runtimeinfo.AcquireFileLeaseContext(lifeCtx, lifePath); acquireErr == nil {
		_ = contender.Release()
		lifeCancel()
		t.Fatal("OS lease released before DB join")
	}
	lifeCancel()
	if contender, acquireErr := acquireStartLockAt(coopPath, 100*time.Millisecond, time.Now, processExists); acquireErr == nil {
		_ = contender.Close()
		t.Fatal("cooperative lease released before DB join")
	}
	close(gate.release)
	select {
	case <-server.shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not join after DB release")
	}
	server.shutdownMu.Lock()
	shutdownErr := server.shutdownErr
	server.shutdownMu.Unlock()
	if shutdownErr != nil {
		t.Fatalf("transient bounded close wait polluted final shutdown result: %v", shutdownErr)
	}
	select {
	case <-fenceReleased:
	default:
		t.Fatal("shutdown fence not released after DB join")
	}
	lifeCtx, lifeCancel = context.WithTimeout(context.Background(), time.Second)
	contender, err := runtimeinfo.AcquireFileLeaseContext(lifeCtx, lifePath)
	lifeCancel()
	if err != nil {
		t.Fatalf("OS lease unavailable after join: %v", err)
	}
	_ = contender.Release()
	if _, err := os.Stat(coopPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cooperative lease remains after join: %v", err)
	}
}

func privateLifetimeTestHome(t *testing.T) string {
	t.Helper()
	var home string
	if runtime.GOOS == "windows" {
		home = t.TempDir()
	} else {
		t.Setenv("TMPDIR", "/tmp")
		var err error
		home, err = os.MkdirTemp("", "a-")
		if err != nil { t.Fatal(err) }
		t.Cleanup(func() { _ = os.RemoveAll(home) })
		t.Setenv("TMPDIR", home)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", "aorph-lifetime")
	return home
}

func TestConstructorFailuresRetainBothLeasesUntilDatabaseJoins(t *testing.T) {
	for _, failurePoint := range []string{"listener", "admin-token"} {
		t.Run(failurePoint, func(t *testing.T) {
			runConstructorFailureLifetimeOracle(t, failurePoint)
		})
	}
}

func runConstructorFailureLifetimeOracle(t *testing.T, failurePoint string) {
	t.Helper()
	repoRoot := t.TempDir() // Create before the constructor goroutine starts.
	home := privateLifetimeTestHome(t)

	gate := &drainConnector{
		path: filepath.Join(home, "ctor.db"), entered: make(chan struct{}),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	store := &TelemetryStore{}
	deps := serverConstructionDeps{
		openTelemetryStore: func() (*TelemetryStore, error) {
			store.db = sql.OpenDB(gate)
			store.db.SetMaxOpenConns(1)
			store.db.SetMaxIdleConns(1)
			store.writeQueue = make(chan telemetryWriteRequest, 8)
			store.done = make(chan struct{})
			store.walMaxBytes = telemetryWALMaxBytes
			if err := store.configureConnection(); err != nil {
				return nil, err
			}
			if err := store.initSchema(); err != nil {
				return nil, err
			}
			store.wg.Add(1)
			go store.asyncWriteWorker()
			return store, nil
		},
		listen: listenDaemon,
		generateAdminToken: func() (string, error) {
			if failurePoint != "admin-token" {
				return generateAdminToken()
			}
			if err := queueHeldConstructorWrite(store, gate); err != nil {
				return "", err
			}
			return "", errors.New("injected admin-token constructor failure")
		},
	}
	if failurePoint == "listener" {
		deps.listen = func() (net.Listener, error) {
			if err := queueHeldConstructorWrite(store, gate); err != nil {
				return nil, err
			}
			return nil, errors.New("injected listener construction failure")
		}
	}

	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	result := make(chan error, 1)
	go func() {
		_, err := newServerWithConstructionDeps(repoRoot, 1, time.Minute, options, deps)
		result <- err
	}()
	var releaseOnce sync.Once
	releaseDriver := func() { releaseOnce.Do(func() { close(gate.release) }) }
	joined := false
	defer func() {
		releaseDriver()
		if !joined {
			select {
			case <-result:
			case <-time.After(5 * time.Second):
				t.Error("constructor goroutine did not join during failure cleanup")
			}
		}
	}()

	select {
	case <-gate.canceled:
	case <-time.After(telemetryCloseDrainTimeout + 2*time.Second):
		t.Fatal("constructor cleanup did not cancel the controlled DB operation")
	}
	// Keep the real driver call beyond Close's caller wait. The constructor
	// must remain the physical owner without preserving that transient timeout.
	boundedWaitElapsed := time.NewTimer(telemetryCloseDrainTimeout + 250*time.Millisecond)
	select {
	case err := <-result:
		boundedWaitElapsed.Stop()
		t.Fatalf("constructor returned before actual DB driver join: %v", err)
	case <-boundedWaitElapsed.C:
	}
	select {
	case err := <-result:
		t.Fatalf("constructor returned while the driver barrier remained held: %v", err)
	default:
	}

	lifePath, err := daemonInstanceLifeGuardPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	if other, acquireErr := runtimeinfo.AcquireFileLeaseContext(ctx, lifePath); acquireErr == nil {
		_ = other.Release()
		cancel()
		t.Fatal("OS lifetime lease was available before the DB join")
	}
	cancel()
	cooperativePath, err := daemonInstanceLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if other, acquireErr := acquireStartLockAt(cooperativePath, 100*time.Millisecond, time.Now, processExists); acquireErr == nil {
		_ = other.Close()
		t.Fatal("cooperative lifetime lease was available before the DB join")
	} else if !strings.Contains(acquireErr.Error(), "timed out") {
		t.Fatalf("unexpected cooperative lease result: %v", acquireErr)
	}

	releaseDriver()
	select {
	case err := <-result:
		joined = true
		if err == nil || !strings.Contains(err.Error(), "injected") || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("constructor did not preserve only its actual failure: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("constructor did not return after the actual DB driver joined")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	other, err := runtimeinfo.AcquireFileLeaseContext(ctx, lifePath)
	if err != nil {
		t.Fatalf("OS lifetime lease remained after DB join: %v", err)
	}
	_ = other.Release()
	if _, err := os.Stat(cooperativePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cooperative lifetime lease remained after cleanup: %v", err)
	}
}

func queueHeldConstructorWrite(store *TelemetryStore, gate *drainConnector) error {
	if err := store.RecordAccess(0, model.AccessEvent{SessionID: "ctor", Operation: "held", OccurredAt: time.Now()}); err != nil {
		return err
	}
	select {
	case <-gate.entered:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("controlled DB write did not enter its driver call")
	}
}

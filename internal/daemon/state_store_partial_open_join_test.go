package daemon

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
	"modernc.org/sqlite"
)

type partialOpenCloseGate struct {
	path    string
	entered chan struct{}
	release chan struct{}
	armed   atomic.Bool
	once    sync.Once
}

func (g *partialOpenCloseGate) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(g.path)
	if err != nil {
		return nil, err
	}
	return partialOpenCloseConn{Conn: conn, gate: g}, nil
}

func (g *partialOpenCloseGate) Driver() driver.Driver { return &sqlite.Driver{} }

type partialOpenCloseConn struct {
	driver.Conn
	gate *partialOpenCloseGate
}

func (c partialOpenCloseConn) Close() error {
	if c.gate.armed.Load() {
		c.gate.once.Do(func() {
			close(c.gate.entered)
			<-c.gate.release
		})
	}
	return c.Conn.Close()
}

func newPartialOpenStoreFixture(t *testing.T) (*TelemetryStore, *partialOpenCloseGate, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mopen-")
	if err != nil {
		t.Fatal(err)
	}
	gate := &partialOpenCloseGate{
		path: filepath.Join(dir, "store.db"), entered: make(chan struct{}), release: make(chan struct{}),
	}
	db := sql.OpenDB(gate)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{
		db: db, writeQueue: make(chan telemetryWriteRequest, 2), done: make(chan struct{}),
		walMaxBytes: telemetryWALMaxBytes,
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate.release) }) }
	t.Cleanup(func() {
		release()
		_ = store.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.WaitClosed(ctx); err != nil {
			t.Errorf("join partial telemetry store: %v", err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove partial store fixture: %v", err)
		}
	})
	if err := store.configureConnection(); err != nil {
		t.Fatalf("configure partial store fixture: %v", err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatalf("initialize partial store fixture: %v", err)
	}
	gate.armed.Store(true)
	return store, gate, release
}

func joinPartialOpenResult(t *testing.T, result <-chan error, label string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Errorf("timed out waiting for %s; releasing gate before physical join", label)
		return <-result
	}
}

func TestCleanupPartialTelemetryStoreResultPhysicallyJoins(t *testing.T) {
	store, gate, release := newPartialOpenStoreFixture(t)
	openErr := errors.New("injected partial open failure")
	result := make(chan error, 1)
	go func() {
		returnedStore, err := cleanupPartialTelemetryStoreResult(store, openErr)
		if returnedStore != nil {
			err = errors.Join(err, errors.New("partial store was returned after open failure"))
		}
		result <- err
	}()
	joined := false
	defer func() {
		release()
		if !joined {
			_ = joinPartialOpenResult(t, result, "partial-open cleanup goroutine")
		}
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("partial store cleanup did not reach physical driver.Close")
	}
	select {
	case err := <-result:
		joined = true
		t.Fatalf("partial-open cleanup returned before driver.Close: %v", err)
	default:
	}
	release()
	select {
	case err := <-result:
		joined = true
		if !errors.Is(err, openErr) {
			t.Fatalf("partial-open cleanup lost original cause: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("partial-open cleanup did not return after physical Close")
	}
}

func TestConstructorJoinsPartialTelemetryBeforeReleasingLeases(t *testing.T) {
	privateLifetimeTestHome(t)
	repoRoot := t.TempDir()
	store, gate, release := newPartialOpenStoreFixture(t)
	openErr := errors.New("injected constructor partial telemetry failure")
	var listenCalls atomic.Int32
	deps := serverConstructionDeps{
		openTelemetryStore: func() (*TelemetryStore, error) { return store, openErr },
		listen: func() (net.Listener, error) {
			listenCalls.Add(1)
			return nil, errors.New("listener must not be reached")
		},
		generateAdminToken: generateAdminToken,
	}
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	result := make(chan error, 1)
	go func() {
		_, err := newServerWithConstructionDeps(repoRoot, 1, time.Minute, options, deps)
		result <- err
	}()
	joined := false
	defer func() {
		release()
		if !joined {
			_ = joinPartialOpenResult(t, result, "partial-open constructor goroutine")
		}
	}()

	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("constructor cleanup did not reach physical driver.Close")
	}
	// Exceed Close's bounded caller wait: the constructor must still retain both
	// outer instance leases until WaitClosed observes the actual driver return.
	select {
	case err := <-result:
		joined = true
		t.Fatalf("constructor returned before physical driver.Close: %v", err)
	case <-time.After(telemetryCloseDrainTimeout + 250*time.Millisecond):
	}
	if listenCalls.Load() != 0 {
		t.Fatalf("listener called %d times after open returned store+error", listenCalls.Load())
	}

	lifePath, err := daemonInstanceLifeGuardPath()
	if err != nil {
		t.Fatal(err)
	}
	lifeCtx, cancelLife := context.WithTimeout(context.Background(), 60*time.Millisecond)
	lifeProbe, lifeErr := runtimeinfo.AcquireFileLeaseContext(lifeCtx, lifePath)
	cancelLife()
	if lifeErr == nil {
		_ = lifeProbe.Release()
		t.Fatal("OS instance lifetime lease became available before physical close")
	}
	ownerPath, err := daemonInstanceLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if ownerProbe, ownerErr := acquireStartLockAt(ownerPath, 100*time.Millisecond, time.Now, processExists); ownerErr == nil {
		_ = ownerProbe.Close()
		t.Fatal("cooperative instance lease became available before physical close")
	}

	release()
	select {
	case err := <-result:
		joined = true
		if !errors.Is(err, openErr) {
			t.Fatalf("constructor lost original open failure: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("constructor did not join after physical driver.Close")
	}
	if listenCalls.Load() != 0 {
		t.Fatalf("listener called %d times; want 0", listenCalls.Load())
	}

	lifeCtx, cancelLife = context.WithTimeout(context.Background(), time.Second)
	lifeProbe, err = runtimeinfo.AcquireFileLeaseContext(lifeCtx, lifePath)
	cancelLife()
	if err != nil {
		t.Fatalf("OS instance lease remained after cleanup joined: %v", err)
	}
	if err := lifeProbe.Release(); err != nil {
		t.Errorf("release OS instance lease probe: %v", err)
	}
	ownerProbe, err := acquireStartLockAt(ownerPath, time.Second, time.Now, processExists)
	if err != nil {
		t.Fatalf("cooperative instance lease remained after cleanup joined: %v", err)
	}
	if err := ownerProbe.Close(); err != nil {
		t.Errorf("release cooperative instance lease probe: %v", err)
	}
}

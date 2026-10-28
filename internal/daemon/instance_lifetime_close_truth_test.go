package daemon

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
	"modernc.org/sqlite"
)

type physicalCloseErrorConnector struct {
	path string
	err  error
}

func (c physicalCloseErrorConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	return physicalCloseErrorConn{Conn: conn, closeErr: c.err}, nil
}

func (c physicalCloseErrorConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type physicalCloseErrorConn struct {
	driver.Conn
	closeErr error
}

func (c physicalCloseErrorConn) Close() error {
	return errors.Join(c.Conn.Close(), c.closeErr)
}

func openPhysicalCloseErrorStore(t *testing.T, closeErr error) *TelemetryStore {
	t.Helper()
	db := sql.OpenDB(physicalCloseErrorConnector{path: filepath.Join(t.TempDir(), "close-error.db"), err: closeErr})
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{db: db, writeQueue: make(chan telemetryWriteRequest, 1), done: make(chan struct{}), walMaxBytes: telemetryWALMaxBytes}
	if err := store.configureConnection(); err != nil {
		t.Fatal(err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = store.WaitClosed(context.Background())
	})
	return store
}

func TestConstructorCleanupPreservesPhysicalDBCloseError(t *testing.T) {
	cause := errors.New("injected constructor failure")
	closeErr := errors.New("injected physical driver close failure")
	store := openPhysicalCloseErrorStore(t, closeErr)
	got := (&Server{telemetry: store}).cleanupFailedConstruction(cause)
	if !errors.Is(got, cause) || !errors.Is(got, closeErr) {
		t.Fatalf("constructor cleanup lost genuine cause/physical close error: %v", got)
	}
	if errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("constructor cleanup retained a transient bounded wait timeout: %v", got)
	}
}

func TestShutdownPreservesPhysicalDBCloseError(t *testing.T) {
	home := privateLifetimeTestHome(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	closeErr := errors.New("injected physical driver close failure")
	store := openPhysicalCloseErrorStore(t, closeErr)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.ShutdownContext(ctx); !errors.Is(err, closeErr) {
		t.Fatalf("shutdown lost genuine physical close error: %v", err)
	}
	select {
	case <-server.shutdownDone:
	default:
		t.Fatal("shutdown returned before its physical owner completed")
	}
}

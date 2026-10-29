package daemon

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"modernc.org/sqlite"
)

type retryBlockedConnector struct {
	path string
	entered chan struct{}
	release chan struct{}
}

func (c *retryBlockedConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil { return nil, err }
	return retryBlockedConn{Conn: conn, owner: c}, nil
}

func (c *retryBlockedConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type retryBlockedConn struct {
	driver.Conn
	owner *retryBlockedConnector
}

func (c retryBlockedConn) Close() error {
	close(c.owner.entered)
	<-c.owner.release
	return c.Conn.Close()
}

func TestCloseRetryOwnerBlockedDriver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	connector := &retryBlockedConnector{path: path, entered: make(chan struct{}), release: make(chan struct{})}
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil { t.Fatal(err) }
	s := &TelemetryStore{db: db, dbPath: path}
	owner := make(chan error, 1)
	closeCall := make(chan error, 1)
	go func() { closeCall <- s.Close() }()
	go func() { owner <- s.WaitClosed(context.Background()) }()
	t.Cleanup(func() {
		close(connector.release)
		<-closeCall
		<-owner
	})
	<-connector.entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitClosed(ctx); !errors.Is(err, context.Canceled) { t.Fatalf("blocked close: %v", err) }
	if s.isTelemetryClosePhysical() { t.Fatal("physical close completed before driver returned") }
}

func TestCloseRetryOwnerRecoversGuardPath(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "db"))
	if err != nil { t.Fatal(err) }
	s := &TelemetryStore{db: db, dbPath: filepath.Join(parent, "db")}
	t.Cleanup(func() {
		_ = os.MkdirAll(parent, 0700)
		_ = s.Close()
		_ = s.WaitClosed(context.Background())
	})
	if err := s.Close(); err == nil { t.Fatal("expected guard path failure") }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	owner := make(chan error, 1)
	go func() { owner <- s.WaitClosed(ctx) }()
	if err := os.MkdirAll(parent, 0700); err != nil { t.Fatal(err) }
	if err := <-owner; err != nil { t.Fatalf("owner did not recover: %v", err) }
}

func TestCloseRetryOwnerCancellationAndNoImplicitClose(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "db"))
	if err != nil { t.Fatal(err) }
	s := &TelemetryStore{db: db, dbPath: filepath.Join(parent, "db")}
	t.Cleanup(func() {
		_ = os.MkdirAll(parent, 0700)
		_ = s.Close()
		_ = s.WaitClosed(context.Background())
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitClosed(ctx); !errors.Is(err, context.Canceled) { t.Fatalf("wait before close: %v", err) }
	if s.isTelemetryClosePhysical() { t.Fatal("wait initiated physical close") }
	if err := s.Close(); err == nil { t.Fatal("expected guard path failure") }
	ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = s.WaitClosed(ctx)
	if !errors.Is(err, context.DeadlineExceeded) { t.Fatalf("persistent failure: %v", err) }
	if s.closeResult() == nil || s.isTelemetryClosePhysical() { t.Fatal("lost guard failure or fabricated completion") }
}

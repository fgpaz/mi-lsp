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
	path    string
	entered chan struct{}
	release chan struct{}
}

func (c *retryBlockedConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
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
	s := &TelemetryStore{db: db, dbPath: path}
	closeDone := make(chan struct{})
	ownerDone := make(chan struct{})
	var closeErr, ownerErr error
	closeLaunched, ownerLaunched := false, false
	ownerCtx, ownerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(func() {
		close(connector.release)
		if closeLaunched {
			<-closeDone
		} else {
			_ = s.Close()
		}
		if ownerLaunched {
			<-ownerDone
			if ownerErr != nil {
				t.Errorf("join blocked owner: %v", ownerErr)
			}
		}
		ownerCancel()
		joinCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.WaitClosed(joinCtx); err != nil {
			t.Errorf("join physical close: %v", err)
		}
	})
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	closeLaunched = true
	go func() { defer close(closeDone); closeErr = s.Close() }()
	ownerLaunched = true
	go func() { defer close(ownerDone); ownerErr = s.WaitClosed(ownerCtx) }()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-connector.entered:
	case <-closeDone:
		t.Fatalf("close returned before driver entered: %v", closeErr)
	case <-timer.C:
		t.Fatal("driver close did not enter")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitClosed(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked close: %v", err)
	}
	if s.isTelemetryClosePhysical() {
		t.Fatal("physical close completed before driver returned")
	}
}

func TestCloseRetryOwnerRecoversGuardPath(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	s := &TelemetryStore{db: db, dbPath: filepath.Join(parent, "db")}
	ownerDone := make(chan struct{})
	var ownerErr error
	closeRequested, ownerLaunched := false, false
	t.Cleanup(func() {
		if err := os.MkdirAll(parent, 0700); err != nil {
			t.Errorf("restore guard path: %v", err)
		}
		if !closeRequested {
			_ = s.Close()
		}
		if ownerLaunched {
			<-ownerDone
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.WaitClosed(ctx); err != nil {
			t.Errorf("join physical close: %v", err)
		}
	})
	closeRequested = true
	if err := s.Close(); err == nil {
		t.Fatal("expected guard path failure")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ownerLaunched = true
	go func() { defer close(ownerDone); ownerErr = s.WaitClosed(ctx) }()
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	<-ownerDone
	if ownerErr != nil {
		t.Fatalf("owner did not recover: %v", ownerErr)
	}
}

func TestCloseRetryOwnerCancellationAndNoImplicitClose(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	s := &TelemetryStore{db: db, dbPath: filepath.Join(parent, "db")}
	t.Cleanup(func() {
		_ = os.MkdirAll(parent, 0700)
		_ = s.Close()
		_ = s.WaitClosed(context.Background())
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitClosed(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait before close: %v", err)
	}
	if s.isTelemetryClosePhysical() {
		t.Fatal("wait initiated physical close")
	}
	if err := s.Close(); err == nil {
		t.Fatal("expected guard path failure")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = s.WaitClosed(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("persistent failure: %v", err)
	}
	if s.closeResult() == nil || s.isTelemetryClosePhysical() {
		t.Fatal("lost guard failure or fabricated completion")
	}
}

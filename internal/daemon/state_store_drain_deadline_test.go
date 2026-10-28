package daemon

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"modernc.org/sqlite"
)

// This gate is a controlled database/sql boundary, not evidence about native
// SQLite latency or a physical WAL size cap. Cancellation is observed, but the
// driver call retains ownership until the test explicitly releases it.
type drainConnector struct {
	path     string
	entered  chan struct{}
	release  chan struct{}
	canceled chan struct{}
	stopRunErr error
	once     sync.Once
}

func (c *drainConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	return &drainConn{Conn: conn, gate: c}, nil
}
func (c *drainConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type drainConn struct {
	driver.Conn
	gate *drainConnector
}

func (c *drainConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "SELECT COALESCE(MAX(seq)") {
		c.gate.once.Do(func() { close(c.gate.entered) })
		select {
		case <-ctx.Done():
			if c.gate.canceled != nil {
				close(c.gate.canceled)
			}
			<-c.gate.release
			return nil, ctx.Err()
		case <-c.gate.release:
		}
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *drainConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.gate.stopRunErr != nil && strings.Contains(query, "UPDATE daemon_runs SET stopped_at") {
		return nil, c.gate.stopRunErr
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func TestCloseBoundsInflightPersistence(t *testing.T) {
	gate := &drainConnector{path: filepath.Join(t.TempDir(), "t.db"), entered: make(chan struct{}), release: make(chan struct{})}
	db := sql.OpenDB(gate)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &TelemetryStore{db: db, writeQueue: make(chan telemetryWriteRequest, 8), done: make(chan struct{}), walMaxBytes: telemetryWALMaxBytes}
	if err := s.configureConnection(); err != nil {
		t.Fatal(err)
	}
	if err := s.initSchema(); err != nil {
		t.Fatal(err)
	}
	s.wg.Add(1)
	go s.asyncWriteWorker()
	var closeResult chan error
	defer func() {
		close(gate.release)
		if closeResult == nil {
			closeResult = make(chan error, 1)
			go func() { closeResult <- s.Close() }()
		}
		<-closeResult
		s.wg.Wait()
		if err := s.WaitClosed(context.Background()); err != nil {
			t.Errorf("cleanup close owner: %v", err)
		}
		_ = db.Close()
	}()
	if err := s.RecordAccess(0, model.AccessEvent{SessionID: "gate", Operation: "inflight", OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("persistence did not enter the sequence query")
	}
	closeResult = make(chan error, 1)
	go func() { closeResult <- s.Close() }()
	select {
	case err := <-closeResult:
		// Preserve the result for cleanup's genuine join.
		closeResult <- err
		if err == nil {
			t.Error("Close reported complete while a driver call still owns the database")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := db.PingContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("database ownership was not retained while call is in flight: %v", err)
		}
	case <-time.After(telemetryCloseDrainTimeout + time.Second):
		t.Error("Close exceeded its drain budget while persistence was in flight")
	}
}

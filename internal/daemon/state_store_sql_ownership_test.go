package daemon

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type controlledSQLRead struct {
	entered    chan struct{}
	release    chan struct{}
	rowsClosed chan struct{}
	connClosed chan struct{}
	closeErr   error
	enteredOne sync.Once
	rowsOne    sync.Once
	connOne    sync.Once
}

var controlledSQLReads = struct {
	sync.Mutex
	byDSN map[string]*controlledSQLRead
}{byDSN: make(map[string]*controlledSQLRead)}

const controlledSQLDriverName = "telemetry-controlled-read"

func init() { sql.Register(controlledSQLDriverName, controlledSQLDriver{}) }

type controlledSQLDriver struct{}

func (controlledSQLDriver) Open(name string) (driver.Conn, error) {
	controlledSQLReads.Lock()
	control := controlledSQLReads.byDSN[name]
	controlledSQLReads.Unlock()
	if control == nil {
		return nil, errors.New("controlled reader fixture not registered")
	}
	return controlledSQLConn{control: control}, nil
}

type controlledSQLConn struct{ control *controlledSQLRead }

func (c controlledSQLConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("Prepare unsupported") }
func (c controlledSQLConn) Close() error {
	c.control.connOne.Do(func() { close(c.control.connClosed) })
	return c.control.closeErr
}
func (controlledSQLConn) Begin() (driver.Tx, error) { return nil, errors.New("Begin unsupported") }
func (c controlledSQLConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (c controlledSQLConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &controlledSQLRows{control: c.control}, nil
}

type controlledSQLRows struct {
	control *controlledSQLRead
	rowSent bool
}

func (*controlledSQLRows) Columns() []string {
	return []string{"id", "occurred_at", "client", "session", "seq", "workspace", "workspace_input", "workspace_root", "workspace_alias", "repo", "operation", "backend", "route", "format", "token_budget", "max_items", "max_chars", "compress", "success", "latency_ms", "warnings", "runtime_key", "entrypoint_id", "error_text", "error_kind", "error_code", "truncated", "result_count", "warning_count", "pattern_mode", "routing_outcome", "failure_stage", "hint_code", "truncation_reason", "decision_json", "bytes_out", "harness", "q_stages"}
}
func (r *controlledSQLRows) Close() error {
	r.control.rowsOne.Do(func() { close(r.control.rowsClosed) })
	return nil
}
func (r *controlledSQLRows) Next(dest []driver.Value) error {
	if !r.rowSent {
		r.rowSent = true
		values := []driver.Value{int64(1), int64(1), "client", "session", int64(1), "workspace", "input", "root", "alias", "repo", "operation", "backend", "route", "json", int64(1), int64(1), int64(1), int64(0), int64(1), int64(1), "[]", "runtime", "entry", "", "", "", int64(0), int64(0), int64(0), "none", "direct", "none", "", "", "", int64(0), "test", int64(0)}
		copy(dest, values)
		return nil
	}
	r.control.enteredOne.Do(func() { close(r.control.entered) })
	<-r.control.release
	return io.EOF
}

func registerControlledRead(t *testing.T) (*TelemetryStore, *controlledSQLRead) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "controlled-reader.db")
	if err := os.WriteFile(path, []byte("private controlled-driver fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	control := &controlledSQLRead{
		entered: make(chan struct{}), release: make(chan struct{}),
		rowsClosed: make(chan struct{}), connClosed: make(chan struct{}),
	}
	controlledSQLReads.Lock()
	controlledSQLReads.byDSN[path] = control
	controlledSQLReads.Unlock()
	db, err := sql.Open(controlledSQLDriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &TelemetryStore{dbPath: path, db: db}
	return s, control
}

func TestTelemetryPublicReaderOwnsRowsUntilCloseAndCloseWaits(t *testing.T) {
	s, control := registerControlledRead(t)
	readerDone := make(chan error, 1)
	readerFinished := make(chan struct{})
	go func() {
		defer close(readerFinished)
		_, err := QueryAccessEvents(s, ExportQuery{Limit: 5})
		readerDone <- err
	}()
	readerLaunched := true
	closeDone := make(chan error, 1)
	closeFinished := make(chan struct{})
	closeLaunched := false
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(control.release) })
		if readerLaunched { <-readerFinished }
		if closeLaunched { <-closeFinished }
		if err := s.Close(); err != nil { t.Errorf("cleanup Close: %v", err) }
		if err := s.WaitClosed(context.Background()); err != nil { t.Errorf("cleanup WaitClosed: %v", err) }
		controlledSQLReads.Lock()
		delete(controlledSQLReads.byDSN, s.dbPath)
		controlledSQLReads.Unlock()
	}()
	select {
	case <-control.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("public export reader did not reach its borrowed Rows boundary")
	}
	go func() {
		defer close(closeFinished)
		closeDone <- s.Close()
	}()
	closeLaunched = true
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 30*time.Millisecond)
	waitErr := s.WaitClosed(waitCtx)
	cancelWait()
	if !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("WaitClosed must remain pending while public Rows are borrowed, got %v", waitErr)
	}
	if !s.WALHealth().Closed {
		t.Fatal("close admission was not closed while reader remained borrowed")
	}
	releaseOnce.Do(func() { close(control.release) })
	if err := <-readerDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("public export reader during shutdown: %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close after reader return: %v", err)
	}
	<-control.rowsClosed
	<-control.connClosed
	if err := s.WaitClosed(context.Background()); err != nil {
		t.Fatalf("physical close join: %v", err)
	}
	if _, err := QueryAccessEvents(s, ExportQuery{Limit: 1}); err == nil {
		t.Fatal("new public read admitted after close")
	}
}

func TestTelemetryDriverCloseErrorPropagatesAfterPhysicalReturn(t *testing.T) {
	s, control := registerControlledRead(t)
	t.Cleanup(func() {
		_ = s.Close()
		_ = s.WaitClosed(context.Background())
		controlledSQLReads.Lock()
		delete(controlledSQLReads.byDSN, s.dbPath)
		controlledSQLReads.Unlock()
	})
	if _, err := s.db.Exec("fixture connection"); err != nil {
		t.Fatalf("open controlled driver connection: %v", err)
	}
	want := errors.New("controlled driver close error")
	control.closeErr = want
	if err := s.Close(); !errors.Is(err, want) {
		t.Fatalf("Close error=%v, want driver error %v", err, want)
	}
	if err := s.WaitClosed(context.Background()); !errors.Is(err, want) {
		t.Fatalf("WaitClosed error=%v, want returned driver error %v", err, want)
	}
	<-control.connClosed
}

func TestTelemetryCloseOwnershipFailureDoesNotClaimPhysicalCompletionAndRetries(t *testing.T) {
	s, control := registerControlledRead(t)
	originalPath := s.dbPath
	t.Cleanup(func() {
		s.dbPath = originalPath
		_ = s.Close()
		_ = s.WaitClosed(context.Background())
		controlledSQLReads.Lock()
		delete(controlledSQLReads.byDSN, originalPath)
		controlledSQLReads.Unlock()
	})
	if _, err := s.db.Exec("fixture connection"); err != nil {
		t.Fatalf("open controlled driver connection: %v", err)
	}
	path := s.dbPath
	s.dbPath = ":memory:"
	if err := s.Close(); err == nil {
		t.Fatal("close with invalid ownership path unexpectedly succeeded")
	}
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := s.WaitClosed(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failed ownership acquisition falsely completed physical close: %v", err)
	}
	cancelWait()
	select {
	case <-control.connClosed:
		t.Fatal("DB.Close ran without physical ownership")
	default:
	}
	s.dbPath = path
	if err := s.Close(); err != nil {
		t.Fatalf("Close retry after restoring ownership path: %v", err)
	}
	<-control.connClosed
	if err := s.WaitClosed(context.Background()); err != nil {
		t.Fatalf("WaitClosed after physical close: %v", err)
	}
}

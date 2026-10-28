package daemon

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"modernc.org/sqlite"
)

var metadataGateDriverSequence atomic.Uint64

type metadataRollbackGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	armed   atomic.Bool
}

type metadataGateDriver struct{ gate *metadataRollbackGate }

func (d metadataGateDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return metadataGateConn{Conn: conn, gate: d.gate}, nil
}

type metadataGateConn struct {
	driver.Conn
	gate *metadataRollbackGate
}

func (c metadataGateConn) Begin() (driver.Tx, error) {
	tx, err := c.Conn.Begin()
	if err != nil {
		return nil, err
	}
	return metadataGateTx{Tx: tx, gate: c.gate}, nil
}

func (c metadataGateConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	var tx driver.Tx
	var err error
	if beginTx, ok := c.Conn.(driver.ConnBeginTx); ok {
		tx, err = beginTx.BeginTx(ctx, opts)
	} else {
		tx, err = c.Conn.Begin()
	}
	if err != nil {
		return nil, err
	}
	return metadataGateTx{Tx: tx, gate: c.gate}, nil
}

type metadataGateTx struct {
	driver.Tx
	gate *metadataRollbackGate
}

func (tx metadataGateTx) Rollback() error {
	if tx.gate.armed.Load() {
		tx.gate.once.Do(func() {
			close(tx.gate.entered)
			<-tx.gate.release
		})
	}
	return tx.Tx.Rollback()
}

func newMetadataAdmissionFixture(t *testing.T) *TelemetryStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "mlmeta-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.db")
	store := openMetadataFixtureStore(t, path, true)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = store.Close()
		if err := store.WaitClosed(ctx); err != nil {
			t.Errorf("join metadata store close: %v", err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove private metadata fixture: %v", err)
		}
	})
	return store
}

func openMetadataFixtureStore(t *testing.T, path string, initialize bool) *TelemetryStore {
	return openMetadataFixtureStoreWithDriver(t, "sqlite", path, initialize)
}

func openMetadataFixtureStoreWithDriver(t *testing.T, driverName, path string, initialize bool) *TelemetryStore {
	t.Helper()
	db, err := sql.Open(driverName, path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{
		dbPath:        path,
		db:            db,
		writeQueue:    make(chan telemetryWriteRequest, 4),
		done:          make(chan struct{}),
		walMaxBytes:   telemetryWALMaxBytes,
		closeComplete: make(chan struct{}),
	}
	if initialize {
		if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
			_ = db.Close()
			t.Fatalf("configure private fixture WAL: %v", err)
		}
		if err := store.initSchema(); err != nil {
			_ = db.Close()
			t.Fatalf("initialize private fixture schema: %v", err)
		}
	}
	return store
}

func joinMetadataMutation(t *testing.T, label string, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Errorf("timed out waiting for %s; releasing gates before physical join", label)
		return <-results
	}
}

func readStoppedAt(t *testing.T, db *sql.DB, pid int) sql.NullInt64 {
	t.Helper()
	var stopped sql.NullInt64
	if err := db.QueryRow(`SELECT stopped_at FROM daemon_runs WHERE pid = ?`, pid).Scan(&stopped); err != nil {
		t.Fatalf("read stopped_at for pid %d: %v", pid, err)
	}
	return stopped
}

func TestMetadataMutationSerializesAcrossStoresAtPhysicalSQL(t *testing.T) {
	first := newMetadataAdmissionFixture(t)
	second := openMetadataFixtureStore(t, first.dbPath, false)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = second.Close()
		if err := second.WaitClosed(ctx); err != nil {
			t.Errorf("join second metadata store close: %v", err)
		}
	})

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	firstJoined, secondJoined, secondStarted := false, false, false
	go func() {
		firstDone <- first.withTelemetryMutationContext(context.Background(), func(ctx context.Context) error {
			if _, err := first.db.ExecContext(ctx,
				`INSERT INTO daemon_runs(pid, endpoint, started_at) VALUES (?, ?, ?)`, 1, "first", time.Now().Unix()); err != nil {
				return err
			}
			close(entered)
			<-release
			return nil
		})
	}()
	// Always release the actual owner before joining either goroutine, including
	// every fatal/assertion path below.
	defer func() {
		unblock()
		if !firstJoined {
			if err := joinMetadataMutation(t, "first physical metadata mutation", firstDone); err != nil {
				t.Errorf("join first physical metadata mutation: %v", err)
			}
		}
		if secondStarted && !secondJoined {
			if err := joinMetadataMutation(t, "launched contending metadata mutation", secondDone); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("join contending metadata mutation: %v", err)
			}
		}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first mutation did not reach physical SQL barrier")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	secondStarted = true
	go func() { secondDone <- second.stopRunContext(ctx, 1, time.Now()) }()
	select {
	case err := <-secondDone:
		secondJoined = true
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("contending mutation error = %v; want context deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("contending mutation did not finish within bounded acquisition context")
	}

	if stopped := readStoppedAt(t, first.db, 1); stopped.Valid {
		t.Fatalf("contending UPDATE changed stopped_at while owner held: %v", stopped.Int64)
	}
	unblock()
	firstErr := joinMetadataMutation(t, "released first physical metadata mutation", firstDone)
	firstJoined = true
	if firstErr != nil {
		t.Fatalf("first mutation: %v", firstErr)
	}
	runID, err := second.StartRun(model.DaemonState{PID: 2, StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("metadata mutation after released owner: %v", err)
	}
	if runID == 0 {
		t.Fatal("StartRun returned zero run ID")
	}
	if err := second.ReplaceRuntimeSnapshots(runID, []model.WorkerStatus{{RuntimeKey: "recovered"}}); err != nil {
		t.Fatalf("snapshot replacement after owner release: %v", err)
	}
	stopTime := time.Unix(1234567890, 0)
	if err := second.stopRunContext(context.Background(), runID, stopTime); err != nil {
		t.Fatalf("stop run after owner release: %v", err)
	}
	stopped := readStoppedAt(t, second.db, 2)
	if !stopped.Valid || stopped.Int64 != stopTime.Unix() {
		t.Fatalf("recovered UPDATE stopped_at = %v; want %d", stopped, stopTime.Unix())
	}
	var snapshotCount int
	if err := second.db.QueryRow(`SELECT COUNT(*) FROM runtime_snapshots WHERE daemon_run_id = ? AND runtime_key = ?`, runID, "recovered").Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if snapshotCount != 1 {
		t.Fatalf("recovered snapshot count = %d; want 1", snapshotCount)
	}
}

func TestSnapshotRollbackPhysicallyJoinsBeforeLeaseRelease(t *testing.T) {
	dir, err := os.MkdirTemp("", "mltx-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.db")
	gate := &metadataRollbackGate{entered: make(chan struct{}), release: make(chan struct{})}
	driverName := fmt.Sprintf("metadata-rollback-%d", metadataGateDriverSequence.Add(1))
	sql.Register(driverName, metadataGateDriver{gate: gate})
	first := openMetadataFixtureStoreWithDriver(t, driverName, path, true)
	var second *TelemetryStore
	t.Cleanup(func() {
		ctxFirst, cancelFirst := context.WithTimeout(context.Background(), 5*time.Second)
		_ = first.Close()
		if err := first.WaitClosed(ctxFirst); err != nil {
			t.Errorf("join first rollback fixture store: %v", err)
		}
		cancelFirst()
		if second != nil {
			ctxSecond, cancelSecond := context.WithTimeout(context.Background(), 5*time.Second)
			_ = second.Close()
			if err := second.WaitClosed(ctxSecond); err != nil {
				t.Errorf("join second rollback fixture store: %v", err)
			}
			cancelSecond()
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove private rollback fixture: %v", err)
		}
	})
	second = openMetadataFixtureStore(t, path, false)

	runID, err := first.StartRun(model.DaemonState{PID: 21, StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("create rollback fixture run: %v", err)
	}
	if err := first.ReplaceRuntimeSnapshots(runID, []model.WorkerStatus{{RuntimeKey: "seed"}}); err != nil {
		t.Fatalf("seed rollback fixture snapshot: %v", err)
	}
	gate.armed.Store(true)

	rollbackCtx, cancelRollback := context.WithCancel(context.Background())
	rollbackDone := make(chan error, 1)
	contenderDone := make(chan error, 1)
	contenderStarted, rollbackJoined, contenderJoined := false, false, false
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(gate.release) }) }
	defer func() {
		cancelRollback()
		unblock()
		if !rollbackJoined {
			_ = joinMetadataMutation(t, "physical snapshot rollback", rollbackDone)
		}
		if contenderStarted && !contenderJoined {
			if err := joinMetadataMutation(t, "rollback-excluded contender", contenderDone); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("join rollback-excluded contender: %v", err)
			}
		}
	}()
	go func() {
		rollbackDone <- first.replaceRuntimeSnapshotsContext(rollbackCtx, runID, []model.WorkerStatus{
			{RuntimeKey: "duplicate"},
			{RuntimeKey: "duplicate"},
		})
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual driver Tx.Rollback did not reach controlled gate")
	}
	cancelRollback()
	contenderCtx, cancelContender := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelContender()
	contenderStarted = true
	go func() { contenderDone <- second.stopRunContext(contenderCtx, runID, time.Now()) }()
	select {
	case err := <-contenderDone:
		contenderJoined = true
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("mutation during physical rollback = %v; want deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rollback-excluded contender exceeded its bounded context")
	}
	var stopped sql.NullInt64
	if err := second.db.QueryRow(`SELECT stopped_at FROM daemon_runs WHERE id = ?`, runID).Scan(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Valid {
		t.Fatalf("contender changed stopped_at during physical rollback: %v", stopped.Int64)
	}
	var seeded int
	if err := second.db.QueryRow(`SELECT COUNT(*) FROM runtime_snapshots WHERE daemon_run_id = ? AND runtime_key = 'seed'`, runID).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	if seeded != 1 {
		t.Fatalf("snapshot visible during rollback = %d seed rows; want 1", seeded)
	}

	unblock()
	rollbackErr := joinMetadataMutation(t, "completed physical rollback callback", rollbackDone)
	rollbackJoined = true
	if rollbackErr == nil {
		t.Fatal("duplicate snapshot key unexpectedly committed")
	}
	if err := first.ReplaceRuntimeSnapshots(runID, []model.WorkerStatus{{RuntimeKey: "after-rollback"}}); err != nil {
		t.Fatalf("snapshot mutation after physical rollback: %v", err)
	}
	var recovered int
	if err := second.db.QueryRow(`SELECT COUNT(*) FROM runtime_snapshots WHERE daemon_run_id = ? AND runtime_key = 'after-rollback'`, runID).Scan(&recovered); err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered snapshot rows = %d; want 1", recovered)
	}
}

func TestMetadataMutationHonorsContendedCancellationAndRecovers(t *testing.T) {
	first := newMetadataAdmissionFixture(t)
	second := openMetadataFixtureStore(t, first.dbPath, false)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = second.Close()
		if err := second.WaitClosed(ctx); err != nil {
			t.Errorf("join second metadata store close: %v", err)
		}
	})

	entered := make(chan struct{})
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	ownerJoined := false
	go func() {
		ownerDone <- first.withTelemetryMutationContext(context.Background(), func(ctx context.Context) error {
			if _, err := first.db.ExecContext(ctx,
				`INSERT INTO daemon_runs(pid, endpoint, started_at) VALUES (?, ?, ?)`, 11, "owner", time.Now().Unix()); err != nil {
				return err
			}
			close(entered)
			<-release
			return nil
		})
	}()
	defer func() {
		unblock()
		if !ownerJoined {
			if err := joinMetadataMutation(t, "rollback-gate owner", ownerDone); err != nil {
				t.Errorf("join owner after cancellation: %v", err)
			}
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("owner did not reach physical SQL barrier")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := second.stopRunContext(ctx, 1, time.Now()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended metadata cancellation = %v; want context deadline", err)
	}
	if stopped := readStoppedAt(t, first.db, 11); stopped.Valid {
		t.Fatalf("canceled waiter changed stopped_at: %v", stopped.Int64)
	}
	unblock()
	ownerErr := joinMetadataMutation(t, "released metadata owner", ownerDone)
	ownerJoined = true
	if ownerErr != nil {
		t.Fatalf("owner mutation: %v", ownerErr)
	}
	runID, err := second.StartRun(model.DaemonState{PID: 12, StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("metadata mutation after canceled waiter/release: %v", err)
	}
	if err := second.ReplaceRuntimeSnapshots(runID, []model.WorkerStatus{{RuntimeKey: "after-cancel"}}); err != nil {
		t.Fatalf("snapshot recovery after canceled waiter: %v", err)
	}
	stopTime := time.Unix(1234567891, 0)
	if err := second.stopRunContext(context.Background(), runID, stopTime); err != nil {
		t.Fatalf("stop recovered run: %v", err)
	}
	stopped := readStoppedAt(t, second.db, 12)
	if !stopped.Valid || stopped.Int64 != stopTime.Unix() {
		t.Fatalf("recovered UPDATE stopped_at = %v; want %d", stopped, stopTime.Unix())
	}
}

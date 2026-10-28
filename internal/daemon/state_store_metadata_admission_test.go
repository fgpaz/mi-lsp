package daemon

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

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
	t.Helper()
	db, err := sql.Open("sqlite", path)
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
	firstJoined, secondJoined := false, false
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
			if err := <-firstDone; err != nil {
				t.Errorf("join first physical metadata mutation: %v", err)
			}
		}
		if !secondJoined {
			if err := <-secondDone; err != nil && !errors.Is(err, context.DeadlineExceeded) {
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

	var runs int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM daemon_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("contending SQL mutated rows while owner held: runs=%d, want 1", runs)
	}
	unblock()
	if err := <-firstDone; err != nil {
		firstJoined = true
		t.Fatalf("first mutation: %v", err)
	}
	firstJoined = true
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
	if err := second.stopRunContext(context.Background(), runID, time.Now()); err != nil {
		t.Fatalf("stop run after owner release: %v", err)
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
			if err := <-ownerDone; err != nil {
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
	var runs int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM daemon_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("canceled waiter caused late SQL: runs=%d, want 1", runs)
	}
	unblock()
	if err := <-ownerDone; err != nil {
		ownerJoined = true
		t.Fatalf("owner mutation: %v", err)
	}
	ownerJoined = true
	if _, err := second.StartRun(model.DaemonState{PID: 12, StartedAt: time.Now()}); err != nil {
		t.Fatalf("metadata mutation after canceled waiter/release: %v", err)
	}
}

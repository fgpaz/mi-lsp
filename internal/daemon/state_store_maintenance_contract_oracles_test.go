package daemon

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// These fixtures are private to each test and exercise the production owner,
// admission, purge and VACUUM paths rather than a model of those paths.
func maintenanceContractStore(t *testing.T, path string) *TelemetryStore {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS access_events (occurred_at INTEGER NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close private database: %v", err)
		}
	})
	return &TelemetryStore{db: db, dbPath: path, walMaxBytes: telemetryWALMaxBytes}
}

func maintenanceContractCounts(t *testing.T, before, after WALHealth, reason string) {
	t.Helper()
	if after.RejectedWrites != before.RejectedWrites+1 || after.FailedWrites != before.FailedWrites || after.LastWriteFailure != reason {
		t.Fatalf("admission accounting: before=%+v after=%+v want rejection +1, failed unchanged, reason %q", before, after, reason)
	}
}

func TestMaintenanceContractCrossStoreCanceledAcquisitionAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	owner := maintenanceContractStore(t, path)
	contender := maintenanceContractStore(t, path)
	if _, err := owner.db.Exec(`INSERT INTO access_events(occurred_at) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- owner.withTelemetryWriteOwnershipContext(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	// This cleanup always unblocks and joins the only launched owner, including
	// when an assertion fails before the ownership barrier is reached.
	joined := false
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		if !joined {
			if err := <-done; err != nil {
				t.Errorf("owner completion: %v", err)
			}
		}
	}()
	select {
	case <-entered:
	case err := <-done:
		joined = true
		t.Fatalf("owner failed before barrier: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("owner did not acquire shared guard")
	}

	before := contender.WALHealth()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	purged, vacuumed, err := contender.purgeAndVacuumContext(ctx, 1, 1<<62)
	if !errors.Is(err, context.DeadlineExceeded) || purged != 0 || vacuumed {
		t.Fatalf("contended acquisition: purged=%d vacuumed=%v err=%v", purged, vacuumed, err)
	}
	maintenanceContractCounts(t, before, contender.WALHealth(), "deadline")

	cancelCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	before = contender.WALHealth()
	_, vacuumed, err = contender.purgeAndVacuumContext(cancelCtx, 1, 1<<62)
	if !errors.Is(err, context.Canceled) || vacuumed {
		t.Fatalf("canceled acquisition: vacuumed=%v err=%v", vacuumed, err)
	}
	maintenanceContractCounts(t, before, contender.WALHealth(), "canceled")
	var count int
	if err := contender.db.QueryRow(`SELECT count(*) FROM access_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("refused maintenance changed events: count=%d err=%v", count, err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-done; err != nil {
		joined = true
		t.Fatalf("owner completion: %v", err)
	}
	joined = true
	purged, vacuumed, err = contender.purgeAndVacuumContext(context.Background(), 1, 1<<62)
	if err != nil || purged != 1 || !vacuumed {
		t.Fatalf("maintenance recovery: purged=%d vacuumed=%v err=%v", purged, vacuumed, err)
	}
}

func TestMaintenanceContractClosedPublicEntry(t *testing.T) {
	s := maintenanceContractStore(t, filepath.Join(t.TempDir(), "closed.db"))
	if _, err := s.db.Exec(`INSERT INTO access_events(occurred_at) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	s.stateMu.Lock()
	s.closed = true // public admission closed, while SQLite is still open
	s.stateMu.Unlock()
	before := s.WALHealth()
	purged, vacuumed, err := s.PurgeAndVacuum(1, 1<<62)
	if err == nil || !isTelemetryAdmissionError(err) || purged != 0 || vacuumed {
		t.Fatalf("closed maintenance: purged=%d vacuumed=%v err=%v", purged, vacuumed, err)
	}
	maintenanceContractCounts(t, before, s.WALHealth(), "closed")
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM access_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("closed maintenance changed events: count=%d err=%v", count, err)
	}
}

func TestMaintenanceContractSQLFailureIsNotAdmissionRefusal(t *testing.T) {
	s := maintenanceContractStore(t, filepath.Join(t.TempDir(), "sql-error.db"))
	if _, err := s.db.Exec(`DROP TABLE access_events`); err != nil {
		t.Fatal(err)
	}
	before := s.WALHealth()
	purged, vacuumed, err := s.purgeAndVacuumContext(context.Background(), 1, 1<<62)
	if err == nil || !strings.Contains(err.Error(), "purge events:") || purged != 0 || vacuumed {
		t.Fatalf("expected purge SQL failure: purged=%d vacuumed=%v err=%v", purged, vacuumed, err)
	}
	after := s.WALHealth()
	if after.RejectedWrites != before.RejectedWrites || after.FailedWrites != before.FailedWrites {
		t.Fatalf("SQL error counted as admission refusal: before=%+v after=%+v", before, after)
	}
	if _, err := s.db.Exec(`CREATE TABLE access_events (occurred_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, vacuumed, err := s.purgeAndVacuumContext(context.Background(), 1, 1<<62); err != nil || !vacuumed {
		t.Fatalf("recovery after SQL failure: vacuumed=%v err=%v", vacuumed, err)
	}
}

func TestMaintenanceContractHeldPhaseThresholdOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threshold.db")
	s := maintenanceContractStore(t, path)
	readerDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer readerDB.Close()
	reader, err := readerDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()
	var count int
	if err := reader.QueryRow(`SELECT count(*) FROM access_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO access_events(occurred_at) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	wal, err := s.walSize()
	if err != nil || wal <= 1 {
		t.Fatalf("pinned WAL: bytes=%d err=%v", wal, err)
	}
	s.walMaxBytes = wal - 1
	before := s.WALHealth()
	purged, vacuumed, err := s.purgeAndVacuumContext(context.Background(), 1, 1<<62)
	if err == nil || !strings.Contains(err.Error(), "purge events:") || purged != 0 || vacuumed {
		t.Fatalf("held purge admission: purged=%d vacuumed=%v err=%v", purged, vacuumed, err)
	}
	reason := walAdmissionReason(err)
	if reason != "busy" && reason != "wal_limit" && reason != "deadline" {
		t.Fatalf("unexpected held-phase reason %q: %v", reason, err)
	}
	maintenanceContractCounts(t, before, s.WALHealth(), reason)
	if err := reader.Rollback(); err != nil {
		t.Fatal(err)
	}
	s.walMaxBytes = telemetryWALMaxBytes
	if _, vacuumed, err := s.purgeAndVacuumContext(context.Background(), 1, 1<<62); err != nil || !vacuumed {
		t.Fatalf("recovery after threshold: vacuumed=%v err=%v", vacuumed, err)
	}
}

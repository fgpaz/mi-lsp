package daemon

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestVacuumPhaseAdmissionAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &TelemetryStore{dbPath: path, db: db, walMaxBytes: telemetryWALMaxBytes}
	defer closeMaintenanceStore(t, s)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE access_events (occurred_at INTEGER NOT NULL)",
		"CREATE TABLE daemon_runs (stopped_at INTEGER)",
		"INSERT INTO access_events(occurred_at) VALUES (1)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	reader, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if reader != nil {
			if err := reader.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("release pinned reader: %v", err)
			}
		}
	}()
	var n int
	if err := reader.QueryRow("SELECT count(*) FROM access_events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if _, err := db.Exec("INSERT INTO access_events(occurred_at) VALUES (?)", i+10); err != nil {
			t.Fatal(err)
		}
	}
	before, err := maintenanceWALBytes(path)
	if err != nil || before == 0 {
		t.Fatalf("pinned WAL unavailable: bytes=%d err=%v", before, err)
	}
	s.walMaxBytes = before - 1

	// Exercise the real VACUUM phase while already holding D's ownership-only
	// gate. A pinned reader prevents checkpoint recovery, so admission must
	// reject before VACUUM can mutate the database.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = s.withTelemetryWriteOwnershipContext(ctx, func(ownedCtx context.Context) error {
		return s.vacuumWithLeaseHeldContext(ownedCtx)
	})
	if !errors.Is(err, errTelemetryWALLimit) && !errors.Is(err, errTelemetryCheckpointBusy) &&
		!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("VACUUM refusal has no recognized admission cause: %v", err)
	}
	after, err := maintenanceWALBytes(path)
	if err != nil || after != before {
		t.Fatalf("rejected VACUUM changed WAL: before=%d after=%d err=%v", before, after, err)
	}
	if rejected := s.WALHealth().RejectedWrites; rejected != 1 {
		t.Fatalf("rejected VACUUM admission count = %d, want exactly 1", rejected)
	}

	if err := reader.Rollback(); err != nil {
		t.Fatal(err)
	}
	reader = nil
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer recoveryCancel()
	if err := s.Checkpoint(); err != nil {
		t.Fatalf("checkpoint after releasing pin: %v", err)
	}
	s.walMaxBytes = telemetryWALMaxBytes
	err = s.withTelemetryWriteOwnershipContext(recoveryCtx, func(ownedCtx context.Context) error {
		return s.vacuumWithLeaseHeldContext(ownedCtx)
	})
	if err != nil {
		t.Fatalf("VACUUM after recovery: %v", err)
	}
}

func TestPurgeAndVacuumUsesSingleOwnershipWithoutRecursion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "composition.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	s := &TelemetryStore{dbPath: path, db: db, walMaxBytes: telemetryWALMaxBytes}
	defer closeMaintenanceStore(t, s)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE access_events (occurred_at INTEGER NOT NULL)",
		"INSERT INTO access_events(occurred_at) VALUES (1)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, vacuumed, err := s.purgeAndVacuumContext(ctx, 1, 1<<62)
	if err != nil || !vacuumed {
		t.Fatalf("purge-and-vacuum: vacuumed=%v err=%v", vacuumed, err)
	}
}

func closeMaintenanceStore(t *testing.T, store *TelemetryStore) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Errorf("close telemetry store: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.WaitClosed(ctx); err != nil {
		t.Errorf("join telemetry store close: %v", err)
	}
}

func maintenanceWALBytes(path string) (int64, error) {
	info, err := os.Stat(path + "-wal")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

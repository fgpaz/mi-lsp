package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	_ "modernc.org/sqlite"
)

type mutationWALFixture struct {
	s *TelemetryStore
	reader *sql.Tx
	path string
	run int64
}

func newMutationWALFixture(t *testing.T) *mutationWALFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := sql.Open("sqlite", path)
	if err != nil { t.Fatal(err) }
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	s := &TelemetryStore{db: db, walMaxBytes: telemetryWALMaxBytes}
	f := &mutationWALFixture{s: s, path: path}
	t.Cleanup(func() {
		if f.reader != nil { _ = f.reader.Rollback() }
		if err := s.Close(); err != nil { t.Errorf("close: %v", err) }
		if err := s.WaitClosed(context.Background()); err != nil { t.Errorf("join: %v", err) }
	})
	if err := s.configureConnection(); err != nil { t.Fatal(err) }
	if err := s.initSchema(); err != nil { t.Fatal(err) }
	f.run, err = s.StartRun(model.DaemonState{PID: 1, StartedAt: time.Unix(1, 0)})
	if err != nil { t.Fatal(err) }
	if err := s.ReplaceRuntimeSnapshots(f.run, []model.WorkerStatus{{RuntimeKey: "seed"}}); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO access_events(operation, success, occurred_at) VALUES ('old', 1, 1)`); err != nil { t.Fatal(err) }
	if err := s.StopRun(f.run, time.Unix(2, 0)); err != nil { t.Fatal(err) }
	f.reader, err = db.Begin()
	if err != nil { t.Fatal(err) }
	var count int
	if err := f.reader.QueryRow(`SELECT count(*) FROM access_events`).Scan(&count); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO access_events(operation, success, occurred_at) VALUES ('pin', 1, 3)`); err != nil { t.Fatal(err) }
	bytes := f.size(t)
	if bytes < 2 { t.Fatalf("fixture WAL too small: %d", bytes) }
	s.walMaxBytes = bytes - 1
	return f
}

func (f *mutationWALFixture) size(t *testing.T) int64 {
	t.Helper()
	info, err := os.Stat(f.path + "-wal")
	if err != nil { t.Fatal(err) }
	return info.Size()
}

func (f *mutationWALFixture) attempt(t *testing.T, label string, op func() error) {
	t.Helper()
	before := f.size(t)
	if before <= f.s.walMaxBytes { t.Fatalf("%s fixture below threshold: %d <= %d", label, before, f.s.walMaxBytes) }
	h := f.s.WALHealth()
	err := op()
	after := f.size(t)
	if err == nil { t.Errorf("%s admitted above threshold: WAL before=%d after=%d limit=%d rejected before=%d after=%d", label, before, after, f.s.walMaxBytes, h.RejectedWrites, f.s.WALHealth().RejectedWrites) }
}

func (f *mutationWALFixture) recover(t *testing.T) {
	t.Helper()
	if err := f.reader.Rollback(); err != nil { t.Fatal(err) }
	f.reader = nil
	if err := f.s.Checkpoint(); err != nil { t.Fatalf("checkpoint after release: %v", err) }
}

func TestWALSnapshotGrowthStopsAtAdmissionThreshold(t *testing.T) {
	f := newMutationWALFixture(t)
	for i := 0; i < 4; i++ {
		status := model.WorkerStatus{RuntimeKey: fmt.Sprintf("worker-%d", i), PID: i + 10}
		f.attempt(t, "replace snapshots", func() error { return f.s.ReplaceRuntimeSnapshots(f.run, []model.WorkerStatus{status}) })
	}
	f.recover(t)
	if err := f.s.ReplaceRuntimeSnapshots(f.run, []model.WorkerStatus{{RuntimeKey: "recovered"}}); err != nil { t.Fatalf("recovery: %v", err) }
}

func TestWALRunMetadataRespectsAdmissionThreshold(t *testing.T) {
	f := newMutationWALFixture(t)
	for i := 0; i < 3; i++ {
		f.attempt(t, "stop run", func() error { return f.s.StopRun(f.run, time.Unix(int64(10+i), 0)) })
		f.attempt(t, "start run", func() error { _, err := f.s.StartRun(model.DaemonState{PID: 100+i, StartedAt: time.Unix(int64(20+i), 0)}); return err })
	}
	f.recover(t)
	if _, err := f.s.StartRun(model.DaemonState{PID: 200, StartedAt: time.Unix(30, 0)}); err != nil { t.Fatalf("recovery: %v", err) }
}

func TestWALRetentionRespectsAdmissionThreshold(t *testing.T) {
	f := newMutationWALFixture(t)
	cutoff := time.Unix(100, 0)
	f.attempt(t, "purge events", func() error { _, err := f.s.PurgeOldEvents(cutoff); return err })
	f.attempt(t, "purge runs", func() error { _, err := f.s.PurgeOldRuns(cutoff); return err })
	f.attempt(t, "purge and vacuum", func() error { _, _, err := f.s.PurgeAndVacuum(1, 1<<62); return err })
	f.recover(t)
	if _, err := f.s.PurgeOldEvents(cutoff); err != nil { t.Fatalf("recovery: %v", err) }
}

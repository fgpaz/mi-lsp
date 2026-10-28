package daemon

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	_ "modernc.org/sqlite"
)

func orphanTelemetryStore(t *testing.T) (*TelemetryStore, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := sql.Open("sqlite", path)
	if err != nil { t.Fatal(err) }
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s := &TelemetryStore{db: db}
	if err := s.configureConnection(); err != nil { t.Fatal(err) }
	if err := s.initSchema(); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO access_events(operation, success, occurred_at) VALUES ('orphan-fixture', 1, 1)`); err != nil { t.Fatal(err) }
	return s, db, path
}

func TestOrphanCheckpointIdleWAL(t *testing.T) {
	s, db, _ := orphanTelemetryStore(t)
	if _, err := db.Exec(`INSERT INTO access_events(operation, success, occurred_at) VALUES ('orphan-fixture', 1, 2)`); err != nil { t.Fatal(err) }
	if err := s.Checkpoint(); err != nil { t.Fatalf("idle WAL checkpoint should succeed: %v", err) }
}

func TestOrphanCheckpointPinnedReaderReportsBusy(t *testing.T) {
	s, db, path := orphanTelemetryStore(t)
	reader, err := sql.Open("sqlite", path)
	if err != nil { t.Fatal(err) }
	defer reader.Close()
	reader.SetMaxOpenConns(1)
	tx, err := reader.Begin()
	if err != nil { t.Fatal(err) }
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM access_events`).Scan(&count); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO access_events(operation, success, occurred_at) VALUES ('orphan-fixture', 1, 3)`); err != nil { t.Fatal(err) }
	if err := s.Checkpoint(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "busy") {
		t.Fatalf("checkpoint with pinned reader must report busy, got %v", err)
	}
}

func TestOrphanWALHealthExposesBoundAndCounters(t *testing.T) {
	s, _, _ := orphanTelemetryStore(t)
	s.walMaxBytes = 12345
	s.reject("wal_limit")
	h := s.WALHealth()
	if h.WALMaxBytes != 12345 || h.RejectedWrites != 1 || h.LastFailure != "wal_limit" { t.Fatalf("unexpected WAL health: %+v", h) }
}

func TestOrphanRecordAccessRejectsAfterClose(t *testing.T) {
	s, _, _ := orphanTelemetryStore(t)
	if err := s.Close(); err != nil { t.Fatal(err) }
	if err := s.RecordAccess(1, model.AccessEvent{}); err == nil { t.Fatal("write after close must be rejected") }
	if !s.WALHealth().Closed { t.Fatal("closed state must remain visible") }
}

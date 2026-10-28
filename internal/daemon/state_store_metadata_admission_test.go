package daemon

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func newMetadataAdmissionFixture(t *testing.T) *TelemetryStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "mi-lsp-meta-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &TelemetryStore{dbPath: path, db: db, walMaxBytes: telemetryWALMaxBytes}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return s
}

func TestMetadataMutationsSerializeAcrossStores(t *testing.T) {
	first := newMetadataAdmissionFixture(t)
	second := newMetadataAdmissionFixture(t)
	// Both handles intentionally share the same actual SQLite file.
	_ = second.db.Close()
	second.db, _ = sql.Open("sqlite", first.dbPath)
	second.db.SetMaxOpenConns(1)
	second.db.SetMaxIdleConns(1)

	stores := []*TelemetryStore{first, second}
	var wg sync.WaitGroup
	errs := make(chan error, len(stores))
	for i, store := range stores {
		wg.Add(1)
		go func(i int, store *TelemetryStore) {
			defer wg.Done()
			run, err := store.StartRun(model.DaemonState{PID: i + 1, StartedAt: time.Now()})
			if err == nil {
				err = store.ReplaceRuntimeSnapshots(run, []model.WorkerStatus{{RuntimeKey: "runtime"}})
			}
			if err == nil {
				err = store.stopRunContext(context.Background(), run, time.Now())
			}
			errs <- err
		}(i, store)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMetadataMutationHonorsCancellationAndRecovers(t *testing.T) {
	s := newMetadataAdmissionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.stopRunContext(ctx, 1, time.Now()); err == nil {
		t.Fatal("canceled metadata mutation unexpectedly succeeded")
	}
	run, err := s.StartRun(model.DaemonState{PID: 1, StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("mutation after canceled admission: %v", err)
	}
	if err := s.ReplaceRuntimeSnapshots(run, []model.WorkerStatus{{RuntimeKey: "recovered"}}); err != nil {
		t.Fatalf("snapshot after canceled admission: %v", err)
	}
}

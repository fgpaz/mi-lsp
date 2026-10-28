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

	_ "modernc.org/sqlite"
)

func newMutationGuardStore(t *testing.T, path string) *TelemetryStore {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open private sqlite fixture: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &TelemetryStore{dbPath: path, db: db, walMaxBytes: telemetryWALMaxBytes}
	if err := s.configureConnection(); err != nil {
		_ = db.Close()
		t.Fatalf("configure fixture: %v", err)
	}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		t.Fatalf("initialize fixture schema: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ownership_probe (value TEXT NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatalf("create fixture table: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close fixture store: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.WaitClosed(ctx); err != nil {
			t.Errorf("join fixture close: %v", err)
		}
	})
	return s
}

func TestTelemetryOwnershipCrossStoreCancellationPhysicalReturnAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	first := newMutationGuardStore(t, path)
	second := newMutationGuardStore(t, path)

	blockerDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open SQLite writer blocker: %v", err)
	}
	defer blockerDB.Close()
	blockerDB.SetMaxOpenConns(1)
	blockerTx, err := blockerDB.Begin()
	if err != nil {
		t.Fatalf("begin SQLite writer blocker: %v", err)
	}
	defer blockerTx.Rollback()
	if _, err := blockerTx.Exec(`INSERT INTO ownership_probe(value) VALUES ('blocker')`); err != nil {
		t.Fatalf("acquire SQLite write lock: %v", err)
	}
	if _, err := first.db.Exec(`PRAGMA busy_timeout=2000`); err != nil {
		t.Fatalf("set bounded driver wait: %v", err)
	}
	entered := make(chan struct{})
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	ownerDone := make(chan error, 1)
	ownerFinished := make(chan struct{})
	go func() {
		defer close(ownerFinished)
		ownerDone <- first.withTelemetryMutationContext(ownerCtx, func(context.Context) error {
			close(entered)
			// database/sql Exec (without a context) blocks in the real SQLite
			// driver behind the private writer transaction until it is released.
			_, err := first.db.Exec(`INSERT INTO ownership_probe(value) VALUES ('owner')`)
			return err
		})
	}()
	var competitorFinished chan struct{}
	defer func() {
		_ = blockerTx.Rollback()
		select {
		case <-ownerFinished:
		case <-time.After(3 * time.Second):
			t.Errorf("owner goroutine did not physically join during cleanup")
		}
		if competitorFinished != nil {
			select {
			case <-competitorFinished:
			case <-time.After(3 * time.Second):
				t.Errorf("competitor goroutine did not join during cleanup")
			}
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first owner did not enter its real mutation callback")
	}

	competingCtx, cancelCompeting := context.WithCancel(context.Background())
	competitorDone := make(chan error, 1)
	competitorStarted := make(chan struct{})
	competitorFinished = make(chan struct{})
	go func() {
		defer close(competitorFinished)
		close(competitorStarted)
		competitorDone <- second.withTelemetryMutationContext(competingCtx, func(ctx context.Context) error {
			_, err := second.db.ExecContext(ctx, `INSERT INTO ownership_probe(value) VALUES ('late')`)
			return err
		})
	}()
	select {
	case <-competitorStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("competing owner did not start")
	}
	time.Sleep(50 * time.Millisecond)
	cancelOwner()
	cancelCompeting()
	cancelOwner()
	cancelCompeting()

	select {
	case err := <-competitorDone:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("competing owner must report context cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("competing owner did not stop at its canceled acquisition")
	}
	select {
	case err := <-ownerDone:
		t.Fatalf("physical owner returned before SQLite released its write lock: %v", err)
	default:
	}

	if err := blockerTx.Rollback(); err != nil {
		t.Fatalf("release SQLite writer blocker: %v", err)
	}
	select {
	case err := <-ownerDone:
		if err != nil {
			t.Fatalf("owner's physical SQL after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owner did not join after its physical callback returned")
	}

	var count int
	if err := first.db.QueryRow(`SELECT count(*) FROM ownership_probe`).Scan(&count); err != nil {
		t.Fatalf("query recovery count: %v", err)
	}
	if count != 1 {
		t.Fatalf("canceled competitor wrote late SQL: rows=%d, want only owner row", count)
	}
	if err := second.withTelemetryMutationContext(context.Background(), func(ctx context.Context) error {
		_, err := second.db.ExecContext(ctx, `INSERT INTO ownership_probe(value) VALUES ('recovered')`)
		return err
	}); err != nil {
		t.Fatalf("cross-store ownership recovery: %v", err)
	}
	if err := first.withTelemetryMutationContext(context.Background(), func(context.Context) error {
		return errors.New("injected callback failure")
	}); err == nil {
		t.Fatal("injected callback failure was lost")
	}
	if err := first.withTelemetryMutationContext(context.Background(), func(ctx context.Context) error {
		_, err := first.db.ExecContext(ctx, `INSERT INTO ownership_probe(value) VALUES ('after-failure')`)
		return err
	}); err != nil {
		t.Fatalf("ownership gate not released after callback error: %v", err)
	}
}

func TestTelemetryAdmissionRefusalCountedOnceAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission.db")
	s := newMutationGuardStore(t, path)
	readerDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open pinned reader: %v", err)
	}
	defer readerDB.Close()
	readerDB.SetMaxOpenConns(1)
	reader, err := readerDB.Begin()
	if err != nil {
		t.Fatalf("begin pinned reader: %v", err)
	}
	defer reader.Rollback()
	var count int
	if err := reader.QueryRow(`SELECT count(*) FROM ownership_probe`).Scan(&count); err != nil {
		t.Fatalf("pin WAL reader: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO ownership_probe(value) VALUES ('pinned')`); err != nil {
		t.Fatalf("grow WAL under pin: %v", err)
	}
	wal, err := s.walSize()
	if err != nil || wal == 0 {
		t.Fatalf("measure pinned WAL: bytes=%d err=%v", wal, err)
	}
	s.walMaxBytes = wal - 1
	before := s.WALHealth().RejectedWrites
	err = s.withTelemetryMutationContext(context.Background(), func(ctx context.Context) error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO ownership_probe(value) VALUES ('must-not-write')`)
		return err
	})
	if err == nil {
		t.Fatal("mutation above threshold unexpectedly succeeded")
	}
	if got := walAdmissionReason(err); got != "busy" && got != "wal_limit" {
		t.Fatalf("admission refusal has unexpected class %q: %v", got, err)
	}
	if got := s.WALHealth().RejectedWrites - before; got != 1 {
		t.Fatalf("admission refusal count=%d, want exactly 1 (err=%v)", got, err)
	}
	if err := reader.Rollback(); err != nil {
		t.Fatalf("release reader pin: %v", err)
	}
	if err := s.Checkpoint(); err != nil {
		t.Fatalf("checkpoint after releasing pin: %v", err)
	}
	s.walMaxBytes = telemetryWALMaxBytes
	if err := s.withTelemetryMutationContext(context.Background(), func(ctx context.Context) error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO ownership_probe(value) VALUES ('recovered')`)
		return err
	}); err != nil {
		t.Fatalf("mutation after admission recovery: %v", err)
	}
}

func TestTelemetryCanonicalPathSharesExistingAndBootstrapAliases(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "physical")
	aliasDir := filepath.Join(root, "alias")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable on this platform: %v", err)
	}
	target := filepath.Join(targetDir, "store.db")
	if err := os.WriteFile(target, []byte("private fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(aliasDir, "store.db")
	canonicalTarget, err := canonicalTelemetryDatabasePath(target)
	if err != nil {
		t.Fatalf("canonical existing target: %v", err)
	}
	canonicalAlias, err := canonicalTelemetryDatabasePath(alias)
	if err != nil {
		t.Fatalf("canonical existing alias: %v", err)
	}
	if canonicalAlias != canonicalTarget {
		t.Fatalf("existing aliases diverged: target=%q alias=%q", canonicalTarget, canonicalAlias)
	}
	missingTarget := filepath.Join(targetDir, "new.db")
	missingAlias := filepath.Join(aliasDir, "new.db")
	canonicalMissingTarget, err := canonicalTelemetryDatabasePath(missingTarget)
	if err != nil {
		t.Fatalf("canonical bootstrap target: %v", err)
	}
	canonicalMissingAlias, err := canonicalTelemetryDatabasePath(missingAlias)
	if err != nil {
		t.Fatalf("canonical bootstrap alias: %v", err)
	}
	if canonicalMissingAlias != canonicalMissingTarget {
		t.Fatalf("bootstrap aliases diverged: target=%q alias=%q", canonicalMissingTarget, canonicalMissingAlias)
	}
}

func TestTelemetryOwnershipGateConcurrentInitialization(t *testing.T) {
	s := &TelemetryStore{}
	const callers = 32
	var wg sync.WaitGroup
	gates := make(chan chan struct{}, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gates <- s.telemetryOwnershipGate()
		}()
	}
	wg.Wait()
	close(gates)
	var shared chan struct{}
	for gate := range gates {
		if shared == nil {
			shared = gate
		} else if shared != gate {
			t.Fatal("concurrent ownership initialization produced distinct gates")
		}
	}
}

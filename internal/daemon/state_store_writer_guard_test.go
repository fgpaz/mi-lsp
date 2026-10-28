package daemon

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openWriterGuardTestStore(t *testing.T, dsn, dbPath string) *TelemetryStore {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return &TelemetryStore{db: db, dbPath: dbPath}
}

func TestTelemetryWriterGuardTwoStoresCancelOwnedCallbackAndRecover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	first := openWriterGuardTestStore(t, path, path)
	second := openWriterGuardTestStore(t, path, path)
	if _, err := first.db.Exec("PRAGMA journal_mode=WAL"); err != nil { t.Fatal(err) }
	if _, err := first.db.Exec(`CREATE TABLE guarded(value INTEGER NOT NULL)`); err != nil { t.Fatal(err) }

	entered := make(chan struct{})
	finish := make(chan struct{})
	firstDone := make(chan error, 1)
	finishOnce := make(chan struct{})
	closeFinish := func() { select { case <-finishOnce: default: close(finishOnce); close(finish) } }
	joined := false
	defer func() {
		closeFinish()
		if !joined {
			select {
			case <-firstDone:
			case <-time.After(3 * time.Second): t.Error("first writer callback goroutine did not join")
			}
		}
	}()
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	go func() {
		firstDone <- first.withTelemetryWriterLeaseContext(firstCtx, func(ctx context.Context) error {
			close(entered)
			<-finish
			_, err := first.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (1)`)
			return err
		})
	}()
	select {
	case <-entered:
	case err := <-firstDone:
		joined = true
		t.Fatalf("first callback exited before barrier: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("first callback did not reach barrier")
	}
	cancelFirst()

	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelSecond()
	secondEntered := false
	secondErr := second.withTelemetryWriterLeaseContext(secondCtx, func(ctx context.Context) error {
		secondEntered = true
		_, err := second.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (2)`)
		return err
	})
	if !errors.Is(secondErr, context.DeadlineExceeded) || secondEntered {
		t.Fatalf("competing store did not time out before callback: err=%v entered=%v", secondErr, secondEntered)
	}
	select {
	case err := <-firstDone:
		joined = true
		t.Fatalf("cancel released lease before callback returned: %v", err)
	default:
	}
	closeFinish()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		joined = true
		t.Fatalf("first callback result = %v, want canceled", err)
	}
	joined = true

	if err := second.withTelemetryWriterLeaseContext(context.Background(), func(ctx context.Context) error {
		_, err := second.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (3)`)
		return err
	}); err != nil { t.Fatalf("second store recovery write: %v", err) }
	var count int
	if err := second.db.QueryRow(`SELECT count(*) FROM guarded`).Scan(&count); err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("persisted rows = %d, want recovery write only", count) }
}

func TestTelemetryWriterGuardDatabaseSymlinkAliasSharesGuard(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.db")
	alias := filepath.Join(dir, "alias.db")
	owner := openWriterGuardTestStore(t, target, target)
	if _, err := owner.db.Exec(`CREATE TABLE guarded(value INTEGER NOT NULL)`); err != nil { t.Fatal(err) }
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("filesystem does not permit symlink fixture: %v", err)
	}
	contender := openWriterGuardTestStore(t, alias, alias)
	entered := make(chan struct{})
	finish := make(chan struct{})
	ownerDone := make(chan error, 1)
	finishOnce := make(chan struct{})
	closeFinish := func() { select { case <-finishOnce: default: close(finishOnce); close(finish) } }
	joined := false
	defer func() {
		closeFinish()
		if !joined {
			select { case <-ownerDone: case <-time.After(3 * time.Second): t.Error("symlink owner callback did not join") }
		}
	}()
	go func() {
		ownerDone <- owner.withTelemetryWriterLeaseContext(context.Background(), func(ctx context.Context) error {
			close(entered)
			<-finish
			_, err := owner.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (1)`)
			return err
		})
	}()
	select {
	case <-entered:
	case err := <-ownerDone:
		joined = true
		t.Fatalf("owner callback exited before barrier: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("owner callback did not reach barrier")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	called := false
	err := contender.withTelemetryWriterLeaseContext(ctx, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("symlink alias did not contend on target guard: err=%v callback=%v", err, called)
	}
	closeFinish()
	if err := <-ownerDone; err != nil { joined = true; t.Fatalf("owner callback: %v", err) }
	joined = true
	if err := contender.withTelemetryWriterLeaseContext(context.Background(), func(ctx context.Context) error {
		_, err := contender.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (2)`)
		return err
	}); err != nil { t.Fatalf("alias recovery write: %v", err) }
	var count int
	if err := owner.db.QueryRow(`SELECT count(*) FROM guarded`).Scan(&count); err != nil { t.Fatal(err) }
	if count != 2 { t.Fatalf("rows through aliased DB = %d, want 2", count) }
}

func TestTelemetryWriterGuardRejectsInMemoryURIAndAmbiguousWhitespace(t *testing.T) {
	cases := []struct { name, dsn, path string }{
		{name: "explicit memory", dsn: ":memory:", path: ":memory:"},
		{name: "memory URI", dsn: "file:guard-b1-memory?mode=memory&cache=shared", path: "file:guard-b1-memory?mode=memory&cache=shared"},
		{name: "surrounding whitespace", dsn: "file:whitespace.db", path: "  " + filepath.Join(t.TempDir(), "whitespace.db") + " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := openWriterGuardTestStore(t, tc.dsn, tc.path)
			called := false
			err := store.withTelemetryWriterLeaseContext(context.Background(), func(context.Context) error { called = true; return nil })
			if err == nil || called { t.Fatalf("invalid DB identity accepted: err=%v callback=%v", err, called) }
			if strings.Contains(err.Error(), "release telemetry writer lease") { t.Fatalf("unexpected lease acquired for invalid identity: %v", err) }
		})
	}
	unknown := openWriterGuardTestStore(t, ":memory:", "")
	called := false
	if err := unknown.withTelemetryWriterLeaseContext(context.Background(), func(context.Context) error { called = true; return nil }); err == nil || called {
		t.Fatalf("unknown in-memory main path accepted: callback=%v", called)
	}
}

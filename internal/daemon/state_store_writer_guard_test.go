package daemon

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestTelemetryWriterGuardTwoStoresCancelOwnedCallbackAndRecover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	open := func() *TelemetryStore {
		db, err := sql.Open("sqlite", path)
		if err != nil { t.Fatal(err) }
		db.SetMaxOpenConns(1)
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil { t.Fatal(err) }
		return &TelemetryStore{db: db, dbPath: path}
	}
	first, second := open(), open()
	t.Cleanup(func() { _ = first.db.Close(); _ = second.db.Close() })
	if _, err := first.db.Exec(`CREATE TABLE guarded(value INTEGER NOT NULL)`); err != nil { t.Fatal(err) }

	entered := make(chan struct{})
	finish := make(chan struct{})
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- first.withTelemetryWriterLeaseContext(firstCtx, func(ctx context.Context) error {
			close(entered)
			<-finish
			_, err := first.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (1)`)
			return err
		})
	}()
	<-entered
	cancelFirst()

	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelSecond()
	secondEntered := false
	secondErr := second.withTelemetryWriterLeaseContext(secondCtx, func(ctx context.Context) error {
		secondEntered = true
		_, err := second.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (2)`)
		return err
	})
	if !errors.Is(secondErr, context.DeadlineExceeded) || secondEntered {
		close(finish)
		t.Fatalf("competing store did not time out before callback: err=%v entered=%v", secondErr, secondEntered)
	}
	select {
	case err := <-firstDone:
		close(finish)
		t.Fatalf("cancel released lease before callback returned: %v", err)
	default:
	}
	close(finish)
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first callback result = %v, want canceled", err)
	}

	if err := second.withTelemetryWriterLeaseContext(context.Background(), func(ctx context.Context) error {
		_, err := second.db.ExecContext(ctx, `INSERT INTO guarded(value) VALUES (3)`)
		return err
	}); err != nil { t.Fatalf("second store recovery write: %v", err) }
	var count int
	if err := second.db.QueryRow(`SELECT count(*) FROM guarded`).Scan(&count); err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("persisted rows = %d, want recovery write only", count) }
}

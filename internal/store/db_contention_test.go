package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Every connection a workspace DB pool opens must wait on locks: the read pool
// holds up to eight connections, and a db.Exec pragma reaches only one of them.
func TestWorkspaceDBEveryPoolConnectionHasBusyTimeout(t *testing.T) {
	root := t.TempDir()
	writer, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	path := WorkspaceDBPath(root)
	opens := []struct {
		name string
		open func() (*sql.DB, error)
	}{
		{"Open", func() (*sql.DB, error) { return writer, nil }},
		{"OpenReadOnly", func() (*sql.DB, error) { return OpenReadOnly(root) }},
		{"OpenExistingForScopedRefresh", func() (*sql.DB, error) { return OpenExistingForScopedRefresh(root) }},
		{"OpenReadOnlyExistingWithWAL", func() (*sql.DB, error) { return OpenReadOnlyExistingWithWAL(root, path) }},
	}
	for _, tc := range opens {
		db, err := tc.open()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		ctx := context.Background()
		var conns []*sql.Conn
		without := 0
		for i := 0; i < 8; i++ {
			// Hold each connection so the pool opens a new one, up to its limit.
			cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			conn, err := db.Conn(cctx)
			cancel()
			if err != nil {
				break
			}
			conns = append(conns, conn)
			var timeout int
			if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
				t.Fatalf("%s connection %d: %v", tc.name, i, err)
			}
			if timeout != 5000 {
				without++
			}
		}
		for _, conn := range conns {
			_ = conn.Close()
		}
		if len(conns) == 0 {
			t.Errorf("%s opened no connection", tc.name)
		}
		if without > 0 {
			t.Errorf("%s: %d of %d pool connections without busy_timeout=5000", tc.name, without, len(conns))
		} else {
			t.Logf("%s: %d pool connections, all busy_timeout=5000", tc.name, len(conns))
		}
		if tc.name != "Open" {
			_ = db.Close()
		}
	}
	// The other pragmas keep their final values.
	for pragma, want := range map[string]string{
		"journal_mode": "wal", "synchronous": "1", "foreign_keys": "1",
		"cache_size": "-40000", "mmap_size": "30000000",
	} {
		var got string
		if err := writer.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil || got != want {
			t.Errorf("PRAGMA %s = %q (%v), want %s", pragma, got, err, want)
		}
	}
}

type firstQueryCounts struct {
	opens, openBusy, openOther, queries, queryBusy, queryOther atomic.Int64
	samples                                                    sync.Map
}

func (c *firstQueryCounts) record(open bool, err error) {
	switch {
	case open && err == nil:
		c.opens.Add(1)
	case err == nil:
		c.queries.Add(1)
	case open && IsLockedError(err):
		c.openBusy.Add(1)
	case open:
		c.openOther.Add(1)
	case IsLockedError(err):
		c.queryBusy.Add(1)
	default:
		c.queryOther.Add(1)
	}
	if err != nil {
		c.samples.LoadOrStore(fmt.Sprintf("open=%v busy=%v", open, IsLockedError(err)), err.Error())
	}
}

func (c *firstQueryCounts) failures() int64 {
	return c.openBusy.Load() + c.openOther.Load() + c.queryBusy.Load() + c.queryOther.Load()
}

func (c *firstQueryCounts) String() string {
	return fmt.Sprintf("opens=%d open_busy=%d open_other=%d queries=%d query_busy=%d query_other=%d",
		c.opens.Load(), c.openBusy.Load(), c.openOther.Load(), c.queries.Load(), c.queryBusy.Load(), c.queryOther.Load())
}

// A first query creates the workspace DB (index.start), then closes its writer
// and answers from a read-only pool with parallel queries while the detached
// index job opens the same DB for writing. Closing the last connection
// checkpoints and resets the WAL under an exclusive lock, and the next opener
// rebuilds the WAL index: a connection without busy_timeout fails at once with
// SQLITE_BUSY (in the product, workspace_db_open_failed in the index job). The
// race is not deterministic, so the test runs many rounds under a time cap.
func TestWorkspaceDBJobOpenWaitsForFirstQuery(t *testing.T) {
	const rounds = 150
	deadline := time.Now().Add(45 * time.Second)
	counts := &firstQueryCounts{}
	base := t.TempDir()
	ran := 0
	for ; ran < rounds && time.Now().Before(deadline); ran++ {
		root := filepath.Join(base, fmt.Sprint(ran))
		start, err := Open(root)
		if err != nil {
			t.Fatalf("round %d: index.start open: %v", ran, err)
		}
		if err := UpsertWorkspaceMeta(context.Background(), start, "index_job", "queued"); err != nil {
			t.Fatalf("round %d: index.start write: %v", ran, err)
		}
		var job sync.WaitGroup
		job.Add(1)
		go func() {
			defer job.Done()
			db, err := Open(root)
			counts.record(true, err)
			if err != nil {
				return
			}
			counts.record(false, UpsertWorkspaceMeta(context.Background(), db, "index_job", "running"))
			_ = db.Close()
		}()
		_ = start.Close()
		reader, err := OpenReadOnly(root)
		counts.record(true, err)
		if err == nil {
			var queries sync.WaitGroup
			for c := 0; c < 4; c++ {
				queries.Add(1)
				go func() {
					defer queries.Done()
					var n int
					counts.record(false, reader.QueryRow("SELECT count(*) FROM workspace_meta").Scan(&n))
				}()
			}
			queries.Wait()
			_ = reader.Close()
		}
		job.Wait()
	}
	t.Logf("rounds=%d %s", ran, counts)
	counts.samples.Range(func(k, v any) bool { t.Logf("%v: %v", k, v); return true })
	if ran < 20 {
		t.Fatalf("only %d rounds ran before the time cap", ran)
	}
	if counts.failures() != 0 {
		t.Fatalf("first-query contention failed: %s", counts)
	}
}

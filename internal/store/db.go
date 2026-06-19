package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

const (
	sqliteRetryAttempts = 4
	sqliteRetryDelay    = 25 * time.Millisecond
)

func WorkspaceDBPath(root string) string {
	return filepath.Join(root, ".mi-lsp", "index.db")
}

func Open(root string) (*sql.DB, error) {
	stateDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, WorkspaceDBPath(root))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := configureWorkspaceDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := EnsureSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func WithSQLiteReadRetry(ctx context.Context, fn func() error) error {
	var err error
	delay := sqliteRetryDelay
	for attempt := 0; attempt < sqliteRetryAttempts; attempt++ {
		err = fn()
		if !IsLockedError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return err
}

func QueryContextWithRetry(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	err := WithSQLiteReadRetry(ctx, func() error {
		var queryErr error
		rows, queryErr = db.QueryContext(ctx, query, args...)
		return queryErr
	})
	return rows, err
}

func Reset(root string) error {
	if err := os.Remove(WorkspaceDBPath(root)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func configureWorkspaceDB(db *sql.DB) error {
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-40000",  // ~40MB cache
		"PRAGMA mmap_size=30000000", // 30MB memory-mapped I/O
	} {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}

// PublishIndexPragmaOptimize runs PRAGMA optimize after publishing the index.
// Called by L1/L2 after index publication to compact and optimize the database.
func PublishIndexPragmaOptimize(db *sql.DB) error {
	// Best-effort and bounded: PRAGMA optimize refreshes query-planner statistics but
	// must not block the publish path under lock contention. Callers ignore the error.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, "PRAGMA optimize")
	return err
}

func IsCorruptionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "database disk image is malformed") ||
		strings.Contains(message, "database or disk is full") ||
		strings.Contains(message, "file is not a database") ||
		strings.Contains(message, "malformed")
}

func IsLockedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is busy") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked") ||
		strings.Contains(message, "sql logic error: database is locked")
}

func QuarantineCorruptDB(root string) (string, error) {
	source := WorkspaceDBPath(root)
	if _, err := os.Stat(source); err != nil {
		return "", err
	}
	target := fmt.Sprintf("%s.corrupt-%s", source, time.Now().UTC().Format("20060102T150405Z"))
	if err := os.Rename(source, target); err != nil {
		return "", err
	}
	return target, nil
}

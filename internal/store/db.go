package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

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
	} {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
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

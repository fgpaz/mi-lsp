package store

import (
	"database/sql"
	"os"
	"path/filepath"

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

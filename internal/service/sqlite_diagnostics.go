package service

import (
	"database/sql"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// openWorkspaceDB opens a database connection for the given workspace.
// If readOnly is true, uses OpenReadOnly for concurrent read access.
// Otherwise, uses Open for write access (serialized).
func openWorkspaceDB(registration model.WorkspaceRegistration, operation string, readOnly bool) (*sql.DB, error) {
	var db *sql.DB
	var err error
	if readOnly {
		db, err = store.OpenReadOnly(registration.Root)
	} else {
		db, err = store.Open(registration.Root)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite open failed: operation=%s workspace=%s root=%s db_path=%s mode=%s: %w",
			operation,
			registration.Name,
			registration.Root,
			store.WorkspaceDBPath(registration.Root),
			map[bool]string{true: "readonly", false: "readwrite"}[readOnly],
			err,
		)
	}
	return db, nil
}

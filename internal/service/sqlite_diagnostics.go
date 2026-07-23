package service

import (
	"database/sql"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const workspaceDBOpenErrorCode = "workspace_db_open_failed"

type workspaceDBOpenError struct {
	cause error
}

func (e *workspaceDBOpenError) Error() string {
	return workspaceDBOpenErrorCode
}

func (e *workspaceDBOpenError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

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
		// Keep the underlying cause for local classification, but never expose
		// workspace roots, database paths, or driver text through Error().
		return nil, &workspaceDBOpenError{cause: err}
	}
	return db, nil
}

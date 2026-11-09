package service

import (
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const workspaceDBOpenErrorCode = "workspace_db_open_failed"

// errWorktreeIndexAbsent is returned when a read-only open would have to create
// an index inside a linked worktree. Worktrees never get an index of their own.
var errWorktreeIndexAbsent = errors.New("worktree_index_absent")

// refuseWorktreeIndexCreation stops a read-only open from creating
// .mi-lsp/index.db inside a linked worktree that has none. An existing index is
// left alone, and ordinary roots are unaffected.
func refuseWorktreeIndexCreation(root string) error {
	if _, err := os.Stat(store.WorkspaceDBPath(root)); err == nil {
		return nil
	}
	linked, err := workspace.IsLinkedWorktree(root)
	if err != nil {
		return err
	}
	if linked {
		return errWorktreeIndexAbsent
	}
	return nil
}

type workspaceDBOpenError struct {
	cause error
	root  string
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
	started := time.Now()
	defer traceServiceTiming("db_open", started)
	var db *sql.DB
	var err error
	if readOnly {
		if err := refuseWorktreeIndexCreation(registration.Root); err != nil {
			return nil, err
		}
		db, err = store.OpenReadOnly(registration.Root)
	} else {
		db, err = store.Open(registration.Root)
	}
	if err != nil {
		// Keep the underlying cause for local classification, but never expose
		// workspace roots, database paths, or driver text through Error().
		return nil, &workspaceDBOpenError{cause: err, root: registration.Root}
	}
	return db, nil
}

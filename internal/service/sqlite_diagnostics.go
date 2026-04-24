package service

import (
	"database/sql"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func openWorkspaceDB(registration model.WorkspaceRegistration, operation string) (*sql.DB, error) {
	db, err := store.Open(registration.Root)
	if err != nil {
		return nil, fmt.Errorf("sqlite open failed: operation=%s workspace=%s root=%s db_path=%s: %w",
			operation,
			registration.Name,
			registration.Root,
			store.WorkspaceDBPath(registration.Root),
			err,
		)
	}
	return db, nil
}

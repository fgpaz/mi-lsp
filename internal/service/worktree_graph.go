package service

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// openNavigationGraphDB never creates an index for a linked worktree. Graph
// queries may use an explicitly indexed physical worktree, but never its base
// checkout; without a local DB callers must degrade or report unavailable.
func openNavigationGraphDB(registration model.WorkspaceRegistration, operation string) (*sql.DB, bool, error) {
	gitMarker := filepath.Join(registration.Root, ".git")
	gitInfo, statErr := os.Stat(gitMarker)
	if statErr != nil || !gitInfo.Mode().IsRegular() {
		db, err := openWorkspaceDB(registration, operation, true)
		return db, false, err
	}
	overlay, linked, err := workspace.TryResolveWorktreeOverlay(registration.Root)
	if err != nil {
		return nil, false, err
	}
	if !linked {
		db, err := openWorkspaceDB(registration, operation, true)
		return db, false, err
	}

	physicalRoot := overlay.Root
	stateDir := filepath.Join(physicalRoot, ".mi-lsp")
	stateInfo, err := os.Lstat(stateDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, true, nil
		}
		return nil, true, err
	}
	if stateInfo.Mode()&os.ModeSymlink != 0 || !stateInfo.IsDir() {
		return nil, true, errors.New("worktree-local graph state directory is not physical")
	}

	path := store.WorkspaceDBPath(physicalRoot)
	dbInfo, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, true, nil
		}
		return nil, true, err
	}
	if dbInfo.Mode()&os.ModeSymlink != 0 || !dbInfo.Mode().IsRegular() {
		return nil, true, errors.New("worktree-local graph database is not a regular physical file")
	}
	basePath := store.WorkspaceDBPath(overlay.BaseRoot)
	baseInfo, baseErr := os.Stat(basePath)
	if baseErr == nil {
		worktreeInfo, statErr := os.Stat(path)
		if statErr != nil {
			return nil, true, statErr
		}
		if os.SameFile(baseInfo, worktreeInfo) {
			return nil, true, errors.New("worktree-local graph database aliases the base checkout database")
		}
	} else if !errors.Is(baseErr, os.ErrNotExist) {
		return nil, true, baseErr
	}
	db, err := store.OpenReadOnlyExistingWithWAL(physicalRoot, path)
	return db, true, err
}

func worktreeGraphUnavailable() *model.GraphQueryError {
	return &model.GraphQueryError{
		Code:    "GPH_QUERY_BACKEND_UNAVAILABLE",
		Message: "worktree-local graph index is unavailable; the base worktree graph is not reused",
	}
}

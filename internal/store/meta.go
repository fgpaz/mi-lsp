package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docidentity"
)

type metaExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type metaQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const WorkspaceMetaDocIdentitySnapshotVersion = "doc_identity_snapshot_version"

func stampDocIdentitySnapshotTx(ctx context.Context, tx *sql.Tx) error {
	return UpsertWorkspaceMeta(ctx, tx, WorkspaceMetaDocIdentitySnapshotVersion, docidentity.ExtractionVersion)
}

func invalidateDocIdentitySnapshotTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM workspace_meta WHERE key = ?", WorkspaceMetaDocIdentitySnapshotVersion)
	return err
}

// DocIdentitySnapshotCurrent reports whether persisted document mentions were
// produced by the current extractor grammar. Database errors are returned,
// rather than being treated as an unversioned snapshot.
func DocIdentitySnapshotCurrent(ctx context.Context, db *sql.DB) (bool, error) {
	value, ok, err := WorkspaceMetaValue(ctx, db, WorkspaceMetaDocIdentitySnapshotVersion)
	if err != nil {
		return false, err
	}
	return ok && value == docidentity.ExtractionVersion, nil
}

func UpsertWorkspaceMeta(ctx context.Context, exec metaExecutor, key string, value string) error {
	_, err := exec.ExecContext(ctx, "INSERT OR REPLACE INTO workspace_meta(key, value) VALUES(?, ?)", key, value)
	return err
}

func UpsertWorkspaceMetaMap(ctx context.Context, exec metaExecutor, metadata map[string]string) error {
	for key, value := range metadata {
		if err := UpsertWorkspaceMeta(ctx, exec, key, value); err != nil {
			return err
		}
	}
	return nil
}

// ReadWorkspaceGenerationSnapshot reads active generation metadata without creating
// the workspace database or changing its state. It is safe to call for an absent DB.
func ReadWorkspaceGenerationSnapshot(ctx context.Context, root string) (string, error) {
	path := WorkspaceDBPath(root)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "none", nil
		}
		return "unavailable", err
	}
	db, err := sql.Open(driverName, path+"?_pragma=query_only(ON)&_pragma=foreign_keys(ON)")
	if err != nil {
		return "unavailable", err
	}
	defer db.Close()
	keys := []string{WorkspaceMetaLastIndexGeneration, WorkspaceMetaActiveCatalogGeneration, WorkspaceMetaActiveDocsGeneration, WorkspaceMetaActiveMemoryGeneration}
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok, err := WorkspaceMetaValue(ctx, db, key)
		if err != nil {
			return "unavailable", err
		}
		if ok && strings.TrimSpace(value) != "" {
			values = append(values, key+"="+value)
		}
	}
	if len(values) == 0 {
		return "none", nil
	}
	return strings.Join(values, "\x00"), nil
}

func WorkspaceMetaValue(ctx context.Context, db *sql.DB, key string) (string, bool, error) {
	return workspaceMetaValueConn(ctx, db, key)
}

func workspaceMetaValueConn(ctx context.Context, q metaQueryer, key string) (string, bool, error) {
	var value sql.NullString
	if err := q.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key = ?", key).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	if !value.Valid {
		return "", true, nil
	}
	return value.String, true, nil
}

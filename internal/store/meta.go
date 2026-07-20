package store

import (
	"context"
	"database/sql"
)

type metaExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type metaQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
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

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	workspaceMetaReentrySnapshotJSON    = "memory_snapshot_json"
	workspaceMetaReentrySnapshotBuiltAt = "memory_snapshot_built_at"
)

func SaveReentrySnapshot(ctx context.Context, db *sql.DB, snapshot model.ReentryMemorySnapshot) error {
	return saveReentrySnapshot(ctx, db, snapshot)
}

func saveReentrySnapshot(ctx context.Context, exec metaExecutor, snapshot model.ReentryMemorySnapshot) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	metadata := map[string]string{
		workspaceMetaReentrySnapshotJSON:    string(body),
		workspaceMetaReentrySnapshotBuiltAt: snapshot.SnapshotBuiltAt.UTC().Format(time.RFC3339Nano),
	}
	return UpsertWorkspaceMetaMap(ctx, exec, metadata)
}

func LoadReentrySnapshot(ctx context.Context, db *sql.DB) (model.ReentryMemorySnapshot, bool, error) {
	raw, ok, err := WorkspaceMetaValue(ctx, db, workspaceMetaReentrySnapshotJSON)
	if err != nil || !ok || raw == "" {
		return model.ReentryMemorySnapshot{}, ok && raw != "", err
	}
	var snapshot model.ReentryMemorySnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return model.ReentryMemorySnapshot{}, false, err
	}
	if snapshot.SnapshotBuiltAt.IsZero() {
		if builtAt, builtOk, builtErr := WorkspaceMetaValue(ctx, db, workspaceMetaReentrySnapshotBuiltAt); builtErr == nil && builtOk && builtAt != "" {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, builtAt); parseErr == nil {
				snapshot.SnapshotBuiltAt = parsed
			}
		}
	}
	return snapshot, true, nil
}

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// RecoverGraphGenerations invalidates stale staged work without touching the active pointer.
// Cleanup is scoped to the supplied workspace and leaves legacy tables untouched.
func RecoverGraphGenerations(ctx context.Context, db *sql.DB, workspaceRoot string, now time.Time, staleAfter time.Duration) (int, error) {
	if workspaceRoot == "" || staleAfter <= 0 {
		return 0, fmt.Errorf("workspace root and positive stale interval are required")
	}
	cutoff := now.Add(-staleAfter).UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `select generation_id from graph_generations where workspace_root=? and status=? and created_at<?`, workspaceRoot, model.GraphGenerationStaged, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update graph_generations set status=?, error=? where generation_id=? and status=?`, model.GraphGenerationInvalid, "recovered stale or dead-owner generation", id, model.GraphGenerationStaged); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `delete from graph_nodes where generation_id=?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `delete from graph_edges where generation_id=?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `delete from graph_evidence where generation_id=?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `delete from graph_unresolved where generation_id=?`, id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

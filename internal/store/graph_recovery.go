package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const graphRecoveryError = "GPH_CRASH_RECOVERY_REQUIRED"

// RecoverGraphState repairs graph publication and migration state while the caller-owned
// exclusive workspace lock prevents concurrent writers.
func RecoverGraphState(ctx context.Context, db *sql.DB, workspaceIdentity string, now time.Time) error {
	if workspaceIdentity == "" {
		return fmt.Errorf("workspace identity is required")
	}
	t, err := beginGraphImmediate(ctx, db)
	if err != nil {
		return err
	}
	defer t.rollback(ctx)
	ts := now.UTC().Format(time.RFC3339Nano)
	rows, err := t.c.QueryContext(ctx, `SELECT migration_id,status,preflight_digest,backup_digest,prior_active_generation_id FROM graph_migrations WHERE status IN ('prepared','applying','validated')`)
	if err != nil {
		return err
	}
	type migration struct {
		id, status               string
		preflight, backup, prior []byte
	}
	var migrations []migration
	for rows.Next() {
		var m migration
		if err = rows.Scan(&m.id, &m.status, &m.preflight, &m.backup, &m.prior); err != nil {
			rows.Close()
			return err
		}
		migrations = append(migrations, m)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, m := range migrations {
		next, code := "rolled_back", "GPH_CRASH_RECOVERY_ROLLBACK"
		if m.status != "prepared" {
			valid := len(m.preflight) == 32 && len(m.backup) == 32
			if valid && len(m.prior) > 0 {
				p, ve := validateGraphGenerationConn(ctx, t.c, model.GraphDigest(mustDigest(m.prior)))
				valid = ve == nil && p.Status == model.GraphGenerationRetired
			}
			if !valid {
				next, code = "failed", graphRecoveryError
			}
		}
		r, execErr := t.c.ExecContext(ctx, `UPDATE graph_migrations SET status=?,error_code=?,completed_at=? WHERE migration_id=? AND status=?`, next, code, ts, m.id, m.status)
		if execErr != nil {
			return execErr
		}
		n, rowsErr := r.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if n != 1 {
			return fmt.Errorf("migration recovery affected %d rows", n)
		}
	}

	// A staged generation is abandoned by definition under the exclusive workspace lock.
	rows, err = t.c.QueryContext(ctx, `SELECT generation_id FROM graph_generations WHERE workspace_identity=? AND status='staged'`, workspaceIdentity)
	if err != nil {
		return err
	}
	var staged [][]byte
	for rows.Next() {
		var id []byte
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		staged = append(staged, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range staged {
		for _, table := range []string{"graph_evidence", "graph_edges", "graph_nodes", "graph_unresolved"} {
			if _, err = t.c.ExecContext(ctx, "DELETE FROM "+table+" WHERE generation_id=?", id); err != nil {
				return err
			}
		}
		r, err := t.c.ExecContext(ctx, `UPDATE graph_generations SET status='invalid',error_code=? WHERE generation_id=? AND workspace_identity=? AND status='staged'`, graphRecoveryError, id, workspaceIdentity)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("staged recovery affected %d rows", n)
		}
	}

	var active, previous []byte
	activeErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active)
	if activeErr != nil && !errors.Is(activeErr, sql.ErrNoRows) {
		return activeErr
	}
	previousErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previous)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return previousErr
	}
	if len(active) > 0 {
		id, e := scanDigest(active)
		if e == nil {
			g, ve := validateGraphGenerationConn(ctx, t.c, id)
			if ve == nil && g.Status == model.GraphGenerationActive {
				return t.commit(ctx)
			}
			r, execErr := t.c.ExecContext(ctx, `UPDATE graph_generations SET status='invalid',error_code=? WHERE generation_id=? AND status IN ('active','retired','staged')`, graphRecoveryError, active)
			if execErr != nil {
				return execErr
			}
			n, rowsErr := r.RowsAffected()
			if rowsErr != nil {
				return rowsErr
			}
			if n != 1 {
				return fmt.Errorf("active recovery affected %d rows", n)
			}
		}
	}
	priorValid := false
	if len(previous) > 0 {
		prior, e := scanDigest(previous)
		if e == nil {
			pg, ve := validateGraphGenerationConn(ctx, t.c, prior)
			if ve == nil && pg.Status == model.GraphGenerationRetired {
				r, e := t.c.ExecContext(ctx, `UPDATE graph_generations SET status='active',published_at=?,error_code=NULL WHERE generation_id=? AND status='retired'`, ts, previous)
				if e != nil {
					return e
				}
				n, e := r.RowsAffected()
				if e != nil {
					return e
				}
				if n != 1 {
					return fmt.Errorf("prior recovery affected %d rows", n)
				}
				priorValid = true
			}
		}
	}
	if !priorValid {
		if err := t.commit(ctx); err != nil {
			return err
		}
		return ErrGraphCrashRecoveryRequired
	}
	var n int64
	if r, e := t.c.ExecContext(ctx, `UPDATE workspace_meta SET value=? WHERE key=?`, previous, graphActiveMeta); e != nil {
		return e
	} else if n, e = r.RowsAffected(); e != nil {
		return e
	}
	if n != 1 {
		return ErrGraphCrashRecoveryRequired
	}
	if r, e := t.c.ExecContext(ctx, `UPDATE workspace_meta SET value=NULL WHERE key=?`, graphPreviousMeta); e != nil {
		return e
	} else if n, e = r.RowsAffected(); e != nil {
		return e
	} else if n != 1 {
		return ErrGraphCrashRecoveryRequired
	}
	return t.commit(ctx)
}

func mustDigest(b []byte) model.GraphDigest { var d model.GraphDigest; copy(d[:], b); return d }

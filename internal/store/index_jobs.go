package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	IndexModeFull    = "full"
	IndexModeDocs    = "docs"
	IndexModeCatalog = "catalog"

	IndexJobQueued          = "queued"
	IndexJobRunning         = "running"
	IndexJobPublishing      = "publishing"
	IndexJobCancelRequested = "cancel_requested"
	IndexJobCanceled        = "canceled"
	IndexJobSucceeded       = "succeeded"
	IndexJobFailed          = "failed"
)

type IndexJob struct {
	JobID           string `json:"job_id"`
	GenerationID    string `json:"generation_id"`
	WorkspaceName   string `json:"workspace"`
	WorkspaceRoot   string `json:"workspace_root"`
	Mode            string `json:"mode"`
	Clean           bool   `json:"clean,omitempty"`
	Status          string `json:"status"`
	Phase           string `json:"phase,omitempty"`
	CurrentStage    string `json:"current_stage,omitempty"`
	CurrentPath     string `json:"current_path,omitempty"`
	FilesTotal      int    `json:"files_total,omitempty"`
	PID             int    `json:"pid,omitempty"`
	RequestedCancel bool   `json:"requested_cancel,omitempty"`
	Error           string `json:"error,omitempty"`
	Files           int    `json:"files,omitempty"`
	Symbols         int    `json:"symbols,omitempty"`
	Docs            int    `json:"docs,omitempty"`
	CreatedAt       string `json:"created_at"`
	StartedAt       string `json:"started_at,omitempty"`
	FinishedAt      string `json:"finished_at,omitempty"`
	UpdatedAt       string `json:"updated_at"`
	OwnerToken      string `json:"-"`
	FencingToken    int64  `json:"-"`
}

type IndexJobProgress struct {
	CurrentStage string
	CurrentPath  string
	Files        int
	Symbols      int
	Docs         int
	FilesTotal   int
}

type ActiveIndexJobError struct {
	Job IndexJob
}

func (e *ActiveIndexJobError) Error() string {
	return fmt.Sprintf("index job already active: %s (%s)", e.Job.JobID, e.Job.Status)
}

var (
	ErrStaleIndexJobOwner        = errors.New("index job owner fence is stale or terminal state already won")
	ErrInvalidIndexJobTransition = errors.New("invalid index job state transition")
)

type indexJobOwnership struct {
	workspaceRoot string
	jobID         string
	ownerToken    string
	fencingToken  int64
	pid           int
	releasedAt    sql.NullString
}

func ensureIndexJobOwnershipSchema(db *sql.DB) error {
	for _, migration := range []struct {
		column string
		ddl    string
	}{
		{column: "requested_cancel", ddl: `ALTER TABLE index_jobs ADD COLUMN requested_cancel INTEGER NOT NULL DEFAULT 0`},
		{column: "publication_started", ddl: `ALTER TABLE index_jobs ADD COLUMN publication_started INTEGER NOT NULL DEFAULT 0`},
	} {
		if err := addIndexJobColumnConcurrent(db, migration.column, migration.ddl); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS index_job_ownership (
			workspace_root TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			owner_token TEXT NOT NULL,
			fencing_token INTEGER NOT NULL,
			pid INTEGER NOT NULL DEFAULT 0,
			acquired_at TEXT NOT NULL,
			released_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_index_job_ownership_job ON index_job_ownership(job_id, released_at)`,
	} {
		if err := execIndexJobMigration(db, statement); err != nil {
			return err
		}
	}
	return nil
}

func addIndexJobColumnConcurrent(db *sql.DB, column string, ddl string) error {
	for attempt := 0; attempt < 8; attempt++ {
		if _, err := db.Exec(ddl); err == nil || isExpectedDuplicateColumn(err, column) {
			return nil
		} else if !isSQLiteLockedError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	_, err := db.Exec(ddl)
	if err == nil || isExpectedDuplicateColumn(err, column) {
		return nil
	}
	return err
}

func execIndexJobMigration(db *sql.DB, statement string) error {
	for attempt := 0; attempt < 8; attempt++ {
		if _, err := db.Exec(statement); err == nil {
			return nil
		} else if !isSQLiteLockedError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	_, err := db.Exec(statement)
	return err
}

func isExpectedDuplicateColumn(err error, column string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column name") && strings.Contains(message, strings.ToLower(column))
}

func isSQLiteLockedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") || strings.Contains(message, "busy")
}

func indexJobSelectColumns(db *sql.DB) (string, error) {
	hasCancelColumn, err := tableHasColumn(db, "index_jobs", "requested_cancel")
	if err != nil {
		return "", err
	}
	requestedCancel := "requested_cancel"
	if !hasCancelColumn {
		requestedCancel = "0 AS requested_cancel"
	}
	return fmt.Sprintf(`job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, pid, %s, COALESCE(error, ''),
	       COALESCE(current_stage, ''), COALESCE(current_path, ''), files_total,
	       files, symbols, docs, created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''), updated_at`, requestedCancel), nil
}

func loadIndexJobOwnership(ctx context.Context, tx *sql.Tx, workspaceRoot string) (indexJobOwnership, error) {
	var ownership indexJobOwnership
	err := tx.QueryRowContext(ctx, `
		SELECT workspace_root, job_id, owner_token, fencing_token, pid, released_at
		FROM index_job_ownership
		WHERE workspace_root = ?
	`, workspaceRoot).Scan(
		&ownership.workspaceRoot,
		&ownership.jobID,
		&ownership.ownerToken,
		&ownership.fencingToken,
		&ownership.pid,
		&ownership.releasedAt,
	)
	return ownership, err
}

func currentIndexJobOwner(ctx context.Context, db *sql.DB, jobID string) (string, int64, error) {
	var ownerToken string
	var fencingToken int64
	err := db.QueryRowContext(ctx, `
		SELECT owner_token, fencing_token
		FROM index_job_ownership
		WHERE job_id = ? AND released_at IS NULL
	`, jobID).Scan(&ownerToken, &fencingToken)
	if err != nil {
		return "", 0, err
	}
	return ownerToken, fencingToken, nil
}

func loadIndexJobOwnershipByJob(ctx context.Context, tx *sql.Tx, jobID string) (indexJobOwnership, error) {
	var ownership indexJobOwnership
	err := tx.QueryRowContext(ctx, `
		SELECT workspace_root, job_id, owner_token, fencing_token, pid, released_at
		FROM index_job_ownership
		WHERE job_id = ?
	`, jobID).Scan(
		&ownership.workspaceRoot,
		&ownership.jobID,
		&ownership.ownerToken,
		&ownership.fencingToken,
		&ownership.pid,
		&ownership.releasedAt,
	)
	return ownership, err
}

func attachIndexJobOwner(ctx context.Context, db *sql.DB, job *IndexJob) error {
	ownerToken, fencingToken, err := currentIndexJobOwner(ctx, db, job.JobID)
	if err == nil {
		job.OwnerToken = ownerToken
		job.FencingToken = fencingToken
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return nil
	}
	return err
}

type IndexJobFence struct {
	OwnerToken   string
	FencingToken int64
}

// ControlPlaneIndexJob returns the public job projection. Cancellation and
// status callers never receive the owner capability needed for publication or
// completion.
func ControlPlaneIndexJob(job IndexJob) IndexJob {
	job.OwnerToken = ""
	job.FencingToken = 0
	return job
}

func resolveIndexJobFence(fence IndexJobFence) (string, int64, error) {
	if fence.OwnerToken == "" || fence.FencingToken <= 0 {
		return "", 0, ErrStaleIndexJobOwner
	}
	return fence.OwnerToken, fence.FencingToken, nil
}

func releaseIndexJobOwnershipTx(ctx context.Context, tx *sql.Tx, jobID string, ownerToken string, fencingToken int64, now string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE index_job_ownership
		SET released_at = ?
		WHERE job_id = ? AND owner_token = ? AND fencing_token = ? AND released_at IS NULL
	`, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	return nil
}

func NormalizeIndexMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", IndexModeFull:
		return IndexModeFull, nil
	case "doc", "docs-only", IndexModeDocs:
		return IndexModeDocs, nil
	case "symbols", "code", IndexModeCatalog:
		return IndexModeCatalog, nil
	default:
		return "", fmt.Errorf("invalid index mode %q; valid modes: full, docs, catalog", mode)
	}
}

func CreateIndexJob(ctx context.Context, db *sql.DB, workspaceName string, workspaceRoot string, mode string, clean bool) (IndexJob, error) {
	return createIndexJob(ctx, db, workspaceName, workspaceRoot, mode, clean, true)
}

func CreateIndexJobUnchecked(ctx context.Context, db *sql.DB, workspaceName string, workspaceRoot string, mode string, clean bool) (IndexJob, error) {
	return createIndexJob(ctx, db, workspaceName, workspaceRoot, mode, clean, false)
}

func createIndexJob(ctx context.Context, db *sql.DB, workspaceName string, workspaceRoot string, mode string, clean bool, checkActive bool) (IndexJob, error) {
	_ = checkActive // reservation and job creation are always guarded atomically
	normalizedMode, err := NormalizeIndexMode(mode)
	if err != nil {
		return IndexJob{}, err
	}
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return IndexJob{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	job := IndexJob{
		JobID:         newIndexID("idxjob"),
		GenerationID:  newIndexID("idxgen"),
		WorkspaceName: workspaceName,
		WorkspaceRoot: workspaceRoot,
		Mode:          normalizedMode,
		Clean:         clean,
		Status:        IndexJobQueued,
		Phase:         "queued",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return IndexJob{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO index_jobs(job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.JobID, job.GenerationID, job.WorkspaceName, job.WorkspaceRoot, job.Mode, boolToInt(job.Clean), job.Status, job.Phase, job.CreatedAt, job.UpdatedAt); err != nil {
		return IndexJob{}, err
	}

	if err := reserveIndexJobTx(ctx, tx, job); err != nil {
		return IndexJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO index_generations(generation_id, job_id, workspace_name, workspace_root, mode, status, created_at)
		VALUES(?, ?, ?, ?, ?, 'building', ?)
	`, job.GenerationID, job.JobID, job.WorkspaceName, job.WorkspaceRoot, job.Mode, job.CreatedAt); err != nil {
		return IndexJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return IndexJob{}, err
	}
	if err := attachIndexJobOwner(ctx, db, &job); err != nil {
		return IndexJob{}, err
	}
	return job, nil
}

func reserveIndexJobTx(ctx context.Context, tx *sql.Tx, job IndexJob) error {
	legacy, legacyErr := loadActiveIndexJobTx(ctx, tx, job.WorkspaceRoot, job.JobID)
	if legacyErr == nil {
		legacyOwnership, ownershipErr := loadIndexJobOwnershipByJob(ctx, tx, legacy.JobID)
		if ownershipErr != nil && !errors.Is(ownershipErr, sql.ErrNoRows) {
			return ownershipErr
		}
		legacyOwned := ownershipErr == nil && !legacyOwnership.releasedAt.Valid
		if legacyOwned || (legacy.PID > 0 && indexJobProcessExists(legacy.PID)) {
			return &ActiveIndexJobError{Job: legacy}
		}
		staleAt := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `
			UPDATE index_jobs
			SET status = 'failed', phase = 'failed', current_stage = 'failed', error = 'stale index job process exited', finished_at = ?, updated_at = ?
			WHERE job_id = ? AND status IN ('queued', 'running', 'publishing', 'cancel_requested')
		`, staleAt, staleAt, legacy.JobID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrStaleIndexJobOwner
		}
	} else if !errors.Is(legacyErr, sql.ErrNoRows) {
		return legacyErr
	}

	ownership, err := loadIndexJobOwnership(ctx, tx, job.WorkspaceRoot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !ownership.releasedAt.Valid {
		active, activeErr := loadIndexJobTx(ctx, tx, ownership.jobID)
		if activeErr == nil && isIndexJobActive(active.Status) {
			if ownership.pid <= 0 || indexJobProcessExists(ownership.pid) {
				return &ActiveIndexJobError{Job: active}
			}
			released, releaseErr := tx.ExecContext(ctx, `
				UPDATE index_job_ownership
				SET released_at = ?
				WHERE workspace_root = ? AND job_id = ? AND owner_token = ? AND fencing_token = ? AND released_at IS NULL
			`, time.Now().UTC().Format(time.RFC3339Nano), job.WorkspaceRoot, ownership.jobID, ownership.ownerToken, ownership.fencingToken)
			if releaseErr != nil {
				return releaseErr
			}
			rows, rowsErr := released.RowsAffected()
			if rowsErr != nil {
				return rowsErr
			}
			if rows != 1 {
				return ErrStaleIndexJobOwner
			}
		} else if activeErr == nil && !isIndexJobActive(active.Status) {
			if _, releaseErr := tx.ExecContext(ctx, `
				UPDATE index_job_ownership SET released_at = ?
				WHERE workspace_root = ? AND job_id = ? AND owner_token = ? AND fencing_token = ? AND released_at IS NULL
			`, time.Now().UTC().Format(time.RFC3339Nano), job.WorkspaceRoot, ownership.jobID, ownership.ownerToken, ownership.fencingToken); releaseErr != nil {
				return releaseErr
			}
		} else if !errors.Is(activeErr, sql.ErrNoRows) {
			return activeErr
		}
	}

	fencingToken := int64(1)
	if err == nil {
		fencingToken = ownership.fencingToken + 1
	}
	ownerToken := newIndexID("idxowner")
	result, err := tx.ExecContext(ctx, `
		INSERT INTO index_job_ownership(workspace_root, job_id, owner_token, fencing_token, pid, acquired_at, released_at)
		VALUES(?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT(workspace_root) DO UPDATE SET
			job_id = excluded.job_id,
			owner_token = excluded.owner_token,
			fencing_token = excluded.fencing_token,
			pid = excluded.pid,
			acquired_at = excluded.acquired_at,
			released_at = NULL
		WHERE index_job_ownership.released_at IS NOT NULL
	`, job.WorkspaceRoot, job.JobID, ownerToken, fencingToken, 0, job.CreatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}

	current, currentErr := loadIndexJobOwnership(ctx, tx, job.WorkspaceRoot)
	if currentErr == nil && !current.releasedAt.Valid {
		active, activeErr := loadIndexJobTx(ctx, tx, current.jobID)
		if activeErr == nil {
			return &ActiveIndexJobError{Job: active}
		}
		return &ActiveIndexJobError{Job: IndexJob{JobID: current.jobID, WorkspaceRoot: job.WorkspaceRoot, Status: IndexJobRunning}}
	}
	if currentErr != nil {
		return currentErr
	}
	return ErrStaleIndexJobOwner
}

func loadIndexJobTx(ctx context.Context, tx *sql.Tx, jobID string) (IndexJob, error) {
	return scanIndexJobRow(tx.QueryRowContext(ctx, `
		SELECT job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, pid, requested_cancel, COALESCE(error, ''),
		       COALESCE(current_stage, ''), COALESCE(current_path, ''), files_total,
		       files, symbols, docs, created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''), updated_at
		FROM index_jobs WHERE job_id = ?
	`, jobID))
}

func loadActiveIndexJobTx(ctx context.Context, tx *sql.Tx, workspaceRoot string, excludeJobID string) (IndexJob, error) {
	return scanIndexJobRow(tx.QueryRowContext(ctx, `
		SELECT job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, pid, requested_cancel, COALESCE(error, ''),
		       COALESCE(current_stage, ''), COALESCE(current_path, ''), files_total,
		       files, symbols, docs, created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''), updated_at
		FROM index_jobs
		WHERE workspace_root = ? AND job_id <> ? AND status IN ('queued', 'running', 'publishing', 'cancel_requested')
		ORDER BY rowid DESC LIMIT 1
	`, workspaceRoot, excludeJobID))
}

func scanIndexJobRow(row *sql.Row) (IndexJob, error) {
	return scanIndexJob(row)
}

func isIndexJobActive(status string) bool {
	switch status {
	case IndexJobQueued, IndexJobRunning, IndexJobPublishing, IndexJobCancelRequested:
		return true
	default:
		return false
	}
}

// staleIndexJobForRead classifies active rows without granting ownership. A
// legacy row has no reservation and therefore requires a real PID; PID 0 is
// stale. Any modern row with a live ownership reservation is authoritative,
// including PID 0 while the worker has not recorded its PID yet.
func staleIndexJobForRead(ctx context.Context, db *sql.DB, job IndexJob) (bool, error) {
	if !isIndexJobActive(job.Status) {
		return false, nil
	}

	var ownerToken string
	var fencingToken int64
	var pid int
	var released sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT owner_token, fencing_token, pid, released_at
		FROM index_job_ownership
		WHERE job_id = ?
	`, job.JobID).Scan(&ownerToken, &fencingToken, &pid, &released)
	if err == nil && ownerToken != "" && fencingToken > 0 && !released.Valid {
		if pid <= 0 {
			return false, nil
		}
		return !indexJobProcessExists(pid), nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return false, err
	}

	// Legacy rows have no ownership capability. They are only live when their
	// recorded positive PID still exists; PID 0 is never treated as live.
	if job.PID <= 0 {
		return true, nil
	}
	return !indexJobProcessExists(job.PID), nil
}

func loadIndexJobRow(ctx context.Context, db *sql.DB, jobID string) (IndexJob, bool, error) {
	columns, err := indexJobSelectColumns(db)
	if err != nil {
		return IndexJob{}, false, err
	}
	var job IndexJob
	err = WithSQLiteReadRetry(ctx, func() error {
		row := db.QueryRowContext(ctx, `SELECT `+columns+`
			FROM index_jobs
			WHERE job_id = ?`, jobID)
		var scanErr error
		job, scanErr = scanIndexJob(row)
		return scanErr
	})
	if errors.Is(err, sql.ErrNoRows) {
		return IndexJob{}, false, nil
	}
	if err != nil {
		return IndexJob{}, false, err
	}
	return job, true, nil
}

func loadLatestIndexJobRow(ctx context.Context, db *sql.DB) (IndexJob, bool, error) {
	columns, err := indexJobSelectColumns(db)
	if err != nil {
		return IndexJob{}, false, err
	}
	var job IndexJob
	err = WithSQLiteReadRetry(ctx, func() error {
		row := db.QueryRowContext(ctx, `SELECT `+columns+`
			FROM index_jobs
			ORDER BY created_at DESC
			LIMIT 1`)
		var scanErr error
		job, scanErr = scanIndexJob(row)
		return scanErr
	})
	if errors.Is(err, sql.ErrNoRows) {
		return IndexJob{}, false, nil
	}
	if err != nil {
		return IndexJob{}, false, err
	}
	return job, true, nil
}

func loadIndexJobSnapshot(ctx context.Context, db *sql.DB, jobID string) (IndexJob, bool, error) {
	job, ok, err := loadIndexJobRow(ctx, db, jobID)
	if err != nil || !ok {
		return job, ok, err
	}
	if err := attachIndexJobOwner(ctx, db, &job); err != nil {
		return IndexJob{}, false, err
	}
	return job, true, nil
}

func reconcileIndexJobRead(ctx context.Context, db *sql.DB, job IndexJob) (IndexJob, bool, error) {
	if err := attachIndexJobOwner(ctx, db, &job); err != nil {
		return IndexJob{}, false, err
	}
	stale, err := staleIndexJobForRead(ctx, db, job)
	if err != nil {
		return IndexJob{}, false, err
	}
	if !stale || job.OwnerToken == "" {
		return job, true, nil
	}

	markErr := indexJobMarkFailed(ctx, db, job.JobID, "stale index job process exited", IndexJobFence{
		OwnerToken:   job.OwnerToken,
		FencingToken: job.FencingToken,
	})
	if markErr != nil && !errors.Is(markErr, ErrStaleIndexJobOwner) {
		return IndexJob{}, false, fmt.Errorf("reconcile stale index job %s: mark failed: %w", job.JobID, markErr)
	}

	// A failed CAS means cancellation or another terminal writer won. Reload
	// the database row instead of manufacturing a terminal projection locally.
	return indexJobReload(ctx, db, job.JobID)
}

func ActiveIndexJob(ctx context.Context, db *sql.DB, workspaceRoot string) (IndexJob, bool, error) {
	columns, err := indexJobSelectColumns(db)
	if err != nil {
		return IndexJob{}, false, err
	}
	rows, err := QueryContextWithRetry(ctx, db, `SELECT `+columns+`
		FROM index_jobs
		WHERE status IN ('queued', 'running', 'publishing', 'cancel_requested')
		ORDER BY rowid DESC
		LIMIT 128`)
	if err != nil {
		return IndexJob{}, false, err
	}
	var candidates []IndexJob
	for rows.Next() {
		job, scanErr := scanIndexJob(rows)
		if scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return IndexJob{}, false, fmt.Errorf("scan active index jobs: %v; close rows: %w", scanErr, closeErr)
			}
			return IndexJob{}, false, scanErr
		}
		if job.WorkspaceRoot == workspaceRoot {
			candidates = append(candidates, job)
		}
	}
	rowsErr := rows.Err()
	closeErr := rows.Close()
	if rowsErr != nil {
		if closeErr != nil {
			return IndexJob{}, false, fmt.Errorf("read active index jobs: %v; close rows: %w", rowsErr, closeErr)
		}
		return IndexJob{}, false, rowsErr
	}
	if closeErr != nil {
		return IndexJob{}, false, closeErr
	}

	for _, candidate := range candidates {
		job, ok, reconcileErr := reconcileIndexJobRead(ctx, db, candidate)
		if reconcileErr != nil {
			return IndexJob{}, false, reconcileErr
		}
		if ok && isIndexJobActive(job.Status) {
			return job, true, nil
		}
	}
	return IndexJob{}, false, nil
}

func GetIndexJob(ctx context.Context, db *sql.DB, jobID string) (IndexJob, bool, error) {
	job, ok, err := loadIndexJobRow(ctx, db, jobID)
	if err != nil || !ok {
		return job, ok, err
	}
	return reconcileIndexJobRead(ctx, db, job)
}

func LatestIndexJob(ctx context.Context, db *sql.DB) (IndexJob, bool, error) {
	job, ok, err := loadLatestIndexJobRow(ctx, db)
	if err != nil || !ok {
		return job, ok, err
	}
	return reconcileIndexJobRead(ctx, db, job)
}

func MarkIndexJobRunning(ctx context.Context, db *sql.DB, jobID string, pid int, phase string, fence IndexJobFence) error {
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'running', phase = ?, current_stage = ?, current_path = '', files_total = 0,
		    pid = ?, started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE job_id = ? AND requested_cancel = 0 AND status IN ('queued', 'running')
		  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = index_jobs.job_id AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
	`, phase, phase, pid, now, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE index_job_ownership SET pid = ?
		WHERE job_id = ? AND owner_token = ? AND fencing_token = ? AND released_at IS NULL
	`, pid, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	return tx.Commit()
}

func MarkIndexJobPhase(ctx context.Context, db *sql.DB, jobID string, status string, phase string, fence IndexJobFence) error {
	if status != IndexJobPublishing {
		return ErrInvalidIndexJobTransition
	}
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := db.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'publishing', phase = ?, current_stage = ?, current_path = '', publication_started = 1, updated_at = ?
		WHERE job_id = ? AND requested_cancel = 0
		  AND ((status = 'running' AND publication_started = 0) OR (status = 'publishing' AND publication_started = 1))
		  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = index_jobs.job_id AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
	`, phase, phase, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	return nil
}

func MarkIndexJobProgress(ctx context.Context, db *sql.DB, jobID string, progress IndexJobProgress, fence IndexJobFence) error {
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := db.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = CASE WHEN status = 'publishing' THEN 'publishing' ELSE 'running' END,
		    current_stage = ?,
		    current_path = ?,
		    files = ?,
		    symbols = ?,
		    docs = ?,
		    files_total = ?,
		    updated_at = ?
		WHERE job_id = ?
		  AND requested_cancel = 0
		  AND status IN ('running', 'publishing')
		  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = index_jobs.job_id AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
	`, progress.CurrentStage, progress.CurrentPath, progress.Files, progress.Symbols, progress.Docs, progress.FilesTotal, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	return nil
}

func completeIndexJobTx(ctx context.Context, tx *sql.Tx, jobID string, files, symbols, docs int, ownerToken string, fencingToken int64, publication bool) error {
	statusPredicate := "status = 'running' AND publication_started = 0"
	if publication {
		statusPredicate = "status = 'publishing' AND publication_started = 1"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE index_jobs
		SET status = 'succeeded', phase = 'done', current_stage = 'done', current_path = '',
		    files = ?, symbols = ?, docs = ?,
		    files_total = CASE WHEN files_total > ? THEN files_total ELSE ? END,
		    error = NULL, finished_at = ?, updated_at = ?
		WHERE job_id = ? AND requested_cancel = 0 AND `+statusPredicate+`
		  AND EXISTS (SELECT 1 FROM index_job_ownership o
		              WHERE o.job_id = index_jobs.job_id AND o.owner_token = ?
		                AND o.fencing_token = ? AND o.released_at IS NULL)`,
		files, symbols, docs, files, files, now, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	return releaseIndexJobOwnershipTx(ctx, tx, jobID, ownerToken, fencingToken, now)
}

func MarkIndexJobSucceeded(ctx context.Context, db *sql.DB, jobID string, files int, symbols int, docs int, fence IndexJobFence) error {
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := completeIndexJobTx(ctx, tx, jobID, files, symbols, docs, ownerToken, fencingToken, false); err != nil {
		return err
	}
	return tx.Commit()
}

func MarkIndexGenerationSkipped(ctx context.Context, db *sql.DB, jobID string, message string, fence IndexJobFence) error {
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'skipped', error = ?
		WHERE job_id = ? AND status = 'building'
		  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = ? AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
	`, message, jobID, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	return nil
}

func MarkIndexJobFailed(ctx context.Context, db *sql.DB, jobID string, message string, fence IndexJobFence) error {
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'failed', phase = 'failed', current_stage = 'failed', error = ?, finished_at = ?, updated_at = ?
		WHERE job_id = ? AND requested_cancel = 0 AND publication_started = 0
		  AND status IN ('queued', 'running', 'publishing')
		  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = index_jobs.job_id AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
	`, message, now, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	generationResult, err := tx.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'failed', error = ?
		WHERE job_id = ? AND status IN ('building', 'staged')
	`, message, jobID)
	if err != nil {
		return err
	}
	generationRows, err := generationResult.RowsAffected()
	if err != nil {
		return err
	}
	if generationRows != 1 {
		return ErrStaleIndexJobOwner
	}
	var mode string
	if err := tx.QueryRowContext(ctx, `SELECT mode FROM index_jobs WHERE job_id = ?`, jobID).Scan(&mode); err != nil {
		return err
	}
	if mode == IndexModeFull || mode == IndexModeCatalog || mode == "incremental" {
		if err := setGraphRuntimeStateTx(ctx, tx, GraphRuntimeStale, ""); err != nil {
			return err
		}
	}
	if err := releaseIndexJobOwnershipTx(ctx, tx, jobID, ownerToken, fencingToken, now); err != nil {
		return err
	}
	return tx.Commit()
}

var (
	// These seams keep stale-read reconciliation testable without routing reloads
	// through the public loaders and re-entering reconciliation recursively.
	indexJobMarkFailed = MarkIndexJobFailed
	indexJobReload     = loadIndexJobSnapshot
)

// indexJobCancelBeforeCASHook is a no-op production seam used only to make
// cancel/publication ordering deterministic in package tests.
var indexJobCancelBeforeCASHook = func() error { return nil }

// RequestIndexJobCancelByID is a control-plane request. It never returns an
// owner capability. Queued jobs are canceled and released atomically because no
// worker or publication can observe them; running jobs retain ownership and move
// to cancel_requested for cooperative worker confirmation.
func RequestIndexJobCancelByID(ctx context.Context, db *sql.DB, jobID string) (IndexJob, error) {
	if err := indexJobCancelBeforeCASHook(); err != nil {
		return IndexJob{}, err
	}
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return IndexJob{}, err
	}
	job, ok, err := GetIndexJob(ctx, db, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	if !ok {
		return IndexJob{}, sql.ErrNoRows
	}
	if !isIndexJobActive(job.Status) {
		return ControlPlaneIndexJob(job), nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return IndexJob{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// A queued job has not started a worker or publication. Make its terminal
	// transition and release the reservation in the same SQLite transaction.
	result, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'canceled', phase = 'canceled', current_stage = 'canceled', current_path = '',
		    requested_cancel = 1, finished_at = ?, updated_at = ?
		WHERE job_id = ? AND requested_cancel = 0 AND status = 'queued'
		  AND EXISTS (SELECT 1 FROM index_job_ownership o
		             WHERE o.job_id = index_jobs.job_id AND o.released_at IS NULL)
	`, now, now, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return IndexJob{}, err
	}
	if rows == 1 {
		ownership, ownershipErr := loadIndexJobOwnershipByJob(ctx, tx, jobID)
		if ownershipErr != nil || ownership.releasedAt.Valid {
			if ownershipErr != nil {
				return IndexJob{}, ownershipErr
			}
			return IndexJob{}, ErrStaleIndexJobOwner
		}
		generationResult, generationErr := tx.ExecContext(ctx, `
			UPDATE index_generations
			SET status = 'canceled', error = 'canceled'
			WHERE job_id = ? AND status = 'building'
		`, jobID)
		if generationErr != nil {
			return IndexJob{}, generationErr
		}
		generationRows, generationErr := generationResult.RowsAffected()
		if generationErr != nil {
			return IndexJob{}, generationErr
		}
		if generationRows != 1 {
			return IndexJob{}, ErrStaleIndexJobOwner
		}
		if err := releaseIndexJobOwnershipTx(ctx, tx, jobID, ownership.ownerToken, ownership.fencingToken, now); err != nil {
			return IndexJob{}, err
		}
		if err := tx.Commit(); err != nil {
			return IndexJob{}, err
		}
		job, ok, err = GetIndexJob(ctx, db, jobID)
		if err != nil {
			return IndexJob{}, err
		}
		if !ok {
			return IndexJob{}, sql.ErrNoRows
		}
		return ControlPlaneIndexJob(job), nil
	}

	// Running workers keep their reservation and observe the cooperative flag.
	result, err = tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET requested_cancel = 1,
		    status = 'cancel_requested',
		    phase = 'cancel_requested',
		    current_stage = 'cancel_requested',
		    updated_at = ?
		WHERE job_id = ? AND requested_cancel = 0 AND status = 'running'
		  AND EXISTS (SELECT 1 FROM index_job_ownership o
		             WHERE o.job_id = index_jobs.job_id AND o.released_at IS NULL)
	`, now, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return IndexJob{}, err
	}
	if rows != 1 {
		_ = tx.Rollback()
		job, ok, err = GetIndexJob(ctx, db, jobID)
		if err != nil {
			return IndexJob{}, err
		}
		if !ok {
			return IndexJob{}, sql.ErrNoRows
		}
		return ControlPlaneIndexJob(job), nil
	}
	if err := tx.Commit(); err != nil {
		return IndexJob{}, err
	}
	job, ok, err = GetIndexJob(ctx, db, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	if !ok {
		return IndexJob{}, sql.ErrNoRows
	}
	return ControlPlaneIndexJob(job), nil
}

// RequestIndexJobCancel is retained as the control-plane name used by callers.
// It intentionally accepts only a job ID; owner-bound transitions require a
// fence explicitly through MarkIndexJobCanceled or another owner API.
func RequestIndexJobCancel(ctx context.Context, db *sql.DB, jobID string) (IndexJob, error) {
	return RequestIndexJobCancelByID(ctx, db, jobID)
}

var (
	indexJobProcessExists    = processExists
	indexJobTerminateProcess = terminateProcess
	indexJobWaitForExit      = waitForProcessExit
)

// CancelIndexJob is a control-plane cancellation entry point. The supervisor
// reads the current job, requests cancellation by ID, then uses the fence from
// that read only for the owner-bound force-cancel transition.
func CancelIndexJob(ctx context.Context, db *sql.DB, jobID string, force bool) (IndexJob, error) {
	if !force {
		return RequestIndexJobCancelByID(ctx, db, jobID)
	}

	job, ok, err := GetIndexJob(ctx, db, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	if !ok {
		return IndexJob{}, sql.ErrNoRows
	}
	if !isIndexJobActive(job.Status) || job.Status == IndexJobPublishing {
		return ControlPlaneIndexJob(job), nil
	}
	jobFence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	requested, err := RequestIndexJobCancelByID(ctx, db, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	if !isIndexJobActive(requested.Status) {
		return requested, nil
	}
	job.Status = requested.Status
	job.Phase = requested.Phase
	job.CurrentStage = requested.CurrentStage
	job.RequestedCancel = requested.RequestedCancel

	if job.PID > 0 && indexJobProcessExists(job.PID) {
		if err := indexJobTerminateProcess(job.PID); err != nil && indexJobProcessExists(job.PID) {
			return IndexJob{}, fmt.Errorf("terminate index job pid %d: %w", job.PID, err)
		}
		if !indexJobWaitForExit(job.PID, 2*time.Second) {
			result, updateErr := db.ExecContext(ctx, `
				UPDATE index_jobs
				SET phase = 'terminating', current_stage = 'terminating', updated_at = ?
				WHERE job_id = ? AND status = 'cancel_requested' AND requested_cancel = 1
				  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = index_jobs.job_id AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
			`, time.Now().UTC().Format(time.RFC3339Nano), jobID, jobFence.OwnerToken, jobFence.FencingToken)
			if updateErr != nil {
				return IndexJob{}, updateErr
			}
			rows, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return IndexJob{}, rowsErr
			}
			if rows != 1 {
				return IndexJob{}, ErrStaleIndexJobOwner
			}
			job, _, err = GetIndexJob(ctx, db, jobID)
			return ControlPlaneIndexJob(job), err
		}
	}
	if err := MarkIndexJobCanceled(ctx, db, jobID, jobFence); err != nil {
		return IndexJob{}, err
	}
	if job.PID > 0 {
		removed, removeErr := RemoveWorkspaceIndexLockForOwner(job.WorkspaceRoot, job.PID, job.OwnerToken)
		if removeErr != nil {
			return IndexJob{}, removeErr
		}
		if !removed {
			lockInfo := readIndexLockInfo(filepath.Join(job.WorkspaceRoot, ".mi-lsp", "index.lock"))
			if lockInfo.OwnerToken == "" {
				if _, removeErr := RemoveWorkspaceIndexLockForPID(job.WorkspaceRoot, job.PID); removeErr != nil {
					return IndexJob{}, removeErr
				}
			}
		}
	}
	job, _, err = GetIndexJob(ctx, db, jobID)
	return ControlPlaneIndexJob(job), err
}

func IsIndexJobCancelRequested(ctx context.Context, db *sql.DB, jobID string) (bool, error) {
	var requested int
	var status string
	columns, err := indexJobSelectColumns(db)
	if err != nil {
		return false, err
	}
	requestedExpression := "requested_cancel"
	if strings.Contains(columns, "0 AS requested_cancel") {
		requestedExpression = "0"
	}
	if err := WithSQLiteReadRetry(ctx, func() error {
		return db.QueryRowContext(ctx, "SELECT "+requestedExpression+", status FROM index_jobs WHERE job_id = ?", jobID).Scan(&requested, &status)
	}); err != nil {
		return false, err
	}
	return requested != 0 || status == IndexJobCancelRequested, nil
}

func MarkIndexJobCanceled(ctx context.Context, db *sql.DB, jobID string, fence IndexJobFence) error {
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'canceled', phase = 'canceled', current_stage = 'canceled', current_path = '',
		    requested_cancel = 1, finished_at = ?, updated_at = ?
		WHERE job_id = ? AND requested_cancel = 1 AND publication_started = 0
		  AND status IN ('queued', 'running', 'cancel_requested')
		  AND EXISTS (SELECT 1 FROM index_job_ownership o WHERE o.job_id = index_jobs.job_id AND o.owner_token = ? AND o.fencing_token = ? AND o.released_at IS NULL)
	`, now, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	generationResult, err := tx.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'canceled', error = 'canceled'
		WHERE job_id = ? AND status = 'building'
	`, jobID)
	if err != nil {
		return err
	}
	generationRows, err := generationResult.RowsAffected()
	if err != nil {
		return err
	}
	if generationRows != 1 {
		return ErrStaleIndexJobOwner
	}
	if err := releaseIndexJobOwnershipTx(ctx, tx, jobID, ownerToken, fencingToken, now); err != nil {
		return err
	}
	return tx.Commit()
}

type indexJobScanner interface {
	Scan(dest ...any) error
}

func scanIndexJob(scanner indexJobScanner) (IndexJob, error) {
	var requested int
	var clean int
	var job IndexJob
	if err := scanner.Scan(
		&job.JobID,
		&job.GenerationID,
		&job.WorkspaceName,
		&job.WorkspaceRoot,
		&job.Mode,
		&clean,
		&job.Status,
		&job.Phase,
		&job.PID,
		&requested,
		&job.Error,
		&job.CurrentStage,
		&job.CurrentPath,
		&job.FilesTotal,
		&job.Files,
		&job.Symbols,
		&job.Docs,
		&job.CreatedAt,
		&job.StartedAt,
		&job.FinishedAt,
		&job.UpdatedAt,
	); err != nil {
		return IndexJob{}, err
	}
	job.Clean = clean != 0
	job.RequestedCancel = requested != 0
	return job, nil
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for indexJobProcessExists(pid) {
		if timeout <= 0 || time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
	return true
}

func staleIndexJob(job IndexJob) bool {
	switch job.Status {
	case IndexJobRunning, IndexJobPublishing, IndexJobCancelRequested:
		return job.PID > 0 && !indexJobProcessExists(job.PID)
	default:
		return false
	}
}

func newIndexID(prefix string) string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(data[:])
	}
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), os.Getpid())
}

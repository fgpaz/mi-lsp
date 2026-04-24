package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
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
	normalizedMode, err := NormalizeIndexMode(mode)
	if err != nil {
		return IndexJob{}, err
	}
	if active, ok, err := ActiveIndexJob(ctx, db, workspaceRoot); err != nil {
		return IndexJob{}, err
	} else if ok {
		return IndexJob{}, &ActiveIndexJobError{Job: active}
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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO index_generations(generation_id, job_id, workspace_name, workspace_root, mode, status, created_at)
		VALUES(?, ?, ?, ?, ?, 'building', ?)
	`, job.GenerationID, job.JobID, job.WorkspaceName, job.WorkspaceRoot, job.Mode, job.CreatedAt); err != nil {
		return IndexJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return IndexJob{}, err
	}
	return job, nil
}

func ActiveIndexJob(ctx context.Context, db *sql.DB, workspaceRoot string) (IndexJob, bool, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, pid, requested_cancel, COALESCE(error, ''),
		       COALESCE(current_stage, ''), COALESCE(current_path, ''), files_total,
		       files, symbols, docs, created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''), updated_at
		FROM index_jobs
		WHERE workspace_root = ?
		  AND status IN ('queued', 'running', 'publishing', 'cancel_requested')
		ORDER BY updated_at DESC
	`, workspaceRoot)
	if err != nil {
		return IndexJob{}, false, err
	}
	defer rows.Close()

	for rows.Next() {
		job, err := scanIndexJob(rows)
		if err != nil {
			return IndexJob{}, false, err
		}
		if staleIndexJob(job) {
			_ = MarkIndexJobFailed(ctx, db, job.JobID, "stale index job process exited")
			continue
		}
		return job, true, nil
	}
	if err := rows.Err(); err != nil {
		return IndexJob{}, false, err
	}
	return IndexJob{}, false, nil
}

func GetIndexJob(ctx context.Context, db *sql.DB, jobID string) (IndexJob, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, pid, requested_cancel, COALESCE(error, ''),
		       COALESCE(current_stage, ''), COALESCE(current_path, ''), files_total,
		       files, symbols, docs, created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''), updated_at
		FROM index_jobs
		WHERE job_id = ?
	`, jobID)
	job, err := scanIndexJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return IndexJob{}, false, nil
		}
		return IndexJob{}, false, err
	}
	return job, true, nil
}

func LatestIndexJob(ctx context.Context, db *sql.DB) (IndexJob, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT job_id, generation_id, workspace_name, workspace_root, mode, clean, status, phase, pid, requested_cancel, COALESCE(error, ''),
		       COALESCE(current_stage, ''), COALESCE(current_path, ''), files_total,
		       files, symbols, docs, created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''), updated_at
		FROM index_jobs
		ORDER BY created_at DESC
		LIMIT 1
	`)
	job, err := scanIndexJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return IndexJob{}, false, nil
		}
		return IndexJob{}, false, err
	}
	if staleIndexJob(job) {
		_ = MarkIndexJobFailed(ctx, db, job.JobID, "stale index job process exited")
		job.Status = IndexJobFailed
		job.Error = "stale index job process exited"
	}
	return job, true, nil
}

func MarkIndexJobRunning(ctx context.Context, db *sql.DB, jobID string, pid int, phase string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'running', phase = ?, current_stage = ?, current_path = '', files_total = 0,
		    pid = ?, started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE job_id = ?
	`, phase, phase, pid, now, now, jobID)
	return err
}

func MarkIndexJobPhase(ctx context.Context, db *sql.DB, jobID string, status string, phase string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = ?, phase = ?, current_stage = ?, current_path = '', updated_at = ?
		WHERE job_id = ?
	`, status, phase, phase, now, jobID)
	return err
}

func MarkIndexJobProgress(ctx context.Context, db *sql.DB, jobID string, progress IndexJobProgress) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'running',
		    current_stage = ?,
		    current_path = ?,
		    files = ?,
		    symbols = ?,
		    docs = ?,
		    files_total = ?,
		    updated_at = ?
		WHERE job_id = ?
		  AND requested_cancel = 0
		  AND status IN ('queued', 'running', 'publishing')
	`, progress.CurrentStage, progress.CurrentPath, progress.Files, progress.Symbols, progress.Docs, progress.FilesTotal, now, jobID)
	return err
}

func MarkIndexJobSucceeded(ctx context.Context, db *sql.DB, jobID string, files int, symbols int, docs int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'succeeded', phase = 'done', current_stage = 'done', current_path = '',
		    files = ?, symbols = ?, docs = ?,
		    files_total = CASE WHEN files_total > ? THEN files_total ELSE ? END,
		    error = NULL, finished_at = ?, updated_at = ?
		WHERE job_id = ?
	`, files, symbols, docs, files, files, now, now, jobID)
	return err
}

func MarkIndexGenerationSkipped(ctx context.Context, db *sql.DB, jobID string, message string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'skipped', error = ?
		WHERE job_id = ? AND status <> 'published'
	`, message, jobID)
	return err
}

func MarkIndexJobFailed(ctx context.Context, db *sql.DB, jobID string, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'failed', phase = 'failed', current_stage = 'failed', error = ?, finished_at = ?, updated_at = ?
		WHERE job_id = ?
	`, message, now, now, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'failed', error = ?
		WHERE job_id = ? AND status <> 'published'
	`, message, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func RequestIndexJobCancel(ctx context.Context, db *sql.DB, jobID string) (IndexJob, error) {
	job, ok, err := GetIndexJob(ctx, db, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	if !ok {
		return IndexJob{}, sql.ErrNoRows
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := IndexJobCancelRequested
	phase := job.Phase
	if job.Status == IndexJobQueued {
		status = IndexJobCanceled
		phase = "canceled"
	}
	if job.Status == IndexJobSucceeded || job.Status == IndexJobFailed || job.Status == IndexJobCanceled {
		return job, nil
	}
	_, err = db.ExecContext(ctx, `
		UPDATE index_jobs
		SET requested_cancel = 1, status = ?, phase = ?, current_stage = ?,
		    finished_at = CASE WHEN ? = 'canceled' THEN ? ELSE finished_at END, updated_at = ?
		WHERE job_id = ?
	`, status, phase, status, status, now, now, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	job, _, err = GetIndexJob(ctx, db, jobID)
	return job, err
}

func CancelIndexJob(ctx context.Context, db *sql.DB, jobID string, force bool) (IndexJob, error) {
	if !force {
		return RequestIndexJobCancel(ctx, db, jobID)
	}

	job, ok, err := GetIndexJob(ctx, db, jobID)
	if err != nil {
		return IndexJob{}, err
	}
	if !ok {
		return IndexJob{}, sql.ErrNoRows
	}
	if job.Status == IndexJobSucceeded || job.Status == IndexJobFailed || job.Status == IndexJobCanceled {
		return job, nil
	}

	if job.PID > 0 && processExists(job.PID) {
		if err := terminateProcess(job.PID); err != nil && processExists(job.PID) {
			return IndexJob{}, fmt.Errorf("terminate index job pid %d: %w", job.PID, err)
		}
		waitForProcessExit(job.PID, 2*time.Second)
	}
	if err := MarkIndexJobCanceled(ctx, db, jobID); err != nil {
		return IndexJob{}, err
	}
	if job.PID > 0 {
		if _, err := RemoveWorkspaceIndexLockForPID(job.WorkspaceRoot, job.PID); err != nil {
			return IndexJob{}, err
		}
	}
	job, _, err = GetIndexJob(ctx, db, jobID)
	return job, err
}

func IsIndexJobCancelRequested(ctx context.Context, db *sql.DB, jobID string) (bool, error) {
	var requested int
	var status string
	if err := db.QueryRowContext(ctx, "SELECT requested_cancel, status FROM index_jobs WHERE job_id = ?", jobID).Scan(&requested, &status); err != nil {
		return false, err
	}
	return requested != 0 || status == IndexJobCancelRequested, nil
}

func MarkIndexJobCanceled(ctx context.Context, db *sql.DB, jobID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status = 'canceled', phase = 'canceled', current_stage = 'canceled', current_path = '',
		    requested_cancel = 1, finished_at = ?, updated_at = ?
		WHERE job_id = ?
	`, now, now, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'canceled', error = 'canceled'
		WHERE job_id = ? AND status <> 'published'
	`, jobID); err != nil {
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
	for processExists(pid) {
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
		return job.PID > 0 && !processExists(job.PID)
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

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/store"
)

var errIndexJobCanceled = errors.New("index job canceled")

// indexJobProgressAfterMarkHook is a no-op production seam for deterministic
// cancellation tests at the progress boundary.
var indexJobProgressAfterMarkHook = func(context.Context, *sql.DB, string, indexer.Progress) error { return nil }

// indexJobBeforeCancelHook is a no-op production seam for deterministic tests
// of a terminal publication winning immediately before cooperative cancel.
var indexJobBeforeCancelHook = func(context.Context, *sql.DB, string, store.IndexJobFence) error { return nil }

var spawnDetachedIndexJobProcess = startDetachedIndexJobProcess

func (a *App) indexStart(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, err := a.resolveIndexWorkspace(request)
	if err != nil {
		return model.Envelope{}, err
	}
	mode, err := requestedIndexMode(request.Payload)
	if err != nil {
		return model.Envelope{}, err
	}
	wait, _ := request.Payload["wait"].(bool)
	clean, _ := request.Payload["clean"].(bool)

	db, err := openWorkspaceDB(registration, "index.start", false) // readWrite
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	job, err := store.CreateIndexJob(ctx, db, registration.Name, registration.Root, mode, clean)
	if err != nil {
		if activeErr, ok := err.(*store.ActiveIndexJobError); ok {
			return activeIndexJobEnvelope(registration, store.ControlPlaneIndexJob(activeErr.Job)), nil
		}
		return model.Envelope{}, err
	}

	if !wait {
		pid, spawnErr := a.spawnIndexJob(ctx, db, registration, job.JobID)
		if spawnErr != nil {
			markErr := store.MarkIndexJobFailed(ctx, db, job.JobID, spawnErr.Error(), store.IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken})
			if markErr != nil {
				return model.Envelope{}, fmt.Errorf("%w; mark index job failed: %v", spawnErr, markErr)
			}
			return model.Envelope{}, spawnErr
		}
		job.PID = pid
		job.Status = store.IndexJobRunning
		job.Phase = "spawned"
		return model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "index-job",
			Mode:      mode,
			Items:     []store.IndexJob{store.ControlPlaneIndexJob(job)},
			Warnings:  []string{"index job started asynchronously; use `mi-lsp index status " + job.JobID + "` to inspect progress"},
		}, nil
	}

	resultJob, result, err := a.runIndexJob(ctx, registration, job.JobID)
	if err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "index-job",
		Mode:      mode,
		Items:     []store.IndexJob{store.ControlPlaneIndexJob(resultJob)},
		Stats:     result.Stats,
		Warnings:  result.Warnings,
	}, nil
}

func activeIndexJobEnvelope(registration model.WorkspaceRegistration, job store.IndexJob) model.Envelope {
	statusCommand := "mi-lsp index status " + job.JobID
	nextHint := statusCommand
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "index-job",
		Mode:      job.Mode,
		Items:     []store.IndexJob{store.ControlPlaneIndexJob(job)},
		Warnings: []string{
			"index job already active; use `" + statusCommand + "` to inspect progress",
			"backoff recommended: retry index.start after the active job finishes",
		},
		Hint:     "existing index job returned instead of starting a duplicate job",
		NextHint: &nextHint,
	}
}

func (a *App) indexRunJob(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, err := a.resolveIndexWorkspace(request)
	if err != nil {
		return model.Envelope{}, err
	}
	jobID := stringPayload(request.Payload, "job_id")
	if jobID == "" {
		return model.Envelope{}, errors.New("job_id is required")
	}
	job, result, err := a.runIndexJob(ctx, registration, jobID)
	if err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "index-job",
		Mode:      job.Mode,
		Items:     []store.IndexJob{store.ControlPlaneIndexJob(job)},
		Stats:     result.Stats,
		Warnings:  result.Warnings,
	}, nil
}

func (a *App) indexStatus(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, err := a.resolveIndexWorkspace(request)
	if err != nil {
		return model.Envelope{}, err
	}
	db, err := openWorkspaceDB(registration, "index.status", true) // readOnly
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	jobID := stringPayload(request.Payload, "job_id")
	var (
		job store.IndexJob
		ok  bool
	)
	if jobID != "" {
		job, ok, err = store.GetIndexJob(ctx, db, jobID)
	} else {
		job, ok, err = store.LatestIndexJob(ctx, db)
	}
	if err != nil {
		return model.Envelope{}, err
	}
	if !ok {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "index-job", Items: []store.IndexJob{}, Warnings: []string{"no index jobs found"}}, nil
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "index-job", Mode: job.Mode, Items: []store.IndexJob{store.ControlPlaneIndexJob(job)}}, nil
}

func (a *App) indexCancel(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, err := a.resolveIndexWorkspace(request)
	if err != nil {
		return model.Envelope{}, err
	}
	jobID := stringPayload(request.Payload, "job_id")
	if jobID == "" {
		return model.Envelope{}, errors.New("job_id is required")
	}
	db, err := openWorkspaceDB(registration, "index.cancel", false) // readWrite
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()
	force, _ := request.Payload["force"].(bool)
	current, ok, err := store.GetIndexJob(ctx, db, jobID)
	if err != nil {
		return model.Envelope{}, err
	}
	if !ok {
		return model.Envelope{}, fmt.Errorf("index job %s not found", jobID)
	}
	if current.Status != store.IndexJobQueued && current.Status != store.IndexJobRunning && current.Status != store.IndexJobPublishing && current.Status != store.IndexJobCancelRequested {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "index-job", Mode: current.Mode, Items: []store.IndexJob{store.ControlPlaneIndexJob(current)}}, nil
	}
	job, err := store.CancelIndexJob(ctx, db, jobID, force)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.Envelope{}, fmt.Errorf("index job %s not found", jobID)
		}
		return model.Envelope{}, err
	}
	warnings := []string{}
	if force {
		warnings = append(warnings, "index job force-canceled; a live PID was terminated when present")
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "index-job", Mode: job.Mode, Items: []store.IndexJob{store.ControlPlaneIndexJob(job)}, Warnings: warnings}, nil
}

func markIndexJobCanceledCooperatively(ctx context.Context, db *sql.DB, jobID string, fence store.IndexJobFence) (store.IndexJob, error) {
	markErr := store.MarkIndexJobCanceled(ctx, db, jobID, fence)
	job, ok, getErr := store.GetIndexJob(ctx, db, jobID)
	if getErr != nil {
		return store.IndexJob{}, getErr
	}
	if !ok {
		return store.IndexJob{}, fmt.Errorf("index job %s not found", jobID)
	}
	if markErr == nil {
		return job, errIndexJobCanceled
	}
	if !errors.Is(markErr, store.ErrStaleIndexJobOwner) {
		return job, markErr
	}
	// A publication may have committed and released the fence between the
	// cancel request and this owner-bound CAS. Reconcile terminal state instead
	// of propagating a stale-owner error from a successful job.
	switch job.Status {
	case store.IndexJobSucceeded:
		return job, nil
	case store.IndexJobCanceled:
		return job, errIndexJobCanceled
	default:
		return job, markErr
	}
}

func (a *App) runIndexJob(ctx context.Context, registration model.WorkspaceRegistration, jobID string) (store.IndexJob, indexer.Result, error) {
	db, err := openWorkspaceDB(registration, "index.run-job", false) // readWrite
	if err != nil {
		return store.IndexJob{}, indexer.Result{}, err
	}
	defer db.Close()

	job, ok, err := store.GetIndexJob(ctx, db, jobID)
	if err != nil {
		return store.IndexJob{}, indexer.Result{}, err
	}
	if !ok {
		return store.IndexJob{}, indexer.Result{}, fmt.Errorf("index job %s not found", jobID)
	}
	jobFence := store.IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if job.Status == store.IndexJobCanceled {
		return job, indexer.Result{Warnings: []string{"index job was already canceled"}}, nil
	}
	if job.Status == store.IndexJobSucceeded {
		return job, indexer.Result{Warnings: []string{"index job already succeeded"}}, nil
	}
	if job.RequestedCancel || job.Status == store.IndexJobCancelRequested {
		if err := indexJobBeforeCancelHook(ctx, db, jobID, jobFence); err != nil {
			return store.IndexJob{}, indexer.Result{}, err
		}
		job, cancelErr := markIndexJobCanceledCooperatively(ctx, db, jobID, jobFence)
		if cancelErr != nil && !errors.Is(cancelErr, errIndexJobCanceled) {
			return store.IndexJob{}, indexer.Result{}, cancelErr
		}
		if job.Status == store.IndexJobSucceeded {
			return job, indexer.Result{Warnings: []string{"index job already succeeded"}}, nil
		}
		return job, indexer.Result{Warnings: []string{"index job canceled before execution"}}, nil
	}

	var result indexer.Result
	err = store.WithWorkspaceIndexLockOwned(registration.Root, "index."+job.Mode, jobFence.OwnerToken, func() error {
		if err := store.MarkIndexJobRunning(ctx, db, jobID, os.Getpid(), "indexing", jobFence); err != nil {
			return err
		}
		progress := newIndexJobProgressReporter(db, jobID, jobFence)
		if err := progress.report(ctx, indexer.Progress{Stage: "indexing", Force: true}); err != nil {
			return err
		}
		switch job.Mode {
		case store.IndexModeDocs:
			result, err = indexer.IndexWorkspaceDocsOnlyWithProgressForJob(ctx, registration.Root, job.GenerationID, jobID, jobFence, progress.report)
		case store.IndexModeCatalog:
			result, err = indexer.IndexWorkspaceCatalogOnlyWithProgressForJob(ctx, registration.Root, job.Clean, job.GenerationID, jobID, jobFence, progress.report)
		default:
			hasExistingCatalog := false
			if stats, statsErr := store.WorkspaceStats(ctx, db); statsErr == nil {
				hasExistingCatalog = stats.Files > 0 || stats.Symbols > 0
			}
			if !job.Clean && hasExistingCatalog {
				result, err = indexer.IncrementalIndexWithGraphProgressForJob(ctx, registration.Root, job.GenerationID, jobID, jobFence, progress.report, indexer.GraphIndexOptions{RoslynObserver: a.graphObserver()})
				if err != nil && (errors.Is(err, store.ErrStaleIndexJobOwner) || errors.Is(err, errIndexJobCanceled)) {
					return err
				}
				if err == nil {
					warning := "incremental=true"
					if result.Stats.Files == 0 {
						warning = "no changes detected"
					}
					result.Warnings = appendStringIfMissing(result.Warnings, warning)
					// The incremental publisher applies file rows and performs the
					// owner-bound terminal transition in one transaction, including
					// the explicitly non-graph skipped-generation case.
					return nil
				}
			}
			result, err = indexer.IndexWorkspaceWithGraphProgressForJob(ctx, registration.Root, job.Clean, job.GenerationID, jobID, jobFence, progress.report, indexer.GraphIndexOptions{RoslynObserver: a.graphObserver()})
		}
		return err
	})
	if err != nil {
		if errors.Is(err, errIndexJobCanceled) {
			job, _, getErr := store.GetIndexJob(ctx, db, jobID)
			if getErr != nil {
				return store.IndexJob{}, indexer.Result{}, getErr
			}
			result.Warnings = appendStringIfMissing(result.Warnings, "index job canceled")
			return job, result, nil
		}
		if errors.Is(err, store.ErrStaleIndexJobOwner) {
			current, ok, getErr := store.GetIndexJob(ctx, db, jobID)
			if getErr != nil {
				return store.IndexJob{}, indexer.Result{}, fmt.Errorf("read index job after stale owner: %w", getErr)
			}
			if !ok {
				return store.IndexJob{}, indexer.Result{}, fmt.Errorf("index job %s not found after stale owner", jobID)
			}
			switch current.Status {
			case store.IndexJobSucceeded:
				result.Warnings = appendStringIfMissing(result.Warnings, "index job already succeeded")
				return current, result, nil
			case store.IndexJobCanceled:
				result.Warnings = appendStringIfMissing(result.Warnings, "index job canceled")
				return current, result, nil
			default:
				return current, result, store.ErrStaleIndexJobOwner
			}
		}
		markErr := store.MarkIndexJobFailed(ctx, db, jobID, err.Error(), jobFence)
		if markErr != nil {
			return store.IndexJob{}, indexer.Result{}, fmt.Errorf("%w; mark index job failed: %v", err, markErr)
		}
		return store.IndexJob{}, indexer.Result{}, err
	}

	// Publication is terminal before embeddings begin. Embeddings only enrich
	// recall data, so they run without the owner-bound progress reporter and
	// cannot change a succeeded job back to failed.
	if job.Mode == store.IndexModeDocs || job.Mode == store.IndexModeFull {
		var embeddingErr error
		result.Warnings, embeddingErr = a.appendWikiEmbeddingWarnings(ctx, registration.Root, result.Warnings, nil)
		if embeddingErr != nil {
			result.Warnings = appendStringIfMissing(result.Warnings, "embeddings unavailable after publication; index job succeeded")
		}
	}
	job, _, err = store.GetIndexJob(ctx, db, jobID)
	return job, result, err
}

type indexJobProgressReporter struct {
	db              *sql.DB
	jobID           string
	fence           store.IndexJobFence
	interval        time.Duration
	cancelInterval  time.Duration
	lastProgressAt  time.Time
	lastCancelCheck time.Time
	lastStage       string
}

func newIndexJobProgressReporter(db *sql.DB, jobID string, fence store.IndexJobFence) *indexJobProgressReporter {
	return &indexJobProgressReporter{
		db:             db,
		jobID:          jobID,
		fence:          fence,
		interval:       time.Second,
		cancelInterval: 100 * time.Millisecond,
	}
}

func (r *indexJobProgressReporter) report(ctx context.Context, progress indexer.Progress) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	if progress.Force || r.lastCancelCheck.IsZero() || now.Sub(r.lastCancelCheck) >= r.cancelInterval {
		r.lastCancelCheck = now
		canceled, err := store.IsIndexJobCancelRequested(ctx, r.db, r.jobID)
		if err != nil {
			return err
		}
		if canceled {
			_, err := markIndexJobCanceledCooperatively(ctx, r.db, r.jobID, r.fence)
			return err
		}
	}

	if !progress.Force && !r.lastProgressAt.IsZero() && now.Sub(r.lastProgressAt) < r.interval && progress.Stage == r.lastStage {
		return nil
	}
	if err := store.MarkIndexJobProgress(ctx, r.db, r.jobID, store.IndexJobProgress{
		CurrentStage: progress.Stage,
		CurrentPath:  progress.Path,
		Files:        progress.Files,
		Symbols:      progress.Symbols,
		Docs:         progress.Docs,
		FilesTotal:   progress.FilesTotal,
	}, r.fence); err != nil {
		return err
	}
	if err := indexJobProgressAfterMarkHook(ctx, r.db, r.jobID, progress); err != nil {
		return err
	}
	canceled, err := store.IsIndexJobCancelRequested(ctx, r.db, r.jobID)
	if err != nil {
		return err
	}
	if canceled {
		if err := store.MarkIndexJobCanceled(ctx, r.db, r.jobID, r.fence); err != nil {
			return err
		}
		return errIndexJobCanceled
	}
	r.lastProgressAt = now
	r.lastStage = progress.Stage
	return nil
}

func (a *App) spawnIndexJob(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, jobID string) (int, error) {
	pid, err := spawnDetachedIndexJobProcess(registration, jobID)
	if err != nil {
		return 0, err
	}
	job, ok, err := store.GetIndexJob(ctx, db, jobID)
	if err != nil {
		return pid, err
	}
	if !ok {
		return pid, fmt.Errorf("index job %s not found after spawn", jobID)
	}
	fence := store.IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := store.MarkIndexJobRunning(ctx, db, jobID, pid, "spawned", fence); err != nil {
		return pid, err
	}
	return pid, nil
}

func startDetachedIndexJobProcess(registration model.WorkspaceRegistration, jobID string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	logDir := filepath.Join(registration.Root, ".mi-lsp", "index-jobs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, jobID+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()

	cmd := exec.CommandContext(context.Background(), executable, "--workspace", registration.Name, "--format", "json", "index", "run-job", jobID)
	if neutralCWD := detachedIndexJobCWD(); neutralCWD != "" {
		cmd.Dir = neutralCWD
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "MI_LSP_CLIENT_NAME=mi-lsp-index-job")
	processutil.ConfigureDetachedCommand(cmd)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

func detachedIndexJobCWD() string {
	if dir := os.TempDir(); dir != "" {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			return dir
		}
	}
	if dir, err := os.UserHomeDir(); err == nil && dir != "" {
		if stat, statErr := os.Stat(dir); statErr == nil && stat.IsDir() {
			return dir
		}
	}
	return ""
}

func (a *App) resolveIndexWorkspace(request model.CommandRequest) (model.WorkspaceRegistration, error) {
	path := stringPayload(request.Payload, "path")
	if path == "" {
		path = request.Context.Workspace
	}
	return a.ResolveWorkspace(path)
}

func requestedIndexMode(payload map[string]any) (string, error) {
	mode := stringPayload(payload, "mode")
	docsOnly, _ := payload["docs_only"].(bool)
	if docsOnly {
		mode = store.IndexModeDocs
	}
	return store.NormalizeIndexMode(mode)
}

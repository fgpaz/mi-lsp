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

	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	job, err := store.CreateIndexJob(ctx, db, registration.Name, registration.Root, mode, clean)
	if err != nil {
		if activeErr, ok := err.(*store.ActiveIndexJobError); ok {
			return model.Envelope{}, fmt.Errorf("index already running for workspace %s: %w", registration.Name, activeErr)
		}
		return model.Envelope{}, err
	}

	if !wait {
		pid, spawnErr := a.spawnIndexJob(ctx, db, registration, job.JobID)
		if spawnErr != nil {
			_ = store.MarkIndexJobFailed(ctx, db, job.JobID, spawnErr.Error())
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
			Items:     []store.IndexJob{job},
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
		Items:     []store.IndexJob{resultJob},
		Stats:     result.Stats,
		Warnings:  result.Warnings,
	}, nil
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
		Items:     []store.IndexJob{job},
		Stats:     result.Stats,
		Warnings:  result.Warnings,
	}, nil
}

func (a *App) indexStatus(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, err := a.resolveIndexWorkspace(request)
	if err != nil {
		return model.Envelope{}, err
	}
	db, err := store.Open(registration.Root)
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
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "index-job", Mode: job.Mode, Items: []store.IndexJob{job}}, nil
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
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()
	force, _ := request.Payload["force"].(bool)
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
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "index-job", Mode: job.Mode, Items: []store.IndexJob{job}, Warnings: warnings}, nil
}

func (a *App) runIndexJob(ctx context.Context, registration model.WorkspaceRegistration, jobID string) (store.IndexJob, indexer.Result, error) {
	db, err := store.Open(registration.Root)
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
	if job.Status == store.IndexJobCanceled {
		return job, indexer.Result{Warnings: []string{"index job was already canceled"}}, nil
	}
	if job.Status == store.IndexJobSucceeded {
		return job, indexer.Result{Warnings: []string{"index job already succeeded"}}, nil
	}

	var result indexer.Result
	err = store.WithWorkspaceIndexLock(registration.Root, "index."+job.Mode, func() error {
		if err := store.MarkIndexJobRunning(ctx, db, jobID, os.Getpid(), "indexing"); err != nil {
			return err
		}
		progress := newIndexJobProgressReporter(db, jobID)
		if err := progress.report(ctx, indexer.Progress{Stage: "indexing", Force: true}); err != nil {
			return err
		}
		switch job.Mode {
		case store.IndexModeDocs:
			result, err = indexer.IndexWorkspaceDocsOnlyWithProgress(ctx, registration.Root, job.GenerationID, progress.report)
		case store.IndexModeCatalog:
			result, err = indexer.IndexWorkspaceCatalogOnlyWithProgress(ctx, registration.Root, job.Clean, job.GenerationID, progress.report)
		default:
			hasExistingCatalog := false
			if stats, statsErr := store.WorkspaceStats(ctx, db); statsErr == nil {
				hasExistingCatalog = stats.Files > 0 || stats.Symbols > 0
			}
			if !job.Clean && hasExistingCatalog {
				result, err = indexer.IncrementalIndex(ctx, registration.Root)
				if err == nil {
					warning := "incremental=true"
					if result.Stats.Files == 0 {
						warning = "no changes detected"
					}
					result.Warnings = appendStringIfMissing(result.Warnings, warning)
					if genErr := store.MarkIndexGenerationSkipped(ctx, db, jobID, "incremental update did not publish a full generation"); genErr != nil {
						return genErr
					}
					if err := progress.report(ctx, indexer.Progress{Stage: "done", Files: result.Stats.Files, Symbols: result.Stats.Symbols, Docs: result.Docs, Force: true}); err != nil {
						return err
					}
					return store.MarkIndexJobSucceeded(ctx, db, jobID, result.Stats.Files, result.Stats.Symbols, result.Docs)
				}
			}
			result, err = indexer.IndexWorkspaceWithProgress(ctx, registration.Root, job.Clean, job.GenerationID, progress.report)
		}
		if err != nil {
			return err
		}
		if err := progress.report(ctx, indexer.Progress{Stage: "publishing", Files: result.Stats.Files, Symbols: result.Stats.Symbols, Docs: result.Docs, Force: true}); err != nil {
			return err
		}
		if err := store.MarkIndexJobPhase(ctx, db, jobID, store.IndexJobPublishing, "publishing"); err != nil {
			return err
		}
		return store.MarkIndexJobSucceeded(ctx, db, jobID, result.Stats.Files, result.Stats.Symbols, result.Docs)
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
		_ = store.MarkIndexJobFailed(ctx, db, jobID, err.Error())
		return store.IndexJob{}, indexer.Result{}, err
	}
	job, _, err = store.GetIndexJob(ctx, db, jobID)
	return job, result, err
}

type indexJobProgressReporter struct {
	db              *sql.DB
	jobID           string
	interval        time.Duration
	cancelInterval  time.Duration
	lastProgressAt  time.Time
	lastCancelCheck time.Time
	lastStage       string
}

func newIndexJobProgressReporter(db *sql.DB, jobID string) *indexJobProgressReporter {
	return &indexJobProgressReporter{
		db:             db,
		jobID:          jobID,
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
			if err := store.MarkIndexJobCanceled(ctx, r.db, r.jobID); err != nil {
				return err
			}
			return errIndexJobCanceled
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
	}); err != nil {
		return err
	}
	r.lastProgressAt = now
	r.lastStage = progress.Stage
	return nil
}

func (a *App) spawnIndexJob(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, jobID string) (int, error) {
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
	cmd.Dir = registration.Root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "MI_LSP_CLIENT_NAME=mi-lsp-index-job")
	processutil.ConfigureDetachedCommand(cmd)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	if err := store.MarkIndexJobRunning(ctx, db, jobID, pid, "spawned"); err != nil {
		return pid, err
	}
	return pid, nil
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

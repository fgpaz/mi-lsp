package indexer

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// IndexMode selects the indexing strategy for a background job.
type IndexMode int

const (
	// IndexModeFull rebuilds the whole index.
	IndexModeFull IndexMode = iota
	// IndexModeIncremental indexes only git-changed files when a prior index exists.
	IndexModeIncremental
)

// IndexJobState is the observable state of a background index job.
type IndexJobState struct {
	JobID string `json:"job_id"`
	Phase string `json:"phase"`
	Done  bool   `json:"done"`
	Err   string `json:"err,omitempty"`
}

// jobRegistry tracks background indexing jobs.
type jobRegistry struct {
	mu    sync.RWMutex
	jobs  map[string]IndexJobState
}

var jobs = &jobRegistry{
	jobs: make(map[string]IndexJobState),
}

// newJobID generates a unique job ID based on root and current timestamp.
func newJobID(root string) string {
	return fmt.Sprintf("job-%d-%s", os.Getpid(), fmt.Sprintf("%d", time.Now().UnixNano()%1e9))
}

// set stores or updates a job in the registry.
func (jr *jobRegistry) set(jobID string, state IndexJobState) {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	jr.jobs[jobID] = state
}

// get retrieves a job from the registry.
func (jr *jobRegistry) get(jobID string) (IndexJobState, bool) {
	jr.mu.RLock()
	defer jr.mu.RUnlock()
	state, ok := jr.jobs[jobID]
	return state, ok
}

// finish marks a job as done with an optional error.
func (jr *jobRegistry) finish(jobID string, err error) {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	if state, ok := jr.jobs[jobID]; ok {
		state.Done = true
		state.Phase = "done"
		if err != nil {
			state.Err = err.Error()
		}
		jr.jobs[jobID] = state
	}
}

// indexTimeout returns the configured index timeout (default 5 minutes, configurable via MI_LSP_INDEX_TIMEOUT).
func indexTimeout() time.Duration {
	if envVal := os.Getenv("MI_LSP_INDEX_TIMEOUT"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			return d
		}
	}
	return 5 * time.Minute
}

// StartBackgroundIndex starts an async index job and returns its jobID immediately.
//
// The job runs in a background goroutine with per-stage timeouts. The state is
// tracked in the package-level job registry and can be queried via IndexJobStatus.
// The mode parameter determines whether to do a full index or incremental (git-aware).
//
// Lock acquisition is handled by the caller (workspace_ops) with timeout and degradation.
func StartBackgroundIndex(ctx context.Context, root string, clean bool, mode IndexMode) (string, error) {
	jobID := newJobID(root)
	jobs.set(jobID, IndexJobState{
		JobID: jobID,
		Phase: "queued",
		Done:  false,
	})

	// Spawn background goroutine
	go func() {
		// Create a context with timeout for the entire indexing operation
		ic, cancel := context.WithTimeout(context.WithoutCancel(ctx), indexTimeout())
		defer cancel()

		// Update phase to "running"
		jobs.set(jobID, IndexJobState{
			JobID: jobID,
			Phase: "running",
			Done:  false,
		})

		var err error

		// Run the appropriate indexing path based on mode
		if mode == IndexModeIncremental {
			// Incremental path: use git-aware diff indexing
			_, err = IndexWorkspace(ic, root, false)
		} else {
			// Full path: complete rebuild
			_, err = IndexWorkspace(ic, root, clean)
		}

		// Mark job as finished
		jobs.finish(jobID, err)
	}()

	return jobID, nil
}

// IndexJobStatus returns the state of a background index job.
//
// The returned bool indicates whether the job was found in the registry.
func IndexJobStatus(jobID string) (IndexJobState, bool) {
	return jobs.get(jobID)
}

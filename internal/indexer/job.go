package indexer

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/store"
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
	mu     sync.RWMutex
	jobs   map[string]IndexJobState
	paused bool
	lastCompletion time.Time
}

var jobs = &jobRegistry{
	jobs: make(map[string]IndexJobState),
}

// launchBackgroundIndex is a private seam; production always runs the job asynchronously.
var launchBackgroundIndex = func(run func()) { go run() }

// TryPauseBackgroundJobs excludes new background jobs only when the registry is known idle.
// The returned resume is idempotent and cannot release a later pause.
func TryPauseBackgroundJobs() (func(), bool) {
	registry := jobs
	if registry == nil {
		return nil, false
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.jobs == nil || registry.paused {
		return nil, false
	}
	for _, job := range registry.jobs {
		if !job.Done {
			return nil, false
		}
	}
	registry.paused = true
	var once sync.Once
	return func() { once.Do(func() { registry.mu.Lock(); registry.paused = false; registry.mu.Unlock() }) }, true
}

// BackgroundJobLastCompletion returns the actual latest admitted job completion.
func BackgroundJobLastCompletion() time.Time {
	registry := jobs
	if registry == nil {
		return time.Time{}
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.lastCompletion
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
		jr.lastCompletion = time.Now()
	}
}

// indexTimeout returns the configured index timeout (default 30 minutes, configurable via MI_LSP_INDEX_TIMEOUT).
func indexTimeout() time.Duration {
	if envVal := os.Getenv("MI_LSP_INDEX_TIMEOUT"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			return d
		}
	}
	return 30 * time.Minute
}

// IndexTimeout returns the configured index timeout, used to bound a synchronous
// auto-index so it cannot hang indefinitely (AUD-01). Configurable via
// MI_LSP_INDEX_TIMEOUT; default 30 minutes.
func IndexTimeout() time.Duration { return indexTimeout() }

// EmbeddingIndexTimeout estimates a full index budget when the workspace has
// embeddings enabled. The embeddings phase renews the HTTP timeout per batch,
// so the outer CLI/job budget must scale with the number of expected batches
// instead of using a fixed short deadline.
func EmbeddingIndexTimeout(totalChunks int, batchSize int, requestTimeout time.Duration, configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	base := IndexTimeout()
	if totalChunks <= 0 {
		return base
	}
	if batchSize <= 0 {
		batchSize = 32
	}
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	perBatch := requestTimeout
	if perBatch > 15*time.Second {
		perBatch = 15 * time.Second
	}
	if perBatch < 5*time.Second {
		perBatch = 5 * time.Second
	}
	batches := (totalChunks + batchSize - 1) / batchSize
	budget := 15*time.Minute + time.Duration(batches)*perBatch
	if budget < base {
		budget = base
	}
	if max := maxEmbeddingIndexTimeout(); max > 0 && budget > max {
		return max
	}
	return budget
}

func maxEmbeddingIndexTimeout() time.Duration {
	if envVal := os.Getenv("MI_LSP_INDEX_MAX_TIMEOUT"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			return d
		}
	}
	return 6 * time.Hour
}

// SmartSyncTimeout returns the short window during which workspace.add/init index
// synchronously before degrading to a background job (hybrid smart-sync, FD1).
// Configurable via MI_LSP_INDEX_SYNC_TIMEOUT; default 20s. Small/incremental repos
// finish well within it (preserving init-then-query); very large first indexes exceed
// it and continue in the background returning a job_id.
func SmartSyncTimeout() time.Duration {
	if envVal := os.Getenv("MI_LSP_INDEX_SYNC_TIMEOUT"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			return d
		}
	}
	return 20 * time.Second
}

// StartBackgroundIndex starts an async index job and returns its jobID immediately.
//
// The job runs in a background goroutine with per-stage timeouts. The state is
// tracked in the package-level job registry and can be queried via IndexJobStatus.
// The mode parameter determines whether to do a full index or incremental (git-aware).
//
// Lock acquisition is handled by the caller (workspace_ops) with timeout and degradation.
func StartBackgroundIndex(ctx context.Context, root string, clean bool, mode IndexMode) (string, error) {
	registry := jobs
	if registry == nil {
		return "", fmt.Errorf("background indexing unavailable")
	}
	registry.mu.Lock()
	if registry.jobs == nil || registry.paused {
		registry.mu.Unlock()
		return "", fmt.Errorf("background indexing unavailable")
	}
	jobID := newJobID(root)
	if _, exists := registry.jobs[jobID]; exists {
		base := jobID
		for suffix := 1; ; suffix++ {
			jobID = fmt.Sprintf("%s-%d", base, suffix)
			if _, exists = registry.jobs[jobID]; !exists {
				break
			}
		}
	}
	registry.jobs[jobID] = IndexJobState{JobID: jobID, Phase: "queued"}
	registry.mu.Unlock()

	// The queued state is visible before launch and remains active until finish.
	launchBackgroundIndex(func() {
		// Create a context with timeout for the entire indexing operation
		ic, cancel := context.WithTimeout(context.WithoutCancel(ctx), indexTimeout())
		defer cancel()

		// Update phase to "running"
		registry.set(jobID, IndexJobState{
			JobID: jobID,
			Phase: "running",
			Done:  false,
		})

		// Acquire the workspace index lock so the background index serializes with
		// other indexers (a degraded sync attempt has already released its lock).
		err := store.WithWorkspaceIndexLock(root, "workspace.background-index", func() error {
			if mode == IndexModeIncremental {
				// Incremental path: git-aware diff indexing.
				_, e := IndexWorkspace(ic, root, false)
				return e
			}
			// Full path: complete rebuild.
			_, e := IndexWorkspace(ic, root, clean)
			return e
		})

		// Mark job as finished
		registry.finish(jobID, err)
	})

	return jobID, nil
}

// IndexJobStatus returns the state of a background index job.
//
// The returned bool indicates whether the job was found in the registry.
func IndexJobStatus(jobID string) (IndexJobState, bool) {
	return jobs.get(jobID)
}

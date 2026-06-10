package indexer

import (
	"context"
	"errors"
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

// StartBackgroundIndex starts an async index job and returns its jobID immediately.
//
// Wave 0 stub: L1 (async-indexing lane) replaces this body with the real
// goroutine + per-stage timeout + job registry implementation. The signature is
// the locked cross-lane interface consumed by the daemon core (L2).
func StartBackgroundIndex(ctx context.Context, root string, clean bool, mode IndexMode) (string, error) {
	return "", errors.New("not implemented: StartBackgroundIndex")
}

// IndexJobStatus returns the state of a background index job.
//
// Wave 0 stub: L1 replaces this with a real lookup against the job registry.
func IndexJobStatus(jobID string) (IndexJobState, bool) {
	return IndexJobState{}, false
}

package nav

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// WikiFanOutOptions configures the behavior of FanOutWiki.
type WikiFanOutOptions struct {
	// WorkspaceFilter restricts the fan-out to specific workspace names.
	// If empty, all registered workspaces are queried.
	WorkspaceFilter []string

	// Timeout is the per-workspace timeout for the fan-out operation.
	// Defaults to 30 seconds.
	Timeout time.Duration

	// Parallel is the maximum number of concurrent goroutines.
	// Defaults to 4 (derived from internal/service/ask.go AllWorkspaces pattern).
	Parallel int
}

// WorkspaceFailure describes a failure in workspace-scoped operation.
type WorkspaceFailure struct {
	Alias  string // workspace name
	Reason string // error message
}

// WikiFanOutItem is the result from a single workspace in the fan-out.
type WikiFanOutItem struct {
	Workspace string         // workspace name
	Items     []any          // results from the operation
	Stats     map[string]any // optional diagnostic stats
	Err       error          // error (if any) during the operation
}

// WikiFanOutResult is the aggregated result of FanOutWiki.
type WikiFanOutResult struct {
	// Items is the list of per-workspace results.
	Items []WikiFanOutItem

	// WorkspacesQueried is the total number of workspaces attempted.
	WorkspacesQueried int

	// WorkspacesFailed lists workspaces that encountered errors.
	WorkspacesFailed []WorkspaceFailure

	// TruncatedPerWS indicates if any workspace result was truncated.
	TruncatedPerWS bool
}

// FanOutWiki fans out a operation across registered workspaces with bounded concurrency.
//
// The function:
// - Lists all registered workspaces (or filters by WorkspaceFilter if provided).
// - For each workspace, launches a bounded goroutine (semaphore=Parallel, default 4).
// - Calls fn(ctx, ws) with a timeout context (Timeout, default 30s).
// - Captures errors in WorkspacesFailed and does NOT abort on per-workspace failures.
// - Returns aggregated results.
//
// Usage (for a wiki subcommand):
//
//	result, err := FanOutWiki(ctx, WikiFanOutOptions{}, func(ctx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
//	    // ws is a single workspace; consult your doc index and return results
//	    items, stats, err := queryWikiDocIndex(ctx, ws)
//	    return items, stats, err
//	})
//
// If fn is nil, returns an error.
func FanOutWiki(
	ctx context.Context,
	opts WikiFanOutOptions,
	fn func(ctx context.Context, ws model.WorkspaceRegistration) (items []any, stats map[string]any, err error),
) (*WikiFanOutResult, error) {
	if fn == nil {
		return nil, fmt.Errorf("FanOutWiki: fn must not be nil")
	}

	// Set defaults
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Parallel <= 0 {
		opts.Parallel = 4
	}

	// List workspaces
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Filter if needed
	if len(opts.WorkspaceFilter) > 0 {
		filterMap := make(map[string]bool)
		for _, name := range opts.WorkspaceFilter {
			filterMap[name] = true
		}
		var filtered []model.WorkspaceRegistration
		for _, ws := range workspaces {
			if filterMap[ws.Name] {
				filtered = append(filtered, ws)
			}
		}
		workspaces = filtered
	}

	if len(workspaces) == 0 {
		return &WikiFanOutResult{
			Items:             []WikiFanOutItem{},
			WorkspacesQueried: 0,
			WorkspacesFailed:  []WorkspaceFailure{},
		}, nil
	}

	// Fan-out with bounded concurrency
	type result struct {
		ws    model.WorkspaceRegistration
		items []any
		stats map[string]any
		err   error
	}

	resultsChan := make(chan result, len(workspaces))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, opts.Parallel)

	for _, ws := range workspaces {
		wg.Add(1)
		go func(wsReg model.WorkspaceRegistration) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create a timeout context for this workspace
			subCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
			defer cancel()

			// Call the user's function
			items, stats, err := fn(subCtx, wsReg)
			resultsChan <- result{
				ws:    wsReg,
				items: items,
				stats: stats,
				err:   err,
			}
		}(ws)
	}

	// Wait for all goroutines
	wg.Wait()
	close(resultsChan)

	// Aggregate results
	var items []WikiFanOutItem
	var failed []WorkspaceFailure

	for res := range resultsChan {
		item := WikiFanOutItem{
			Workspace: res.ws.Name,
			Items:     res.items,
			Stats:     res.stats,
			Err:       res.err,
		}
		items = append(items, item)

		if res.err != nil {
			failed = append(failed, WorkspaceFailure{
				Alias:  res.ws.Name,
				Reason: res.err.Error(),
			})
		}
	}

	return &WikiFanOutResult{
		Items:             items,
		WorkspacesQueried: len(workspaces),
		WorkspacesFailed:  failed,
		TruncatedPerWS:    false, // subcommands can set this if they apply per-ws limits
	}, nil
}

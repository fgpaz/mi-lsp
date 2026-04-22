package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type PerfSmokeOptions struct {
	Callers        int
	MaxWorkingSet  uint64
	MaxPrivate     uint64
	MaxHandles     uint64
	StartOptions   StartOptions
	MaxWorkers     int
	IdleTimeout    time.Duration
	RequestTimeout time.Duration
}

type PerfSmokeResult struct {
	Callers       int                      `json:"callers"`
	Failures      int                      `json:"failures"`
	DaemonState   model.DaemonState        `json:"state"`
	DaemonProcess model.DaemonProcessStats `json:"daemon_process"`
	Watchers      model.DaemonWatcherStats `json:"watchers"`
	Passed        bool                     `json:"passed"`
	Warnings      []string                 `json:"warnings,omitempty"`
}

func RunPerfSmoke(ctx context.Context, repoRoot string, options PerfSmokeOptions) (PerfSmokeResult, error) {
	if options.Callers <= 0 {
		options.Callers = 16
	}
	if options.MaxWorkers <= 0 {
		options.MaxWorkers = 3
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = 30 * time.Minute
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 5 * time.Second
	}
	startOptions := NormalizeStartOptions(options.StartOptions)
	state, _, err := SpawnBackgroundWithOptions(repoRoot, options.MaxWorkers, options.IdleTimeout, startOptions)
	if err != nil {
		return PerfSmokeResult{}, err
	}

	failures := 0
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < options.Callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(ctx, options.RequestTimeout)
			defer cancel()
			response, err := NewClient().Execute(reqCtx, model.CommandRequest{
				ProtocolVersion: model.ProtocolVersion,
				Operation:       "system.status",
				Context:         model.QueryOptions{ClientName: "mi-lsp-perf-smoke", Format: "json"},
			})
			if err != nil || !response.Ok {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	status, err := daemonStatusSnapshot(ctx, options.RequestTimeout)
	if err != nil {
		return PerfSmokeResult{}, err
	}
	status.Callers = options.Callers
	status.Failures = failures
	status.DaemonState = state
	status.Passed = failures == 0
	if options.MaxWorkingSet > 0 && status.DaemonProcess.WorkingSetBytes > options.MaxWorkingSet {
		status.Passed = false
		status.Warnings = append(status.Warnings, fmt.Sprintf("working_set_bytes=%d exceeds %d", status.DaemonProcess.WorkingSetBytes, options.MaxWorkingSet))
	}
	if options.MaxPrivate > 0 && status.DaemonProcess.PrivateBytes > options.MaxPrivate {
		status.Passed = false
		status.Warnings = append(status.Warnings, fmt.Sprintf("private_bytes=%d exceeds %d", status.DaemonProcess.PrivateBytes, options.MaxPrivate))
	}
	if options.MaxHandles > 0 && status.DaemonProcess.HandleCount > options.MaxHandles {
		status.Passed = false
		status.Warnings = append(status.Warnings, fmt.Sprintf("handle_count=%d exceeds %d", status.DaemonProcess.HandleCount, options.MaxHandles))
	}
	return status, nil
}

func daemonStatusSnapshot(ctx context.Context, timeout time.Duration) (PerfSmokeResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := NewClient().Execute(reqCtx, model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "system.status",
		Context:         model.QueryOptions{ClientName: "mi-lsp-perf-smoke", Format: "json"},
	})
	if err != nil {
		return PerfSmokeResult{}, err
	}
	body, err := json.Marshal(response.Items)
	if err != nil {
		return PerfSmokeResult{}, err
	}
	var decoded []struct {
		State         model.DaemonState        `json:"state"`
		DaemonProcess model.DaemonProcessStats `json:"daemon_process"`
		Watchers      model.DaemonWatcherStats `json:"watchers"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded) == 0 {
		return PerfSmokeResult{}, errors.New("daemon status did not return process stats")
	}
	return PerfSmokeResult{
		DaemonState:   decoded[0].State,
		DaemonProcess: decoded[0].DaemonProcess,
		Watchers:      decoded[0].Watchers,
	}, nil
}

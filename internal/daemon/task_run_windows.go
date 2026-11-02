//go:build windows
// +build windows

package daemon

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

type taskRunPhase uint8

const (
	taskRunNotEntered taskRunPhase = iota
	taskRunGUIDKnown
	taskRunUncertain
)

type taskRunRequest struct {
	TaskPath          string
	ExpectedXMLSHA256 string
	Nonce             string
}

type taskRunResult struct {
	Phase        taskRunPhase
	InstanceGUID string
	Err          error
}

var (
	taskRunHashRE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	taskRunNonceRE = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// invokeTaskRunContext is intentionally uncalled by product code. A future
// caller must establish Task ownership, current-user/private-root custody,
// definition/image identity and durable replay exclusion before invoking it.
func invokeTaskRunContext(ctx context.Context, request taskRunRequest) taskRunResult {
	return invokeTaskRunWithFactory(ctx, request, newTaskRunCOMSession)
}

type taskRunSession interface {
	Preflight(context.Context, taskRunRequest) error
	PrepareRun(context.Context) error
	Run(string) (string, error)
	Close() error
}

type taskRunSessionFactory func() (taskRunSession, error)

func invokeTaskRunWithFactory(ctx context.Context, request taskRunRequest, factory taskRunSessionFactory) taskRunResult {
	if ctx == nil {
		return taskRunResult{Phase: taskRunNotEntered, Err: errors.New("nil context")}
	}
	if factory == nil {
		return taskRunResult{Phase: taskRunNotEntered, Err: errors.New("nil COM session factory")}
	}
	if !validTaskSchedulerPath(request.TaskPath) || !taskRunHashRE.MatchString(request.ExpectedXMLSHA256) || !taskRunNonceRE.MatchString(request.Nonce) {
		return taskRunResult{Phase: taskRunNotEntered, Err: errors.New("invalid Task Run request")}
	}
	if err := ctx.Err(); err != nil {
		return taskRunResult{Phase: taskRunNotEntered, Err: err}
	}

	// This state mutex is the single arbitration point between cancellation and
	// the Run boundary. aborted prevents any late preflight worker from entering.
	var mu sync.Mutex
	entered, aborted := false, false
	stopErr := func() error {
		mu.Lock()
		defer mu.Unlock()
		if err := ctx.Err(); err != nil {
			aborted = true
			return err
		}
		if aborted {
			return context.Canceled
		}
		return nil
	}
	completed := make(chan taskRunResult, 1)
	go func() {
		result := taskRunResult{Phase: taskRunNotEntered}
		// Cooperative gate immediately before factory. Cancellation can still
		// race the call itself; each subsequent stage has its own gate.
		if err := stopErr(); err != nil {
			result.Err = err
			completed <- result
			return
		}
		session, err := factory()
		if err != nil {
			if cancelErr := stopErr(); cancelErr != nil { result.Err = errors.Join(err, cancelErr) } else { result.Err = err }
			completed <- result
			return
		}
		if session == nil {
			result.Err = errors.New("COM session factory returned nil session")
			completed <- result
			return
		}
		defer func() {
			if closeErr := session.Close(); closeErr != nil {
				if result.Err == nil { result.Err = fmt.Errorf("COM cleanup: %w", closeErr) } else { result.Err = errors.Join(result.Err, fmt.Errorf("COM cleanup: %w", closeErr)) }
				if result.Phase == taskRunGUIDKnown { result.Phase, result.InstanceGUID = taskRunUncertain, "" }
			}
			completed <- result
		}()
		if err := stopErr(); err != nil { result.Err = err; return }
		if err := session.Preflight(ctx, request); err != nil {
			if cancelErr := stopErr(); cancelErr != nil { result.Err = errors.Join(err, cancelErr) } else { result.Err = err }
			return
		}
		if err := stopErr(); err != nil { result.Err = err; return }
		// Resolve the dispatch name before the cancellation/entry arbitration.
		// The COM adapter also checks context between its setup calls.
		if err := session.PrepareRun(ctx); err != nil {
			if cancelErr := stopErr(); cancelErr != nil { result.Err = errors.Join(err, cancelErr) } else { result.Err = err }
			return
		}
		mu.Lock()
		if aborted || ctx.Err() != nil {
			aborted = true
			mu.Unlock()
			result.Err = ctx.Err()
			if result.Err == nil { result.Err = context.Canceled }
			return
		}
		// Mark the invocation boundary immediately before the synchronous COM Run.
		entered = true
		mu.Unlock()
		guid, err := session.Run(request.Nonce)
		mu.Lock()
		wasAborted := aborted || ctx.Err() != nil
		mu.Unlock()
		if err != nil {
			result.Phase, result.Err = taskRunUncertain, err
			return
		}
		if wasAborted || !validTaskInstanceGUID(guid) {
			result.Phase = taskRunUncertain
			if wasAborted { result.Err = context.Canceled } else { result.Err = errors.New("Task Run returned invalid InstanceGuid") }
			return
		}
		result.Phase, result.InstanceGUID = taskRunGUIDKnown, guid
	}()

	select {
	case result := <-completed:
		mu.Lock()
		cancelled := aborted || ctx.Err() != nil
		wasEntered := entered
		if cancelled { aborted = true }
		mu.Unlock()
		if cancelled && wasEntered {
			return taskRunResult{Phase: taskRunUncertain, Err: ctx.Err()}
		}
		if cancelled {
			return taskRunResult{Phase: taskRunNotEntered, Err: ctx.Err()}
		}
		return result
	case <-ctx.Done():
		mu.Lock()
		aborted = true
		wasEntered := entered
		mu.Unlock()
		if wasEntered { return taskRunResult{Phase: taskRunUncertain, Err: ctx.Err()} }
		return taskRunResult{Phase: taskRunNotEntered, Err: ctx.Err()}
	}
}

func validTaskSchedulerPath(s string) bool {
	if len(s) < 2 || s[0] != '\\' || strings.ContainsAny(s, "/\x00") || strings.HasSuffix(s, "\\") { return false }
	for _, part := range strings.Split(s[1:], "\\") {
		if part == "" || part == "." || part == ".." { return false }
	}
	return true
}

func validTaskInstanceGUID(s string) bool {
	if len(s) != 38 || s[0] != '{' || s[37] != '}' { return false }
	for i, c := range s[1:37] {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' { return false }
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) { return false }
	}
	return s != "{00000000-0000-0000-0000-000000000000}"
}

// Deliberately no new budget is introduced here; ctx is the caller's budget.

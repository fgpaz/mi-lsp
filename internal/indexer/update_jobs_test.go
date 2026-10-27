package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Tests isolate the real registry; no tests here run in parallel. The private
// launchBackgroundIndex seam captures the actual job closure without running IO.
func isolateUpdateJobs(t *testing.T) {
	t.Helper()
	previousJobs, previousLaunch := jobs, launchBackgroundIndex
	jobs = &jobRegistry{jobs: make(map[string]IndexJobState)}
	t.Cleanup(func() { jobs = previousJobs; launchBackgroundIndex = previousLaunch })
}

func TestPauseBackgroundJobsVetoesQueuedAndRunning(t *testing.T) {
	for _, phase := range []string{"queued", "running"} {
		t.Run(phase, func(t *testing.T) {
			isolateUpdateJobs(t)
			jobs.set("active", IndexJobState{JobID: "active", Phase: phase})
			resume, idle := TryPauseBackgroundJobs()
			if idle || resume != nil {
				t.Fatalf("%s must veto pause", phase)
			}
			state, ok := IndexJobStatus("active")
			if !ok || state.Done || state.Phase != phase {
				t.Fatalf("registry mutated: %+v, %v", state, ok)
			}
			jobs.finish("active", nil)
			resume, idle = TryPauseBackgroundJobs()
			if !idle || resume == nil {
				t.Fatal("finished job must allow pause")
			}
			resume()
		})
	}
}

func TestPauseBackgroundJobsUnknownAndFinished(t *testing.T) {
	isolateUpdateJobs(t)
	jobs = &jobRegistry{}
	resume, idle := TryPauseBackgroundJobs()
	if idle || resume != nil {
		t.Fatal("uninitialized registry is unknown")
	}
	jobs = &jobRegistry{jobs: map[string]IndexJobState{"done": {JobID: "done", Phase: "done", Done: true}}}
	resume, idle = TryPauseBackgroundJobs()
	if !idle || resume == nil {
		t.Fatal("finished-only registry should pause")
	}
	resume()
}

func TestPausedAdmissionNoJobOrFilesystemActivity(t *testing.T) {
	isolateUpdateJobs(t)
	launched := 0
	launchBackgroundIndex = func(func()) { launched++ }
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	resume, idle := TryPauseBackgroundJobs()
	if !idle || resume == nil {
		t.Fatal("expected idle pause")
	}
	defer resume()
	id, err := StartBackgroundIndex(context.Background(), root, false, IndexModeFull)
	if err == nil || id != "" || strings.Contains(err.Error(), root) {
		t.Fatalf("rejection must be sanitized: id=%q err=%v", id, err)
	}
	if launched != 0 || len(jobs.jobs) != 0 {
		t.Fatalf("paused start registered or launched: %d %v", launched, jobs.jobs)
	}
	if _, err = os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("paused start accessed fixture root: %v", err)
	}
}

func TestPauseResumeIdempotentAndAllowsAdmission(t *testing.T) {
	isolateUpdateJobs(t)
	var callbacks []func()
	launchBackgroundIndex = func(fn func()) { callbacks = append(callbacks, fn) }
	resume, idle := TryPauseBackgroundJobs()
	if !idle || resume == nil {
		t.Fatal("pause failed")
	}
	second, ok := TryPauseBackgroundJobs()
	if ok || second != nil {
		t.Fatal("second pause must not replace first")
	}
	resume()
	resume()
	id, err := StartBackgroundIndex(context.Background(), filepath.Join(t.TempDir(), "fixture"), false, IndexModeFull)
	if err != nil || id == "" || len(callbacks) != 1 {
		t.Fatalf("admission id=%q err=%v callbacks=%d", id, err, len(callbacks))
	}
	state, ok := IndexJobStatus(id)
	if !ok || state.Done || state.Phase != "queued" {
		t.Fatalf("queued state=%+v ok=%v", state, ok)
	}
	pause, ok := TryPauseBackgroundJobs()
	if ok || pause != nil {
		t.Fatal("queued job must veto")
	}
	jobs.finish(id, nil)
	later, ok := TryPauseBackgroundJobs()
	if !ok || later == nil {
		t.Fatal("completed job must allow pause")
	}
	resume()
	_, err = StartBackgroundIndex(context.Background(), "unused", false, IndexModeFull)
	if err == nil {
		t.Fatal("old resume must not release later barrier")
	}
	later()
}

func TestStartAndPauseAreMutuallyExclusive(t *testing.T) {
	isolateUpdateJobs(t)
	var mu sync.Mutex
	var callbacks []func()
	launchBackgroundIndex = func(fn func()) { mu.Lock(); callbacks = append(callbacks, fn); mu.Unlock() }
	for i := 0; i < 100; i++ {
		start := make(chan struct{})
		type outcome struct {
			id  string
			err error
		}
		started := make(chan outcome, 1)
		type pauseResult struct {
			resume func()
			idle   bool
		}
		paused := make(chan pauseResult, 1)
		go func() {
			<-start
			id, err := StartBackgroundIndex(context.Background(), "unused", false, IndexModeFull)
			started <- outcome{id, err}
		}()
		go func() { <-start; resume, idle := TryPauseBackgroundJobs(); paused <- pauseResult{resume, idle} }()
		close(start)
		s, p := <-started, <-paused
		if p.idle {
			if p.resume == nil || s.err == nil || s.id != "" {
				t.Fatalf("pause admitted concurrent job: start=%+v", s)
			}
			p.resume()
		} else {
			if p.resume != nil || s.err != nil || s.id == "" {
				t.Fatalf("job admission did not veto pause: start=%+v", s)
			}
			state, ok := IndexJobStatus(s.id)
			if !ok || state.Done {
				t.Fatalf("queued job not observable: %+v", state)
			}
			jobs.finish(s.id, nil)
		}
	}
}

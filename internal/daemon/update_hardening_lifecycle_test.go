package daemon

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
)

func TestUpdateIndexerPauseRejectsNewJobWithoutWorkspaceWrites(t *testing.T) {
	root := t.TempDir()
	release, idle := indexer.TryPauseBackgroundJobs()
	if !idle {
		t.Fatal("fresh process indexer unexpectedly busy")
	}
	defer release()
	if _, err := indexer.StartBackgroundIndex(context.Background(), root, false, indexer.IndexModeFull); err == nil {
		t.Fatal("job admitted while indexer barrier held")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("paused admission touched workspace: %v %v", entries, err)
	}
}

func TestUpdateBackgroundBarrierAbortsAndReleasesAdmissions(t *testing.T) {
	// The actual controller acquires the indexer API barrier through this private
	// injection point; this fake records its ownership, not an indexer stub.
	paused := false
	releaseCalls := 0
	gate := newUpdateAdmissionGate()
	held, ok := gate.Begin("admin-http")
	if !ok {
		t.Fatal("admission failed")
	}
	releaseJobs := func() { paused = false; releaseCalls++ }
	idle := tryPauseUpdateResources(gate, func() (func(), bool) { paused = true; return releaseJobs, true })
	if idle != nil {
		idle()
		t.Fatal("controller admitted restart with active handler")
	}
	if paused || releaseCalls != 1 {
		t.Fatalf("index admissions not released on abort: paused=%v calls=%d", paused, releaseCalls)
	}
	held()
	idle = tryPauseUpdateResources(gate, func() (func(), bool) { paused = true; return releaseJobs, true })
	if idle == nil || !paused {
		t.Fatal("idle controller failed to hold job barrier")
	}
	if _, admitted := gate.Begin("admin-http"); admitted {
		t.Fatal("HTTP admitted during job barrier")
	}
	idle()
	if paused || releaseCalls != 2 {
		t.Fatal("successful release did not resume job admissions")
	}
}

func TestUpdateBackgroundQueuedJobDefersButFinishedJobDoesNot(t *testing.T) {
	gate := newUpdateAdmissionGate()
	if release := tryPauseUpdateResources(gate, func() (func(), bool) { return nil, false }); release != nil {
		release()
		t.Fatal("queued job reported idle")
	}
	called := false
	release := tryPauseUpdateResources(gate, func() (func(), bool) { return func() { called = true }, true })
	if release == nil {
		t.Fatal("finished job caused permanent veto")
	}
	release()
	if !called {
		t.Fatal("indexer barrier not released")
	}
}

func TestUpdateRetentionStopJoinsActivePurge(t *testing.T) {
	gate := newUpdateAdmissionGate()
	started, unblock, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	retention := newUpdateRetention(gate, func() { close(started); <-unblock; close(finished) })
	retention.startForTest()
	<-started
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("purge active during idle barrier")
	}
	stopped := make(chan struct{})
	go func() { retention.stopAndWait(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("stop returned before purge completed")
	default:
	}
	close(unblock)
	<-finished
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("retention did not join")
	}
	if release, idle := gate.TryIdleBarrier(); !idle {
		t.Fatal("completed retention remains busy")
	} else {
		release()
	}
}

func TestUpdateManagerShutdownJoinsReapBeforeReturning(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	m := newUpdateManagerWithReapForTest(func() { close(entered); <-release })
	m.triggerReapForTest()
	<-entered
	done := make(chan struct{})
	go func() { m.Shutdown(); close(done) }()
	select {
	case <-done:
		t.Fatal("shutdown returned before active reap finished")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("manager shutdown did not join reap")
	}
}

func TestUpdateSpawnFailureAttemptsOwnServiceRecovery(t *testing.T) {
	var events []string
	ops := updateRestartOps{
		CleanupOld: func() error { events = append(events, "prepare"); return nil },
		Shutdown:   func() error { events = append(events, "old-shutdown"); return nil },
		Spawn: func(updateRestartInvocation) error {
			events = append(events, "spawn")
			return errors.New("spawn failed")
		},
		Recover: func(updateRestartInvocation) error { events = append(events, "recover"); return nil },
	}
	if err := restartUpdatedDaemon(context.Background(), updateRestartInvocation{}, ops); err == nil {
		t.Fatal("spawn failure lost")
	}
	if len(events) != 4 || events[3] != "recover" {
		t.Fatalf("no recovery attempt: %v", events)
	}
}

func TestUpdateAdminBusyNeverCallsShutdown(t *testing.T) {
	called := false
	err := prepareAdminForUpdate(context.Background(), func(context.Context) (bool, error) { return false, nil }, func() error { called = true; return nil }, func() error { return nil })
	if err == nil || called {
		t.Fatalf("busy admin shutdown called=%v err=%v", called, err)
	}
	err = prepareAdminForUpdate(context.Background(), func(context.Context) (bool, error) { return false, errors.New("timeout") }, func() error { called = true; return nil }, func() error { return nil })
	if err == nil || called {
		t.Fatalf("timeout admin shutdown called=%v err=%v", called, err)
	}
}

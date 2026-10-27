package daemon

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// manualBatchTimer lets a test deliver an already-dispatched timer callback
// even after Stop, modelling the race that time.Timer.Stop cannot prevent.
type manualBatchTimer struct{}

func (*manualBatchTimer) Stop() bool { return false }
func joinWatcherFixture(t *testing.T) *FileWatcher {
	t.Helper()
	w, err := NewFileWatcher(model.WorkspaceRegistration{Root: t.TempDir()}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Stop() })
	return w
}
func waitWatcherEvent(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
func assertWatcherStopJoins(t *testing.T, w *FileWatcher, entered <-chan struct{}, release func(), callbackDone <-chan struct{}) {
	t.Helper()
	waitWatcherEvent(t, entered, "index callback")
	stopped := make(chan struct{})
	go func() { w.Stop(); close(stopped) }()
	waitWatcherEvent(t, w.stopCh, "stop admission")
	select {
	case <-stopped:
		t.Error("Stop returned while an admitted callback was still running")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	waitWatcherEvent(t, callbackDone, "callback exit")
	waitWatcherEvent(t, stopped, "Stop completion")
}
func TestWatcherStopJoinsBatchCallback(t *testing.T) {
	w := joinWatcherFixture(t)
	callbacks := make(chan func(), 1)
	w.batchTimerFactory = func(_ time.Duration, fn func()) watcherTimer { callbacks <- fn; return &manualBatchTimer{} }
	entered := make(chan struct{})
	released := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(released) }) }
	defer release()
	w.reindexBatchFn = func(events []watcherEvent) error {
		if len(events) != 1 {
			t.Errorf("events=%v", events)
		}
		close(entered)
		<-released
		return nil
	}
	w.scheduleBatchReindex(filepath.Join(w.workspaceRoot, "fixture.go"))
	var callback func()
	select {
	case callback = <-callbacks:
	case <-time.After(2 * time.Second):
		t.Fatal("no batch timer")
	}
	go func() { defer close(done); callback() }()
	assertWatcherStopJoins(t, w, entered, release, done)
}
func TestWatcherStopJoinsPerFileBatchCallback(t *testing.T) {
	w := joinWatcherFixture(t)
	callbacks := make(chan func(), 1)
	w.batchTimerFactory = func(_ time.Duration, fn func()) watcherTimer { callbacks <- fn; return &manualBatchTimer{} }
	entered := make(chan struct{})
	released := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(released) }) }
	defer release()
	w.reindexFileFn = func(string) error { close(entered); <-released; return nil }
	w.scheduleBatchReindex(filepath.Join(w.workspaceRoot, "fixture.go"))
	var callback func()
	select {
	case callback = <-callbacks:
	case <-time.After(2 * time.Second):
		t.Fatal("no batch timer")
	}
	go func() { defer close(done); callback() }()
	assertWatcherStopJoins(t, w, entered, release, done)
}
func TestWatcherStopJoinsSingleFileTimerCallback(t *testing.T) {
	w := joinWatcherFixture(t)
	entered := make(chan struct{})
	released := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(released) }) }
	defer release()
	// The single-file timer must use the existing injectable reindexFileFn too;
	// current source bypasses it, so this captures that missing regression seam.
	w.reindexFileFn = func(string) error { close(entered); <-released; close(done); return nil }
	w.scheduleReindex(filepath.Join(w.workspaceRoot, "fixture.go"))
	assertWatcherStopJoins(t, w, entered, release, done)
}
func TestWatcherLateTimerCannotIndexOrRearmAfterStop(t *testing.T) {
	w := joinWatcherFixture(t)
	callbacks := make(chan func(), 4)
	var timers atomic.Int32
	var indexes atomic.Int32
	w.batchTimerFactory = func(_ time.Duration, fn func()) watcherTimer {
		timers.Add(1)
		callbacks <- fn
		return &manualBatchTimer{}
	}
	w.reindexBatchFn = func([]watcherEvent) error { indexes.Add(1); return nil }
	path := filepath.Join(w.workspaceRoot, "fixture.go")
	w.scheduleBatchReindex(path)
	var callback func()
	select {
	case callback = <-callbacks:
	case <-time.After(2 * time.Second):
		t.Fatal("no initial timer")
	}
	w.Stop()
	done := make(chan struct{})
	go func() { callback(); close(done) }()
	waitWatcherEvent(t, done, "late timer return")
	w.scheduleBatchReindex(path)
	w.scheduleReindex(path)
	if indexes.Load() != 0 || timers.Load() != 1 || w.PendingEvents() != 0 {
		t.Fatalf("post-stop index=%d timers=%d pending=%d", indexes.Load(), timers.Load(), w.PendingEvents())
	}
}

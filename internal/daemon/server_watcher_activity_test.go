package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

// watcherActivityFixture uses the Manager's real watcher and timer, replacing
// only the batch's indexing boundary so its actual entry can be held open.
func watcherActivityFixture(t *testing.T) (*Server, *FileWatcher, <-chan error) {
	t.Helper()
	// Unix sockets need a short path; Windows named pipes do not use sun_path.
	base := ""
	if runtime.GOOS != "windows" { base = "/tmp" }
	home, err := os.MkdirTemp(base, "a3-")
	if err != nil { t.Fatalf("FIXTURE_INVALID private home: %v", err) }
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	for _, key := range []string{"HOME", "USERPROFILE", "TMP", "TMPDIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} { t.Setenv(key, home) }
	t.Setenv("USERNAME", fmt.Sprintf("a3-%d", os.Getpid()))
	root := filepath.Join(home, "w")
	if err := os.Mkdir(root, 0700); err != nil { t.Fatalf("FIXTURE_INVALID workspace: %v", err) }
	options := DefaultStartOptions()
	options.WatchMode = WatchModeLazy
	s, err := NewServerWithOptions(root, 1, 2*time.Second, options)
	if err != nil { t.Fatalf("FIXTURE_INVALID server: %v", err) }
	s.manager.EnsureFileWatcher(model.WorkspaceRegistration{Root: root})
	s.manager.mu.Lock()
	w := s.manager.watchers[canonicalWatcherRootKey(root)]
	s.manager.mu.Unlock()
	if w == nil { s.Shutdown(); t.Fatal("FIXTURE_INVALID manager did not register watcher") }
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background()) }()
	t.Cleanup(func() {
		s.Shutdown()
		select { case <-done: case <-time.After(8*time.Second): t.Error("FIXTURE_INVALID Serve did not join") }
		select { case <-s.shutdownDone: case <-time.After(8*time.Second): t.Error("FIXTURE_INVALID shutdown did not join") }
	})
	return s, w, done
}

func watcherActivityProbe(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialDaemon(ctx)
	if err != nil { t.Fatalf("FIXTURE_INVALID status dial: %v", err) }
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if err := worker.WriteFrame(conn, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status"}); err != nil { t.Fatalf("FIXTURE_INVALID status write: %v", err) }
	var response model.Envelope
	if err := worker.ReadFrame(conn, &response); err != nil || !response.Ok || response.Backend != "daemon" { t.Fatalf("FIXTURE_INVALID status response: ok=%v backend=%s err=%v", response.Ok, response.Backend, err) }
}

func TestWatcherBatchKeepsServerAlivePastIdle(t *testing.T) {
	s, w, _ := watcherActivityFixture(t)
	entered := make(chan []watcherEvent, 1)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	w.reindexBatchFn = func(events []watcherEvent) error { entered <- events; <-release; return nil }
	path := filepath.Join(w.workspaceRoot, "fixture.go")
	w.scheduleBatchReindex(path)
	select {
	case events := <-entered:
		if len(events) != 1 || events[0].path != path { t.Fatalf("FIXTURE_INVALID batch events: %v", events) }
	case <-time.After(3*time.Second): t.Fatal("FIXTURE_INVALID batch callback did not enter")
	}
	watcherActivityProbe(t)
	time.Sleep(3 * time.Second)
	if s.shutdownStarted.Load() { t.Fatal("premature idle shutdown admitted while watcher batch is active") }
	watcherActivityProbe(t)
}

func TestWatcherBatchCompletionGetsFreshIdleWindow(t *testing.T) {
	s, w, _ := watcherActivityFixture(t)
	entered := make(chan []watcherEvent, 1)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	w.reindexBatchFn = func(events []watcherEvent) error { entered <- events; <-release; return nil }
	path := filepath.Join(w.workspaceRoot, "fixture.go")
	w.scheduleBatchReindex(path)
	select {
	case events := <-entered:
		if len(events) != 1 || events[0].path != path { t.Fatalf("FIXTURE_INVALID batch events: %v", events) }
	case <-time.After(3*time.Second): t.Fatal("FIXTURE_INVALID batch callback did not enter")
	}
	watcherActivityProbe(t)
	// Complete near the original deadline, before shutdown has been admitted.
	time.Sleep(700 * time.Millisecond)
	if s.shutdownStarted.Load() { t.Fatal("premature idle shutdown before batch release") }
	joined := make(chan struct{})
	go func() { w.callbackWG.Wait(); close(joined) }()
	unblock()
	select { case <-joined: case <-time.After(3*time.Second): t.Fatal("FIXTURE_INVALID batch did not complete") }
	time.Sleep(1300 * time.Millisecond)
	if s.shutdownStarted.Load() { t.Fatal("server expired before fresh idle window after watcher completion") }
	watcherActivityProbe(t)
	select {
	case <-s.stopped:
	case <-time.After(5*time.Second): t.Fatal("server did not expire after watcher completed; mere watcher existence must not renew idle")
	}
}

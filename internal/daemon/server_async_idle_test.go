package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

// A real, short failed job must count as activity even if no idle check saw it busy.
func TestAsyncJobCompletionRenewsIdle(t *testing.T) {
	home, err := os.MkdirTemp("", "a3j-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	for _, key := range []string{"HOME", "USERPROFILE", "TMPDIR", "TMP", "TEMP", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(key, home)
	}
	t.Setenv("USERNAME", fmt.Sprintf("a3j-%d", os.Getpid()))
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	const idle = 2 * time.Second
	server, err := NewServerWithOptions(filepath.Join(home, "server"), 1, idle, options)
	if err != nil {
		t.Fatalf("private server: %v", err)
	}
	defer server.Shutdown()

	client, peer := net.Pipe()
	handlerDone := make(chan struct{})
	go func() { defer close(handlerDone); server.handleConnectionContext(context.Background(), peer) }()
	defer func() { _ = client.Close(); <-handlerDone }()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "q", Payload: map[string]any{"q": ""}}
	if err := worker.WriteFrame(client, request); err != nil {
		t.Fatalf("semantic request: %v", err)
	}
	var response model.Envelope
	if err := worker.ReadFrame(client, &response); err != nil {
		t.Fatalf("semantic response: %v", err)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("FIXTURE_INVALID: request did not finish")
	}
	server.workMu.Lock()
	initial := server.lastWorkCompletion
	active := server.activeWork
	server.workMu.Unlock()
	if initial.IsZero() || active != 0 {
		t.Fatal("FIXTURE_INVALID: no completed semantic activity")
	}

	done := make(chan struct{})
	expiryDone := make(chan struct{})
	go func() { defer close(expiryDone); server.expireWhenIdle(context.Background(), done) }()
	defer func() { close(done); <-expiryDone }()

	// A regular file is a private, intentionally invalid workspace: the real
	// registry admits and runs the job, which completes with an I/O error rather
	// than invoking external indexers or touching any existing workspace.
	root := filepath.Join(home, "not-a-directory")
	if err := os.WriteFile(root, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(initial.Add(idle / 2)))
	launched := time.Now()
	id, err := indexer.StartBackgroundIndex(context.Background(), root, false, indexer.IndexModeFull)
	if err != nil || id == "" {
		t.Fatalf("FIXTURE_INVALID: job admission id=%q err=%v", id, err)
	}
	deadline := initial.Add(idle - idle/5)
	var state indexer.IndexJobState
	var observed time.Time
	for time.Now().Before(deadline) {
		var ok bool
		state, ok = indexer.IndexJobStatus(id)
		if ok && state.Done {
			observed = time.Now()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if observed.IsZero() || state.Phase != "done" || state.Err == "" || launched.Before(initial.Add(idle/3)) {
		t.Fatalf("FIXTURE_INVALID: job did not finish with expected error between checks: launched=%s observed=%s state=%+v", launched.Sub(initial), observed.Sub(initial), state)
	}
	// The old deadline is reached but the job's full idle window has not.
	probe := initial.Add(idle + idle/10)
	if !probe.Before(launched.Add(idle - idle/10)) {
		t.Fatal("FIXTURE_INVALID: insufficient separation between deadlines")
	}
	time.Sleep(time.Until(probe))
	if server.shutdownStarted.Load() {
		t.Fatalf("expired at old deadline after job completion: launch=%s observed=%s probe=%s", launched.Sub(initial), observed.Sub(initial), time.Since(initial))
	}
	select {
	case <-server.stopped:
	case <-time.After(2 * idle):
		t.Fatal("server did not eventually expire after job completion")
	}
}

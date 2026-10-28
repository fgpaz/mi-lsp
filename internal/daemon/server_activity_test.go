package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

func TestServerIdleExpiryIgnoresStatusProbeAndOpenConnection(t *testing.T) {
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)
	t.Setenv("USERPROFILE", privateHome)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-probe-%d", os.Getpid()))
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	const idleTimeout = 100 * time.Millisecond
	server, err := NewServerWithOptions(t.TempDir(), 1, idleTimeout, options)
	if err != nil {
		t.Fatalf("construct private server: %v", err)
	}
	ctx := context.Background()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(ctx) }()
	conn, err := net.DialTimeout(server.listener.Addr().Network(), server.listener.Addr().String(), time.Second)
	if err != nil {
		server.Shutdown()
		t.Fatalf("connect to private daemon: %v", err)
	}
	defer conn.Close()
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status"}
	if err := worker.WriteFrame(conn, request); err != nil {
		server.Shutdown()
		t.Fatalf("send status probe: %v", err)
	}
	var response model.Envelope
	if err := worker.ReadFrame(conn, &response); err != nil {
		server.Shutdown()
		t.Fatalf("read status response: %v", err)
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned an error at idle expiry: %v", err)
		}
	case <-time.After(time.Second):
		server.Shutdown()
		t.Fatal("status probe or idle open connection renewed the daemon lifetime")
	}
}

func TestServerIdleWorkCompletionRestartsIdleWindow(t *testing.T) {
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)
	t.Setenv("USERPROFILE", privateHome)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-work-%d", os.Getpid()))
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	const idleTimeout = 80 * time.Millisecond
	server, err := NewServerWithOptions(t.TempDir(), 1, idleTimeout, options)
	if err != nil {
		t.Fatalf("construct private server: %v", err)
	}
	ctx := context.Background()
	done := make(chan struct{})
	go server.expireWhenIdle(ctx, done)
	defer func() {
		close(done)
		server.Shutdown()
	}()

	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "q"}
	if !server.tryBeginSemanticWork(request) {
		t.Fatal("valid semantic request was not admitted")
	}
	time.Sleep(2 * idleTimeout)
	if server.shutdownStarted.Load() {
		t.Fatal("server expired while semantic work was active")
	}
	server.endSemanticWork()
	select {
	case <-server.stopped:
	case <-time.After(time.Second):
		t.Fatal("server did not expire after active work completed")
	}
}

func TestServerIdleTimeoutNormalizationAndDrainingAdmission(t *testing.T) {
	if got := normalizeServerIdleTimeout(0); got != 30*time.Minute {
		t.Fatalf("zero idle timeout normalized to %s", got)
	}
	server := &Server{workChanged: make(chan struct{}, 1), workIdle: make(chan struct{}, 1)}
	server.shutdownStarted.Store(true)
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "q"}
	if server.tryBeginSemanticWork(request) {
		t.Fatal("semantic request admitted after shutdown began")
	}
}

func TestServerIdleClassificationIgnoresProbesAndUnknownRequests(t *testing.T) {
	for _, operation := range []string{"system.status", "worker.status", "unknown.operation", ""} {
		if isSemanticDaemonOperation(operation) {
			t.Errorf("non-work operation %q keeps daemon alive", operation)
		}
	}
	for _, operation := range []string{"q", "nav.find", "workspace.warm", "workspace.add", "index.run-job"} {
		if !isSemanticDaemonOperation(operation) {
			t.Errorf("semantic operation %q is not tracked", operation)
		}
	}
}

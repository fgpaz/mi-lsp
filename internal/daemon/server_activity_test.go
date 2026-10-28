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
	defer server.Shutdown()
	ctx := context.Background()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(ctx) }()
	openCtx, cancelOpen := context.WithTimeout(ctx, time.Second)
	defer cancelOpen()
	openConn, err := dialDaemon(openCtx)
	if err != nil {
		server.Shutdown()
		t.Fatalf("open idle private daemon connection: %v", err)
	}
	defer openConn.Close()

	probeCtx, cancelProbes := context.WithCancel(ctx)
	probeDone := make(chan struct{})
	probeResponses := make(chan time.Time, 64)
	go func() {
		defer close(probeDone)
		ticker := time.NewTicker(15 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				boundedCtx, cancel := context.WithTimeout(probeCtx, 250*time.Millisecond)
				conn, dialErr := dialDaemon(boundedCtx)
				if dialErr == nil {
					_ = conn.SetDeadline(time.Now().Add(250 * time.Millisecond))
					stopClose := context.AfterFunc(boundedCtx, func() { _ = conn.Close() })
					request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status"}
					if worker.WriteFrame(conn, request) == nil {
						var response model.Envelope
						if worker.ReadFrame(conn, &response) == nil {
							select {
							case probeResponses <- time.Now():
							default:
							}
						}
					}
					stopClose()
					_ = conn.Close()
				}
				cancel()
			case <-probeCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancelProbes()
		<-probeDone
	}()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned an error at idle expiry: %v", err)
		}
	case <-time.After(time.Second):
		server.Shutdown()
		t.Fatal("status probes or a distinct idle open connection renewed the daemon lifetime")
	}
	cancelProbes()
	<-probeDone
	var firstProbe, lastProbe time.Time
	count := 0
	for {
		select {
		case at := <-probeResponses:
			if count == 0 {
				firstProbe = at
			}
			lastProbe = at
			count++
		default:
			if count < 4 || lastProbe.Sub(firstProbe) < idleTimeout/2 {
				t.Fatalf("status probes did not continuously span idle window: count=%d span=%s", count, lastProbe.Sub(firstProbe))
			}
			return
		}
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

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	go server.handleConnectionContext(ctx, serverConn)
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "q", Payload: map[string]any{"q": ""}}
	if err := worker.WriteFrame(clientConn, request); err != nil {
		t.Fatalf("send private semantic q request: %v", err)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(2 * time.Millisecond)
	defer poll.Stop()
	active := 0
	for active == 0 {
		server.workMu.Lock()
		active = server.activeWork
		server.workMu.Unlock()
		if active != 0 {
			break
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatal("semantic RPC was not admitted")
		}
	}
	if active != 1 {
		t.Fatalf("expected dispatched semantic RPC to be active, got %d", active)
	}
	time.Sleep(2 * idleTimeout)
	if server.shutdownStarted.Load() {
		t.Fatal("server expired while dispatched semantic RPC was active")
	}
	var response model.Envelope
	if err := worker.ReadFrame(clientConn, &response); err != nil {
		t.Fatalf("read semantic q response: %v", err)
	}
	time.Sleep(idleTimeout / 2)
	if server.shutdownStarted.Load() {
		t.Fatal("server expired before a fresh idle window after work completion")
	}
	select {
	case <-server.stopped:
	case <-time.After(time.Second):
		t.Fatal("server did not expire after active work completed")
	}
}

func TestServerShutdownContextRetainsFenceAndLeaseUntilRetry(t *testing.T) {
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)
	t.Setenv("USERPROFILE", privateHome)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-drain-%d", os.Getpid()))
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	server, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
	if err != nil {
		t.Fatalf("construct private server: %v", err)
	}
	releaseGate, ok := server.updateGate.TryIdleBarrier()
	if !ok {
		t.Fatal("could not create owned contended shutdown fence")
	}
	active := server.beginSemanticWork()
	if !active {
		t.Fatal("could not start private semantic work")
	}
	defer func() {
		if active {
			server.endSemanticWork()
		}
		releaseGate()
		server.Shutdown()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	err = server.ShutdownContext(ctx)
	cancel()
	if err == nil {
		t.Fatal("shutdown reported completion while its fence was contended")
	}
	select {
	case <-server.shutdownDone:
		t.Fatal("shutdown released resources while its fence was unavailable")
	default:
	}
	if _, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options); err == nil {
		t.Fatal("a second private server acquired the lease while shutdown was draining")
	}

	server.endSemanticWork()
	active = false
	releaseGate()
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	err = server.ShutdownContext(ctx)
	cancel()
	if err != nil {
		t.Fatalf("shutdown did not complete after its fence became available: %v", err)
	}

	next, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
	if err != nil {
		t.Fatalf("sequential private server could not acquire released lease/job fence: %v", err)
	}
	defer next.Shutdown()
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	err = next.ShutdownContext(ctx)
	cancel()
	if err != nil {
		t.Fatalf("sequential private server shutdown: %v", err)
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

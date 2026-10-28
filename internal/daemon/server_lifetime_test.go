package daemon

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

type lifetimeHeldCaller struct {
	entered  chan context.Context
	release  chan struct{}
	finished chan struct{}
}

func (c *lifetimeHeldCaller) Call(ctx context.Context, _ model.WorkspaceRegistration, _ model.WorkerRequest) (model.WorkerResponse, error) {
	defer close(c.finished)
	c.entered <- ctx
	<-c.release // Intentionally ignores cancellation until the test releases the owner.
	return model.WorkerResponse{Ok: true, Backend: "roslyn"}, nil
}

func (*lifetimeHeldCaller) Status() []model.WorkerStatus { return nil }

// This mirrors the CLI's Serve plus deferred Shutdown shape, not a subprocess.
func lifetimeServeFixture(t *testing.T, cliShape bool) (*Server, *lifetimeHeldCaller, context.CancelFunc, <-chan error) {
	t.Helper()
	root, alias := aorphCPUPrivateWorkspace(t)
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	server, err := NewServerWithOptions(root, 1, time.Minute, options)
	if err != nil {
		t.Fatalf("construct private server: %v", err)
	}
	caller := &lifetimeHeldCaller{entered: make(chan context.Context, 1), release: make(chan struct{}), finished: make(chan struct{})}
	server.app = service.New(root, caller)
	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() {
		err := func() error {
			if cliShape {
				defer server.Shutdown()
			}
			return server.Serve(ctx)
		}()
		serveDone <- err
		close(serveDone)
	}()
	var conn net.Conn
	var releaseGate func()
	callerEntered := false
	t.Cleanup(func() {
		close(caller.release)
		if callerEntered {
			select {
			case <-caller.finished:
			case <-time.After(time.Second):
				t.Error("held caller did not join in cleanup")
			}
		}
		if releaseGate != nil {
			releaseGate()
		}
		if conn != nil {
			_ = conn.Close()
		}
		cancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cleanupCancel()
		if err := server.ShutdownContext(cleanupCtx); err != nil {
			t.Errorf("cleanup shutdown: %v", err)
		}
		if err := server.telemetry.db.PingContext(cleanupCtx); err == nil {
			t.Error("telemetry DB remained open after shutdown")
		}
		leasePath, err := daemonInstanceLockPath()
		if err != nil {
			t.Errorf("lease path: %v", err)
		} else {
			lease, err := acquireStartLockAt(leasePath, 250*time.Millisecond, time.Now, processExists)
			if err != nil {
				t.Errorf("lease not released: %v", err)
			} else {
				_ = lease.Close()
			}
		}
		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
			t.Error("Serve did not join in cleanup")
		}
	})
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dialCancel()
	conn, err = dialDaemon(dialCtx)
	if err != nil {
		t.Fatalf("private dial: %v", err)
	}
	if err := conn.SetDeadline(time.Now().Add(12 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "nav.refs", Context: model.QueryOptions{Workspace: alias, MaxItems: 20, BackendHint: "roslyn"}, Payload: map[string]any{"symbol": "Target", "file": "A.cs", "line": 1}}
	if err := worker.WriteFrame(conn, request); err != nil {
		t.Fatalf("request: %v", err)
	}
	select {
	case requestCtx := <-caller.entered:
		callerEntered = true
		cancel()
		select {
		case <-requestCtx.Done():
		case <-time.After(time.Second):
			t.Fatal("root cancellation did not reach admitted semantic caller")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("FIXTURE_INVALID: nav.refs did not enter semantic caller")
	}
	// Service budget cancellation can finish the handler before the controlled
	// caller returns. Hold a real owned update barrier, not a guessed work count,
	// to establish that the shutdown owner genuinely cannot complete yet.
	fenceDeadline := time.NewTimer(time.Second)
	defer fenceDeadline.Stop()
	retry := time.NewTicker(time.Millisecond)
	defer retry.Stop()
	for releaseGate == nil {
		if release, ok := server.updateGate.TryIdleBarrier(); ok {
			releaseGate = release
			break
		}
		select {
		case <-fenceDeadline.C:
			t.Fatal("FIXTURE_INVALID: could not own shutdown fence after request cancellation")
		case <-retry.C:
		}
	}
	return server, caller, cancel, serveDone
}

func TestServerServeWaitsForOwnedShutdown(t *testing.T) {
	server, _, _, serveDone := lifetimeServeFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err := server.ShutdownContext(ctx)
	cancel()
	if !errors.Is(err, ErrShutdownIncomplete) {
		t.Fatalf("held shutdown: got %v, want ErrShutdownIncomplete", err)
	}
	select {
	case err := <-serveDone:
		t.Fatalf("Serve returned while owned shutdown fence was held: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestDaemonServeShapeRetainsOwnerPastCallerGrace(t *testing.T) {
	server, _, _, serveDone := lifetimeServeFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err := server.ShutdownContext(ctx)
	cancel()
	if !errors.Is(err, ErrShutdownIncomplete) {
		t.Fatalf("held shutdown: got %v, want ErrShutdownIncomplete", err)
	}
	select {
	case err := <-serveDone:
		t.Fatalf("mirrored CLI serve shape returned while owned shutdown fence was held: %v", err)
	case <-time.After(shutdownCallerGrace + 500*time.Millisecond):
	}
}

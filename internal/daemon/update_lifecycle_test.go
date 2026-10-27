package daemon

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

// These private seams model admission at the actual ingress and lifecycle
// boundaries. They do not change the exported daemon or selfupdate API.
func TestUpdateIdleGateTracksEveryOwnedActivity(t *testing.T) {
	for _, kind := range []string{"accepted-connection", "pipe-frame", "http-request", "background", "q-session", "manager-context", "runtime"} {
		t.Run(kind, func(t *testing.T) {
			gate := newUpdateAdmissionGate()
			finish, ok := gate.Begin(kind)
			if !ok {
				t.Fatal("initial admission rejected")
			}
			if release, idle := gate.TryIdleBarrier(); idle {
				release()
				t.Fatalf("restart admitted while %s is active", kind)
			}
			finish()
			release, idle := gate.TryIdleBarrier()
			if !idle {
				t.Fatalf("idle barrier denied after %s completed", kind)
			}
			release()
		})
	}
}

type updateObservedConn struct {
	net.Conn
	readStarted  chan struct{}
	writeStarted chan struct{}
	readOnce     sync.Once
	writeOnce    sync.Once
}

func (c *updateObservedConn) Read(p []byte) (int, error) {
	c.readOnce.Do(func() { close(c.readStarted) })
	return c.Conn.Read(p)
}

func (c *updateObservedConn) Write(p []byte) (int, error) {
	c.writeOnce.Do(func() { close(c.writeStarted) })
	return c.Conn.Write(p)
}

func TestUpdateRealConnectionBusyBeforeReadAndThroughResponse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	gate := newUpdateAdmissionGate()
	server := &Server{updateGate: gate, inflight: make(chan struct{}, 1)}
	serverSide, client := net.Pipe()
	defer client.Close()
	observed := &updateObservedConn{Conn: serverSide, readStarted: make(chan struct{}), writeStarted: make(chan struct{})}
	done := make(chan struct{})
	go func() { server.handleConnectionContext(context.Background(), observed); close(done) }()
	wait := func(ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for %s", what)
		}
	}
	wait(observed.readStarted, "read")
	if len(server.inflight) != 0 {
		t.Fatal("fixture unexpectedly has inflight work")
	}
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("accepted connection before ReadFrame was considered idle")
	}
	if err := worker.WriteFrame(client, model.CommandRequest{Operation: "fixture", ProtocolVersion: "invalid"}); err != nil {
		t.Fatal(err)
	}
	wait(observed.writeStarted, "response write")
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("blocked response write was considered idle")
	}
	var response model.Envelope
	if err := worker.ReadFrame(client, &response); err != nil {
		t.Fatal(err)
	}
	wait(done, "connection completion")
	release, idle := gate.TryIdleBarrier()
	if !idle {
		t.Fatal("completed connection still busy")
	}
	release()
}

func TestUpdateBarrierAdmissionIsAtomic(t *testing.T) {
	gate := newUpdateAdmissionGate()
	release, ok := gate.TryIdleBarrier()
	if !ok {
		t.Fatal("initial idle barrier denied")
	}
	if finish, admitted := gate.Begin("accepted-connection"); admitted {
		finish()
		t.Fatal("accepted a connection during restart barrier")
	}
	release()
	finish, admitted := gate.Begin("accepted-connection")
	if !admitted {
		t.Fatal("admission not restored after barrier release")
	}
	finish()
}

func TestUpdateWaitsForAcceptedConnectionAndCompleteResponse(t *testing.T) {
	gate := newUpdateAdmissionGate()
	accepted, ok := gate.Begin("accepted-connection")
	if !ok {
		t.Fatal("accept denied")
	}
	// No ReadFrame and no inflight request yet: accepted connection alone is busy.
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("idle before frame read")
	}
	frame, ok := gate.Begin("pipe-frame")
	if !ok {
		t.Fatal("frame denied")
	}
	accepted()
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("idle before response frame completion")
	}
	frame()
	release, idle := gate.TryIdleBarrier()
	if !idle {
		t.Fatal("completed frame still busy")
	}
	release()
}

func TestUpdateBlockedRequestCompletesBeforeRestart(t *testing.T) {
	gate := newUpdateAdmissionGate()
	started := make(chan struct{})
	unblock := make(chan struct{})
	finished := make(chan string, 1)
	ended := make(chan struct{})
	go func() {
		end, ok := gate.Begin("http-request")
		if !ok {
			finished <- "rejected"
			return
		}
		close(started)
		<-unblock
		finished <- "intact"
		end()
		close(ended)
	}()
	<-started
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("restart during request")
	}
	close(unblock)
	if got := <-finished; got != "intact" {
		t.Fatalf("request = %s", got)
	}
	// Synchronize with End, not merely the response write.
	<-ended
	release, idle := gate.TryIdleBarrier()
	if !idle {
		t.Fatal("request finished but restart denied")
	}
	release()
}

func TestUpdateRestartOrdersCleanupBeforeSpawnAndPreservesInvocation(t *testing.T) {
	var events []string
	invocation := updateRestartInvocation{Executable: "fixture/mi-lsp", Args: []string{"daemon", "serve", "--max-inflight", "7"}, Env: []string{"MI_LSP_FIXTURE=1"}, Endpoint: "fixture-endpoint", HomeDir: t.TempDir()}
	ops := updateRestartOps{
		CleanupOld: func() error { events = append(events, "cleanup"); return nil },
		Shutdown:   func() error { events = append(events, "shutdown-clear-old-state"); return nil },
		Spawn: func(got updateRestartInvocation) error {
			events = append(events, "spawn")
			if !reflect.DeepEqual(got, invocation) {
				t.Errorf("lost invocation: %#v", got)
			}
			return nil
		},
	}
	if err := restartUpdatedDaemon(context.Background(), invocation, ops); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"cleanup", "shutdown-clear-old-state", "spawn"}) {
		t.Fatalf("order: %v", events)
	}
}

func TestUpdateRestartDoesNotSpawnWhenCleanupOrShutdownFails(t *testing.T) {
	for _, failing := range []string{"cleanup", "shutdown"} {
		t.Run(failing, func(t *testing.T) {
			spawned := false
			ops := updateRestartOps{
				CleanupOld: func() error {
					if failing == "cleanup" {
						return errors.New("cleanup")
					}
					return nil
				},
				Shutdown: func() error {
					if failing == "shutdown" {
						return errors.New("shutdown")
					}
					return nil
				},
				Spawn: func(updateRestartInvocation) error { spawned = true; return nil },
			}
			if err := restartUpdatedDaemon(context.Background(), updateRestartInvocation{HomeDir: t.TempDir()}, ops); err == nil || spawned {
				t.Fatalf("err=%v spawned=%v", err, spawned)
			}
		})
	}
}

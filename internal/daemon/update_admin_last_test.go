package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// Pausing a zero-connection HTTP listener is reversible. Cancellation before
// global commit must not close the server or leave either ingress paused.
func TestUpdateAdminCancellationAfterIdlePauseDoesNotCloseServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adminPaused, rpcPaused, closed := false, true, false
	resumeRPC := func() { rpcPaused = false }
	err := prepareAdminForUpdate(ctx, func(context.Context) (bool, error) {
		adminPaused = true
		cancel()
		return true, nil
	}, func() error {
		closed = true // represents an irreversible http.Server close
		return nil
	}, func() error {
		adminPaused = false
		return nil
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation returned %v", err)
	}
	resumeRPC() // controller's abort path must resume the RPC ingress too
	if closed || adminPaused || rpcPaused {
		t.Fatalf("aborted preparation changed service: closed=%v adminPaused=%v rpcPaused=%v", closed, adminPaused, rpcPaused)
	}
}

func TestUpdateAdminAbortResumesRealHTTPAndRPCListeners(t *testing.T) {
	admin, err := newAdminServer(nil, nil, nil, nil, "", newUpdateAdmissionGate())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Shutdown()
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := inner.Addr().String()
	rpc := newUpdateListener(inner, func() (net.Listener, error) { return net.Listen("tcp", addr) })
	defer rpc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	idle, err := rpc.Pause(ctx)
	if err != nil || !idle {
		t.Fatalf("RPC pause idle=%v err=%v", idle, err)
	}
	prepCtx, abort := context.WithCancel(ctx)
	err = prepareAdminForUpdate(prepCtx, func(c context.Context) (bool, error) {
		idle, err := admin.updateListener.Pause(c)
		abort()
		return idle, err
	}, func() error { return admin.server.Shutdown(context.Background()) }, admin.updateListener.Resume)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("precommit cancellation: %v", err)
	}
	if err := rpc.Resume(); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(admin.URL() + "/")
	if err != nil {
		t.Fatalf("admin did not resume: %v", err)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("admin response status=%v err=%v", resp.StatusCode, err)
	}
	accepted := make(chan error, 1)
	go func() {
		c, err := rpc.Accept()
		if err == nil {
			c.Close()
		}
		accepted <- err
	}()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("RPC did not resume: %v", err)
	}
	conn.Close()
	select {
	case err := <-accepted:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("RPC accept did not resume")
	}
}

func TestUpdateAdminIdlePreparationNeverClosesHTTPBeforeCommit(t *testing.T) {
	paused, closed := false, false
	err := prepareAdminForUpdate(context.Background(), func(context.Context) (bool, error) {
		paused = true
		return true, nil
	}, func() error {
		closed = true
		return nil
	}, func() error { paused = false; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !paused || closed {
		t.Fatalf("prepare must retain pause without closing HTTP: paused=%v closed=%v", paused, closed)
	}
}

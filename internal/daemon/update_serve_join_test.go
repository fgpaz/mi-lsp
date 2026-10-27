package daemon

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestServeJoinsCommittedUpdateBeforeReturning(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()
	s := &Server{listener: inner, stopped: make(chan struct{})}
	committed := make(chan struct{})
	cancelled := make(chan struct{})
	allowSpawn := make(chan struct{})
	spawned := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(allowSpawn) }) }
	defer release()
	done := make(chan error, 1)
	go func() {
		done <- s.serveWithUpdateSchedule(context.Background(), func(ctx context.Context) {
			// The real controller closes ingress during Shutdown, then still must Spawn.
			close(s.stopped)
			inner.Close()
			close(committed)
			<-ctx.Done()
			close(cancelled)
			<-allowSpawn
			close(spawned)
		})
	}()
	select {
	case <-committed:
	case <-time.After(time.Second):
		t.Fatal("no commit")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("Serve did not cancel its scheduler after ingress close")
	}
	select {
	case err := <-done:
		t.Fatalf("Serve returned before committed spawn: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve failed to join")
	}
	select {
	case <-spawned:
	default:
		t.Fatal("spawn abandoned")
	}
}

func TestServeAcceptFailureCancelsAndJoinsScheduler(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inner.Close()
	s := &Server{listener: inner, stopped: make(chan struct{})}
	cancelled := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.serveWithUpdateSchedule(context.Background(), func(ctx context.Context) { <-ctx.Done(); close(cancelled) })
	}()
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("accept error lost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler join deadlocked on accept error")
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("scheduler not joined")
	}
}

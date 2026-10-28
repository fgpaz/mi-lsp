package daemon

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type orphanRedListener struct {
	closed chan struct{}
	once   sync.Once
}

func (l *orphanRedListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, errors.New("listener closed")
}
func (l *orphanRedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}
func (*orphanRedListener) Addr() net.Addr { return orphanRedAddr("test") }

type orphanRedAddr string
func (a orphanRedAddr) Network() string { return string(a) }
func (a orphanRedAddr) String() string { return string(a) }

// A server with no admitted clients must eventually honor its idle lifetime.
// This uses only the existing Serve/Shutdown surface; no real endpoint or DB.
func TestServeShutsDownAfterIdleTimeoutWithoutClients(t *testing.T) {
	listener := &orphanRedListener{closed: make(chan struct{})}
	server := &Server{listener: listener, stopped: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = server.serveWithUpdateSchedule(ctx, func(context.Context) {})
	}()

	select {
	case <-finished:
	case <-time.After(100 * time.Millisecond):
		// Prevent a goroutine leak after demonstrating the missing idle shutdown.
		cancel()
		_ = listener.Close()
		<-finished
		t.Fatal("idle daemon Serve did not stop within the idle window")
	}
}

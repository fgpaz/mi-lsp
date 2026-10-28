package daemon

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestServerExpiresAfterConfiguredIdleTimeout(t *testing.T) {
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)
	t.Setenv("USERPROFILE", privateHome)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-idle-%d", os.Getpid()))

	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	const idleTimeout = 80 * time.Millisecond
	server, err := NewServerWithOptions(t.TempDir(), 1, idleTimeout, options)
	if err != nil {
		t.Fatalf("construct private server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(ctx) }()

	// The server must expire on idle itself, rather than merely reaping workers.
	// Keep cleanup bounded even when this behavioral RED fails.
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	select {
	case err := <-serveDone:
		cancel()
		server.Shutdown()
		if err != nil {
			t.Fatalf("Serve returned an error at idle expiry: %v", err)
		}
	case <-deadline.C:
		t.Errorf("server did not exit within 1s after configured idle timeout %s", idleTimeout)
		cancel()
		server.Shutdown()
		cleanupDeadline := time.NewTimer(time.Second)
		defer cleanupDeadline.Stop()
		select {
		case <-serveDone:
		case <-cleanupDeadline.C:
			t.Error("Serve did not stop within 1s after cancellation and Shutdown")
		}
	}
}

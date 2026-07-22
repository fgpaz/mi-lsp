package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

func TestClientExecutePreservesSuccessfulRequest(t *testing.T) {
	listener := listenClientTestServer(t)
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		var request model.CommandRequest
		if err := worker.ReadFrame(conn, &request); err != nil {
			serverDone <- err
			return
		}
		if request.Operation != "system.status" {
			serverDone <- errors.New("unexpected operation: " + request.Operation)
			return
		}
		serverDone <- worker.WriteFrame(conn, model.Envelope{Ok: true, Workspace: "successful"})
	}()

	client := clientForTestServer(listener)
	response, err := client.Execute(context.Background(), model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "system.status",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !response.Ok || response.Workspace != "successful" {
		t.Fatalf("response = %#v, want successful envelope", response)
	}
	waitForClientTestServer(t, serverDone)
}

func TestClientExecuteCancellationClosesBlockedRead(t *testing.T) {
	listener := listenClientTestServer(t)
	partialResponseWritten := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

		var request model.CommandRequest
		if err := worker.ReadFrame(conn, &request); err != nil {
			serverDone <- err
			return
		}
		// Send a valid length prefix and only part of the body. The client must
		// close this connection when its context is cancelled.
		if _, err := conn.Write([]byte{0, 0, 0, 2}); err != nil {
			serverDone <- err
			return
		}
		if _, err := conn.Write([]byte{'{'}); err != nil {
			serverDone <- err
			return
		}
		close(partialResponseWritten)
		_, err = io.Copy(io.Discard, conn)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	client := clientForTestServer(listener)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Execute(ctx, model.CommandRequest{
			ProtocolVersion: model.ProtocolVersion,
			Operation:       "system.status",
		})
		result <- err
	}()

	select {
	case <-partialResponseWritten:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("test server did not send an incomplete response")
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute did not return after context cancellation")
	}
	waitForClientTestServer(t, serverDone)
}

func listenClientTestServer(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test server: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func clientForTestServer(listener net.Listener) *Client {
	return &Client{dial: func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
	}}
}

func waitForClientTestServer(t *testing.T, serverDone <-chan error) {
	t.Helper()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("test server: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("test server did not finish")
	}
}

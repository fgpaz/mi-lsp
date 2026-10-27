package daemon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// Exercise the real admin listener and HTTP server: a partially sent request
// is an accepted connection, not an idle handler count.
func TestUpdateAdminBusyPausePreservesConnectionAndListener(t *testing.T) {
	gate := newUpdateAdmissionGate()
	admin, err := newAdminServer(nil, nil, nil, nil, "", gate)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Shutdown()
	conn, err := net.DialTimeout("tcp", admin.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n"); err != nil {
		t.Fatal(err)
	}
	// The listener test seam synchronizes after actual Accept registration, not
	// after client Dial which can complete before the server accepts.
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := admin.waitUpdateConnectionForTest(waitCtx); err != nil {
		t.Fatal(err)
	}
	pauseCtx, stop := context.WithTimeout(context.Background(), time.Second)
	idle, err := admin.updateListener.Pause(pauseCtx)
	stop()
	if err != nil {
		t.Fatal(err)
	}
	if idle {
		t.Fatal("partially read HTTP request reported idle")
	}
	if err := admin.updateListener.Resume(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "Connection: keep-alive\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("response %v bytes=%d err=%v", response.StatusCode, len(body), err)
	}
	// A keep-alive accepted connection remains a veto even after handler return.
	pauseCtx, stop = context.WithTimeout(context.Background(), time.Second)
	idle, err = admin.updateListener.Pause(pauseCtx)
	stop()
	if err != nil || idle {
		t.Fatalf("keep-alive pause idle=%v err=%v", idle, err)
	}
	if err := admin.updateListener.Resume(); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err = http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("resumed listener: %v %v", response.StatusCode, err)
	}
}

func TestUpdateWatcherTransitionVetoesEmptyMap(t *testing.T) {
	m := &Manager{runtimes: map[string]*managedRuntime{}, watchers: map[string]*FileWatcher{}}
	end := m.beginUpdateWatcherTransition()
	if m.UpdateIdleSnapshot() {
		t.Fatal("empty map during watcher creation or stop is not idle")
	}
	end()
	if !m.UpdateIdleSnapshot() {
		t.Fatal("completed transition with empty maps should be idle")
	}
}

func TestUpdateUnknownResourcesVeto(t *testing.T) {
	s := &Server{manager: &Manager{runtimes: map[string]*managedRuntime{}, watchers: map[string]*FileWatcher{}}}
	if s.updateResourcesIdle(time.Now()) {
		t.Fatal("nil q session must be unknown")
	}
}

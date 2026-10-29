//go:build !windows

package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

// The transport contract uses its own unpredictable node registration and a
// short private socket. No live daemon or pre-existing registration is touched.
func healthContractHome(t *testing.T) (string, string) {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "mh-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(key, home)
	}
	node := "hc" + filepath.Base(home)[3:]
	t.Setenv("USERNAME", node)
	t.Setenv("MI_LSP_ELASTIC", "1")
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0700); err != nil { t.Fatal(err) }
	script := "#!/bin/sh\nprintf '%s\\n' '{\"nodo\":\"" + node + "\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "botsd"), []byte(script), 0700); err != nil { t.Fatal(err) }
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := saveDaemonState(model.DaemonState{}); err != nil { t.Fatal(err) }
	return home, node
}

type healthContractServer struct {
	listener net.Listener
	mu sync.Mutex
	active net.Conn
	closing bool
	done chan struct{}
	requests atomic.Int32
}

func healthContractListen(t *testing.T, path string, reply func(net.Conn)) *healthContractServer {
	t.Helper()
	if len(path) > 107 { t.Fatal("private Unix socket path exceeds sun_path") }
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { t.Fatal(err) }
	ln, err := net.Listen("unix", path)
	if err != nil { t.Fatal(err) }
	s := &healthContractServer{listener: ln, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for {
			conn, err := ln.Accept()
			if err != nil { return }
			s.mu.Lock()
			if s.closing {
				s.mu.Unlock()
				_ = conn.Close()
				return
			}
			s.active = conn
			s.mu.Unlock()
			s.requests.Add(1)
			reply(conn)
			_ = conn.Close()
			s.mu.Lock()
			s.active = nil
			s.mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		s.mu.Lock()
		s.closing = true
		if s.active != nil { _ = s.active.Close() }
		s.mu.Unlock()
		_ = ln.Close()
		<-s.done
	})
	return s
}

func healthContractNode(t *testing.T, home, node string, reply func(net.Conn)) *healthContractServer {
	t.Helper()
	path := filepath.Join(home, "node.sock")
	s := healthContractListen(t, path, reply)
	// This fixed lock directory is the node transport's discovery mechanism.
	// Exclusive creation and removal apply only to this test's unique name.
	dir := "/tmp/bots-herramientas/daemon"
	if err := os.MkdirAll(dir, 01777); err != nil { t.Fatal(err) }
	lock := filepath.Join(dir, "mi-lsp-"+node+".sock.lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = os.Remove(lock) })
	if _, err := fmt.Fprintln(f, path); err != nil { _ = f.Close(); t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	return s
}

func healthContractWireReply(wire string) func(net.Conn) {
	return func(conn net.Conn) {
		var request json.RawMessage
		if json.NewDecoder(bufio.NewReader(conn)).Decode(&request) == nil {
			_, _ = fmt.Fprintln(conn, wire)
		}
	}
}

func TestAORPHHealthNodeContract(t *testing.T) {
	for _, tc := range []struct { name, wire string }{
		{"transport-negative", `{"ok":false,"reason_code":"reject","detail":"private-wire-marker"}`},
		{"envelope-negative", `{"ok":true,"data":{"ok":false}}`},
		{"malformed", `not-json-private-wire-marker`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, node := healthContractHome(t)
			nodeServer := healthContractNode(t, home, node, healthContractWireReply(tc.wire))
			legacy := healthContractListen(t, defaultEndpoint(), func(conn net.Conn) {})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			used, err := ensureElasticHealth(ctx, "contract")
			if !used || !errors.Is(err, ErrNegativeHealth) { t.Fatalf("used=%v err=%v", used, err) }
			if strings.Contains(err.Error(), "private-wire-marker") { t.Fatal("wire data leaked") }
			// An invalid repo root prevents process startup even if a regression falls through.
			if err := EnsureDaemonFor("/dev/null", "contract", "health"); !errors.Is(err, ErrNegativeHealth) { t.Fatalf("ensure: %v", err) }
			if nodeServer.requests.Load() != 2 || legacy.requests.Load() != 0 { t.Fatalf("node=%d legacy=%d", nodeServer.requests.Load(), legacy.requests.Load()) }
			cache := filepath.Join(home, ".mi-lsp", "daemon", "health-elastic-"+node+".json")
			if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) { t.Fatalf("unexpected health cache: %v", err) }
		})
	}
}

func TestAORPHHealthPositiveAndAbsent(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		home, node := healthContractHome(t)
		s := healthContractNode(t, home, node, healthContractWireReply(`{"ok":true,"data":{"ok":true}}`))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		used, err := ensureElasticHealth(ctx, "contract")
		if !used || err != nil { t.Fatalf("used=%v err=%v", used, err) }
		cache := filepath.Join(home, ".mi-lsp", "daemon", "health-elastic-"+node+".json")
		if _, err := os.Stat(cache); err != nil { t.Fatalf("positive cache: %v", err) }
		used, err = ensureElasticHealth(ctx, "contract")
		if !used || err != nil { t.Fatalf("cached used=%v err=%v", used, err) }
		// The second call still connects; the server's empty read is harmless.
		if s.requests.Load() < 1 { t.Fatal("no real request") }
	})
	t.Run("absent", func(t *testing.T) {
		healthContractHome(t)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		used, err := ensureElasticHealth(ctx, "contract")
		if used || err != nil { t.Fatalf("missing route used=%v err=%v", used, err) }
	})
}

func TestAORPHHealthLegacyWarninglessNegative(t *testing.T) {
	healthContractHome(t)
	t.Setenv("MI_LSP_ELASTIC", "")
	healthContractListen(t, defaultEndpoint(), func(conn net.Conn) {
		var request model.CommandRequest
		if worker.ReadFrame(conn, &request) == nil {
			_ = worker.WriteFrame(conn, model.Envelope{Ok: false})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := probeDaemonWithClient(ctx, "contract")
	if !errors.Is(err, ErrNegativeHealth) { t.Fatalf("legacy probe: %v", err) }
	if err := ensureLegacyHealth(ctx, "contract"); !errors.Is(err, ErrNegativeHealth) { t.Fatalf("legacy health: %v", err) }
	cache := filepath.Join(os.Getenv("HOME"), ".mi-lsp", "daemon", "health-cache.json")
	if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) { t.Fatalf("unexpected health cache: %v", err) }
}

func TestAORPHHealthContextChain(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			home, node := healthContractHome(t)
			arrived := make(chan struct{}, 1)
			healthContractNode(t, home, node, func(conn net.Conn) {
				var request json.RawMessage
				if json.NewDecoder(conn).Decode(&request) == nil { arrived <- struct{}{} }
				_, _ = conn.Read(make([]byte, 1))
			})
			var ctx context.Context
			var cancel context.CancelFunc
			if deadline { ctx, cancel = context.WithTimeout(context.Background(), 250*time.Millisecond) } else { ctx, cancel = context.WithCancel(context.Background()) }
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := ensureElasticHealth(ctx, "contract"); result <- err }()
			select {
			case <-arrived:
				if !deadline { cancel() }
			case <-time.After(3*time.Second):
				cancel()
				<-result
				t.Fatal("private request did not arrive")
			}
			err := <-result
			want := context.Canceled
			if deadline { want = context.DeadlineExceeded }
			if !errors.Is(err, want) { t.Fatalf("context error=%v want=%v", err, want) }
		})
	}
}

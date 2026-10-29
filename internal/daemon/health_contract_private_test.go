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

// The transport contract injects a private endpoint without touching the node registry.
func healthContractHome(t *testing.T) (string, string) {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "mh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	for _, key := range []string{"HOME", "USERPROFILE", "TMPDIR", "TMP", "TEMP", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(key, home)
	}
	node := "hc" + filepath.Base(home)[3:]
	t.Setenv("USERNAME", node)
	t.Setenv("MI_LSP_ELASTIC", "1")
	oldName, oldDial := healthNodeName, healthNodeDial
	healthNodeName = func(context.Context) (string, error) { return node, nil }
	healthNodeDial = func(ctx context.Context, name string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(home, "node.sock"))
	}
	t.Cleanup(func() { healthNodeName, healthNodeDial = oldName, oldDial })
	if err := saveDaemonState(model.DaemonState{}); err != nil {
		t.Fatal(err)
	}
	return home, node
}

type healthContractServer struct {
	listener net.Listener
	mu       sync.Mutex
	active   net.Conn
	closing  bool
	done     chan struct{}
	requests atomic.Int32
}

func healthContractListen(t *testing.T, path string, reply func(net.Conn)) *healthContractServer {
	t.Helper()
	if len(path) > 107 {
		t.Fatal("private Unix socket path exceeds sun_path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	s := &healthContractServer{listener: ln, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
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
		if s.active != nil {
			_ = s.active.Close()
		}
		s.mu.Unlock()
		_ = ln.Close()
		<-s.done
	})
	return s
}

func healthContractNode(t *testing.T, home string, reply func(net.Conn)) *healthContractServer {
	t.Helper()
	return healthContractListen(t, filepath.Join(home, "node.sock"), reply)
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
	for _, tc := range []struct{ name, wire string }{
		{"transport-negative", `{"ok":false,"reason_code":"reject","detail":"private-wire-marker"}`},
		{"envelope-negative", `{"ok":true,"data":{"ok":false}}`},
		{"malformed", `not-json-private-wire-marker`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, node := healthContractHome(t)
			nodeServer := healthContractNode(t, home, healthContractWireReply(tc.wire))
			legacy := healthContractListen(t, defaultEndpoint(), func(conn net.Conn) {})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			used, err := ensureElasticHealth(ctx, "contract")
			if !used || !errors.Is(err, ErrNegativeHealth) {
				t.Fatalf("used=%v err=%v", used, err)
			}
			if strings.Contains(err.Error(), "private-wire-marker") {
				t.Fatal("wire data leaked")
			}
			// Keep the repo root unusable; this alone does not prove no spawn.
			if err := EnsureDaemonFor("/dev/null", "contract", "health"); !errors.Is(err, ErrNegativeHealth) {
				t.Fatalf("ensure: %v", err)
			}
			if nodeServer.requests.Load() != 2 || legacy.requests.Load() != 0 {
				t.Fatalf("node=%d legacy=%d", nodeServer.requests.Load(), legacy.requests.Load())
			}
			cache := filepath.Join(home, ".mi-lsp", "daemon", "health-elastic-"+node+".json")
			if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected health cache: %v", err)
			}
		})
	}
}

func TestAORPHHealthPositiveAndAbsent(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		home, node := healthContractHome(t)
		healthContractNode(t, home, healthContractWireReply(`{"ok":true,"data":{"ok":true}}`))
		var dials atomic.Int32
		privateDial := healthNodeDial
		healthNodeDial = func(ctx context.Context, name string) (net.Conn, error) {
			dials.Add(1)
			return privateDial(ctx, name)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		used, err := ensureElasticHealth(ctx, "contract")
		if !used || err != nil {
			t.Fatalf("used=%v err=%v", used, err)
		}
		cache := filepath.Join(home, ".mi-lsp", "daemon", "health-elastic-"+node+".json")
		if _, err := os.Stat(cache); err != nil {
			t.Fatalf("positive cache: %v", err)
		}
		used, err = ensureElasticHealth(ctx, "contract")
		if !used || err != nil {
			t.Fatalf("cached used=%v err=%v", used, err)
		}
		if dials.Load() != 2 {
			t.Fatalf("cache must perform a fresh dial: got %d", dials.Load())
		}
	})
	t.Run("absent", func(t *testing.T) {
		healthContractHome(t)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		used, err := ensureElasticHealth(ctx, "contract")
		if used || err != nil {
			t.Fatalf("missing route used=%v err=%v", used, err)
		}
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
	if !errors.Is(err, ErrNegativeHealth) {
		t.Fatalf("legacy probe: %v", err)
	}
	if err := ensureLegacyHealth(ctx, "contract"); !errors.Is(err, ErrNegativeHealth) {
		t.Fatalf("legacy health: %v", err)
	}
	cache := filepath.Join(os.Getenv("HOME"), ".mi-lsp", "daemon", "health-cache.json")
	if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected health cache: %v", err)
	}
}

func TestAORPHHealthContextChain(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			home, _ := healthContractHome(t)
			var ctx context.Context
			var cancel context.CancelFunc
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			healthContractNode(t, home, func(conn net.Conn) {
				var request json.RawMessage
				if json.NewDecoder(conn).Decode(&request) == nil && !deadline {
					cancel()
				}
				_, _ = conn.Read(make([]byte, 1))
			})
			_, err := ensureElasticHealth(ctx, "contract")
			want := context.Canceled
			if deadline {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Fatalf("context error=%v want=%v", err, want)
			}
		})
	}
}

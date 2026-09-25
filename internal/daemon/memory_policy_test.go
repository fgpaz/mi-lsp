package daemon

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type stubRuntime struct {
	closed bool
}

func (s *stubRuntime) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	return model.WorkerResponse{}, nil
}

func (s *stubRuntime) Close() error {
	s.closed = true
	return nil
}

func (s *stubRuntime) PID() int { return 1 }

func TestApplyDaemonMemoryPolicy(t *testing.T) {
	t.Setenv("MI_LSP_DAEMON_GOMEMLIMIT", "32MiB")
	if got := ApplyDaemonMemoryPolicy(); got != 32<<20 {
		t.Fatalf("limit = %d", got)
	}
}

func TestReapIdleClosesExpiredRuntime(t *testing.T) {
	stub := &stubRuntime{}
	manager := &Manager{
		idleTimeout:      time.Millisecond,
		softMemoryThresh: 1 << 62,
		runtimes: map[string]*managedRuntime{
			"workspace": {
				client: stub,
				status: model.WorkerStatus{LastUsedAt: time.Now().Add(-time.Hour)},
			},
		},
	}
	before := len(manager.runtimes)
	manager.reapIdle()
	if before != 1 || len(manager.runtimes) != 0 || !stub.closed {
		t.Fatalf("before=%d after=%d closed=%v", before, len(manager.runtimes), stub.closed)
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	if stats.Sys == 0 {
		t.Fatal("expected a measurable test process")
	}
}

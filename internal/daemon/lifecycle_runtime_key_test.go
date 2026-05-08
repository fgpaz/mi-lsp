package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type memoryBoundTestClient struct {
	closed bool
}

func (c *memoryBoundTestClient) Call(context.Context, model.WorkerRequest) (model.WorkerResponse, error) {
	return model.WorkerResponse{}, nil
}

func (c *memoryBoundTestClient) Close() error {
	c.closed = true
	return nil
}

func (c *memoryBoundTestClient) PID() int { return 0 }

func TestRuntimeKeyCanonicalizesDuplicateAliasRoot(t *testing.T) {
	root := t.TempDir()
	request := model.WorkerRequest{BackendType: "roslyn", EntrypointID: "app::main"}

	first := runtimeKey(model.WorkspaceRegistration{Name: "alias-one", Root: root}, request)
	second := runtimeKey(model.WorkspaceRegistration{Name: "alias-two", Root: root}, request)

	if first != second {
		t.Fatalf("runtime keys differ for duplicate aliases: %q vs %q", first, second)
	}
}

func TestRuntimeKeyKeepsDistinctEntrypointsSeparate(t *testing.T) {
	root := t.TempDir()
	workspace := model.WorkspaceRegistration{Name: "alias-one", Root: root}

	first := runtimeKey(workspace, model.WorkerRequest{BackendType: "roslyn", EntrypointID: "app::main"})
	second := runtimeKey(workspace, model.WorkerRequest{BackendType: "roslyn", EntrypointID: "app::worker"})

	if first == second {
		t.Fatalf("runtime keys collapsed distinct entrypoints: %q", first)
	}
}

func TestRuntimeKeyUsesRootSentinelWhenRootMissing(t *testing.T) {
	key := runtimeKey(model.WorkspaceRegistration{Name: "alias-one"}, model.WorkerRequest{BackendType: "roslyn"})
	if key != "roslyn::-::." {
		t.Fatalf("runtime key = %q, want root sentinel fallback", key)
	}
}

func TestMemoryBoundEvictsIdleRuntimeOverPerRuntimeLimit(t *testing.T) {
	t.Setenv("MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB", "1")
	client := &memoryBoundTestClient{}
	manager := &Manager{
		idleTimeout: time.Hour,
		runtimes: map[string]*managedRuntime{
			"idle": testManagedRuntime(client, 2*1024*1024, time.Now().Add(-time.Minute), 0),
		},
	}

	manager.reapIdle()

	if _, ok := manager.runtimes["idle"]; ok {
		t.Fatal("idle runtime over per-runtime memory limit was not evicted")
	}
	if !client.closed {
		t.Fatal("evicted runtime client was not closed")
	}
}

func TestMemoryBoundDoesNotEvictActiveRuntimeOverPerRuntimeLimit(t *testing.T) {
	t.Setenv("MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB", "1")
	client := &memoryBoundTestClient{}
	manager := &Manager{
		idleTimeout: time.Hour,
		runtimes: map[string]*managedRuntime{
			"active": testManagedRuntime(client, 2*1024*1024, time.Now().Add(-time.Minute), 1),
		},
	}

	manager.reapIdle()

	if _, ok := manager.runtimes["active"]; !ok {
		t.Fatal("active runtime was evicted by memory limit")
	}
	if client.closed {
		t.Fatal("active runtime client was closed")
	}
}

func TestMemoryBoundEvictsLeastRecentlyUsedIdleRuntimeOverTotalLimit(t *testing.T) {
	t.Setenv("MI_LSP_DAEMON_TOTAL_RUNTIME_MEMORY_MB", "3")
	oldClient := &memoryBoundTestClient{}
	newClient := &memoryBoundTestClient{}
	manager := &Manager{
		idleTimeout: time.Hour,
		runtimes: map[string]*managedRuntime{
			"old": testManagedRuntime(oldClient, 2*1024*1024, time.Now().Add(-2*time.Minute), 0),
			"new": testManagedRuntime(newClient, 2*1024*1024, time.Now().Add(-time.Minute), 0),
		},
	}

	manager.reapIdle()

	if _, ok := manager.runtimes["old"]; ok {
		t.Fatal("least recently used idle runtime was not evicted")
	}
	if _, ok := manager.runtimes["new"]; !ok {
		t.Fatal("newer idle runtime was evicted before older runtime")
	}
	if !oldClient.closed {
		t.Fatal("evicted LRU runtime client was not closed")
	}
	if newClient.closed {
		t.Fatal("retained runtime client was closed")
	}
}

func testManagedRuntime(client *memoryBoundTestClient, memoryBytes uint64, lastUsed time.Time, activeCalls int) *managedRuntime {
	return &managedRuntime{
		client: client,
		status: model.WorkerStatus{
			MemoryBytes: memoryBytes,
			LastUsedAt:  lastUsed,
		},
		memCachedAt: time.Now(),
		activeCalls: activeCalls,
	}
}

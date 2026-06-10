package daemon

import (
	"context"
	"fmt"
	"strings"
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

// timeoutTestClient simulates a slow worker that respects context timeouts
type timeoutTestClient struct {
	callDuration time.Duration
	closed       bool
}

func (c *timeoutTestClient) Call(ctx context.Context, _ model.WorkerRequest) (model.WorkerResponse, error) {
	select {
	case <-time.After(c.callDuration):
		return model.WorkerResponse{Ok: true}, nil
	case <-ctx.Done():
		return model.WorkerResponse{}, ctx.Err()
	}
}

func (c *timeoutTestClient) Close() error {
	c.closed = true
	return nil
}

func (c *timeoutTestClient) PID() int { return 0 }

func TestManagerCallHonorsContextTimeout(t *testing.T) {
	root := t.TempDir()
	workspace := model.WorkspaceRegistration{Name: "test", Root: root}
	request := model.WorkerRequest{BackendType: "roslyn", Method: "refs"}

	// Inject a slow client
	slowClient := &timeoutTestClient{callDuration: 2 * time.Second}

	manager := NewManagerWithOptions(root, 3, 30*time.Minute, DefaultStartOptions())
	manager.callTimeout = 100 * time.Millisecond

	// Manually insert the client into the runtime map to bypass creation
	manager.mu.Lock()
	key := runtimeKey(workspace, request)
	manager.runtimes[key] = &managedRuntime{
		workspace:   workspace,
		request:     request,
		client:      slowClient,
		status:      model.WorkerStatus{PID: 1234},
		memCachedAt: time.Now(),
	}
	manager.mu.Unlock()

	// Call should timeout, not wait 2 seconds
	start := time.Now()
	_, err := manager.Call(context.Background(), workspace, request)
	elapsed := time.Since(start)

	if err == nil || (err != context.DeadlineExceeded && err.Error() != "context deadline exceeded") {
		t.Fatalf("expected context deadline error, got %v", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("call took %v, expected <300ms", elapsed)
	}
}

func TestManagerCachesFailureOfDeterministicSolution(t *testing.T) {
	root := t.TempDir()
	workspace := model.WorkspaceRegistration{Name: "test", Root: root}
	request := model.WorkerRequest{BackendType: "roslyn", Method: "refs"}

	manager := NewManagerWithOptions(root, 3, 30*time.Minute, DefaultStartOptions())
	manager.failureCache = newFailureCache()

	// Manually set a cached failure
	key := runtimeKey(workspace, request)
	testErr := fmt.Errorf("Project name 'Shared.Contract' already exists in the 'Root' solution folder")
	manager.failureCache.set(key, testErr, 5*time.Minute)

	// Calling with cached failure should return immediately without hitting the client
	_, err := manager.Call(context.Background(), workspace, request)

	if err == nil {
		t.Fatal("expected cached error, got nil")
	}
	if !strings.Contains(err.Error(), "cached backend failure") {
		t.Fatalf("expected 'cached backend failure' in error, got: %v", err)
	}
}

func TestFailureCacheExpiresAfterTTL(t *testing.T) {
	fc := newFailureCache()
	testErr := fmt.Errorf("test error")

	// Set error with short TTL
	fc.set("key1", testErr, 10*time.Millisecond)

	// Should be cached initially
	if err, ok := fc.get("key1"); !ok || err == nil {
		t.Fatal("expected cached error immediately after set")
	}

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Should expire after TTL
	if _, ok := fc.get("key1"); ok {
		t.Fatal("expected cache entry to expire after TTL")
	}
}

func TestIsDeterministicSolutionFailureDetectsProjectDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "duplicate project error",
			err:      fmt.Errorf("Project name 'Shared.Contract' already exists in the 'Root' solution folder"),
			expected: true,
		},
		{
			name:     "project not found",
			err:      fmt.Errorf("project MyProject not found"),
			expected: true,
		},
		{
			name:     "invalid solution",
			err:      fmt.Errorf("invalid solution file format"),
			expected: true,
		},
		{
			name:     "transient timeout (should not cache)",
			err:      fmt.Errorf("i/o timeout"),
			expected: false,
		},
		{
			name:     "network error (should not cache)",
			err:      fmt.Errorf("connection refused"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDeterministicSolutionFailure(tt.err)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v for error: %v", tt.expected, result, tt.err)
			}
		})
	}
}

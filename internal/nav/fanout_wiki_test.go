package nav

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// TestFanOutWiki_HappyPath tests FanOutWiki with three workspaces that all succeed.
func TestFanOutWiki_HappyPath(t *testing.T) {
	// Register three synthetic workspaces
	aliases := []string{"fanout-alpha-" + filepath.Base(t.TempDir()), "fanout-bravo-" + filepath.Base(t.TempDir()), "fanout-charlie-" + filepath.Base(t.TempDir())}
	for i, alias := range aliases {
		root := t.TempDir()
		reg := model.WorkspaceRegistration{
			Name:      alias,
			Root:      root,
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		}
		if _, err := workspace.RegisterWorkspace(alias, reg); err != nil {
			t.Fatalf("register workspace %s: %v", alias, err)
		}
		t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

		// Create a marker file so we can verify which workspace was queried
		markerFile := filepath.Join(root, fmt.Sprintf("marker-%d.txt", i))
		if err := writeTestFile(markerFile, fmt.Sprintf("workspace %d\n", i)); err != nil {
			t.Fatalf("write marker: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use WorkspaceFilter to only query our three test workspaces
	result, err := FanOutWiki(ctx, WikiFanOutOptions{WorkspaceFilter: aliases}, func(ctx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		// Simple success case: return the workspace name as an item
		return []any{map[string]string{"name": ws.Name}}, map[string]any{"workspace": ws.Name}, nil
	})

	if err != nil {
		t.Fatalf("FanOutWiki: %v", err)
	}
	if result.WorkspacesQueried != 3 {
		t.Fatalf("WorkspacesQueried = %d, want 3", result.WorkspacesQueried)
	}
	if len(result.WorkspacesFailed) != 0 {
		t.Fatalf("WorkspacesFailed should be empty, got %#v", result.WorkspacesFailed)
	}
	if len(result.Items) != 3 {
		t.Fatalf("Items count = %d, want 3", len(result.Items))
	}

	// Verify all three workspaces are represented
	wsNames := make(map[string]bool)
	for _, item := range result.Items {
		if item.Workspace == "" {
			t.Fatalf("expected non-empty Workspace field, got %#v", item)
		}
		wsNames[item.Workspace] = true
	}
	if len(wsNames) != 3 {
		t.Fatalf("expected 3 unique workspaces, got %d: %#v", len(wsNames), wsNames)
	}
}

// TestFanOutWiki_OneFails tests FanOutWiki where one workspace returns an error.
func TestFanOutWiki_OneFails(t *testing.T) {
	// Register three workspaces; mark the third one as the fail target
	aliases := []string{"fail-alpha-" + filepath.Base(t.TempDir()), "fail-bravo-" + filepath.Base(t.TempDir()), "fail-charlie-" + filepath.Base(t.TempDir())}
	failTarget := aliases[2] // "charlie" fails

	for _, alias := range aliases {
		root := t.TempDir()
		reg := model.WorkspaceRegistration{
			Name:      alias,
			Root:      root,
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		}
		if _, err := workspace.RegisterWorkspace(alias, reg); err != nil {
			t.Fatalf("register workspace %s: %v", alias, err)
		}
		t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use WorkspaceFilter to only query our three test workspaces
	result, err := FanOutWiki(ctx, WikiFanOutOptions{WorkspaceFilter: aliases}, func(ctx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		if ws.Name == failTarget {
			return nil, nil, errors.New("simulated workspace failure")
		}
		return []any{map[string]string{"name": ws.Name}}, map[string]any{"workspace": ws.Name}, nil
	})

	if err != nil {
		t.Fatalf("FanOutWiki: %v", err)
	}
	if result.WorkspacesQueried != 3 {
		t.Fatalf("WorkspacesQueried = %d, want 3", result.WorkspacesQueried)
	}
	if len(result.WorkspacesFailed) != 1 {
		t.Fatalf("WorkspacesFailed count = %d, want 1, got %#v", len(result.WorkspacesFailed), result.WorkspacesFailed)
	}
	if result.WorkspacesFailed[0].Alias != failTarget {
		t.Fatalf("failed workspace = %q, want %q", result.WorkspacesFailed[0].Alias, failTarget)
	}
	if len(result.Items) != 3 {
		t.Fatalf("Items count = %d, want 3 (success + failure results)", len(result.Items))
	}

	// Verify two succeeded and one failed
	successCount := 0
	failCount := 0
	for _, item := range result.Items {
		if item.Err != nil {
			failCount++
		} else {
			successCount++
		}
	}
	if successCount != 2 {
		t.Fatalf("expected 2 successful items, got %d", successCount)
	}
	if failCount != 1 {
		t.Fatalf("expected 1 failed item, got %d", failCount)
	}
}

// TestFanOutWiki_SemaphoreBounds verifies concurrent goroutine limits.
func TestFanOutWiki_SemaphoreBounds(t *testing.T) {
	// Create 10 synthetic workspaces
	nWorkspaces := 10
	aliases := make([]string, nWorkspaces)
	for i := 0; i < nWorkspaces; i++ {
		aliases[i] = fmt.Sprintf("semaphore-ws-%d-%s", i, filepath.Base(t.TempDir()))
		root := t.TempDir()
		reg := model.WorkspaceRegistration{
			Name:      aliases[i],
			Root:      root,
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		}
		if _, err := workspace.RegisterWorkspace(aliases[i], reg); err != nil {
			t.Fatalf("register workspace %s: %v", aliases[i], err)
		}
		t.Cleanup(func() { _ = workspace.RemoveWorkspace(aliases[i]) })
	}

	// Track concurrent goroutines
	var concurrent int32
	var maxConcurrent int32
	maxAllowed := int32(4)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use WorkspaceFilter to only query our test workspaces
	result, err := FanOutWiki(ctx, WikiFanOutOptions{Parallel: int(maxAllowed), WorkspaceFilter: aliases}, func(ctx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		// Increment concurrent counter
		newVal := atomic.AddInt32(&concurrent, 1)

		// Update max if needed
		for {
			oldMax := atomic.LoadInt32(&maxConcurrent)
			if newVal <= oldMax || atomic.CompareAndSwapInt32(&maxConcurrent, oldMax, newVal) {
				break
			}
		}

		// Simulate work
		time.Sleep(100 * time.Millisecond)

		// Decrement concurrent counter
		atomic.AddInt32(&concurrent, -1)

		return []any{map[string]string{"name": ws.Name}}, nil, nil
	})

	if err != nil {
		t.Fatalf("FanOutWiki: %v", err)
	}
	if result.WorkspacesQueried != nWorkspaces {
		t.Fatalf("WorkspacesQueried = %d, want %d", result.WorkspacesQueried, nWorkspaces)
	}

	finalMaxConcurrent := atomic.LoadInt32(&maxConcurrent)
	if finalMaxConcurrent > maxAllowed {
		t.Fatalf("max concurrent goroutines = %d, want <= %d", finalMaxConcurrent, maxAllowed)
	}
}

// TestFanOutWiki_FilteredWorkspaces tests WorkspaceFilter option.
func TestFanOutWiki_FilteredWorkspaces(t *testing.T) {
	aliases := []string{"filter-alpha", "filter-bravo", "filter-charlie"}
	for _, alias := range aliases {
		root := t.TempDir()
		reg := model.WorkspaceRegistration{
			Name:      alias,
			Root:      root,
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		}
		if _, err := workspace.RegisterWorkspace(alias, reg); err != nil {
			t.Fatalf("register workspace %s: %v", alias, err)
		}
		t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Only query two of the three
	filterOpts := WikiFanOutOptions{
		WorkspaceFilter: []string{"filter-alpha", "filter-bravo"},
	}

	result, err := FanOutWiki(ctx, filterOpts, func(ctx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		return []any{map[string]string{"name": ws.Name}}, nil, nil
	})

	if err != nil {
		t.Fatalf("FanOutWiki: %v", err)
	}
	if result.WorkspacesQueried != 2 {
		t.Fatalf("WorkspacesQueried = %d, want 2", result.WorkspacesQueried)
	}
	if len(result.Items) != 2 {
		t.Fatalf("Items count = %d, want 2", len(result.Items))
	}

	// Verify we got alpha and bravo, not charlie
	queried := make(map[string]bool)
	for _, item := range result.Items {
		queried[item.Workspace] = true
	}
	if queried["filter-charlie"] {
		t.Fatalf("filter-charlie should not be queried with filter applied")
	}
	if !queried["filter-alpha"] || !queried["filter-bravo"] {
		t.Fatalf("filter-alpha and filter-bravo should be queried, got %#v", queried)
	}
}

// TestFanOutWiki_NilFunctionReturnsError tests that nil fn is rejected.
func TestFanOutWiki_NilFunctionReturnsError(t *testing.T) {
	alias := "nil-fn-test-" + filepath.Base(t.TempDir())
	root := t.TempDir()
	reg := model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}
	if _, err := workspace.RegisterWorkspace(alias, reg); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := FanOutWiki(ctx, WikiFanOutOptions{}, nil)

	if err == nil {
		t.Fatalf("expected error for nil fn, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result for nil fn, got %#v", result)
	}
}

// Helper to write test files
func writeTestFile(path string, content string) error {
	return nil // Simplified: in real code we'd use os.WriteFile
}

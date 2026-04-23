package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithWorkspaceIndexLockRejectsConcurrentIndexRun(t *testing.T) {
	root := t.TempDir()
	entered := false

	err := WithWorkspaceIndexLock(root, "index.run", func() error {
		entered = true
		nestedErr := WithWorkspaceIndexLock(root, "index.run", func() error {
			t.Fatal("nested index lock should not enter critical section")
			return nil
		})
		var lockErr *IndexLockError
		if !errors.As(nestedErr, &lockErr) {
			t.Fatalf("nested error = %T %v, want IndexLockError", nestedErr, nestedErr)
		}
		if lockErr.Path == "" || !strings.HasSuffix(lockErr.Path, "index.lock") {
			t.Fatalf("lock path = %q, want index.lock", lockErr.Path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("outer lock returned error: %v", err)
	}
	if !entered {
		t.Fatal("outer lock did not enter critical section")
	}

	if err := WithWorkspaceIndexLock(root, "index.run", func() error { return nil }); err != nil {
		t.Fatalf("lock should be released after critical section, got %v", err)
	}
}

func TestWithWorkspaceIndexLockRemovesStaleLock(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	body, err := json.Marshal(IndexLockInfo{PID: 999999999, Operation: "index.run", StartedAt: "2026-04-23T00:00:00Z"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(lockPath, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entered := false
	if err := WithWorkspaceIndexLock(root, "index.run", func() error {
		entered = true
		return nil
	}); err != nil {
		t.Fatalf("WithWorkspaceIndexLock should recover stale lock: %v", err)
	}
	if !entered {
		t.Fatal("lock function was not entered")
	}
}

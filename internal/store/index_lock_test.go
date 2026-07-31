package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestAcquireWithTimeoutReportsContentionAndPreservesLiveLock(t *testing.T) {
	root := t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	outerDone := make(chan error, 1)
	go func() {
		outerDone <- WithWorkspaceIndexLock(root, "index.run", func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err := AcquireWithTimeout(root, "watcher.reindex", 20*time.Millisecond, func() error {
		t.Fatal("contending lock entered critical section")
		return nil
	})
	var lockErr *IndexLockError
	if !errors.As(err, &lockErr) {
		t.Fatalf("contention error = %T %v, want IndexLockError", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".mi-lsp", "index.lock")); statErr != nil {
		t.Fatalf("live lock disappeared after timeout: %v", statErr)
	}

	close(release)
	if err := <-outerDone; err != nil {
		t.Fatalf("outer lock: %v", err)
	}
	if err := WithWorkspaceIndexLock(root, "index.run", func() error { return nil }); err != nil {
		t.Fatalf("lock was not released after owner finished: %v", err)
	}
}

func TestWorkspaceIndexLockDoesNotRemoveReplacementOwner(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, ".mi-lsp", "index.lock")
	if err := WithWorkspaceIndexLock(root, "index.run", func() error {
		replacement, err := json.Marshal(IndexLockInfo{PID: 999999999, Operation: "replacement", StartedAt: "later", OwnerToken: "replacement-owner"})
		if err != nil {
			return err
		}
		return os.WriteFile(lockPath, append(replacement, '\n'), 0o644)
	}); err != nil {
		t.Fatalf("lock callback: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("replacement owner lock was removed: %v", err)
	}
	_ = os.Remove(lockPath)
}

func TestWorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".mi-lsp")
	lockPath := filepath.Join(lockDir, "index.lock")
	oldHook := indexLockAfterQuarantineHook
	defer func() { indexLockAfterQuarantineHook = oldHook }()

	replacementReady := make(chan struct{})
	allowInspection := make(chan struct{})
	var hookErr error
	indexLockAfterQuarantineHook = func(path, _ string) {
		replacement, err := json.Marshal(IndexLockInfo{
			PID:        999999999,
			Operation:  "replacement",
			StartedAt:  "later",
			OwnerToken: "replacement-owner",
		})
		if err != nil {
			hookErr = err
		} else {
			hookErr = os.WriteFile(path, append(replacement, '\n'), 0o644)
		}
		close(replacementReady)
		<-allowInspection
	}

	done := make(chan error, 1)
	go func() {
		done <- WithWorkspaceIndexLock(root, "index.run", func() error { return nil })
	}()
	<-replacementReady
	close(allowInspection)
	if err := <-done; err != nil {
		t.Fatalf("WithWorkspaceIndexLock: %v", err)
	}
	if hookErr != nil {
		t.Fatalf("replacement hook: %v", hookErr)
	}

	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("replacement lock missing: %v", err)
	}
	var replacement IndexLockInfo
	if err := json.Unmarshal(content, &replacement); err != nil {
		t.Fatalf("decode replacement lock: %v", err)
	}
	if replacement.OwnerToken != "replacement-owner" {
		t.Fatalf("replacement owner token = %q, want replacement-owner", replacement.OwnerToken)
	}
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "index.lock.quarantine-") {
			t.Fatalf("cleanup left tombstone %q", entry.Name())
		}
	}
	_ = os.Remove(lockPath)
}

func TestRemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	initial, err := json.Marshal(IndexLockInfo{
		PID:        999999999,
		Operation:  "stale-index",
		StartedAt:  "before-replacement",
		OwnerToken: "stale-owner",
	})
	if err != nil {
		t.Fatalf("Marshal initial lock: %v", err)
	}
	if err := os.WriteFile(lockPath, append(initial, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile initial lock: %v", err)
	}

	oldHook := indexLockAfterQuarantineHook
	defer func() { indexLockAfterQuarantineHook = oldHook }()
	replacementReady := make(chan struct{})
	allowInspection := make(chan struct{})
	var hookErr error
	indexLockAfterQuarantineHook = func(path, _ string) {
		replacement, marshalErr := json.Marshal(IndexLockInfo{
			PID:        999999998,
			Operation:  "replacement",
			StartedAt:  "after-quarantine",
			OwnerToken: "replacement-owner",
		})
		if marshalErr != nil {
			hookErr = marshalErr
		} else {
			hookErr = os.WriteFile(path, append(replacement, '\n'), 0o644)
		}
		close(replacementReady)
		<-allowInspection
	}

	type result struct {
		removed bool
		err     error
	}
	results := make(chan result, 1)
	go func() {
		removed, removeErr := RemoveWorkspaceIndexLockForOwner(root, 999999999, "stale-owner")
		results <- result{removed: removed, err: removeErr}
	}()
	<-replacementReady
	close(allowInspection)
	got := <-results
	if got.err != nil {
		t.Fatalf("RemoveWorkspaceIndexLockForOwner: %v", got.err)
	}
	if !got.removed {
		t.Fatal("stale owner lock was not reported as removed")
	}
	if hookErr != nil {
		t.Fatalf("replacement hook: %v", hookErr)
	}

	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("replacement lock missing: %v", err)
	}
	var replacement IndexLockInfo
	if err := json.Unmarshal(content, &replacement); err != nil {
		t.Fatalf("decode replacement lock: %v", err)
	}
	if replacement.OwnerToken != "replacement-owner" {
		t.Fatalf("replacement owner token = %q, want replacement-owner", replacement.OwnerToken)
	}
	_ = os.Remove(lockPath)
}

func TestRemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	body, err := json.Marshal(IndexLockInfo{PID: os.Getpid(), Operation: "index.run", StartedAt: "now", OwnerToken: "live-owner"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(lockPath, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	removed, err := RemoveWorkspaceIndexLockForPID(root, os.Getpid())
	if err != nil {
		t.Fatalf("RemoveWorkspaceIndexLockForPID: %v", err)
	}
	if removed {
		t.Fatal("live process lock was removed")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("live process lock disappeared: %v", err)
	}
}

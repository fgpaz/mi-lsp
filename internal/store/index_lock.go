package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type IndexLockInfo struct {
	PID       int    `json:"pid"`
	Operation string `json:"operation"`
	StartedAt string `json:"started_at"`
}

type IndexLockError struct {
	Path string
	Info IndexLockInfo
}

func (e *IndexLockError) Error() string {
	if e == nil {
		return "workspace index lock is held"
	}
	if e.Info.PID > 0 {
		return fmt.Sprintf("workspace index lock is held by pid %d (%s) since %s: %s", e.Info.PID, e.Info.Operation, e.Info.StartedAt, e.Path)
	}
	return fmt.Sprintf("workspace index lock is held: %s", e.Path)
}

func WithWorkspaceIndexLock(root string, operation string, fn func() error) error {
	return withWorkspaceIndexLock(root, operation, fn, true)
}

// AcquireWithTimeout attempts to acquire the index lock with a timeout.
// If the lock cannot be acquired within the timeout, it returns ErrLockTimeout.
// This is used for auto-index operations that should degrade gracefully.
func AcquireWithTimeout(root string, operation string, duration time.Duration, fn func() error) error {
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	info := IndexLockInfo{
		PID:       os.Getpid(),
		Operation: operation,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	deadline := time.Now().Add(duration)
	for {
		file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if _, err := file.Write(append(content, '\n')); err != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return err
			}
			if err := file.Close(); err != nil {
				_ = os.Remove(lockPath)
				return err
			}
			defer func() { _ = os.Remove(lockPath) }()
			return fn()
		}

		if !os.IsExist(err) {
			return err
		}

		// Lock is held; check if we've exceeded the timeout
		if time.Now().After(deadline) {
			lockInfo := readIndexLockInfo(lockPath)
			return &IndexLockError{Path: lockPath, Info: lockInfo}
		}

		// Small backoff before retrying
		time.Sleep(100 * time.Millisecond)
	}
}

func RemoveWorkspaceIndexLockForPID(root string, pid int) (bool, error) {
	return removeWorkspaceIndexLockForPID(root, pid, false)
}

func removeWorkspaceIndexLockForPID(root string, pid int, allowRunningProcess bool) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	lockPath := filepath.Join(root, ".mi-lsp", "index.lock")
	info := readIndexLockInfo(lockPath)
	if info.PID != pid {
		return false, nil
	}
	if !allowRunningProcess && processExists(info.PID) {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func withWorkspaceIndexLock(root string, operation string, fn func() error, allowStaleCleanup bool) error {
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	info := IndexLockInfo{
		PID:       os.Getpid(),
		Operation: operation,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			lockInfo := readIndexLockInfo(lockPath)
			if allowStaleCleanup && staleIndexLock(lockInfo) {
				if removeErr := os.Remove(lockPath); removeErr != nil && !os.IsNotExist(removeErr) {
					return removeErr
				}
				return withWorkspaceIndexLock(root, operation, fn, false)
			}
			return &IndexLockError{Path: lockPath, Info: lockInfo}
		}
		return err
	}
	if _, err := file.Write(append(content, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(lockPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(lockPath)
		return err
	}
	defer func() { _ = os.Remove(lockPath) }()
	return fn()
}

func staleIndexLock(info IndexLockInfo) bool {
	return info.PID > 0 && !processExists(info.PID)
}

func readIndexLockInfo(path string) IndexLockInfo {
	content, err := os.ReadFile(path)
	if err != nil {
		return IndexLockInfo{}
	}
	var info IndexLockInfo
	_ = json.Unmarshal(content, &info)
	return info
}

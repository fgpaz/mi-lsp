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

func RemoveWorkspaceIndexLockForPID(root string, pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	lockPath := filepath.Join(root, ".mi-lsp", "index.lock")
	info := readIndexLockInfo(lockPath)
	if info.PID != pid {
		return false, nil
	}
	if processExists(info.PID) {
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

package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type IndexLockInfo struct {
	PID          int    `json:"pid"`
	Operation    string `json:"operation"`
	StartedAt    string `json:"started_at"`
	OwnerToken   string `json:"owner_token"`
	FencingToken int64  `json:"fencing_token"`
}

type IndexLockError struct {
	Path string
	Info IndexLockInfo
}

var (
	indexLockCleanupMu           sync.Mutex
	indexLockAfterQuarantineHook = func(string, string) {}
)

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
	return WithWorkspaceIndexLockOwned(root, operation, "", fn)
}

func WithWorkspaceIndexLockOwned(root string, operation string, ownerToken string, fn func() error) error {
	return withWorkspaceIndexLock(root, operation, ownerToken, fn, true)
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
		PID:        os.Getpid(),
		Operation:  operation,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		OwnerToken: lockOwnerToken(""),
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
			defer func() { _, _ = removeIndexLockIfOwned(lockPath, info) }()
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
	return removeWorkspaceIndexLockForPIDAndOwner(root, pid, "", false)
}

func RemoveWorkspaceIndexLockForOwner(root string, pid int, ownerToken string) (bool, error) {
	return removeWorkspaceIndexLockForPIDAndOwner(root, pid, ownerToken, false)
}

func removeWorkspaceIndexLockForPID(root string, pid int, allowRunningProcess bool) (bool, error) {
	return removeWorkspaceIndexLockForPIDAndOwner(root, pid, "", allowRunningProcess)
}

func removeWorkspaceIndexLockForPIDAndOwner(root string, pid int, ownerToken string, allowRunningProcess bool) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	lockPath := filepath.Join(root, ".mi-lsp", "index.lock")
	info := readIndexLockInfo(lockPath)
	if info.PID != pid || (ownerToken != "" && info.OwnerToken != ownerToken) {
		return false, nil
	}
	if !allowRunningProcess && processExists(info.PID) {
		return false, nil
	}
	return removeIndexLockIfOwned(lockPath, info)
}

func withWorkspaceIndexLock(root string, operation string, ownerToken string, fn func() error, allowStaleCleanup bool) error {
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	info := IndexLockInfo{
		PID:        os.Getpid(),
		Operation:  operation,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		OwnerToken: lockOwnerToken(ownerToken),
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
				if _, removeErr := removeIndexLockIfOwned(lockPath, lockInfo); removeErr != nil {
					return removeErr
				}
				if _, statErr := os.Stat(lockPath); statErr == nil {
					return &IndexLockError{Path: lockPath, Info: readIndexLockInfo(lockPath)}
				} else if !os.IsNotExist(statErr) {
					return statErr
				}
				return withWorkspaceIndexLock(root, operation, ownerToken, fn, false)
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
	defer func() { _, _ = removeIndexLockIfOwned(lockPath, info) }()
	return fn()
}

func lockOwnerToken(ownerToken string) string {
	if ownerToken != "" {
		return ownerToken
	}
	return newIndexID("idxlock")
}

func removeIndexLockIfOwned(path string, owner IndexLockInfo) (bool, error) {
	indexLockCleanupMu.Lock()
	defer indexLockCleanupMu.Unlock()

	tombstone := path + ".quarantine-" + newIndexID("idxlock")
	if err := os.Rename(path, tombstone); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// The pathname is now vacant. A replacement owner may acquire it before the
	// tombstone is inspected; the hook makes that interleaving deterministic in
	// tests without weakening the atomic rename boundary.
	indexLockAfterQuarantineHook(path, tombstone)
	current := readIndexLockInfo(tombstone)
	if sameIndexLockOwner(current, owner) && !foreignLiveProcess(current.PID) {
		if err := os.Remove(tombstone); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	// Restore through a no-replace hard link. os.Rename would replace a
	// concurrently acquired lock on Unix, while os.Rename itself has different
	// replacement semantics on Windows. A same-directory hard link is atomic and
	// fails if the pathname has already been claimed by a replacement owner.
	if err := os.Link(tombstone, path); err != nil {
		if os.IsExist(err) {
			if removeErr := os.Remove(tombstone); removeErr != nil && !os.IsNotExist(removeErr) {
				return false, removeErr
			}
			return false, nil
		}
		return false, err
	}
	if err := os.Remove(tombstone); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

func sameIndexLockOwner(left, right IndexLockInfo) bool {
	return left.PID > 0 && left.PID == right.PID && left.OwnerToken == right.OwnerToken && left.StartedAt == right.StartedAt
}

func foreignLiveProcess(pid int) bool {
	return pid > 0 && pid != os.Getpid() && processExists(pid)
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

package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const registryLockPollInterval = 25 * time.Millisecond

// registryLockTimeout bounds how long a writer waits for the registry lock.
var registryLockTimeout = 5 * time.Second

// RegistryLockTimeoutError is returned when the registry lock stays held past
// the wait window (code registry_lock_timeout).
type RegistryLockTimeoutError struct {
	Path    string
	Timeout time.Duration
}

func (e *RegistryLockTimeoutError) Error() string {
	return fmt.Sprintf("registry_lock_timeout: registry lock %s still held after %s", e.Path, e.Timeout)
}

func (e *RegistryLockTimeoutError) Code() string { return "registry_lock_timeout" }

// WithRegistryLock serializes read-modify-write cycles over registry.toml
// across goroutines and processes using an advisory file lock. The lock is
// released by the OS if the holder dies. Do not nest calls.
func WithRegistryLock(fn func() error) error {
	registryPath, err := RegistryPath()
	if err != nil {
		return err
	}
	lockFile, err := os.OpenFile(filepath.Join(filepath.Dir(registryPath), "registry.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	lockPath := lockFile.Name()
	deadline := time.Now().Add(registryLockTimeout)
	for {
		locked, lockErr := tryLockRegistryFile(lockFile)
		if lockErr != nil {
			return lockErr
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			return &RegistryLockTimeoutError{Path: lockPath, Timeout: registryLockTimeout}
		}
		time.Sleep(registryLockPollInterval)
	}
	defer func() { _ = unlockRegistryFile(lockFile) }()
	return fn()
}

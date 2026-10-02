package workspace

import (
	"os"
	"path/filepath"
)

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
	if err := lockRegistryFile(lockFile); err != nil {
		return err
	}
	defer func() { _ = unlockRegistryFile(lockFile) }()
	return fn()
}

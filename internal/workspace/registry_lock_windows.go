//go:build windows

package workspace

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const registryLockRangeLength = 1

// tryLockRegistryFile reports false (and no error) when another holder owns the lock.
func tryLockRegistryFile(file *os.File) (bool, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, registryLockRangeLength, 0, &overlapped)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return false, nil
	}
	return false, err
}

func unlockRegistryFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, registryLockRangeLength, 0, &overlapped)
}

//go:build windows

package workspace

import (
	"os"

	"golang.org/x/sys/windows"
)

const registryLockRangeLength = 1

func lockRegistryFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, registryLockRangeLength, 0, &overlapped)
}

func unlockRegistryFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, registryLockRangeLength, 0, &overlapped)
}

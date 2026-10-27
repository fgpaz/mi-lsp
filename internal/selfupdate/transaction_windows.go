//go:build windows

package selfupdate

import (
	"os"
	"syscall"
)

// MoveFile, unlike replacement rename, refuses an existing destination.
func txBackup(target, backup string) error {
	from, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(backup)
	if err != nil {
		return err
	}
	return syscall.MoveFile(from, to)
}

// Windows rejects deletion of a running image; removal itself is the liveness gate.
func txBackupSafe(_ string, _ os.FileInfo) (bool, error) { return true, nil }

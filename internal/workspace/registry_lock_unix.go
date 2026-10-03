//go:build !windows

package workspace

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockRegistryFile reports false (and no error) when another holder owns the lock.
func tryLockRegistryFile(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockRegistryFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

//go:build !windows

package workspace

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockRegistryFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockRegistryFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

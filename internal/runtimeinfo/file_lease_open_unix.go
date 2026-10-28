//go:build !windows

package runtimeinfo

import (
	"os"

	"golang.org/x/sys/unix"
)

func openFileLeaseGuard(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

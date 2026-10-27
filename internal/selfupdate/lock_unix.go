//go:build !windows

package selfupdate

import (
	"errors"
	"syscall"
)

func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

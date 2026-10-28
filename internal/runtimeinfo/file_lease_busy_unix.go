//go:build !windows

package runtimeinfo

import (
	"errors"
	"syscall"
)

func isFileLeaseBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

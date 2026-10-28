//go:build windows

package runtimeinfo

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isFileLeaseBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

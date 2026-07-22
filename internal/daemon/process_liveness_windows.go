//go:build windows

package daemon

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

const processQueryLimitedInformation = 0x1000
const stillActive = 259

func processExists(pid int) bool {
	return processExistsWith(pid, func(pid uint32) (syscall.Handle, error) {
		return syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	}, syscall.GetExitCodeProcess, syscall.CloseHandle)
}

func processExistsWith(
	pid int,
	openProcess func(uint32) (syscall.Handle, error),
	getExitCodeProcess func(syscall.Handle, *uint32) error,
	closeHandle func(syscall.Handle) error,
) bool {
	if pid <= 0 {
		return false
	}
	handle, err := openProcess(uint32(pid))
	if err != nil {
		// Win32 documents ERROR_INVALID_PARAMETER for a process ID that does
		// not exist. Every other OpenProcess error is ambiguous here, including
		// ERROR_ACCESS_DENIED, so fail closed and treat the process as alive.
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer closeHandle(handle)
	var exitCode uint32
	if err := getExitCodeProcess(handle, &exitCode); err != nil {
		return true
	}
	return exitCode == stillActive
}

//go:build windows

package store

import "syscall"

const processTerminate = 0x0001

func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	handle, err := syscall.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	return syscall.TerminateProcess(handle, 1)
}

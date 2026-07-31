//go:build !windows

package store

import (
	"os"
	"runtime"
	"strconv"
	"syscall"
)

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && err != syscall.EPERM {
		return false
	}
	if runtime.GOOS == "linux" {
		return linuxProcessExists(pid)
	}
	return true
}

func linuxProcessExists(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		// A process that disappeared after kill(pid, 0) is no longer live. For
		// other /proc read failures, retain the conservative live result.
		return !os.IsNotExist(err)
	}
	nameEnd := -1
	for i := len(stat) - 1; i >= 0; i-- {
		if stat[i] == ')' {
			nameEnd = i
			break
		}
	}
	if nameEnd < 0 || nameEnd+2 >= len(stat) {
		return true
	}
	// /proc/<pid>/stat field 3 is the process state; Z is a zombie. A zombie
	// cannot continue an index job and must not keep its reservation or lock.
	return stat[nameEnd+2] != 'Z'
}

//go:build !windows

package service

import "syscall"

// lowerProcessPriority sets the current process to the lowest scheduling
// priority (nice 19). Failures are ignored: priority is best effort.
func lowerProcessPriority() {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 19)
}

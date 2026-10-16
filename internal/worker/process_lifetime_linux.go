//go:build linux

package worker

import (
	"os/exec"
	"syscall"
)

// bindWorkerLifetime asks the kernel to SIGKILL the worker when the daemon
// dies, so a crashed or killed daemon never leaves orphan language servers.
// Go does not retire the OS threads that fork children, so the signal tracks
// the daemon process lifetime.
func bindWorkerLifetime(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}

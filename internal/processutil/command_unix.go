//go:build !windows

package processutil

import (
	"os/exec"
	"syscall"
)

func ConfigureNonInteractiveCommand(cmd *exec.Cmd) {
}

func ConfigureDetachedCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

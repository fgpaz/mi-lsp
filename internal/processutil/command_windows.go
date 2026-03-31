//go:build windows

package processutil

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow  = 0x08000000
	detachedProcess = 0x00000008
)

func ConfigureNonInteractiveCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func ConfigureDetachedCommand(cmd *exec.Cmd) {
	ConfigureNonInteractiveCommand(cmd)
	cmd.SysProcAttr.CreationFlags |= detachedProcess
}

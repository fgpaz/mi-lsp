//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// configureChild suppresses a console window. The child is not detached, so
// context cancellation still reaches it. A daemon the child starts on its own
// is a separate process and is not signaled here.
func configureChild(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

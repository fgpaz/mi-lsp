//go:build windows

package milx

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow}
}

func killManagedProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Use the trusted absolute System32 utility, never a shell or PATH lookup.
	tk := exec.Command(`C:\Windows\System32\taskkill.exe`, "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	tk.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if err := tk.Run(); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}

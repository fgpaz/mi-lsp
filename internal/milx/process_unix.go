//go:build !windows

package milx

import (
    "os/exec"
    "syscall"
)

func configureProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killManagedProcess(cmd *exec.Cmd) error {
    if cmd == nil || cmd.Process == nil { return nil }
    if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH { return err }
    return nil
}

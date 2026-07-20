//go:build windows

package milx

import (
    "os/exec"
    "syscall"
)

func configureProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP} }
func killManagedProcess(cmd *exec.Cmd) error {
    if cmd == nil || cmd.Process == nil { return nil }
    if err := cmd.Process.Kill(); err != nil { return err }
    return nil
}

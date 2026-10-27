//go:build windows

package milx

import (
	"errors"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

func configureProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

func killManagedProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Use the trusted absolute System32 utility, never a shell or PATH lookup.
	tk := processutil.Command(`C:\Windows\System32\taskkill.exe`, "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	if err := tk.Run(); err != nil {
		// A root-process fallback prevents a live child root but does not prove
		// descendants were terminated, so preserve the taskkill failure.
		return errors.Join(err, cmd.Process.Kill())
	}
	return nil
}

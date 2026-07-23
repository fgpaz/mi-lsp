//go:build linux

package milx

import (
	"os/exec"
	"syscall"
)

// NewPlatformIsolationGuard proves the kernel permits the exact private network
// namespace configuration before certifying a host guard.
func NewPlatformIsolationGuard() (*IsolationGuard, error) {
	probe := exec.Command("/bin/true")
	configureLinuxIsolation(probe)
	if err := probe.Run(); err != nil {
		return nil, NewError("GPH_MILX_NETWORK_FORBIDDEN", "isolation", false, "", "platform network isolation is unavailable")
	}
	return &IsolationGuard{networkDenied: true, processTreeContained: true}, nil
}

func configureLinuxIsolation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Unshareflags: syscall.CLONE_NEWNET}
}

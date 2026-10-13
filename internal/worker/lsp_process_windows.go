//go:build windows

package worker

import "os/exec"

// Kotlin is gated off on Windows until Job Object ownership is implemented.
func configureOwnedLSP(cmd *exec.Cmd) {}
func killOwnedLSP(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

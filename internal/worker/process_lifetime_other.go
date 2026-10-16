//go:build !linux

package worker

import "os/exec"

// bindWorkerLifetime is a no-op where the kernel has no parent-death signal;
// workers are still closed by the daemon shutdown and idle reaping paths.
func bindWorkerLifetime(cmd *exec.Cmd) {}

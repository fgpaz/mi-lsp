//go:build !windows

package mcp

import "os/exec"

// configureChild leaves the child attached. CommandContext cancels that
// process directly; a new process group would change how the CLI starts
// the shared daemon.
func configureChild(cmd *exec.Cmd) {}

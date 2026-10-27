package processutil

import (
	"context"
	"os/exec"
)

// Command has the same semantics as exec.Command, with noninteractive
// process attributes on Windows.
func Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	ConfigureNonInteractiveCommand(cmd)
	return cmd
}

// CommandContext preserves exec.CommandContext cancellation behavior.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	ConfigureNonInteractiveCommand(cmd)
	return cmd
}

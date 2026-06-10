//go:build windows

package daemon

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

func defaultEndpoint() string {
	userName := os.Getenv("USERNAME")
	if userName == "" {
		userName = "default"
	}
	userName = strings.ReplaceAll(userName, " ", "-")
	return `\\.\pipe\mi-lsp-` + userName
}

func listenDaemon() (net.Listener, error) {
	// SEC-03: Use SDDL to restrict named pipe access to owner and SYSTEM only.
	// SDDL format: D: for DACL
	// (A;;GA;;;OW) = Allow Generic All to Owner (OW)
	// (A;;GA;;;SY) = Allow Generic All to SYSTEM (SY)
	// This prevents other users from accessing the pipe.
	pipeConfig := &winio.PipeConfig{
		SecurityDescriptor: "D:(A;;GA;;;OW)(A;;GA;;;SY)",
	}
	return winio.ListenPipe(defaultEndpoint(), pipeConfig)
}

func dialDaemon(ctx context.Context) (net.Conn, error) {
	return winio.DialPipeContext(ctx, defaultEndpoint())
}

func daemonServeCommand(repoRoot string, maxWorkers int, idleTimeout time.Duration, options StartOptions) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	commandName, args := daemonServeInvocation(executable, maxWorkers, idleTimeout, options)
	command := exec.Command(commandName, args...)
	processutil.ConfigureDetachedCommand(command)
	command.Dir = repoRoot
	logDir := filepath.Join(repoRoot, ".mi-lsp")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	command.Stdout = logFile
	command.Stderr = logFile
	return command, nil
}

// detachProcess configures the command to run detached on Windows.
func detachProcess(cmd *exec.Cmd) error {
	processutil.ConfigureDetachedCommand(cmd)
	return nil
}

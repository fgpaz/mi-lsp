//go:build windows

package daemon

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
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
	return winio.ListenPipe(defaultEndpoint(), nil)
}

func dialDaemon(ctx context.Context) (net.Conn, error) {
	return winio.DialPipeContext(ctx, defaultEndpoint())
}

func daemonServeCommand(repoRoot string, maxWorkers int, idleTimeout time.Duration) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	commandName, args := daemonServeInvocation(executable, maxWorkers, idleTimeout)
	command := exec.Command(commandName, args...)
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
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008}
	return nil
}

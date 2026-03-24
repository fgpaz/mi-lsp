//go:build !windows

package daemon

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func defaultEndpoint() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "mi-lsp.sock")
	}
	return filepath.Join(home, ".mi-lsp", "mi-lsp.sock")
}

func listenDaemon() (net.Listener, error) {
	endpoint := defaultEndpoint()
	_ = os.Remove(endpoint)
	if err := os.MkdirAll(filepath.Dir(endpoint), 0o755); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, err
	}
	// Restrict socket access to owner only
	if err := os.Chmod(endpoint, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func dialDaemon(ctx context.Context) (net.Conn, error) {
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "unix", defaultEndpoint())
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

// detachProcess configures the command to run detached on Unix-like systems.
func detachProcess(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

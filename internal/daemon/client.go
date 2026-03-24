package daemon

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Execute(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	conn, err := dialDaemon(ctx)
	if err != nil {
		return model.Envelope{}, err
	}
	defer conn.Close()
	if err := worker.WriteFrame(conn, request); err != nil {
		return model.Envelope{}, err
	}
	var response model.Envelope
	if err := worker.ReadFrame(conn, &response); err != nil {
		return model.Envelope{}, err
	}
	return response, nil
}

// EnsureDaemon attempts to start the daemon if it's not already running.
// It performs a quick health check (1s timeout) and spawns a new daemon if needed.
// Returns nil if the daemon is ready, or an error if startup failed or timed out.
// Errors are not fatal—the caller should fall back to direct mode.
func EnsureDaemon(repoRoot string) error {
	// Quick health check with 1s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if _, err := probeDaemon(ctx); err == nil {
		return nil
	}

	// Daemon not running; attempt to spawn it with default settings
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	commandName, args := daemonServeInvocation(executable, 3, 30*time.Minute)
	cmd := exec.Command(commandName, args...)
	cmd.Dir = repoRoot

	// Detach the process so it survives after this invocation
	if err := detachProcess(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release the process handle so it becomes independent
	if err := cmd.Process.Release(); err != nil {
		return err
	}

	// Poll for daemon readiness for up to 3s
	deadline := time.Now().Add(3 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			_, probeErr := probeDaemon(ctx)
			cancel()
			if probeErr == nil {
				return nil
			}
			if time.Now().After(deadline) {
				return errors.New("daemon startup timed out after 3s")
			}
		}
	}
}

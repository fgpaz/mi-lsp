package daemon

import (
	"context"
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

	_, _, err := SpawnBackground(repoRoot, 3, 30*time.Minute)
	return err
}

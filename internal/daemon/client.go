package daemon

import (
	"context"
	"net"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

type Client struct {
	dial func(context.Context) (net.Conn, error)
}

func NewClient() *Client {
	return &Client{dial: dialDaemon}
}

func (c *Client) Execute(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	dial := dialDaemon
	if c != nil && c.dial != nil {
		dial = c.dial
	}
	conn, err := dial(ctx)
	if err != nil {
		return model.Envelope{}, err
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	defer stopCancel()
	if err := worker.WriteFrame(conn, request); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return model.Envelope{}, ctxErr
		}
		return model.Envelope{}, err
	}
	var response model.Envelope
	if err := worker.ReadFrame(conn, &response); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return model.Envelope{}, ctxErr
		}
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

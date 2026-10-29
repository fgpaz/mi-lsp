package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	elasticdaemon "github.com/fgpaz/bots-herramientas/mold/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

type Client struct {
	dial       func(context.Context) (net.Conn, error)
	nodeScoped bool
}

func NewClient() *Client {
	return &Client{dial: dialDaemon, nodeScoped: true}
}

func (c *Client) Execute(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	return c.execute(ctx, request, 0)
}

// ExecuteWithDialTimeout executes a request with an optional timeout limited to
// establishing the daemon connection. Once the connection is established, the
// original ctx governs the request's write, read, response processing, and
// cancellation lifecycle.
func (c *Client) ExecuteWithDialTimeout(ctx context.Context, request model.CommandRequest, dialTimeout time.Duration) (model.Envelope, error) {
	return c.execute(ctx, request, dialTimeout)
}

func (c *Client) execute(ctx context.Context, request model.CommandRequest, dialTimeout time.Duration) (model.Envelope, error) {
	if elasticEnabled() && (c == nil || c.nodeScoped) {
		response, used, err := executeNodeScoped(ctx, request, dialTimeout)
		if used {
			return response, err
		}
		if err != nil {
			return model.Envelope{}, err
		}
	}
	dial := dialDaemon
	if c != nil && c.dial != nil {
		dial = c.dial
	}
	dialCtx := ctx
	dialCancel := func() {}
	if dialTimeout > 0 {
		dialCtx, dialCancel = context.WithTimeout(ctx, dialTimeout)
	}
	conn, err := dial(dialCtx)
	dialCancel()
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

// executeNodeScoped uses the public node-scoped transport before the legacy
// per-profile endpoint. A successful dial commits the request to this route:
// after any write attempt, errors are returned as envelopes and never replayed.
func executeNodeScoped(ctx context.Context, request model.CommandRequest, dialTimeout time.Duration) (model.Envelope, bool, error) {
	node, err := readBabelNode(ctx)
	if err != nil {
		return model.Envelope{}, true, err
	}
	dialCtx := ctx
	dialCancel := func() {}
	if dialTimeout > 0 {
		dialCtx, dialCancel = context.WithTimeout(ctx, dialTimeout)
	}
	conn, err := elasticdaemon.DialContext(dialCtx, "mi-lsp-"+node)
	dialCancel()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Envelope{}, false, nil
		}
		return model.Envelope{}, false, err
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	input, err := json.Marshal(request)
	if err != nil {
		return daemonErrorEnvelope(request, err, "daemon"), true, nil
	}
	wireRequest := elasticdaemon.Request{
		Op: "mi-lsp.command", Caller: elasticCaller(request.Context.ClientName, request.Context.SessionID), Input: input,
	}
	if err := json.NewEncoder(conn).Encode(wireRequest); err != nil {
		return daemonErrorEnvelope(request, requestContextError(ctx, err), "daemon"), true, nil
	}
	var wireResponse elasticdaemon.Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&wireResponse); err != nil {
		return daemonErrorEnvelope(request, requestContextError(ctx, err), "daemon"), true, nil
	}
	if !wireResponse.OK {
		return nodeTransportErrorEnvelope(request, wireResponse), true, nil
	}
	data, err := json.Marshal(wireResponse.Data)
	if err != nil {
		return daemonErrorEnvelope(request, fmt.Errorf("decode shared daemon response: %w", err), "daemon"), true, nil
	}
	var response model.Envelope
	if err := json.Unmarshal(data, &response); err != nil {
		return daemonErrorEnvelope(request, fmt.Errorf("decode shared daemon envelope: %w", err), "daemon"), true, nil
	}
	return response, true, nil
}

func requestContextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func nodeTransportErrorEnvelope(request model.CommandRequest, response elasticdaemon.Response) model.Envelope {
	if response.ReasonCode == "ocupado" {
		return elasticBusyEnvelope(request)
	}
	return daemonErrorEnvelope(request, fmt.Errorf("shared daemon %s: %v", response.ReasonCode, response.Detail), "daemon")
}

// EnsureDaemon attempts to start the daemon if it's not already running.
// It performs a quick health check (1s timeout) and spawns a new daemon if needed.
// Returns nil if the daemon is ready, or an error if startup failed or timed out.
// Errors are not fatal—the caller should fall back to direct mode.
func EnsureDaemon(repoRoot string) error {
	return EnsureDaemonFor(repoRoot, "", "")
}

// EnsureDaemonFor preserves the real CLI caller on health probes so automated
// checks can be distinguished from an explicit `mi-lsp daemon status` call.
func EnsureDaemonFor(repoRoot, callerClient, callerOperation string) error {
	probeClient := healthProbeClientName(callerClient, callerOperation)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if elasticEnabled() {
		if used, err := ensureElasticHealth(ctx, probeClient); used || err != nil {
			return err
		}
	}
	if err := ensureLegacyHealth(ctx, probeClient); err == nil {
		return nil
	}

	_, _, err := SpawnBackground(repoRoot, DefaultMaxWorkers(), 30*time.Minute)
	return err
}

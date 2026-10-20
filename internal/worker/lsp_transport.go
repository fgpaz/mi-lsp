package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

// One reader owns framing for a process generation. Notifications never fill the
// response queue; unmatched (including cancelled) response IDs are discarded.
type lspTransport struct {
	cmd            *exec.Cmd
	in             io.WriteCloser
	out            io.ReadCloser
	responses      chan lspResponse
	done           chan struct{}
	writeMu        sync.Mutex
	stopOnce       sync.Once
	diagnosticMu   sync.Mutex
	diagnostics    map[string]publishedDiagnostics
	diagnosticWake chan struct{}
}

func newLSPTransport(cmd *exec.Cmd, in io.WriteCloser, out io.ReadCloser) *lspTransport {
	t := &lspTransport{cmd: cmd, in: in, out: out, responses: make(chan lspResponse, 16), done: make(chan struct{}), diagnostics: make(map[string]publishedDiagnostics), diagnosticWake: make(chan struct{}, 1)}
	go func() {
		defer close(t.done)
		defer t.stop()
		r := bufio.NewReaderSize(out, 64*1024)
		for {
			raw, err := readLSPMessage(r)
			if err != nil {
				return
			}
			var envelope struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(raw, &envelope) != nil {
				return
			}
			if envelope.Method != "" {
				if envelope.Method == "textDocument/publishDiagnostics" {
					t.acceptDiagnostics(raw)
				}
				if len(envelope.ID) > 0 && string(envelope.ID) != "null" { // No server-initiated mutations/configuration are silently accepted.
					reply := map[string]any{"jsonrpc": "2.0", "id": envelope.ID, "error": map[string]any{"code": -32601, "message": "unsupported client request"}}
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					_ = t.write(ctx, reply)
					cancel()
				}
				continue
			}
			var response lspResponse
			if json.Unmarshal(raw, &response) != nil {
				return
			}
			select {
			case t.responses <- response:
			default:
				return
			}
		}
	}()
	go func() { _ = cmd.Wait(); t.stop() }()
	return t
}
func (t *lspTransport) stop() {
	t.stopOnce.Do(func() { _ = t.in.Close(); _ = t.out.Close(); killOwnedLSP(t.cmd) })
}
func (t *lspTransport) alive() bool {
	select {
	case <-t.done:
		return false
	default:
		return true
	}
}
func (t *lspTransport) write(ctx context.Context, message any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() { t.writeMu.Lock(); defer t.writeMu.Unlock(); result <- writeLSPMessage(t.in, message) }()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.stop()
		return ctx.Err()
	case <-t.done:
		return errors.New("LSP process exited")
	}
}
func (t *lspTransport) request(ctx context.Context, id int, method string, params any) (json.RawMessage, error) {
	if err := t.write(ctx, lspRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			_ = t.write(cancelCtx, lspNotification{JSONRPC: "2.0", Method: "$/cancelRequest", Params: map[string]any{"id": id}})
			cancel()
			// Invalidate the generation: no late response or uncertain initialization
			// can satisfy the next request. Restart is only on a subsequent caller.
			t.stop()
			return nil, ctx.Err()
		case <-t.done:
			return nil, errors.New("LSP process exited")
		case response := <-t.responses:
			if response.ID != id {
				continue
			}
			if response.Error != nil {
				if response.Error.Code == -32801 {
					return nil, lspContentModified
				}
				return nil, errors.New("LSP request failed")
			}
			return response.Result, nil
		}
	}
}

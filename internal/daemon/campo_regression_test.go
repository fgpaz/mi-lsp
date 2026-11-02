package daemon

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestLocalClientDoesNotRequireBotsd(t *testing.T) {
	t.Setenv("MI_LSP_ELASTIC", "")
	t.Setenv("PATH", t.TempDir())
	sentinel := errors.New("local dial reached")
	client := &Client{nodeScoped: true, dial: func(context.Context) (net.Conn, error) { return nil, sentinel }}
	_, err := client.Execute(context.Background(), model.CommandRequest{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected local dial, got %v", err)
	}
}

func TestElasticClientRequiresBotsd(t *testing.T) {
	t.Setenv("MI_LSP_ELASTIC", "1")
	t.Setenv("PATH", t.TempDir())
	_, err := NewClient().Execute(context.Background(), model.CommandRequest{})
	if err == nil || !strings.Contains(err.Error(), "requires botsd") {
		t.Fatalf("expected actionable botsd error, got %v", err)
	}
}

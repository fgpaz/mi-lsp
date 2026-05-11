package worker

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestCanUseGoplsDoesNotPanic(t *testing.T) {
	_ = CanUseGopls("/tmp")
}

func TestNewGoplsClientWithoutBinaryFailsGracefully(t *testing.T) {
	client, err := NewGoplsClient(model.WorkspaceRegistration{
		Root: "/path/that/should/not/contain/gopls",
		Name: "test",
	})
	if err == nil {
		if client == nil {
			t.Fatal("NewGoplsClient returned nil client with nil error")
		}
		return
	}
	if !strings.Contains(err.Error(), "gopls") {
		t.Fatalf("error = %q, want gopls guidance", err.Error())
	}
}

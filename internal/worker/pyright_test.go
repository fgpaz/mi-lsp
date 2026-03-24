package worker

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestCanUsePyright(t *testing.T) {
	// This test just checks that the function doesn't panic
	// Actual availability depends on system setup
	_ = CanUsePyright("/tmp")
}

func TestNewPyrightClientWithoutBinary(t *testing.T) {
	// Create a workspace with a root that is unlikely to have pyright installed
	workspace := model.WorkspaceRegistration{
		Root: "/nonexistent/path",
		Name: "test",
	}

	// This should fail if pyright is not available globally
	client, err := NewPyrightClient(workspace)

	// Either it works (if pyright is installed) or fails gracefully
	if err == nil {
		// If pyright is available, we got a client
		if client == nil {
			t.Errorf("NewPyrightClient returned nil client with nil error")
		}
	} else {
		// Error is expected if pyright is not available
		t.Logf("NewPyrightClient returned expected error: %v", err)
	}
}

func TestDetectPythonPath(t *testing.T) {
	path := detectPythonPath()
	if path == "" {
		t.Errorf("detectPythonPath returned empty string")
	}
}

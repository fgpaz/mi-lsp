package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func NewRustAnalyzerClient(workspace model.WorkspaceRegistration) (*LSPClient, error) {
	binary, err := findRustAnalyzerBinary(workspace.Root)
	if err != nil {
		return nil, err
	}
	return NewLSPClient(LSPConfig{ServerCmd: binary}, workspace), nil
}

func CanUseRustAnalyzer(workspaceRoot string) bool {
	_, err := findRustAnalyzerBinary(workspaceRoot)
	return err == nil
}

func findRustAnalyzerBinary(workspaceRoot string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("MI_LSP_RUST_ANALYZER_PATH")); override != "" {
		if isRegularFile(override) {
			return override, nil
		}
	}
	if path, err := exec.LookPath("rust-analyzer"); err == nil {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".cargo", "bin", "rust-analyzer"))
	}
	candidates = append(candidates,
		filepath.Join(workspaceRoot, "bin", "rust-analyzer"),
		filepath.Join(workspaceRoot, ".bin", "rust-analyzer"),
	)
	for _, candidate := range candidates {
		if isRegularFile(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("rust-analyzer is unavailable; install the rust-analyzer component with rustup or set MI_LSP_RUST_ANALYZER_PATH")
}

package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// NewPyrightClient creates an LSPClient configured for Pyright.
func NewPyrightClient(workspace model.WorkspaceRegistration) (*LSPClient, error) {
	binary, err := findPyrightBinary(workspace.Root)
	if err != nil {
		return nil, err
	}
	config := LSPConfig{
		ServerCmd:  binary,
		ServerArgs: []string{"--stdio"},
		InitOptions: map[string]any{
			"pythonPath": detectPythonPath(),
		},
	}
	return NewLSPClient(config, workspace), nil
}

// CanUsePyright checks if Pyright is available for the workspace.
func CanUsePyright(workspaceRoot string) bool {
	_, err := findPyrightBinary(workspaceRoot)
	return err == nil
}

func findPyrightBinary(workspaceRoot string) (string, error) {
	// 1. pyright-langserver in PATH
	if path, err := exec.LookPath("pyright-langserver"); err == nil {
		return path, nil
	}

	// 2. Local node_modules
	localBin := filepath.Join(workspaceRoot, "node_modules", ".bin", "pyright-langserver")
	if _, err := os.Stat(localBin); err == nil {
		return localBin, nil
	}

	// Windows .cmd variant
	localCmd := localBin + ".cmd"
	if _, err := os.Stat(localCmd); err == nil {
		return localCmd, nil
	}

	// 3. npm global bin (npm root -g returns lib dir; bin is a sibling)
	if globalRoot, err := globalNpmRoot(); err == nil {
		globalBin := filepath.Join(filepath.Dir(strings.TrimSpace(globalRoot)), "bin", "pyright-langserver")
		if _, statErr := os.Stat(globalBin); statErr == nil {
			return globalBin, nil
		}
		globalCmd := globalBin + ".cmd"
		if _, statErr := os.Stat(globalCmd); statErr == nil {
			return globalCmd, nil
		}
	}

	return "", errors.New("pyright-langserver is unavailable; install pyright via npm or pip")
}

func detectPythonPath() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return "python"
}

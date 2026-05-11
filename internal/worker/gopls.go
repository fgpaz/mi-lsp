package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func NewGoplsClient(workspace model.WorkspaceRegistration) (*LSPClient, error) {
	binary, err := findGoplsBinary(workspace.Root)
	if err != nil {
		return nil, err
	}
	config := LSPConfig{
		ServerCmd:  binary,
		ServerArgs: []string{},
	}
	return NewLSPClient(config, workspace), nil
}

func CanUseGopls(workspaceRoot string) bool {
	_, err := findGoplsBinary(workspaceRoot)
	return err == nil
}

func findGoplsBinary(workspaceRoot string) (string, error) {
	if path, err := exec.LookPath("gopls"); err == nil {
		return path, nil
	}

	for _, rel := range []string{
		filepath.Join("bin", "gopls"),
		filepath.Join("bin", "gopls.exe"),
		filepath.Join(".bin", "gopls"),
		filepath.Join(".bin", "gopls.exe"),
	} {
		candidate := filepath.Join(workspaceRoot, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("gopls is unavailable; install it with `go install golang.org/x/tools/gopls@latest`")
}

package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	if override := strings.TrimSpace(os.Getenv("MI_LSP_GOPLS_PATH")); override != "" {
		if isRegularFile(override) {
			return override, nil
		}
	}
	if path, err := exec.LookPath("gopls"); err == nil {
		return path, nil
	}
	for _, dir := range goplsInstallDirs() {
		for _, name := range []string{"gopls", "gopls.exe"} {
			candidate := filepath.Join(dir, name)
			if isRegularFile(candidate) {
				return candidate, nil
			}
		}
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

// goplsInstallDirs lists where `go install` puts gopls when it is not on PATH:
// $GOBIN, each $GOPATH/bin (default ~/go), then ~/go/bin.
func goplsInstallDirs() []string {
	dirs := []string{}
	if gobin := strings.TrimSpace(os.Getenv("GOBIN")); gobin != "" {
		dirs = append(dirs, gobin)
	}
	home, _ := os.UserHomeDir()
	gopath := strings.TrimSpace(os.Getenv("GOPATH"))
	if gopath == "" && home != "" {
		gopath = filepath.Join(home, "go")
	}
	for _, entry := range filepath.SplitList(gopath) {
		if entry != "" {
			dirs = append(dirs, filepath.Join(entry, "bin"))
		}
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}
	return dirs
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

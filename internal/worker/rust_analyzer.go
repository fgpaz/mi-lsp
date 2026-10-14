package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func NewRustAnalyzerClient(workspace model.WorkspaceRegistration, request model.WorkerRequest) (*LSPClient, error) {
	binary, err := findRustAnalyzerBinary(workspace.Root)
	if err != nil {
		return nil, err
	}
	config := LSPConfig{ServerCmd: binary, Backend: "rust-analyzer"}
	if linked := rustLinkedCargoManifests(workspace, request); len(linked) > 0 {
		config.InitOptions = map[string]any{"linkedProjects": linked}
	}
	return NewLSPClient(config, workspace), nil
}

// rustLinkedCargoManifests tells rust-analyzer which Cargo.toml to load when the
// workspace root is not a Rust crate (monorepos) or when the caller selected an
// explicit entrypoint.
func rustLinkedCargoManifests(workspace model.WorkspaceRegistration, request model.WorkerRequest) []string {
	root := filepath.Clean(workspace.Root)
	if manifest := cargoManifestFromEntrypoint(root, request.EntrypointPath); manifest != "" {
		return []string{manifest}
	}
	if file, _ := request.Payload["file"].(string); file != "" {
		if manifest := nearestCargoManifest(root, file); manifest != "" {
			return []string{manifest}
		}
	}
	if manifest := filepath.Join(root, "Cargo.toml"); isRegularFile(manifest) {
		return []string{manifest}
	}
	return nil
}

func cargoManifestFromEntrypoint(repoRoot string, entrypointPath string) string {
	entrypointPath = strings.TrimSpace(filepath.ToSlash(entrypointPath))
	if entrypointPath == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(entrypointPath))
	if base != "cargo.toml" {
		return ""
	}
	manifest := filepath.Join(repoRoot, filepath.FromSlash(entrypointPath))
	if isRegularFile(manifest) {
		return manifest
	}
	return ""
}

func nearestCargoManifest(repoRoot string, file string) string {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, filepath.FromSlash(file))
	}
	abs = filepath.Clean(abs)
	dir := filepath.Clean(filepath.Dir(abs))
	repoRoot = filepath.Clean(repoRoot)
	for i := 0; i < 48; i++ {
		manifest := filepath.Join(dir, "Cargo.toml")
		if isRegularFile(manifest) {
			return manifest
		}
		parent := filepath.Dir(dir)
		if parent == dir || (dir != repoRoot && !strings.HasPrefix(dir, repoRoot+string(filepath.Separator)) && dir != repoRoot) {
			break
		}
		dir = parent
	}
	return ""
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

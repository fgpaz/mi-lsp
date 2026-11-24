// Package statedir resolves where mi-lsp keeps per-workspace state
// (project.toml, index.db, locks, catalog stamps, logs and job logs).
//
// It is the single source of truth for that location (RF-WKS-007-B04):
//   - default, or MI_LSP_STATE_LOCATION=repo: <root>/.mi-lsp, unchanged;
//   - MI_LSP_STATE_LOCATION=machine: a machine-local directory under the user
//     cache dir keyed by the workspace's physical path, so nothing is written
//     under the repository.
package statedir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Env selects the workspace state location.
const Env = "MI_LSP_STATE_LOCATION"

const (
	LocationRepo    = "repo"
	LocationMachine = "machine"
)

// RepoDir is the repo-local state directory: the default location and the
// legacy location when machine mode is active.
func RepoDir(root string) string {
	return filepath.Join(root, ".mi-lsp")
}

// MachineDir is the machine-local state directory for root:
// <user cache dir>/mi-lsp/workspaces/<sha256(physical path)>. Two repos, or a
// worktree and its parent, resolve to different directories. It returns ""
// when the user cache dir is unavailable and never creates anything.
func MachineDir(root string) string {
	cache, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cache) == "" {
		return ""
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		absolute = filepath.Clean(root)
	}
	if evaluated, evalErr := filepath.EvalSymlinks(absolute); evalErr == nil {
		absolute = evaluated
	}
	key := filepath.Clean(absolute)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		key = strings.ToLower(key)
	}
	hash := sha256.Sum256([]byte(key))
	return filepath.Join(cache, "mi-lsp", "workspaces", hex.EncodeToString(hash[:]))
}

// Machine reports whether state must stay outside the repository. Any value
// other than empty or "repo" counts as machine, so a mistyped value never
// falls back to writing under the repository; Validate reports it.
func Machine() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(Env)))
	return value != "" && value != LocationRepo
}

// Validate rejects unknown values and a machine location that cannot be
// resolved. The CLI calls it before any command runs.
func Validate() error {
	raw := os.Getenv(Env)
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", LocationRepo:
		return nil
	case LocationMachine:
		if MachineDir(".") == "" {
			return fmt.Errorf("%s=machine needs a user cache directory (set XDG_CACHE_HOME or HOME); nothing was written", Env)
		}
		return nil
	default:
		return fmt.Errorf("invalid %s=%q; valid values: repo, machine", Env, raw)
	}
}

// Dir is the state directory for root in the configured location. In machine
// mode it never returns a path under root.
func Dir(root string) string {
	if !Machine() {
		return RepoDir(root)
	}
	if dir := MachineDir(root); dir != "" {
		return dir
	}
	// Unreachable from the CLI, which runs Validate first. Never fall back to
	// the repository.
	panic(Validate())
}

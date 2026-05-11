package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ExecutableSnapshot struct {
	Path    string
	Size    int64
	ModTime time.Time
	SHA256  string
}

func CurrentExecutableSnapshot() ExecutableSnapshot {
	path, err := os.Executable()
	if err != nil {
		return ExecutableSnapshot{}
	}
	snapshot := ExecutableSnapshot{Path: filepath.Clean(path)}
	if info, err := os.Stat(path); err == nil {
		snapshot.Size = info.Size()
		snapshot.ModTime = info.ModTime()
	}
	snapshot.SHA256 = fileSHA256(path)
	return snapshot
}

func (s ExecutableSnapshot) ApplyToDaemonState(state *DaemonState) {
	if state == nil {
		return
	}
	state.ExecutablePath = s.Path
	state.ExecutableSize = s.Size
	state.ExecutableMTime = s.ModTime
	state.ExecutableSHA256 = s.SHA256
}

func DaemonStaleWarning(state DaemonState, cli ExecutableSnapshot) string {
	if strings.TrimSpace(state.ExecutablePath) == "" {
		return "daemon did not report executable metadata; it may be running an older build. Run `mi-lsp daemon restart` after installing a new CLI build."
	}
	if strings.TrimSpace(cli.Path) == "" {
		return ""
	}
	daemonPath := filepath.Clean(state.ExecutablePath)
	cliPath := filepath.Clean(cli.Path)
	if state.ExecutableSHA256 != "" && cli.SHA256 != "" {
		if state.ExecutableSHA256 != cli.SHA256 {
			return fmt.Sprintf("daemon executable appears stale: daemon hash=%s cli hash=%s at %s. Run `mi-lsp daemon restart`.", shortHash(state.ExecutableSHA256), shortHash(cli.SHA256), cliPath)
		}
		return ""
	}
	if state.ExecutableSize > 0 && cli.Size > 0 && state.ExecutableSize != cli.Size {
		return fmt.Sprintf("daemon executable appears stale: daemon size=%d cli size=%d at %s. Run `mi-lsp daemon restart`.", state.ExecutableSize, cli.Size, cliPath)
	}
	if samePath(daemonPath, cliPath) && !state.ExecutableMTime.IsZero() && !cli.ModTime.IsZero() && !state.ExecutableMTime.Equal(cli.ModTime) {
		return fmt.Sprintf("daemon executable appears stale: daemon mtime=%s cli mtime=%s at %s. Run `mi-lsp daemon restart`.", state.ExecutableMTime.UTC().Format(time.RFC3339), cli.ModTime.UTC().Format(time.RFC3339), cliPath)
	}
	if !samePath(daemonPath, cliPath) {
		return fmt.Sprintf("daemon executable path differs from current CLI and no hash is available: daemon=%s cli=%s. Run `mi-lsp daemon restart` after installing a new CLI build.", daemonPath, cliPath)
	}
	return ""
}

func samePath(left string, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func fileSHA256(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

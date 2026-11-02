package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

var goEnvFingerprintFailureNonce atomic.Uint64

// GraphObserverRuntimeFingerprint returns a read-only fingerprint of runtime
// configuration that can affect graph observers. It deliberately does not
// probe or launch workers, create install directories, or expose raw settings.
// If the effective Go environment config cannot be read, the per-call nonce
// makes the result unsuitable for cache reuse.
func GraphObserverRuntimeFingerprint(repoRoot string) string {
	h := sha256.New()
	field := func(name, value string) {
		_, _ = fmt.Fprintf(h, "%d:%s%d:%s;", len(name), name, len(value), value)
	}
	field("runtime", runtime.Version())
	field("goos", runtime.GOOS)
	field("goarch", runtime.GOARCH)
	field("protocol", model.ProtocolVersion)
	for _, name := range []string{
		"PATH", "HOME", "GOROOT", "GOPATH", "GOMODCACHE", "GOCACHE", "GOENV", "GOTOOLCHAIN",
		"GOWORK", "GOFLAGS", "GOPROXY", "GOSUMDB", "CGO_ENABLED", "DOTNET_ROOT",
		"DOTNET_ROOT_X64", "DOTNET_ROOT_ARM64", "DOTNET_MULTILEVEL_LOOKUP", "DOTNET_ROLL_FORWARD",
		"DOTNET_HOST_PATH", "DOTNET_CLI_HOME", "MI_LSP_KOTLIN_ENABLE", "MI_LSP_KOTLIN_LANGUAGE_SERVER_PATH",
		"JAVA_HOME", "JAVA_OPTS", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "GRADLE_OPTS", "GRADLE_USER_HOME",
	} {
		field("env:"+name, os.Getenv(name))
	}
	if digest, err := goEnvConfigDigest(); err != nil {
		field("goenv_config", "unavailable")
		field("uncacheable_nonce", fmt.Sprint(goEnvFingerprintFailureNonce.Add(1)))
	} else {
		field("goenv_config_sha256", digest)
	}

	toolRoot, toolRootKind := ResolveToolRoot(repoRoot)
	field("tool_root", filepath.Clean(toolRoot))
	field("tool_root_kind", toolRootKind)
	field("repo_root", filepath.Clean(repoRoot))
	field("rid", ResolveRID())
	for _, name := range []string{"go", "dotnet", "java"} {
		path, err := lookPathFn(name)
		if err != nil {
			field("command:"+name, "unresolved")
			continue
		}
		field("command:"+name, path)
		fingerprintGraphRuntimePath(h, "command-file:"+name, path, false)
	}

	candidateDirs := append([]string(nil), bundledWorkerDirs(toolRoot, ResolveRID())...)
	if home, err := os.UserHomeDir(); err == nil {
		candidateDirs = append(candidateDirs, filepath.Join(home, ".mi-lsp", "workers", ResolveRID()))
	}
	candidateDirs = append(candidateDirs,
		filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "bin", "Debug", "net10.0"),
		filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "bin", "Release", "net10.0"),
		filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker"),
	)
	for _, dir := range candidateDirs {
		field("worker_dir", filepath.Clean(dir))
		entries, err := os.ReadDir(dir)
		if err != nil {
			field("worker_dir_state", "unavailable")
			continue
		}
		for _, entry := range entries {
			fingerprintGraphRuntimePath(h, "worker_file:"+filepath.Join(dir, entry.Name()), filepath.Join(dir, entry.Name()), false)
		}
	}

	launcher := strings.TrimSpace(os.Getenv("MI_LSP_KOTLIN_LANGUAGE_SERVER_PATH"))
	if launcher != "" {
		fingerprintGraphRuntimePath(h, "kotlin_launcher", launcher, true)
		fingerprintGraphRuntimePath(h, "kotlin_manifest", launcher+".mi-lsp.json", true)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// goEnvConfigDigest asks the official Go tool to resolve GOENV, including its
// default path and the explicit "off" value. Only a digest of the config bytes
// is returned; raw settings and command output never enter the runtime key.
func goEnvConfigDigest() (string, error) {
	goPath, err := lookPathFn("go")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := processutil.CommandContext(ctx, goPath, "env", "GOENV").Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" && strings.EqualFold(strings.TrimSpace(os.Getenv("GOENV")), "off") {
		return "off", nil
	}
	if path == "" {
		return "", fmt.Errorf("go env returned an empty GOENV path")
	}
	if path == "off" {
		return "off", nil
	}
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "absent", nil
	}
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), nil
}

func fingerprintGraphRuntimePath(h interface{ Write([]byte) (int, error) }, label, path string, hashContents bool) {
	_, _ = fmt.Fprintf(h, "%d:%s%d:%s;", len(label), label, len(path), path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		_, _ = fmt.Fprintf(h, "real:%d:%s;", len(resolved), resolved)
	}
	info, err := os.Stat(path)
	if err != nil {
		_, _ = fmt.Fprintf(h, "state:missing:%T;", err)
		return
	}
	_, _ = fmt.Fprintf(h, "stat:%d:%d:%d:%t:%#v;", info.Size(), info.ModTime().UnixNano(), info.Mode(), info.IsDir(), info.Sys())
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			_, _ = fmt.Fprintf(h, "readdir-error:%T;", err)
			return
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = fmt.Fprintf(h, "child:%d:%s;", len(name), name)
		}
		return
	}
	if (hashContents || strings.EqualFold(filepath.Ext(path), ".json")) && info.Mode().IsRegular() && info.Size() >= 0 && info.Size() <= 2<<20 {
		data, err := os.ReadFile(path)
		if err == nil {
			digest := sha256.Sum256(data)
			_, _ = fmt.Fprintf(h, "content:%x;", digest)
		}
	}
}

package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// GraphObserverRuntimeFingerprint returns a read-only fingerprint of runtime
// configuration that can affect graph observers. It deliberately does not
// probe or launch workers, create install directories, or expose raw settings.
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

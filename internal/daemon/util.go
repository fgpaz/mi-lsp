package daemon

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}

func daemonServeInvocation(executable string, maxWorkers int, idleTimeout time.Duration, options StartOptions) (string, []string) {
	options = NormalizeStartOptions(options)
	args := []string{
		"daemon", "serve",
		"--max-workers", strconvItoa(maxWorkers),
		"--idle-timeout", idleTimeout.String(),
		"--watch-mode", options.WatchMode,
		"--max-watched-roots", strconvItoa(options.MaxWatchedRoots),
		"--max-inflight", strconvItoa(options.MaxInflight),
	}
	if isGoRunExecutable(executable) {
		return "go", append([]string{"run", "./cmd/mi-lsp"}, args...)
	}
	return executable, args
}

func releaseStartedCommand(command *exec.Cmd) {
	if command == nil {
		return
	}
	closers := map[io.Closer]struct{}{}
	if closer, ok := command.Stdout.(io.Closer); ok && closer != nil {
		closers[closer] = struct{}{}
	}
	if closer, ok := command.Stderr.(io.Closer); ok && closer != nil {
		closers[closer] = struct{}{}
	}
	for closer := range closers {
		_ = closer.Close()
	}
	if command.Process != nil {
		_ = command.Process.Release()
	}
}

func isGoRunExecutable(executable string) bool {
	tempDir := filepath.Clean(os.TempDir())
	candidate := filepath.Clean(executable)
	goBuildRoot := filepath.Join(tempDir, "go-build")
	return strings.Contains(strings.ToLower(candidate), strings.ToLower(goBuildRoot))
}

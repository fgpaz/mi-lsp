package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}

func daemonServeInvocation(executable string, maxWorkers int, idleTimeout time.Duration) (string, []string) {
	args := []string{"daemon", "serve", "--max-workers", strconvItoa(maxWorkers), "--idle-timeout", idleTimeout.String()}
	if isGoRunExecutable(executable) {
		return "go", append([]string{"run", "./cmd/mi-lsp"}, args...)
	}
	return executable, args
}

func isGoRunExecutable(executable string) bool {
	tempDir := filepath.Clean(os.TempDir())
	candidate := filepath.Clean(executable)
	goBuildRoot := filepath.Join(tempDir, "go-build")
	return strings.Contains(strings.ToLower(candidate), strings.ToLower(goBuildRoot))
}

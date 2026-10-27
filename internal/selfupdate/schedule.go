package selfupdate

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

func newAutomaticChildCommand(exe string) *exec.Cmd {
	cmd := processutil.Command(exe, "update")
	processutil.ConfigureDetachedCommand(cmd)
	env := os.Environ()
	cmd.Env = make([]string, 0, len(env)+1)
	for _, v := range env {
		if !strings.HasPrefix(strings.ToUpper(v), "MI_LSP_UPDATE_CHILD=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "MI_LSP_UPDATE_CHILD=1")
	return cmd
}

var startAutomaticChild = func(cmd *exec.Cmd) error {
	if e := cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}

func MaybeStartAutomatic(c Config, daemonAvailable bool) (bool, error) {
	if daemonAvailable || os.Getenv("MI_LSP_UPDATE_CHILD") == "1" {
		return false, nil
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	_, _, reason, e := updatePolicy(c)
	if e != nil {
		return false, e
	}
	if reason != "" {
		return false, nil
	}
	var cache updateCache
	if readJSON(cachePath(c), &cache) == nil && !cache.LastCheck.IsZero() && c.Now().Sub(cache.LastCheck) < 24*time.Hour {
		return false, nil
	}
	if !filepathIsSafeExecutable(c.ExecutablePath) {
		return false, errors.New("invalid executable")
	}
	if e = startAutomaticChild(newAutomaticChildCommand(c.ExecutablePath)); e != nil {
		return false, errors.New("cannot start updater")
	}
	return true, nil
}
func filepathIsSafeExecutable(path string) bool { return path != "" }

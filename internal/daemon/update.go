package daemon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/selfupdate"
)

// CompatibleUpdateOwner verifies a live daemon's installation and release before
// letting it own the scheduler. Unknown, old, and foreign daemons do not own it.
func CompatibleUpdateOwner(ctx context.Context, executable string) bool {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	state, err := probeDaemon(ctx)
	if err != nil || state.ExecutablePath == "" || !updateReleaseCompatible(state.Version) {
		return false
	}
	left, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return false
	}
	right, err := filepath.EvalSymlinks(state.ExecutablePath)
	if err != nil {
		return false
	}
	left, err = filepath.Abs(left)
	if err != nil {
		return false
	}
	right, err = filepath.Abs(right)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func updateReleaseCompatible(version string) bool {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	v = strings.SplitN(v, "+", 2)[0]
	if strings.Contains(v, "-") {
		return false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	_, e3 := strconv.Atoi(parts[2])
	return e1 == nil && e2 == nil && e3 == nil && (major > 0 || minor >= 16)
}

// runUpdateSchedule never performs network work on the serving goroutine.
// Until all long-lived contexts and background work are observable, it does
// not restart the process: an unknown idle state is always treated as busy.
func (s *Server) runUpdateSchedule(ctx context.Context) {
	version := s.state.Version
	if version == "" || version == "dev" || version == "unknown" || strings.Contains(version, "devel") || os.Getenv("MI_LSP_UPDATE_CHILD") == "1" {
		return
	}
	cfg, err := selfupdate.DefaultConfig(version)
	if err != nil {
		return
	}
	_, _ = selfupdate.MaybeStartAutomatic(cfg, false)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case <-ticker.C:
			_, _ = selfupdate.MaybeStartAutomatic(cfg, false)
		}
	}
}

// Admission tracks all activities registered by the daemon. An idle barrier
// excludes new admissions until released; it is not by itself proof that
// uninstrumented background activity or external sessions are idle.
type updateAdmissionGate struct {
	mu      sync.Mutex
	active  int
	barrier bool
}

func newUpdateAdmissionGate() *updateAdmissionGate { return &updateAdmissionGate{} }
func (g *updateAdmissionGate) Begin(_ string) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.barrier {
		return nil, false
	}
	g.active++
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.active--; g.mu.Unlock() }) }, true
}
func (g *updateAdmissionGate) TryIdleBarrier() (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.barrier || g.active != 0 {
		return nil, false
	}
	g.barrier = true
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.barrier = false; g.mu.Unlock() }) }, true
}

type updateRestartInvocation struct {
	Executable string
	Args       []string
	Env        []string
	Endpoint   string
	HomeDir    string
}
type updateRestartOps struct {
	CleanupOld func() error
	Shutdown   func() error
	Spawn      func(updateRestartInvocation) error
}

func restartUpdatedDaemon(ctx context.Context, invocation updateRestartInvocation, ops updateRestartOps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ops.CleanupOld(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ops.Shutdown(); err != nil {
		return err
	}
	return ops.Spawn(invocation)
}

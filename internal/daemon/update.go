package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/selfupdate"
)

// CompatibleUpdateOwner verifies a live daemon's installation and release before
// letting it own the scheduler. Unknown, old, and foreign daemons do not own it.
func CompatibleUpdateOwner(ctx context.Context, executable string) bool {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	state, err := cachedUpdateOwner(ctx)
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

// cachedUpdateOwner avoids a status RPC for every short-lived CLI invocation.
// A cache hit still requires the same daemon state file and a live transport.
func cachedUpdateOwner(ctx context.Context) (model.DaemonState, error) {
	return cachedUpdateOwnerWith(ctx, probeDaemon, func(ctx context.Context) error {
		conn, err := dialDaemon(ctx)
		if err == nil {
			_ = conn.Close()
		}
		return err
	})
}

func cachedUpdateOwnerWith(ctx context.Context, probe func(context.Context) (model.DaemonState, error), dial func(context.Context) error) (model.DaemonState, error) {
	dir, err := daemonRootDir()
	if err != nil {
		return probe(ctx)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "update-owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return probe(ctx)
	}
	defer lock.Close()
	for {
		if err := ctx.Err(); err != nil {
			return model.DaemonState{}, err
		}
		locked, lockErr := tryLockStartGuard(lock)
		if lockErr != nil {
			return model.DaemonState{}, lockErr
		}
		if locked {
			break
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return model.DaemonState{}, ctx.Err()
		case <-timer.C:
		}
	}
	defer unlockStartGuard(lock)
	statePath, err := daemonStatePath()
	if err != nil {
		return probe(ctx)
	}
	info, statErr := os.Stat(statePath)
	path := filepath.Join(dir, "update-owner-cache.json")
	var cached struct {
		Checked        time.Time `json:"checked"`
		StateMod       time.Time `json:"state_mod"`
		StateSize      int64     `json:"state_size"`
		ExecutablePath string    `json:"executable_path"`
		Version        string    `json:"version"`
	}
	body, readErr := os.ReadFile(path)
	if statErr == nil && readErr == nil && json.Unmarshal(body, &cached) == nil &&
		time.Since(cached.Checked) >= 0 && time.Since(cached.Checked) < 70*time.Second &&
		info.ModTime().Equal(cached.StateMod) && info.Size() == cached.StateSize {
		if dialErr := dial(ctx); dialErr == nil {
			return model.DaemonState{ExecutablePath: cached.ExecutablePath, Version: cached.Version}, nil
		}
	}
	state, err := probe(ctx)
	if err == nil && state.ExecutablePath != "" && updateReleaseCompatible(state.Version) {
		if after, err := os.Stat(statePath); statErr == nil && err == nil && info.ModTime().Equal(after.ModTime()) && info.Size() == after.Size() {
			cached.Checked, cached.StateMod, cached.StateSize = time.Now(), after.ModTime(), after.Size()
			cached.ExecutablePath, cached.Version = state.ExecutablePath, state.Version
			if body, err := json.Marshal(cached); err == nil {
				_ = os.WriteFile(path, body, 0600)
			}
		}
	}
	return state, err
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
// Source scheduling is hourly; disk change detection is independent and fast.
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
	controller := time.NewTicker(time.Second)
	defer controller.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case <-ticker.C:
			_, _ = selfupdate.MaybeStartAutomatic(cfg, false)
		case <-controller.C:
			s.maybeRestartUpdated(ctx, cfg)
		}
	}
}

func updateAdminHandler(gate *updateAdmissionGate, next http.Handler) http.Handler {
	if gate == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finish, ok := gate.Begin("admin-http")
		if !ok {
			http.Error(w, "daemon temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		defer finish()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) updateResourcesIdle(now time.Time) bool {
	return s != nil && s.manager != nil && s.qSessions != nil && s.manager.UpdateIdleSnapshot() && !s.qSessions.HasLiveSessions(now)
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
	Cwd        string
}
type updateRestartOps struct {
	CleanupOld func() error
	Shutdown   func() error
	Spawn      func(updateRestartInvocation) error
	Recover    func(updateRestartInvocation) error
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
	err := ops.Spawn(invocation)
	if err != nil && ops.Recover != nil {
		_ = ops.Recover(invocation)
	}
	return err
}

func tryPauseUpdateResources(gate *updateAdmissionGate, pauseJobs func() (func(), bool)) func() {
	releaseJobs, ok := pauseJobs()
	if !ok {
		return nil
	}
	releaseGate, ok := gate.TryIdleBarrier()
	if !ok {
		releaseJobs()
		return nil
	}
	return func() { releaseGate(); releaseJobs() }
}

// The third argument is retained for the existing private test seam; closing
// HTTP is irreversible and must happen only inside the committed Shutdown.
func prepareAdminForUpdate(ctx context.Context, pause func(context.Context) (bool, error), _ func() error, resume func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	idle, err := pause(ctx)
	if err != nil || !idle {
		_ = resume()
		if err != nil {
			return err
		}
		return errors.New("admin busy")
	}
	if err := ctx.Err(); err != nil {
		_ = resume()
		return err
	}
	return nil
}

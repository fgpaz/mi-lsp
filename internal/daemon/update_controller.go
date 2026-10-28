package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/selfupdate"
)

func updateVersionParts(v string) ([3]int, bool) {
	var result [3]int
	v = strings.TrimPrefix(v, "v")
	v = strings.SplitN(v, "+", 2)[0]
	if strings.Contains(v, "-") {
		return result, false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func updateCandidateVerified(running model.DaemonState, status selfupdate.Status, disk string) bool {
	if status.LastResult != "updated" || status.Installed == "" || status.Installed != status.Latest || status.Installed != disk {
		return false
	}
	newVersion, ok := updateVersionParts(disk)
	if !ok {
		return false
	}
	oldVersion, ok := updateVersionParts(running.Version)
	if !ok {
		return false
	}
	if newVersion[0] == 0 && newVersion[1] < 16 {
		return false
	}
	for i := range newVersion {
		if newVersion[i] != oldVersion[i] {
			return newVersion[i] > oldVersion[i]
		}
	}
	return false
}

// maybeRestartUpdated checks file metadata first, then cached state and actual
// disk readback. An unobservable activity or a busy listener always defers.
func (s *Server) maybeRestartUpdated(ctx context.Context, cfg selfupdate.Config) {
	if s.updateGate == nil || s.manager == nil || s.qSessions == nil || s.elastic != nil {
		return
	}
	info, err := os.Stat(cfg.ExecutablePath)
	if err != nil || (info.Size() == s.state.ExecutableSize && info.ModTime().Equal(s.state.ExecutableMTime)) {
		return
	}
	status, err := selfupdate.ReadCached(cfg)
	if err != nil {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	disk, err := selfupdate.ReadInstalled(readCtx, cfg.ExecutablePath)
	cancel()
	if err != nil || !updateCandidateVerified(s.state, status, disk) || !s.updateResourcesIdle(time.Now()) {
		return
	}
	listener, ok := s.listener.(*updateListener)
	if !ok {
		return
	}
	pauseCtx, cancel := context.WithTimeout(ctx, time.Second)
	idle, err := listener.Pause(pauseCtx)
	cancel()
	if err != nil || !idle {
		_ = listener.Resume()
		return
	}
	resume := true
	defer func() {
		if resume {
			_ = listener.Resume()
		}
	}()
	release := tryPauseUpdateResources(s.updateGate, indexer.TryPauseBackgroundJobs)
	if release == nil {
		return
	}
	releaseFence := true
	defer func() {
		if releaseFence {
			release()
			s.signalShutdownRetry()
		}
	}()
	if s.elastic != nil || !s.updateResourcesIdle(time.Now()) {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	invocation := updateRestartInvocation{Executable: cfg.ExecutablePath, Args: append([]string(nil), os.Args[1:]...), Env: updateDaemonEnvironment(os.Environ()), Endpoint: s.state.Endpoint, HomeDir: cfg.HomeDir, Cwd: cwd}
	ops := updateRestartOps{
		CleanupOld: func() error { _, err := os.Stat(invocation.Executable); return err },
		Shutdown: func() error {
			if s.admin == nil || s.admin.updateListener == nil || s.admin.server == nil {
				return errors.New("admin state unknown")
			}
			pauseCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			err := prepareAdminForUpdate(pauseCtx, s.admin.updateListener.Pause, nil, s.admin.updateListener.Resume)
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				_ = s.admin.updateListener.Resume()
				return err
			}
			if s.shutdownStarted.Load() || s.elastic != nil || !s.updateResourcesIdle(time.Now()) {
				_ = s.admin.updateListener.Resume()
				return errors.New("daemon no longer idle")
			}
			resume = false
			releaseFence = false
			shutdownCtx, cancelShutdown := context.WithTimeout(ctx, shutdownCallerGrace)
			defer cancelShutdown()
			return s.shutdownWithFence(shutdownCtx, release, true)
		},
		Spawn: func(inv updateRestartInvocation) error {
			cmd := processutil.Command(inv.Executable, inv.Args...)
			processutil.ConfigureDetachedCommand(cmd)
			cmd.Dir, cmd.Env = inv.Cwd, inv.Env
			if err := cmd.Start(); err != nil {
				return errors.New("daemon restart failed")
			}
			return cmd.Process.Release()
		},
	}
	// Retry a detached launch after a failed spawn; no foreign process is touched.
	ops.Recover = ops.Spawn
	if err := restartUpdatedDaemon(ctx, invocation, ops); err != nil {
		fmt.Fprintf(os.Stderr, "[mi-lsp:update] restart deferred or failed: %v\n", err)
	}
}

func updateDaemonEnvironment(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, v := range env {
		if !strings.EqualFold(strings.SplitN(v, "=", 2)[0], "MI_LSP_UPDATE_CHILD") {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

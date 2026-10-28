package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func daemonInstanceLifeGuardPath() (string, error) {
	dir, err := daemonRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "instance.life.guard"), nil
}

// cleanupFailedConstruction synchronously joins every resource whose lifetime
// began during construction before NewServer returns and its leases are released.
func (s *Server) cleanupFailedConstruction(cause error) error {
	cleanupErr := cause
	if s.updateRetention != nil {
		s.updateRetention.stopAndWait()
	}
	if s.elasticCancel != nil {
		s.elasticCancel()
	}
	if s.listener != nil {
		cleanupErr = errors.Join(cleanupErr, s.listener.Close())
	}
	if s.admin != nil {
		cleanupErr = errors.Join(cleanupErr, s.admin.Shutdown())
	}
	if s.elastic != nil {
		s.elasticLimitsMu.Lock()
		cleanupErr = errors.Join(cleanupErr, s.elastic.Close())
		s.elasticLimitsMu.Unlock()
	}
	if s.manager != nil {
		s.manager.Shutdown()
	}
	if s.telemetry != nil {
		if err := s.telemetry.Close(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		// Close's public wait is bounded; the constructor owner must not return
		// until the underlying worker and database driver have really joined.
		if err := s.telemetry.WaitClosed(context.Background()); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

// clearOwnedDaemonState removes only the exact state record published by this
// server. Missing or replaced state is never treated as ours to unlink.
func clearOwnedDaemonState(owned model.DaemonState) error {
	path, err := daemonStatePath()
	if err != nil {
		return err
	}
	current, err := loadDaemonState()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read daemon state before owner cleanup: %w", err)
	}
	if current.PID != owned.PID || current.Endpoint != owned.Endpoint || current.RunID != owned.RunID || !current.StartedAt.Equal(owned.StartedAt) {
		return errors.New("daemon state changed ownership; refusing cleanup")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

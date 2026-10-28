package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

// withTelemetryWriterLeaseContext executes fn synchronously while owning the
// persistent OS writer guard for the actual database file. Cancellation bounds
// acquisition and is passed to fn; it never releases a lease while fn is still
// executing. This primitive is intentionally not wired into writers yet.
func (s *TelemetryStore) withTelemetryWriterLeaseContext(ctx context.Context, fn func(context.Context) error) (retErr error) {
	if s == nil || s.db == nil {
		return errors.New("telemetry store unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return errors.New("telemetry writer callback is nil")
	}
	dbPath := s.dbPath
	if dbPath == "" {
		var name string
		if err := s.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(new(int), &name, &dbPath); err != nil {
			return fmt.Errorf("resolve telemetry database path: %w", err)
		}
		if name != "main" {
			return errors.New("telemetry database main path unavailable")
		}
	}
	if dbPath == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") || strings.Contains(strings.ToLower(dbPath), "mode=memory") {
		return errors.New("telemetry database path is not a filesystem path")
	}
	if dbPath != strings.TrimSpace(dbPath) {
		return errors.New("telemetry database path has ambiguous surrounding whitespace")
	}
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve telemetry database absolute path: %w", err)
	}
	canonicalDBPath, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		info, statErr := os.Stat(canonicalDBPath)
		if statErr != nil {
			return fmt.Errorf("validate telemetry database file: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return errors.New("telemetry database is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve telemetry database file: %w", err)
	} else {
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(absolute))
		if parentErr != nil {
			return fmt.Errorf("resolve telemetry database parent: %w", parentErr)
		}
		info, statErr := os.Stat(parent)
		if statErr != nil || !info.IsDir() {
			if statErr == nil {
				statErr = errors.New("database parent is not a directory")
			}
			return fmt.Errorf("validate telemetry database parent: %w", statErr)
		}
		canonicalDBPath = filepath.Join(parent, filepath.Base(absolute))
	}
	guard := canonicalDBPath + ".writer.guard"
	lease, err := runtimeinfo.AcquireFileLeaseContext(ctx, guard)
	if err != nil {
		return fmt.Errorf("acquire telemetry writer lease: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, wrapTelemetryLeaseRelease(lease.Release())) }()
	return fn(ctx)
}

func wrapTelemetryLeaseRelease(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("release telemetry writer lease: %w", err)
}


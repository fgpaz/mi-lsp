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
	dbPath := strings.TrimSpace(s.dbPath)
	if dbPath == "" {
		var name string
		if err := s.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(new(int), &name, &dbPath); err != nil {
			return fmt.Errorf("resolve telemetry database path: %w", err)
		}
		if name != "main" {
			return errors.New("telemetry database main path unavailable")
		}
	}
	if dbPath == "" || strings.HasPrefix(dbPath, "file:") || strings.Contains(dbPath, "?mode=memory") {
		return errors.New("telemetry database path is not a filesystem path")
	}
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve telemetry database absolute path: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return fmt.Errorf("resolve telemetry database parent: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("database parent is not a directory")
		}
		return fmt.Errorf("validate telemetry database parent: %w", err)
	}
	guard := filepath.Join(parent, filepath.Base(absolute)+".writer.guard")
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


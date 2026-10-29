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
// persistent OS writer guard for the canonical actual database file.
// Cancellation bounds acquisition and is not used to release the lease while
// fn is executing; callers retain ownership until the callback physically returns.
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
	canonicalDBPath, err := canonicalTelemetryDatabasePath(dbPath)
	if err != nil {
		return err
	}
	guard := canonicalDBPath + ".writer.guard"
	lease, err := runtimeinfo.AcquireFileLeaseContext(ctx, guard)
	if err != nil {
		return fmt.Errorf("acquire telemetry writer lease: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, wrapTelemetryLeaseRelease(lease.Release())) }()
	return fn(ctx)
}

// inspectTelemetryDatabaseAfterLease rechecks both canonical identity and
// existence only after the caller owns the writer guard.
func inspectTelemetryDatabaseAfterLease(path, expectedCanonical string) (bool, error) {
	canonical, err := canonicalTelemetryDatabasePath(path)
	if err != nil {
		return false, fmt.Errorf("revalidate telemetry database path after lease: %w", err)
	}
	if canonical != expectedCanonical {
		return false, fmt.Errorf("telemetry database identity changed while acquiring lease: before=%q after=%q", expectedCanonical, canonical)
	}
	info, err := os.Stat(canonical)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect telemetry database after lease: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("telemetry database is not a regular file after lease")
	}
	return true, nil
}

// canonicalTelemetryDatabasePath resolves an existing database and its
// physical parent for a not-yet-created database, so all cooperating stores
// derive the same persistent writer guard.
func canonicalTelemetryDatabasePath(dbPath string) (string, error) {
	if dbPath == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") || strings.Contains(strings.ToLower(dbPath), "mode=memory") {
		return "", errors.New("telemetry database path is not a filesystem path")
	}
	if dbPath != strings.TrimSpace(dbPath) {
		return "", errors.New("telemetry database path has ambiguous surrounding whitespace")
	}
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		return "", fmt.Errorf("resolve telemetry database absolute path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		info, statErr := os.Stat(canonical)
		if statErr != nil {
			return "", fmt.Errorf("validate telemetry database file: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("telemetry database is not a regular file")
		}
		return canonical, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve telemetry database file: %w", err)
	}
	entry, lstatErr := os.Lstat(absolute)
	if lstatErr == nil && entry.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("telemetry database path is a dangling symlink")
	}
	if lstatErr != nil && !errors.Is(lstatErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect telemetry database path: %w", lstatErr)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", fmt.Errorf("resolve telemetry database parent: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		if err == nil { err = errors.New("database parent is not a directory") }
		return "", fmt.Errorf("validate telemetry database parent: %w", err)
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func wrapTelemetryLeaseRelease(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("release telemetry writer lease: %w", err)
}

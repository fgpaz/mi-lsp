package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// Lock contention that outlived busy_timeout and a real open failure (path,
// permissions, corruption) leave the service with different codes.
func TestWorkspaceDBOpenErrorSeparatesBusyFromOpenFailure(t *testing.T) {
	busy := &workspaceDBOpenError{cause: errors.New("database is locked (5) (SQLITE_BUSY)"), root: "/r"}
	recovery := &workspaceDBOpenError{cause: fmt.Errorf("query: %w", errors.New("database is locked (261)")), root: "/r"}
	for _, err := range []error{busy, recovery, fmt.Errorf("index.start: %w", busy)} {
		if got := sanitizeIntentError(err); got != workspaceDBBusyErrorCode {
			t.Fatalf("sanitizeIntentError(%v) = %q, want %q", err, got, workspaceDBBusyErrorCode)
		}
	}

	root := t.TempDir()
	// A directory where index.db should be: a real open failure, not contention.
	if err := os.MkdirAll(store.WorkspaceDBPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := openWorkspaceDB(model.WorkspaceRegistration{Root: root}, "test.open", false)
	if err == nil {
		_ = db.Close()
		t.Fatal("open over a directory succeeded")
	}
	var openErr *workspaceDBOpenError
	if !errors.As(err, &openErr) || store.IsLockedError(openErr.cause) {
		t.Fatalf("open failure = %v (%T), want workspaceDBOpenError with a non-busy cause", err, err)
	}
	if got := err.Error(); got != workspaceDBOpenErrorCode {
		t.Fatalf("open failure code = %q, want %q", got, workspaceDBOpenErrorCode)
	}
	for _, code := range []string{busy.Error(), err.Error()} {
		if strings.Contains(code, root) || strings.Contains(code, "/r") || strings.Contains(code, "locked") {
			t.Fatalf("code %q leaks a root or driver text", code)
		}
	}
}

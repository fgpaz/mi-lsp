package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestCatalogTelemetryErrorCodeDistinguishesCatalogStates(t *testing.T) {
	root := t.TempDir()
	indexPath := store.WorkspaceDBPath(root)
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		err   error
		ready bool
		want  string
	}{
		{name: "absent", ready: false, want: "nav_find_index_absent"},
		{name: "unreadable", err: errors.New("permission denied"), ready: true, want: "nav_find_index_unreadable"},
		{name: "broken", err: errors.New("database disk image is malformed"), ready: true, want: "nav_find_index_broken"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := catalogTelemetryErrorCode(test.err, root, test.ready); got != test.want {
				t.Fatalf("catalogTelemetryErrorCode() = %q, want %q", got, test.want)
			}
		})
	}
}

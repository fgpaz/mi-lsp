package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

type readonlySnapshotEntry struct {
	Mode   os.FileMode
	Size   int64
	Mtime  int64
	SHA256 string
	IsDir  bool
}

func TestOpenReadOnlyExistingAbsentDoesNotCreateState(t *testing.T) {
	root := t.TempDir()
	before := snapshotFiles(t, root)
	if _, err := OpenReadOnlyExisting(root, WorkspaceDBPath(root)); err == nil {
		t.Fatal("missing database should not open")
	}
	after := snapshotFiles(t, root)
	if len(before) != len(after) {
		t.Fatalf("filesystem changed: before=%v after=%v", before, after)
	}
}

func TestOpenReadOnlyExistingUsesRealReadOnlyMode(t *testing.T) {
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.WriteFile(WorkspaceDBPath(root)+"-wal", []byte("preexisting wal"), 0o644); err != nil {
		t.Fatalf("write preexisting WAL: %v", err)
	}
	if err := os.WriteFile(WorkspaceDBPath(root)+"-shm", []byte("preexisting shm"), 0o644); err != nil {
		t.Fatalf("write preexisting SHM: %v", err)
	}
	before := snapshotFiles(t, root)
	ro, err := OpenReadOnlyExisting(root, WorkspaceDBPath(root))
	if err != nil {
		t.Fatalf("OpenReadOnlyExisting: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ro.PingContext(ctx); err != nil {
		t.Fatalf("PingContext: %v", err)
	}
	if _, err := ro.ExecContext(ctx, "CREATE TABLE probe_forbidden(id INTEGER)"); err == nil {
		t.Fatal("read-only connection accepted a write")
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("Close read-only connection: %v", err)
	}
	after := snapshotFiles(t, root)
	assertSnapshotEqual(t, before, after)
}

func snapshotFiles(t *testing.T, root string) map[string]readonlySnapshotEntry {
	t.Helper()
	result := map[string]readonlySnapshotEntry{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		entry := readonlySnapshotEntry{
			Mode:  info.Mode(),
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
			IsDir: info.IsDir(),
		}
		if !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			digest := sha256.Sum256(data)
			entry.SHA256 = hex.EncodeToString(digest[:])
		}
		result[filepath.ToSlash(path)] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return result
}

func assertSnapshotEqual(t *testing.T, before, after map[string]readonlySnapshotEntry) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("snapshot length changed: before=%d after=%d", len(before), len(after))
	}
	paths := make([]string, 0, len(before))
	for path := range before {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		left, ok := before[path]
		right, exists := after[path]
		if !ok || !exists || left != right {
			t.Fatalf("snapshot changed at %q: before=%#v after=%#v", path, left, right)
		}
	}
}

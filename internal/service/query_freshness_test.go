package service

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestStaleQueryPathsSelectsOnlyChangedIndexedFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "main.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("package main\nfunc Current() {}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	oldHash := fmt.Sprintf("%x", md5.Sum([]byte("old content")))
	if _, err := db.Exec(`INSERT INTO files(file_path, content_hash) VALUES(?, ?)`, "internal/main.go", oldHash); err != nil {
		t.Fatalf("insert file row: %v", err)
	}

	stale, err := staleQueryPaths(context.Background(), db, root, []string{
		"internal/main.go",
		filepath.Join(root, "internal", "main.go"),
		filepath.Join(t.TempDir(), "outside.go"),
	})
	if err != nil {
		t.Fatalf("staleQueryPaths: %v", err)
	}
	if !reflect.DeepEqual(stale, []string{"internal/main.go"}) {
		t.Fatalf("stale paths = %v, want only internal/main.go", stale)
	}

	currentHash := fmt.Sprintf("%x", md5.Sum(content))
	if _, err := db.Exec(`UPDATE files SET content_hash=? WHERE file_path=?`, currentHash, "internal/main.go"); err != nil {
		t.Fatalf("update file hash: %v", err)
	}
	stale, err = staleQueryPaths(context.Background(), db, root, []string{"internal/main.go"})
	if err != nil {
		t.Fatalf("staleQueryPaths for fresh file: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("fresh file marked stale: %v", stale)
	}
	fullIndexHash := fmt.Sprintf("%x", sha1.Sum(content))
	if _, err := db.Exec(`UPDATE files SET content_hash=? WHERE file_path=?`, fullIndexHash, "internal/main.go"); err != nil {
		t.Fatalf("update full-index hash: %v", err)
	}
	stale, err = staleQueryPaths(context.Background(), db, root, []string{"internal/main.go"})
	if err != nil {
		t.Fatalf("staleQueryPaths for full-index SHA-1: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("full-index SHA-1 file marked stale: %v", stale)
	}
	if _, err := db.Exec(`UPDATE files SET content_hash=? WHERE file_path=?`, "legacy-unknown", "internal/main.go"); err != nil {
		t.Fatalf("update unknown legacy hash: %v", err)
	}
	stale, err = staleQueryPaths(context.Background(), db, root, []string{"internal/main.go"})
	if err != nil {
		t.Fatalf("staleQueryPaths for unknown legacy hash: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("unknown legacy hash marked stale: %v", stale)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove indexed file: %v", err)
	}
	stale, err = staleQueryPaths(context.Background(), db, root, []string{"internal/main.go"})
	if err != nil {
		t.Fatalf("staleQueryPaths for deleted file: %v", err)
	}
	if !reflect.DeepEqual(stale, []string{"internal/main.go"}) {
		t.Fatalf("deleted indexed path marked stale = %v, want internal/main.go", stale)
	}
}

func TestQueryPathWithinRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "source.go")
	if err := os.WriteFile(outsideFile, []byte("package source"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "external.go")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if rel, _, ok := queryPathWithinRoot(root, link); ok {
		t.Fatalf("external symlink accepted as %q", rel)
	}
}

func TestRefreshQueryPathsPublishesChangedCodeWithinBudget(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "main.go")
	content := []byte("package main\nfunc Changed() { println(\"fresh\") }\n")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	oldHash := fmt.Sprintf("%x", md5.Sum([]byte("previous published content")))
	if _, err := db.Exec(`INSERT INTO files(file_path, repo_id, repo_name, content_hash, language) VALUES(?, ?, ?, ?, ?)`, "main.go", "repo", "repo", oldHash, "go"); err != nil {
		db.Close()
		t.Fatalf("insert file row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	refreshed, err := RefreshQueryPaths(ctx, root, []string{"main.go"}, 250*time.Millisecond)
	t.Logf("scoped changed-file refresh elapsed=%s", time.Since(started))
	if err != nil {
		t.Fatalf("RefreshQueryPaths: %v", err)
	}
	if !refreshed {
		t.Fatal("RefreshQueryPaths returned without publishing the changed file")
	}

	db, err = store.Open(root)
	if err != nil {
		t.Fatalf("store.Open after refresh: %v", err)
	}
	defer db.Close()
	var gotHash string
	if err := db.QueryRow(`SELECT content_hash FROM files WHERE file_path=?`, "main.go").Scan(&gotHash); err != nil {
		t.Fatalf("read refreshed hash: %v", err)
	}
	if wantHash := fmt.Sprintf("%x", sha1.Sum(content)); gotHash != wantHash {
		t.Fatalf("refreshed hash=%q, want scoped catalog SHA-1 %q", gotHash, wantHash)
	}
	state, err := store.GraphRuntimeState(context.Background(), db)
	if err != nil || state != store.GraphRuntimeStale {
		t.Fatalf("graph runtime state=%q err=%v, want stale after catalog-only refresh", state, err)
	}
}

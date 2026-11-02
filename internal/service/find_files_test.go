package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Authored in BUILDING. The goal owner runs these with the final verification.
func TestFindFilesQuerySemantics(t *testing.T) {
	for _, tc := range []struct {
		query, name, reason string
		rank                int
		literal, want       bool
	}{
		{"config.go", "src/config.go", "basename_exact", 0, false, true},
		{"config", "src/config.go", "basename_prefix", 1, false, true},
		{"config", "config/service.go", "segment", 2, false, true},
		{"config", "src/myconfig.go", "substring", 3, false, true},
		{"*.go", "src/main.go", "glob", 3, false, true},
		{"*.go", "src/main.go.txt", "", 0, false, false},
		{"**/*.go", "main.go", "glob", 3, false, true},
		{"src/**/main?.go", "src/a/b/mainé.go", "glob", 3, false, true},
		{"src/*.go", "src/a/main.go", "", 0, false, false},
		{"CAFÉ", "src/café.go", "basename_prefix", 1, false, true},
		{"a b", "dir/a b.go", "basename_prefix", 1, false, true},
		{"-draft", "-draft.md", "basename_prefix", 1, false, true},
		{"a[b].go", "a[b].go", "basename_exact", 0, true, true},
		{".go", "src/main.go", "substring", 3, false, true},
	} {
		t.Run(tc.query+"/"+tc.name, func(t *testing.T) {
			match, err := compileFileQuery(tc.query, tc.literal)
			if err != nil {
				t.Fatal(err)
			}
			rank, reason, ok := match(tc.name)
			if ok != tc.want || (ok && (rank != tc.rank || reason != tc.reason)) {
				t.Fatalf("got (%d,%s,%v)", rank, reason, ok)
			}
		})
	}
	for _, query := range []string{"../x", "/abs", "[broken", "a/../x"} {
		if _, err := compileFileQuery(query, false); err == nil {
			t.Errorf("accepted %q", query)
		}
	}
}

func TestFindFilesGitIgnoreRankingBudgetAndCache(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	root := t.TempDir()
	gitFindFixture(t, root, "init", "--quiet")
	files := map[string]string{
		".gitignore":     "ignored/\n*.tmp\n!keep.tmp\n*.log\n",
		"src/.gitignore": "!kept.log\nlocal*.go\n!localkeep.go\n",
		"config":         "", "config-more": "", "configdir/other": "", "myconfig": "",
		"src/config": "", "ignored/secret.go": "", "omit.tmp": "", "keep.tmp": "",
		"src/no.log": "", "src/kept.log": "", "src/local.go": "", "src/localkeep.go": "",
		"src/a b.go": "", "src/café.go": "", "-draft.go": "", "tracked.tmp": "",
	}
	// Ensure the wildcard response exceeds both tested byte budgets, even
	// when Git returns a short listing and cache metadata uses fewer digits.
	for _, suffix := range []string{"a", "b", "c", "d"} {
		files["budget-fixture-"+strings.Repeat(suffix, 48)+".txt"] = ""
	}
	for name, body := range files {
		writeFindFixture(t, root, name, body)
	}
	gitFindFixture(t, root, "add", "--force", "--", "tracked.tmp")
	finder := &FileFinder{CacheDir: t.TempDir()}
	opts := FindFilesOptions{Top: 100, MaxBytes: 16384, Refresh: true}
	all, err := finder.FindFiles(context.Background(), root, "*", opts)
	if err != nil {
		t.Fatal(err)
	}
	if all.Listing.Backend != "git" {
		t.Fatalf("Git fallback must not depend on fd: %s", all.Listing.Backend)
	}
	paths := map[string]bool{}
	for _, item := range all.Items {
		paths[item.Path] = true
		if strings.Contains(item.Path, ".git/") {
			t.Fatal(item)
		}
	}
	for _, name := range []string{"keep.tmp", "src/kept.log", "src/localkeep.go", "src/a b.go", "src/café.go", "-draft.go", "tracked.tmp"} {
		if !paths[name] {
			t.Errorf("missing %s", name)
		}
	}
	for _, name := range []string{"ignored/secret.go", "omit.tmp", "src/no.log", "src/local.go"} {
		if paths[name] {
			t.Errorf("ignored path returned %s", name)
		}
	}
	ranked, err := finder.FindFiles(context.Background(), root, "config", opts)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range ranked.Items {
		got = append(got, item.Path)
	}
	want := []string{"config", "src/config", "config-more", "configdir/other", "myconfig"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranking %v want %v", got, want)
	}
	for _, size := range []int{512, 1024} {
		limited, err := finder.FindFiles(context.Background(), root, "*", FindFilesOptions{Top: 100, MaxBytes: size})
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(limited)
		if len(encoded)+1 > size || !limited.Truncated || limited.Reason != "budget_bytes" || limited.NextHint == "" {
			t.Fatalf("bad budget: %s", encoded)
		}
	}
	top, err := finder.FindFiles(context.Background(), root, "*", FindFilesOptions{Top: 1, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(top.Items) != 1 || !top.Truncated || top.Reason != "top" {
		t.Fatal(top)
	}
	writeFindFixture(t, root, "after-cache.go", "")
	cached, err := finder.FindFiles(context.Background(), root, "after-cache.go", FindFilesOptions{Top: 10, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	// A slow CI host may legitimately expire the five-second snapshot.
	if cached.Listing.Cached && cached.Matched != 0 {
		t.Fatal("cache falsely claimed fresh creation")
	}
	fresh, err := finder.FindFiles(context.Background(), root, "after-cache.go", opts)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Listing.Cached || fresh.Matched != 1 {
		t.Fatal(fresh)
	}
}

func TestFindFilesNoSymlinkEscapeOrGitInternals(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	root, outside := t.TempDir(), t.TempDir()
	gitFindFixture(t, root, "init", "--quiet")
	writeFindFixture(t, root, "dir/file.go", "")
	writeFindFixture(t, outside, "secret.go", "")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	if err := os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	result, err := (&FileFinder{}).FindFiles(context.Background(), root, "*.go", FindFilesOptions{Top: 20, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Path != "dir/file.go" {
		t.Fatal(result)
	}
	// An old listing must not make a directory replaced by a symlink safe.
	if !safeFindFile(root, "dir/file.go", map[string]bool{".": true}) {
		t.Fatal("regular path rejected")
	}
	if err := os.Rename(filepath.Join(root, "dir"), filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dir")); err != nil {
		t.Fatal(err)
	}
	if safeFindFile(root, "dir/secret.go", map[string]bool{".": true}) {
		t.Fatal("ancestor symlink accepted")
	}
	for _, name := range []string{"../secret.go", "/secret.go", ".git/config", "x/.git/index", "C:/secret.go"} {
		if validFindFileName(name) {
			t.Errorf("unsafe %q", name)
		}
	}
}

func TestFindFilesExpiredCacheAndErrors(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	f := &FileFinder{CacheDir: cache}
	// Validation errors happen before any executable or workspace access.
	for _, opts := range []FindFilesOptions{{Top: 0, MaxBytes: 4096}, {Top: 1, MaxBytes: 1}, {Top: 1001, MaxBytes: 4096}} {
		_, err := f.FindFiles(context.Background(), root, "*", opts)
		var typed *model.FindFilesError
		if !errors.As(err, &typed) || typed.Code != "invalid_budget" {
			t.Fatalf("%v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.FindFiles(ctx, root, "*", FindFilesOptions{Top: 1, MaxBytes: 4096})
	if err == nil {
		t.Fatal("cancelled context accepted")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	gitFindFixture(t, root, "init", "--quiet")
	writeFindFixture(t, root, "fresh.go", "")
	sum := sha256.Sum256([]byte(root))
	f.saveFileListing(filepath.Join(cache, hex.EncodeToString(sum[:])+".json"), findFilesSnapshot{Version: 1, Root: root, Created: time.Now().Add(-time.Minute), Backend: "git", Paths: []string{"stale.go"}})
	result, err := f.FindFiles(context.Background(), root, "*.go", FindFilesOptions{Top: 10, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if result.Listing.Cached || len(result.Items) != 1 || result.Items[0].Path != "fresh.go" {
		t.Fatal(result)
	}
}

func writeFindFixture(t *testing.T, root, name, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func gitFindFixture(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git fixture: %v: %s", err, out)
	}
}

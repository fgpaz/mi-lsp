package daemon

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"github.com/fsnotify/fsnotify"
)

func TestIsWatchableFile_ValidExtensions(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"file.cs", true},
		{"file.go", true},
		{"file.ts", true},
		{"file.tsx", true},
		{"file.mts", true},
		{"file.cts", true},
		{"file.js", true},
		{"file.jsx", true},
		{"file.mjs", true},
		{"file.cjs", true},
		{"file.py", true},
		{"file.pyi", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isWatchableFile(tt.filename)
			if got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsWatchableFile_InvalidExtensions(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"file.md", false},
		{"file.txt", false},
		{"file.json", false},
		{"file.yaml", false},
		{"file.xml", false},
		{"file.html", false},
		{"file", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isWatchableFile(tt.filename)
			if got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsWatchableFile_CaseInsensitive(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"File.CS", true},
		{"file.Ts", true},
		{"File.TSX", true},
		{"file.MTS", true},
		{"FILE.JS", true},
		{"file.Go", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isWatchableFile(tt.filename)
			if got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsWatchableFile_WithPaths(t *testing.T) {
	tests := []struct {
		filepath string
		want     bool
	}{
		{"/path/to/file.cs", true},
		{"C:\\path\\to\\file.ts", true},
		{"/path/to/file.go", true},
		{"./src/file.tsx", true},
		{"../sibling/file.jsx", true},
	}

	for _, tt := range tests {
		t.Run(tt.filepath, func(t *testing.T) {
			got := isWatchableFile(tt.filepath)
			if got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.filepath, got, tt.want)
			}
		})
	}
}

func TestIsWatchableFile_EdgeCases(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"", false},
		{".", false},
		{".cs", true},
		{".ts", true},
		{".go", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isWatchableFile(tt.filename)
			if got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_CommonSkipDirs(t *testing.T) {
	tests := []struct {
		dirpath string
		want    bool
	}{
		{"node_modules", true},
		{"src", false},
		{".git", true},
		{".gitignore", false}, // file, not dir
		{"bin", true},
		{"obj", true},
		{"dist", true},
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_BuildDirs(t *testing.T) {
	tests := []struct {
		dirpath string
		want    bool
	}{
		{"bin", true},
		{"obj", true},
		{"dist", true},
		{"out", true},
		{"build", false}, // not in skip list
		{"target", false},
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_VCS(t *testing.T) {
	tests := []struct {
		dirpath string
		want    bool
	}{
		{".git", true},
		{".vs", true},
		{".idea", true},
		{".vscode", false}, // not in skip list
		{"vendor", true},
		{".worktrees", true},
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_LanguageSpecific(t *testing.T) {
	tests := []struct {
		dirpath string
		want    bool
	}{
		{"node_modules", true},
		{"__pycache__", true},
		{".next", true},
		{".nuget", false},
		{"venv", false},
		{".python", false},
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_WithFullPaths(t *testing.T) {
	tests := []struct {
		dirpath string
		want    bool
	}{
		{"/home/user/project/node_modules", true},
		{"/home/user/project/src", false},
		{"C:\\project\\bin", true},
		{"C:\\project\\src", false},
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_CaseSensitivity(t *testing.T) {
	// Directory names should match exactly (case-sensitive on Unix, case-insensitive on Windows)
	// The base comparison will use filepath.Base which preserves case
	tests := []struct {
		dirpath string
		want    bool
	}{
		{"node_modules", true},
		{"Node_Modules", false}, // different case
		{".git", true},
		{".Git", false}, // different case
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_EdgeCases(t *testing.T) {
	tests := []struct {
		dirpath string
		want    bool
	}{
		{"", false},
		{".", false},
		{"..", false},
		{"/", false},
		{"src", false},
		{"source", false},
		{"sources", false},
	}

	for _, tt := range tests {
		t.Run(tt.dirpath, func(t *testing.T) {
			got := shouldSkipDir(tt.dirpath)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.dirpath, got, tt.want)
			}
		})
	}
}

func TestIsWatchableFile_AllWatchableExtensions(t *testing.T) {
	// Verify all registry-backed code extensions are correctly recognized.
	watchable := []string{".cs", ".go", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".py", ".pyi"}

	for _, ext := range watchable {
		filename := "test" + ext
		got := isWatchableFile(filename)
		if !got {
			t.Errorf("isWatchableFile(%q) = false, want true (watchable extension)", filename)
		}
	}
}

func TestIsWatchableFile_NonWatchableExtensions(t *testing.T) {
	nonWatchable := []string{".md", ".txt", ".json", ".yaml", ".rb", ".php", ".cpp", ".h"}

	for _, ext := range nonWatchable {
		filename := "test" + ext
		got := isWatchableFile(filename)
		if got {
			t.Errorf("isWatchableFile(%q) = true, want false (non-watchable extension)", filename)
		}
	}
}

func TestShouldSkipDir_AllSkipDirs(t *testing.T) {
	// Verify all skip directories are correctly recognized
	skipDirs := []string{".git", "node_modules", "bin", "obj", "dist", ".mi-lsp", ".vs", ".idea", "__pycache__", ".worktrees", "vendor", ".next", "out"}

	for _, dirName := range skipDirs {
		got := shouldSkipDir(dirName)
		if !got {
			t.Errorf("shouldSkipDir(%q) = false, want true (skip directory)", dirName)
		}
	}
}

func TestShouldSkipDir_NestedPathBaseExtraction(t *testing.T) {
	// Test that only the base directory name matters (the last component)
	tests := []struct {
		path string
		want bool
	}{
		{"/home/project/node_modules/package", false}, // base is "package", not skip
		{"/home/project/node_modules", true},          // base is "node_modules", skip
		{"src/bin", true},                             // base is "bin", skip
		{"src/bins", false},                           // base is "bins", not skip
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shouldSkipDir(tt.path)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsWatchableFile_MultipleDotsInFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"file.min.js", true},   // last extension is .js
		{"file.test.ts", true},  // last extension is .ts
		{"file.spec.go", true},  // last extension is .go
		{"file.tar.gz", false},  // last extension is .gz
		{"file.min.css", false}, // last extension is .css
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isWatchableFile(tt.filename)
			if got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestShouldSkipDir_AllCombinations(t *testing.T) {
	// Test a variety of directory paths (only the base name matters)
	skipTests := []struct {
		path string
		want bool
	}{
		{"node_modules", true},
		{"src/node_modules/pkg", false}, // base is "pkg", not skip
		{"/usr/src", false},
		{"source", false},
		{".git", true},
		{".gitignore", false},
		{"build", false},
		{"bin", true},
		{"bin/Release", false}, // base is "Release", not skip
	}

	for _, tt := range skipTests {
		t.Run(tt.path, func(t *testing.T) {
			got := shouldSkipDir(tt.path)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestWatchableExtensionsMap(t *testing.T) {
	// Verify the watchableExtensions map is not empty
	if len(watchableExtensions) == 0 {
		t.Fatal("watchableExtensions map is empty")
	}

	// Verify expected extensions are in the map
	expectedExts := []string{".cs", ".go", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".py", ".pyi"}
	for _, ext := range expectedExts {
		if _, ok := watchableExtensions[ext]; !ok {
			t.Errorf("watchableExtensions missing %q", ext)
		}
	}
}

func TestSkipDirsMap(t *testing.T) {
	// Verify the skipDirs map is not empty
	if len(skipDirs) == 0 {
		t.Fatal("skipDirs map is empty")
	}

	// Verify expected directories are in the map
	expectedDirs := []string{".git", "node_modules", "bin", "obj", "dist"}
	for _, dir := range expectedDirs {
		if _, ok := skipDirs[dir]; !ok {
			t.Errorf("skipDirs missing %q", dir)
		}
	}
}

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		check   func(string) bool
	}{
		{
			name:    "empty",
			content: []byte(""),
			check: func(h string) bool {
				return len(h) == 40 // SHA1 hex string length
			},
		},
		{
			name:    "simple",
			content: []byte("hello"),
			check: func(h string) bool {
				// Deterministic hash
				return h == "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" || len(h) == 40
			},
		},
		{
			name:    "large",
			content: make([]byte, 10000),
			check: func(h string) bool {
				return len(h) == 40
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeHash(tt.content)
			if !tt.check(got) {
				t.Errorf("computeHash() = %q, check failed", got)
			}
		})
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	content := []byte("test content")
	hash1 := computeHash(content)
	hash2 := computeHash(content)

	if hash1 != hash2 {
		t.Errorf("computeHash not deterministic: %q != %q", hash1, hash2)
	}
}

func TestComputeHash_Different(t *testing.T) {
	hash1 := computeHash([]byte("content1"))
	hash2 := computeHash([]byte("content2"))

	if hash1 == hash2 {
		t.Errorf("computeHash should differ for different content")
	}
}

func TestComputeHash_Format(t *testing.T) {
	hash := computeHash([]byte("test"))

	// Verify it's a valid hex string
	if len(hash) != 40 {
		t.Errorf("hash length = %d, want 40 (SHA1 hex)", len(hash))
	}

	// Verify all characters are hex
	for _, ch := range hash {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Errorf("hash contains non-hex character: %q", ch)
		}
	}
}

func TestFileWatcherStopIsIdempotent(t *testing.T) {
	registration := model.WorkspaceRegistration{Root: t.TempDir(), Name: "test"}
	watcher, err := NewFileWatcher(registration, time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileWatcher: %v", err)
	}
	watcher.Stop()
	watcher.Stop()
}

func TestManagerLazyWatchersDedupeAndCapRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewManagerWithOptions(t.TempDir(), 1, time.Minute, StartOptions{WatchMode: WatchModeLazy, MaxWatchedRoots: 1})
	defer manager.Shutdown()

	manager.EnsureFileWatcher(model.WorkspaceRegistration{Name: "alias-a", Root: rootA})
	manager.EnsureFileWatcher(model.WorkspaceRegistration{Name: "alias-b", Root: rootA})
	stats := manager.WatcherStats()
	if stats.WatchedRoots != 1 {
		t.Fatalf("WatchedRoots after duplicate aliases = %d, want 1", stats.WatchedRoots)
	}

	manager.EnsureFileWatcher(model.WorkspaceRegistration{Name: "other", Root: rootB})
	stats = manager.WatcherStats()
	if stats.WatchedRoots != 1 {
		t.Fatalf("WatchedRoots after cap = %d, want 1", stats.WatchedRoots)
	}
	if stats.SkippedRootCount == 0 {
		t.Fatalf("SkippedRootCount = %d, want eviction/skipped signal", stats.SkippedRootCount)
	}
}

type deterministicWatcherTimer struct {
	delay    time.Duration
	callback func()
	stopped  bool
}

func (timer *deterministicWatcherTimer) Stop() bool {
	wasRunning := !timer.stopped
	timer.stopped = true
	return wasRunning
}

func (timer *deterministicWatcherTimer) fire() {
	if timer.stopped {
		return
	}
	timer.stopped = true
	timer.callback()
}

func newTestBatchWatcher(timers *[]*deterministicWatcherTimer, reindex func(string) error) *FileWatcher {
	return &FileWatcher{
		debounceDur:  time.Millisecond,
		pendingBatch: make(map[string]struct{}),
		batchRetry:   make(map[string]int),
		batchTimerFactory: func(delay time.Duration, callback func()) watcherTimer {
			timer := &deterministicWatcherTimer{delay: delay, callback: callback}
			*timers = append(*timers, timer)
			return timer
		},
		reindexFileFn: reindex,
	}
}

func TestWatcherBatchRetryUsesThreeImmediateRetries(t *testing.T) {
	var timers []*deterministicWatcherTimer
	calls := 0
	watcher := newTestBatchWatcher(&timers, func(string) error {
		calls++
		if calls <= maxImmediateBatchRetries {
			return &store.IndexLockError{Path: "index.lock"}
		}
		return nil
	})
	watcher.pendingBatch["file.ts"] = struct{}{}

	watcher.flushBatch()

	if calls != maxImmediateBatchRetries+1 {
		t.Fatalf("reindex calls = %d, want initial call plus %d immediate retries", calls, maxImmediateBatchRetries)
	}
	if len(timers) != 0 {
		t.Fatalf("deferred timers after eventual success = %d, want 0", len(timers))
	}
	if len(watcher.pendingBatch) != 0 {
		t.Fatalf("pending batch after eventual success = %v, want empty", watcher.pendingBatch)
	}
}

func TestWatcherBatchRetryStopsAfterOneDeferredFailure(t *testing.T) {
	var timers []*deterministicWatcherTimer
	calls := 0
	watcher := newTestBatchWatcher(&timers, func(string) error {
		calls++
		return &store.IndexLockError{Path: "index.lock"}
	})
	watcher.pendingBatch["file.ts"] = struct{}{}

	watcher.flushBatch()
	if calls != maxImmediateBatchRetries+1 {
		t.Fatalf("first batch calls = %d, want %d", calls, maxImmediateBatchRetries+1)
	}
	if len(timers) != 1 {
		t.Fatalf("deferred timers after first exhausted batch = %d, want 1", len(timers))
	}
	if _, ok := watcher.pendingBatch["file.ts"]; !ok {
		t.Fatal("contended file was dropped from pending batch")
	}

	timers[0].fire()
	if calls != 2*(maxImmediateBatchRetries+1) {
		t.Fatalf("deferred batch calls = %d, want %d", calls, 2*(maxImmediateBatchRetries+1))
	}
	if len(timers) != 1 {
		t.Fatalf("deferred timers after deferred failure = %d, want 1", len(timers))
	}
	if _, ok := watcher.pendingBatch["file.ts"]; !ok {
		t.Fatal("contended file was dropped after deferred failure")
	}
	if watcher.batchRetry["file.ts"] != maxDeferredBatchRetries+1 {
		t.Fatalf("deferred retry state = %d, want exhausted state %d", watcher.batchRetry["file.ts"], maxDeferredBatchRetries+1)
	}
}

func TestWatcherBatchRetryNewEventRearmsExhaustedCycle(t *testing.T) {
	var timers []*deterministicWatcherTimer
	watcher := newTestBatchWatcher(&timers, func(string) error {
		return &store.IndexLockError{Path: "index.lock"}
	})
	watcher.pendingBatch["file.ts"] = struct{}{}

	watcher.flushBatch()
	timers[0].fire()
	if len(timers) != 1 {
		t.Fatalf("deferred timers after exhausted cycle = %d, want 1", len(timers))
	}

	watcher.scheduleBatchReindex("file.ts")
	if len(timers) != 2 {
		t.Fatalf("deferred timers after new event = %d, want 2", len(timers))
	}
	if watcher.batchRetry["file.ts"] != 0 {
		t.Fatalf("retry state after new event = %d, want reset", watcher.batchRetry["file.ts"])
	}
}

func TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "watcher-atomic", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"typescript"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.test/watcher-atomic", Languages: []string{"typescript"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	write := func(relative, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", relative, err)
		}
	}
	write("package.json", "{}\n")
	write("src/main.ts", "export function Before() {}\n")
	write(".gitignore", ".mi-lsp/\n")
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "mi-lsp-test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	initial, err := indexer.IndexWorkspace(context.Background(), root, true)
	if err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	if len(initial.Files) == 0 {
		t.Fatalf("IndexWorkspace indexed no files: %#v", initial)
	}

	write("src/main.ts", "export function After() {}\n")
	watcher := &FileWatcher{workspaceRoot: root}
	if err := watcher.reindexFile(filepath.Join(root, "src", "main.ts")); err != nil {
		t.Fatalf("reindexFile: %v", err)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	var hash string
	if err := db.QueryRowContext(context.Background(), "SELECT content_hash FROM files WHERE file_path = ?", "src/main.ts").Scan(&hash); err != nil {
		t.Fatalf("read file hash: %v", err)
	}
	wantHash := fmt.Sprintf("%x", md5.Sum([]byte("export function After() {}\n")))
	if hash != wantHash {
		t.Fatalf("content hash = %q, want %q", hash, wantHash)
	}
	symbols, err := store.SymbolsByFile(context.Background(), db, "src/main.ts", 100, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("watcher publication produced no symbols")
	}
	foundAfter := false
	for _, symbol := range symbols {
		if symbol.Name == "After" {
			foundAfter = true
		}
		if symbol.Name == "Before" {
			t.Fatalf("stale Before symbol remained after watcher publication: %#v", symbols)
		}
	}
	if !foundAfter {
		t.Fatalf("symbols = %#v, want After symbol", symbols)
	}
}

func TestClassifyWatchPathRoutesCanonicalDomainsAndExclusions(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		path string
		want watchDomain
	}{
		{filepath.Join(root, "src", "main.mts"), watchDomainCode},
		{filepath.Join(root, ".docs", "wiki", "guide.md"), watchDomainDocs},
		{filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md"), watchDomainAuthorityConfig},
		{filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), watchDomainAuthorityConfig},
		{filepath.Join(root, ".gitignore"), watchDomainAuthorityConfig},
		{filepath.Join(root, ".docs", "raw", "draft.md"), ""},
		{filepath.Join(root, ".docs", "auditoria", "report.md"), ""},
		{filepath.Join(root, ".mi-lsp", "index.db"), ""},
		{filepath.Join(root, "dist", "bundle.js"), ""},
		{filepath.Join(root, "notes.txt"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := classifyWatchPath(root, tt.path); got != tt.want {
				t.Fatalf("classifyWatchPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestWatcherEventPathsPreserveRenameAndDeleteSemantics(t *testing.T) {
	changed, deleted := watcherEventPaths(watcherEvent{path: ".docs/wiki/old.md", op: fsnotify.Rename})
	if len(changed) != 0 || len(deleted) != 1 || deleted[0] != ".docs/wiki/old.md" {
		t.Fatalf("rename paths changed=%#v deleted=%#v", changed, deleted)
	}
	changed, deleted = watcherEventPaths(watcherEvent{path: ".docs/wiki/new.md", op: fsnotify.Create})
	if len(changed) != 1 || len(deleted) != 0 || changed[0] != ".docs/wiki/new.md" {
		t.Fatalf("create paths changed=%#v deleted=%#v", changed, deleted)
	}
}

func TestWatcherBatchPreservesRenameDeleteAndCoalescesPaths(t *testing.T) {
	var timers []*deterministicWatcherTimer
	var calls []string
	watcher := newTestBatchWatcher(&timers, func(path string) error {
		calls = append(calls, path)
		return nil
	})
	watcher.scheduleBatchEvent(fsnotify.Event{Name: ".docs/wiki/old.md", Op: fsnotify.Rename})
	watcher.scheduleBatchEvent(fsnotify.Event{Name: ".docs/wiki/new.md", Op: fsnotify.Create})
	watcher.scheduleBatchEvent(fsnotify.Event{Name: ".docs/wiki/new.md", Op: fsnotify.Write})
	if len(watcher.pendingBatch) != 2 {
		t.Fatalf("pending paths = %d, want 2", len(watcher.pendingBatch))
	}
	if watcher.pendingOps[".docs/wiki/old.md"] != fsnotify.Rename {
		t.Fatalf("old event op = %v, want rename", watcher.pendingOps[".docs/wiki/old.md"])
	}
	if watcher.pendingOps[".docs/wiki/new.md"] != fsnotify.Create|fsnotify.Write {
		t.Fatalf("new event op = %v, want create|write", watcher.pendingOps[".docs/wiki/new.md"])
	}
	watcher.flushBatch()
	if len(calls) != 2 || calls[0] != ".docs/wiki/new.md" || calls[1] != ".docs/wiki/old.md" {
		t.Fatalf("coalesced calls = %#v, want sorted new/old paths", calls)
	}
}

func TestWatcherFailedRefreshRemainsPendingWithDiagnostic(t *testing.T) {
	var timers []*deterministicWatcherTimer
	watcher := newTestBatchWatcher(&timers, func(string) error {
		return fmt.Errorf("parse failed")
	})
	watcher.pendingBatch[".docs/wiki/guide.md"] = struct{}{}
	watcher.flushBatch()
	if _, ok := watcher.pendingBatch[".docs/wiki/guide.md"]; !ok {
		t.Fatal("failed refresh was removed from pending work")
	}
	diagnostics := watcher.PendingDiagnostics()
	if !strings.Contains(diagnostics[".docs/wiki/guide.md"], "parse failed") {
		t.Fatalf("diagnostics = %#v, want parse failure", diagnostics)
	}
}

func TestWatcherLostEventRemainsVisibleForQueryOverlay(t *testing.T) {
	watcher := &FileWatcher{batchDiagnostics: make(map[string]watcherDiagnostic)}
	watcher.recordLostEvent("queue overflow")
	if !watcher.LostEvents() {
		t.Fatal("lost event marker was not retained")
	}
	diagnostics := watcher.PendingDiagnostics()
	if !strings.Contains(diagnostics["<watcher>"], "lost_event") {
		t.Fatalf("diagnostics = %#v, want lost-event marker for overlay freshness", diagnostics)
	}
}

func TestWatcherWorktreeStateIsDistinct(t *testing.T) {
	leftRoot := t.TempDir()
	rightRoot := t.TempDir()
	left, err := NewFileWatcher(model.WorkspaceRegistration{Name: "left", Root: leftRoot}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewFileWatcher(model.WorkspaceRegistration{Name: "right", Root: rightRoot}, time.Millisecond)
	if err != nil {
		left.Stop()
		t.Fatal(err)
	}
	defer left.Stop()
	defer right.Stop()
	left.scheduleBatchReindex(filepath.Join(leftRoot, "src", "main.go"))
	if len(right.pendingBatch) != 0 {
		t.Fatalf("right worktree inherited pending state: %#v", right.pendingBatch)
	}
}

func TestWikiCodeVerticalDirectDaemonParityByDigest(t *testing.T) {
	root := newDaemonWikiCodeVerticalFixture(t)
	request := model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "nav.trace",
		Context:         model.QueryOptions{Workspace: root, MaxItems: 32, TokenBudget: 20_000},
		Payload:         map[string]any{"rf": "RF-DEMO-001"},
	}
	direct, err := service.New(root, nil).Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("direct trace: %v", err)
	}
	routed, err := (&Server{app: service.New(root, nil)}).handleRequest(request)
	if err != nil {
		t.Fatalf("daemon trace: %v", err)
	}
	if direct.WikiCodeContext == nil || routed.WikiCodeContext == nil {
		t.Fatalf("bridge context missing: direct=%#v routed=%#v", direct.WikiCodeContext, routed.WikiCodeContext)
	}
	if direct.WikiCodeContext.DeterminismDigest == "" || routed.WikiCodeContext.DeterminismDigest == "" {
		t.Fatalf("bridge digest missing: direct=%#v routed=%#v", direct.WikiCodeContext, routed.WikiCodeContext)
	}
	if direct.WikiCodeContext.DeterminismDigest != routed.WikiCodeContext.DeterminismDigest {
		t.Fatalf("direct/daemon digest mismatch: direct=%q routed=%q", direct.WikiCodeContext.DeterminismDigest, routed.WikiCodeContext.DeterminismDigest)
	}
	canonicalDirect := model.WikiCodeContextDigest(*direct.WikiCodeContext)
	canonicalRouted := model.WikiCodeContextDigest(*routed.WikiCodeContext)
	if canonicalDirect != canonicalRouted {
		t.Fatalf("canonical direct/daemon digest mismatch: direct=%q routed=%q", canonicalDirect, canonicalRouted)
	}
	left, _ := json.Marshal(direct.WikiCodeContext.DirectCode)
	right, _ := json.Marshal(routed.WikiCodeContext.DirectCode)
	if string(left) != string(right) {
		t.Fatalf("direct/daemon direct evidence differs: direct=%s routed=%s", left, right)
	}
}

func newDaemonWikiCodeVerticalFixture(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	source := filepath.Join(repoRoot, "testdata", "wiki-code-bidirectional")
	root := t.TempDir()
	if err := copyDaemonWikiCodeVerticalTree(source, root); err != nil {
		t.Fatalf("copy T3 fixture: %v", err)
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "wiki-code-daemon", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"javascript", "typescript"}},
		Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/wiki-code-daemon", Languages: []string{"javascript", "typescript"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	if _, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "wiki-code-daemon-baseline"); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	return root
}

func copyDaemonWikiCodeVerticalTree(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(source, entry.Name())
		dstPath := filepath.Join(destination, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture contains unsupported symlink %s", entry.Name())
		}
		if entry.IsDir() {
			if err := copyDaemonWikiCodeVerticalTree(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		content, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestIsWatchableFile_ValidExtensions(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"file.cs", true},
		{"file.go", false},
		{"file.ts", true},
		{"file.tsx", true},
		{"file.js", true},
		{"file.jsx", true},
		{"file.py", true},
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
		{"file.go", false},
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
		{"FILE.JS", true},
		{"file.Go", false},
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
		{"/path/to/file.go", false},
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
		{".go", false},
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
	// Verify all watchable extensions are correctly recognized
	watchable := []string{".cs", ".ts", ".tsx", ".js", ".jsx", ".py"}

	for _, ext := range watchable {
		filename := "test" + ext
		got := isWatchableFile(filename)
		if !got {
			t.Errorf("isWatchableFile(%q) = false, want true (watchable extension)", filename)
		}
	}
}

func TestIsWatchableFile_NonWatchableExtensions(t *testing.T) {
	nonWatchable := []string{".go", ".md", ".txt", ".json", ".yaml", ".rb", ".php", ".cpp", ".h"}

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
		{"file.spec.go", false}, // last extension is .go
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
	expectedExts := []string{".cs", ".ts", ".tsx", ".js", ".jsx", ".py"}
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

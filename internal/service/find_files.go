package service

import (
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	findFilesMaxListingBytes = 32 << 20
	findFilesCacheAge        = 5 * time.Second
)

// FileFinder has no daemon, shell, semantic-worker or index-registration dependency.
// CacheDir may be empty to disable the best-effort cross-process listing cache.
type FileFinder struct{ CacheDir string }

type FindFilesOptions struct {
	Top      int
	MaxBytes int
	Literal  bool
	Refresh  bool
}

type findFilesSnapshot struct {
	Version int       `json:"version"`
	Root    string    `json:"root"`
	Created time.Time `json:"created"`
	Backend string    `json:"backend"`
	Paths   []string  `json:"paths"`
}

func NewFileFinder() *FileFinder {
	dir, err := os.UserCacheDir()
	if err != nil {
		return &FileFinder{}
	}
	return &FileFinder{CacheDir: filepath.Join(dir, "mi-lsp", "find-files-v1")}
}

// FindFiles searches a bounded, cached native filename listing. Root must be a
// directory; symlinks (including symlink directories) are never returned/followed.
func (f *FileFinder) FindFiles(ctx context.Context, root, query string, opts FindFilesOptions) (model.FindFilesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := model.FindFilesResult{Schema: "find-files/v1", Items: []model.FindFilesItem{}, Budget: model.FindFilesBudget{Top: opts.Top, MaxBytes: opts.MaxBytes}}
	if ctx.Err() != nil {
		return result, findFilesErr("cancelled", "filename search cancelled")
	}
	if opts.Top < 1 || opts.Top > 1000 || opts.MaxBytes < 512 || opts.MaxBytes > 1<<20 {
		return result, findFilesErr("invalid_budget", "top must be 1..1000 and budget-bytes 512..1048576")
	}
	if query == "" || len(query) > 512 || strings.ContainsRune(query, 0) || !utf8.ValidString(query) {
		return result, findFilesErr("invalid_pattern", "provide a UTF-8 filename or glob of 1..512 bytes")
	}
	match, err := compileFileQuery(query, opts.Literal)
	if err != nil {
		return result, err
	}
	root, err = filepath.Abs(root)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return result, findFilesErr("invalid_workspace", "workspace directory is unavailable")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return result, findFilesErr("invalid_workspace", "workspace must be a directory")
	}
	snapshot, cached, err := f.fileListing(ctx, root, opts.Refresh)
	if err != nil {
		return result, err
	}
	age := time.Since(snapshot.Created).Milliseconds()
	if age < 0 {
		age = 0
	}
	result.Listing = model.FindFilesListing{Backend: snapshot.Backend, Cached: cached, AgeMS: age, MaxAgeMS: findFilesCacheAge.Milliseconds()}
	best := &fileMatchHeap{}
	heap.Init(best)
	// Checking parent directories once per request avoids repeated stats while
	// still rejecting an ancestor replaced with an out-of-workspace symlink.
	dirs := map[string]bool{".": true}
	for _, name := range snapshot.Paths {
		if err := ctx.Err(); err != nil {
			return result, findFilesErr("cancelled", "filename search cancelled")
		}
		rank, reason, ok := match(name)
		if !ok || !safeFindFile(root, name, dirs) {
			continue
		}
		result.Matched++
		item := model.FindFilesItem{Path: name, Rank: rank, Reason: reason}
		if best.Len() < opts.Top {
			heap.Push(best, item)
		} else if fileMatchLess(item, (*best)[0]) {
			(*best)[0] = item
			heap.Fix(best, 0)
		}
	}
	result.Items = append(result.Items, (*best)...)
	sort.Slice(result.Items, func(i, j int) bool { return fileMatchLess(result.Items[i], result.Items[j]) })
	if result.Matched > len(result.Items) {
		result.Truncated = true
		result.Reason = "top"
		result.NextHint = "Narrow the filename/glob or increase --top (max 1000)."
	}
	// Account for the entire JSON envelope, metadata, escaping and newline, not
	// just path lengths or an optimistic token estimate.
	encoded, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if len(encoded)+1 > opts.MaxBytes {
		result.Truncated = true
		result.Reason = "budget_bytes"
		result.NextHint = "Narrow the filename/glob or increase --budget-bytes."
		items := result.Items
		result.Items = items[:0]
		encoded, err = json.Marshal(result)
		if err != nil {
			return result, err
		}
		if len(encoded)+1 > opts.MaxBytes {
			return result, findFilesErr("invalid_budget", "budget cannot fit response metadata")
		}
		// Find the largest fitting prefix in O(log top) serializations, even
		// when individual paths are long and the requested budget is tiny.
		lo, hi := 0, len(items)
		for lo < hi {
			mid := lo + (hi-lo+1)/2
			result.Items = items[:mid]
			encoded, err = json.Marshal(result)
			if err != nil {
				return result, err
			}
			if len(encoded)+1 <= opts.MaxBytes {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		result.Items = items[:lo]
	}
	return result, nil
}

func findFilesErr(code, message string) error {
	return &model.FindFilesError{Code: code, Message: message}
}

func fileMatchLess(a, b model.FindFilesItem) bool {
	if a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	ad, bd := strings.Count(a.Path, "/"), strings.Count(b.Path, "/")
	if ad != bd {
		return ad < bd
	}
	if len(a.Path) != len(b.Path) {
		return len(a.Path) < len(b.Path)
	}
	return a.Path < b.Path
}

// A max-heap retains only top candidates instead of allocating every match.
type fileMatchHeap []model.FindFilesItem

func (h fileMatchHeap) Len() int           { return len(h) }
func (h fileMatchHeap) Less(i, j int) bool { return fileMatchLess(h[j], h[i]) }
func (h fileMatchHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *fileMatchHeap) Push(x any)        { *h = append(*h, x.(model.FindFilesItem)) }
func (h *fileMatchHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func compileFileQuery(query string, literal bool) (func(string) (int, string, bool), error) {
	// Slash is portable; backslash is accepted as a Windows path separator,
	// never interpreted as a regexp escape or passed to a subprocess.
	query = strings.TrimPrefix(strings.ReplaceAll(query, "\\", "/"), "./")
	q := strings.ToLower(query)
	if q == "" || strings.HasPrefix(q, "/") || strings.Contains(q, ":") || strings.Count(q, "/") > 64 {
		return nil, findFilesErr("invalid_pattern", "use a relative filename or glob with at most 64 separators")
	}
	for _, segment := range strings.Split(q, "/") {
		if segment == ".." {
			return nil, findFilesErr("invalid_pattern", "parent traversal is not a filename query")
		}
	}
	glob := !literal && strings.ContainsAny(q, "*?[")
	if glob {
		for _, part := range strings.Split(q, "/") {
			if part == "**" {
				continue
			}
			if _, err := path.Match(part, ""); err != nil {
				return nil, findFilesErr("invalid_pattern", "malformed glob; use --literal for metacharacters in filenames")
			}
		}
	}
	return func(name string) (int, string, bool) {
		n := strings.ToLower(name)
		base := path.Base(n)
		if glob {
			var ok bool
			if strings.Contains(q, "/") {
				ok = fileGlobMatch(q, n)
			} else {
				ok, _ = path.Match(q, base)
			}
			return 3, "glob", ok
		}
		switch {
		case base == q:
			return 0, "basename_exact", true
		case strings.HasPrefix(base, q):
			return 1, "basename_prefix", true
		}
		for _, segment := range strings.Split(n, "/") {
			if segment == q || strings.HasPrefix(segment, q) {
				return 2, "segment", true
			}
		}
		if strings.Contains(n, q) {
			return 3, "substring", true
		}
		return 0, "", false
	}, nil
}

// ** matches zero or more whole segments; memoization bounds work to the
// pattern/path segment product. All other syntax is Go path.Match, not regex.
func fileGlobMatch(pattern, name string) bool {
	p, n := strings.Split(pattern, "/"), strings.Split(name, "/")
	type state struct{ p, n int }
	seen, memo := map[state]bool{}, map[state]bool{}
	var visit func(int, int) bool
	visit = func(i, j int) bool {
		s := state{i, j}
		if seen[s] {
			return memo[s]
		}
		seen[s] = true
		ok := false
		if i == len(p) {
			ok = j == len(n)
		} else if p[i] == "**" {
			ok = visit(i+1, j) || (j < len(n) && visit(i, j+1))
		} else if j < len(n) {
			matched, _ := path.Match(p[i], n[j])
			ok = matched && visit(i+1, j+1)
		}
		memo[s] = ok
		return ok
	}
	return visit(0, 0)
}

func validFindFileName(name string) bool {
	if name == "" || name == "." || strings.Contains(name, "\\") || strings.Contains(name, ":") || !utf8.ValidString(name) || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.EqualFold(part, ".git") || part == ".." || part == "" {
			return false
		}
	}
	return true
}

func safeFindFile(root, name string, dirs map[string]bool) bool {
	if !validFindFileName(name) {
		return false
	}
	var safeDir func(string) bool
	safeDir = func(dir string) bool {
		if ok, found := dirs[dir]; found {
			return ok
		}
		ok := safeDir(path.Dir(dir))
		if ok {
			info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(dir)))
			ok = err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
		}
		dirs[dir] = ok
		return ok
	}
	if !safeDir(path.Dir(name)) {
		return false
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
	return err == nil && info.Mode().IsRegular()
}

func (f *FileFinder) fileListing(ctx context.Context, root string, refresh bool) (findFilesSnapshot, bool, error) {
	sum := sha256.Sum256([]byte(root))
	cachePath := filepath.Join(f.CacheDir, hex.EncodeToString(sum[:])+".json")
	if f.CacheDir != "" && !refresh {
		if file, err := os.Open(cachePath); err == nil {
			var s findFilesSnapshot
			err = json.NewDecoder(io.LimitReader(file, findFilesMaxListingBytes+1)).Decode(&s)
			file.Close()
			age := time.Since(s.Created)
			if err == nil && s.Version == 1 && s.Root == root && age >= 0 && age < findFilesCacheAge && (s.Backend == "git" || s.Backend == "rg" || s.Backend == "fd") {
				return s, true, nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	backend, raw, err := nativeFindFiles(ctx, root)
	if err != nil {
		return findFilesSnapshot{}, false, err
	}
	paths := make([]string, 0)
	seen := map[string]bool{}
	for _, name := range strings.Split(string(raw), "\x00") {
		name = strings.TrimPrefix(name, "./")
		// rg/fd use native separators on Windows. Git always uses '/'.
		if filepath.Separator == '\\' {
			name = filepath.ToSlash(name)
		}
		if validFindFileName(name) && !seen[name] {
			paths = append(paths, name)
			seen[name] = true
		}
	}
	snapshot := findFilesSnapshot{Version: 1, Root: root, Created: time.Now(), Backend: backend, Paths: paths}
	if f.CacheDir != "" {
		f.saveFileListing(cachePath, snapshot)
	}
	return snapshot, false, nil
}

func (f *FileFinder) saveFileListing(name string, snapshot findFilesSnapshot) {
	if err := os.MkdirAll(f.CacheDir, 0700); err != nil {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil || len(data) > findFilesMaxListingBytes {
		return
	}
	file, err := os.CreateTemp(f.CacheDir, "listing-*.json")
	if err != nil {
		return
	}
	temp := file.Name()
	defer os.Remove(temp)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(temp, name)
	}
}

type findFilesLimitedBuffer struct {
	data  []byte
	limit int
}

func (b *findFilesLimitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-len(b.data) {
		return 0, errors.New("listing byte limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func nativeFindFiles(ctx context.Context, root string) (string, []byte, error) {
	if ctx.Err() != nil {
		return "", nil, findFilesErr("cancelled", "filename listing cancelled")
	}
	// Git is the portable fd-free path, and owns the complete nested/negated
	// .gitignore semantics (plus info/exclude and global excludes). Tracked files
	// remain discoverable even if later ignored, just as with git ls-files.
	if git, err := exec.LookPath("git"); err == nil {
		probe := exec.CommandContext(ctx, git, "rev-parse", "--is-inside-work-tree")
		probe.Dir = root
		out := &findFilesLimitedBuffer{limit: 128}
		probe.Stdout = out
		if err := probe.Run(); err == nil && strings.TrimSpace(string(out.data)) == "true" {
			raw, err := runFindFilesListing(ctx, root, git, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
			return "git", raw, err
		}
	}
	// Never pass the user's query to an executable. These commands list only;
	// hidden files are included, ignore rules retained, symlinks not followed.
	if rg, err := exec.LookPath("rg"); err == nil {
		raw, err := runFindFilesListing(ctx, root, rg, "--files", "--null", "--hidden", "--no-require-git", "--glob", "!.git", "--", ".")
		return "rg", raw, err
	}
	for _, name := range []string{"fd", "fdfind"} {
		if fd, err := exec.LookPath(name); err == nil {
			raw, err := runFindFilesListing(ctx, root, fd, "--type", "f", "--hidden", "--no-require-git", "--print0", "--exclude", ".git", "--", ".", ".")
			return "fd", raw, err
		}
	}
	// Do not silently replace Git semantics with an approximate custom ignore walker.
	return "", nil, findFilesErr("file_listing_unavailable", "Git worktree or existing rg/fd required for exact ignore-aware listing")
}

func runFindFilesListing(ctx context.Context, root, exe string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = root
	out := &findFilesLimitedBuffer{limit: findFilesMaxListingBytes}
	cmd.Stdout = out
	// Stderr is intentionally discarded: filenames and local configuration can
	// occur in native error output. Return a stable public error instead.
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, findFilesErr("listing_timeout", "filename listing cancelled or exceeded 10 seconds")
	}
	// rg returns 1 for a valid empty listing.
	var exit *exec.ExitError
	if err != nil && !(filepath.Base(exe) == "rg" || filepath.Base(exe) == "rg.exe") {
		return nil, findFilesErr("file_listing_failed", "native filename listing failed or exceeded 32 MiB")
	}
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1 && len(out.data) == 0) {
		return nil, findFilesErr("file_listing_failed", "native filename listing failed or exceeded 32 MiB")
	}
	return out.data, nil
}

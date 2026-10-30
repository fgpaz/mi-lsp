package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

// WorktreeOverlay describes the physical delta from the main checkout at the
// same git-common-dir. Paths are repository-relative and always belong to the
// physical worktree; callers must not read the corresponding base path instead.
type WorktreeOverlay struct {
	Root        string
	BaseRoot    string
	CommonDir   string
	Changed     []string
	Deleted     []string
	Renamed     map[string]string // old path -> new path
	Fingerprint string            // base HEAD plus physical delta content, for cache/snapshot identity
}

// TryResolveWorktreeOverlay distinguishes an ordinary repository from a linked worktree.
func TryResolveWorktreeOverlay(root string) (WorktreeOverlay, bool, error) {
	physicalRoot, ok := gitTopLevel(root)
	if !ok {
		return WorktreeOverlay{}, false, nil
	}
	linked, _, valid := linkedWorktree(physicalRoot)
	if !valid {
		return WorktreeOverlay{}, false, fmt.Errorf("cannot resolve git common-dir: %s", physicalRoot)
	}
	if !linked {
		return WorktreeOverlay{}, false, nil
	}
	overlay, err := ResolveWorktreeOverlay(physicalRoot)
	return overlay, true, err
}

// ResolveWorktreeOverlay returns the committed, staged, unstaged, and
// untracked path delta for a linked worktree without registering or indexing it.
// RegisteredWorktreeBase returns the registered main checkout for this common-dir.
func worktreeOverlayFingerprint(overlay WorktreeOverlay) (string, error) {
	h := sha256.New()
	write := func(value string) {
		_, _ = h.Write([]byte(fmt.Sprintf("%d:%s", len(value), value)))
	}
	baseHead, err := processutil.Command("git", "-C", overlay.BaseRoot, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("resolve overlay base HEAD: %w", err)
	}
	write(filepath.Clean(overlay.CommonDir))
	write(filepath.Clean(overlay.Root))
	write(strings.TrimSpace(string(baseHead)))
	for _, path := range overlay.Changed {
		write("changed:" + filepath.ToSlash(path))
		physical := filepath.Join(overlay.Root, filepath.FromSlash(path))
		info, statErr := filepath.EvalSymlinks(physical)
		if statErr != nil {
			write("missing")
			continue
		}
		rel, relErr := filepath.Rel(overlay.Root, info)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			write("outside-root")
			continue
		}
		body, readErr := os.ReadFile(info)
		if readErr != nil {
			write("unreadable")
			continue
		}
		digest := sha256.Sum256(body)
		write(hex.EncodeToString(digest[:]))
	}
	for _, path := range overlay.Deleted {
		write("deleted:" + filepath.ToSlash(path))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func RegisteredWorktreeBase(overlay WorktreeOverlay) (model.WorkspaceRegistration, bool, error) {
	_, commonDir, valid := linkedWorktree(overlay.Root)
	if !valid {
		return model.WorkspaceRegistration{}, false, fmt.Errorf("cannot resolve worktree common-dir: %s", overlay.Root)
	}
	registry, err := LoadRegistryReadOnly()
	if err != nil {
		return model.WorkspaceRegistration{}, false, err
	}
	registration, found := registeredMainWorktree(commonDir, registry)
	if !found || !sameFilePath(registration.Root, overlay.BaseRoot) {
		return model.WorkspaceRegistration{}, false, nil
	}
	return registration, true, nil
}

func ResolveWorktreeOverlay(root string) (WorktreeOverlay, error) {
	physicalRoot, ok := gitTopLevel(root)
	if !ok {
		return WorktreeOverlay{}, fmt.Errorf("not a git worktree: %s", root)
	}
	linked, commonDir, valid := linkedWorktree(physicalRoot)
	if !valid || !linked {
		return WorktreeOverlay{}, fmt.Errorf("not a linked git worktree: %s", physicalRoot)
	}
	baseRoot := filepath.Dir(commonDir)
	baseTop, ok := gitTopLevel(baseRoot)
	if !ok || filepath.Clean(baseTop) != filepath.Clean(baseRoot) {
		return WorktreeOverlay{}, fmt.Errorf("git common-dir has no verified main worktree: %s", commonDir)
	}
	baseLinked, baseCommon, baseValid := linkedWorktree(baseRoot)
	if !baseValid || baseLinked || filepath.Clean(baseCommon) != filepath.Clean(commonDir) {
		return WorktreeOverlay{}, fmt.Errorf("git common-dir mismatch for base root %s", baseRoot)
	}
	baseRevision, err := processutil.Command("git", "-C", baseRoot, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("resolve base revision: %w", err)
	}
	args := []string{"-C", physicalRoot, "diff", "--name-status", "-z", "-M", strings.TrimSpace(string(baseRevision)), "--"}
	diff, err := processutil.Command("git", args...).Output()
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read worktree delta: %w", err)
	}
	untracked, err := processutil.Command("git", "-C", physicalRoot, "ls-files", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read untracked worktree paths: %w", err)
	}
	baseDiff, err := processutil.Command("git", "-C", baseRoot, "diff", "--name-status", "-z", "-M", strings.TrimSpace(string(baseRevision)), "--").Output()
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read base worktree changes: %w", err)
	}
	baseUntracked, err := processutil.Command("git", "-C", baseRoot, "ls-files", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read base untracked paths: %w", err)
	}
	// Go's package loader reads ignored generated source too. Include those
	// source inputs in overlay detection so a clean Git status cannot select a
	// stale base graph or reuse a snapshot whose fingerprint omitted the file.
	ignoredGoSources := make([][]byte, 0, 2)
	for _, treeRoot := range []string{physicalRoot, baseRoot} {
		paths, err := processutil.Command("git", "-C", treeRoot, "ls-files", "--others", "--ignored", "--exclude-standard", "-z", "--", "*.go", "go.mod", "go.sum").Output()
		if err != nil {
			return WorktreeOverlay{}, fmt.Errorf("read ignored Go source paths: %w", err)
		}
		ignoredGoSources = append(ignoredGoSources, paths)
	}
	overlay := WorktreeOverlay{Root: physicalRoot, BaseRoot: baseRoot, CommonDir: commonDir, Renamed: map[string]string{}}
	seen := map[string]bool{}
	add := func(path string) {
		path = filepath.Clean(filepath.FromSlash(path))
		if path == "." || path == ".." || filepath.IsAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator)) || seen[path] {
			return
		}
		for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
			if segment == ".git" || segment == ".mi-lsp" {
				return
			}
		}
		seen[path] = true
		overlay.Changed = append(overlay.Changed, filepath.ToSlash(path))
	}
	collectDiff := func(data []byte, physical bool) error {
		fields := strings.Split(string(data), "\x00")
		for i := 0; i < len(fields); {
			status := fields[i]
			i++
			if status == "" || i >= len(fields) {
				continue
			}
			oldPath := fields[i]
			i++
			if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
				if i >= len(fields) {
					return fmt.Errorf("malformed git rename status")
				}
				newPath := fields[i]
				i++
				if strings.HasPrefix(status, "R") {
					if physical {
						overlay.Deleted = append(overlay.Deleted, filepath.ToSlash(filepath.Clean(oldPath)))
					} else {
						// The base index may no longer contain the old path, while
						// the linked tree still has it. Re-extract it there.
						add(oldPath)
					}
				}
				if strings.HasPrefix(status, "R") {
					overlay.Renamed[filepath.ToSlash(filepath.Clean(oldPath))] = filepath.ToSlash(filepath.Clean(newPath))
				}
				add(newPath)
				continue
			}
			if strings.HasPrefix(status, "D") {
				if physical {
					overlay.Deleted = append(overlay.Deleted, filepath.ToSlash(filepath.Clean(oldPath)))
				} else {
					add(oldPath) // mask the base deletion state; inspect the physical overlay
				}
				continue
			}
			add(oldPath)
		}
		return nil
	}
	if err := collectDiff(diff, true); err != nil {
		return WorktreeOverlay{}, err
	}
	if err := collectDiff(baseDiff, false); err != nil {
		return WorktreeOverlay{}, err
	}
	for _, paths := range [][]byte{untracked, baseUntracked} {
		for _, path := range strings.Split(string(paths), "\x00") {
			add(path)
		}
	}
	for _, paths := range ignoredGoSources {
		for _, path := range strings.Split(string(paths), "\x00") {
			add(path)
		}
	}
	sort.Strings(overlay.Changed)
	sort.Strings(overlay.Deleted)
	fingerprint, err := worktreeOverlayFingerprint(overlay)
	if err != nil {
		return WorktreeOverlay{}, err
	}
	overlay.Fingerprint = fingerprint
	return overlay, nil
}

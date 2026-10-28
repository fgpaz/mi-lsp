package workspace

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

// WorktreeOverlay describes the physical delta from the main checkout at the
// same git-common-dir. Paths are repository-relative and always belong to the
// physical worktree; callers must not read the corresponding base path instead.
type WorktreeOverlay struct {
	Root     string
	BaseRoot string
	Changed  []string
	Deleted  []string
	Renamed  map[string]string // old path -> new path
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
	overlay := WorktreeOverlay{Root: physicalRoot, BaseRoot: baseRoot, Renamed: map[string]string{}}
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
				if strings.HasPrefix(status, "R") && physical {
					overlay.Deleted = append(overlay.Deleted, filepath.ToSlash(filepath.Clean(oldPath)))
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
	sort.Strings(overlay.Changed)
	sort.Strings(overlay.Deleted)
	return overlay, nil
}

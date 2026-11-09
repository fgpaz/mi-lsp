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

// WorktreeOverlay describes the physical delta of a linked worktree against its
// base commit. Changed and Deleted hold only that delta, so graph decisions see
// the worktree and nothing else. Paths are repository-relative and belong to the
// physical worktree; callers must not read the corresponding base path instead.
type WorktreeOverlay struct {
	Root      string
	BaseRoot  string
	CommonDir string
	Changed   []string          // physical content differs from the base commit (tracked, staged, untracked, ignored Go inputs)
	Deleted   []string          // physically removed relative to the base commit
	Renamed   map[string]string // old path -> new path
	// BaseEdited lists paths whose base checkout differs from its commit. The
	// base index may hold that content, so find and search mask these paths and
	// read the physical file. It is not physical delta and does not mark a graph.
	BaseEdited  []string
	Fingerprint string // base HEAD plus physical delta content
}

// MaskedPaths lists every path the base index must not answer for: the physical
// delta plus base checkout edits. The result is sorted and unique.
func (o WorktreeOverlay) MaskedPaths() []string {
	return sortedUnionPaths(o.Changed, o.Deleted, o.BaseEdited)
}

// ReadPaths lists paths whose physical content replaces the base index answer.
// Callers still skip entries present in Deleted. The result is sorted and unique.
func (o WorktreeOverlay) ReadPaths() []string {
	return sortedUnionPaths(o.Changed, o.BaseEdited)
}

func sortedUnionPaths(lists ...[]string) []string {
	set := map[string]struct{}{}
	for _, list := range lists {
		for _, path := range list {
			set[filepath.ToSlash(path)] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// IsLinkedWorktree reports whether root sits in a linked worktree. Unlike
// TryResolveWorktreeOverlay it does not compute the physical delta.
func IsLinkedWorktree(root string) (bool, error) {
	physicalRoot, ok := gitTopLevel(root)
	if !ok {
		return false, nil
	}
	linked, _, valid := linkedWorktree(physicalRoot)
	if !valid {
		return false, fmt.Errorf("cannot resolve git common-dir: %s", physicalRoot)
	}
	return linked, nil
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
	if !ok || !sameFilePath(baseTop, baseRoot) {
		return WorktreeOverlay{}, fmt.Errorf("git common-dir has no verified main worktree: %s", commonDir)
	}
	baseLinked, baseCommon, baseValid := linkedWorktree(baseRoot)
	if !baseValid || baseLinked || !sameFilePath(baseCommon, commonDir) {
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
	// source inputs so a clean Git status cannot hide a file the loader reads.
	ignoredGoSources := func(treeRoot string) ([]byte, error) {
		return processutil.Command("git", "-C", treeRoot, "ls-files", "--others", "--ignored", "--exclude-standard", "-z", "--", "*.go", "go.mod", "go.sum").Output()
	}
	physicalIgnored, err := ignoredGoSources(physicalRoot)
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read ignored Go source paths: %w", err)
	}
	baseIgnored, err := ignoredGoSources(baseRoot)
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read ignored Go source paths: %w", err)
	}
	overlay := WorktreeOverlay{Root: physicalRoot, BaseRoot: baseRoot, CommonDir: commonDir, Renamed: map[string]string{}}
	physicalSeen := map[string]bool{}
	baseSeen := map[string]bool{}
	clean := func(path string) (string, bool) {
		path = filepath.Clean(filepath.FromSlash(path))
		if path == "." || path == ".." || filepath.IsAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return "", false
		}
		for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
			if segment == ".git" || segment == ".mi-lsp" {
				return "", false
			}
		}
		return filepath.ToSlash(path), true
	}
	// addPhysical records a path whose physical worktree content differs from the base commit.
	addPhysical := func(path string) {
		if p, ok := clean(path); ok && !physicalSeen[p] {
			physicalSeen[p] = true
			overlay.Changed = append(overlay.Changed, p)
		}
	}
	// addBase records a path whose base checkout differs from its commit. It is not
	// physical delta: the linked worktree may still hold the committed content.
	addBase := func(path string) {
		if p, ok := clean(path); ok && !baseSeen[p] {
			baseSeen[p] = true
			overlay.BaseEdited = append(overlay.BaseEdited, p)
		}
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
				if !physical {
					addBase(oldPath)
					addBase(newPath)
					continue
				}
				if strings.HasPrefix(status, "R") {
					overlay.Deleted = append(overlay.Deleted, filepath.ToSlash(filepath.Clean(oldPath)))
					overlay.Renamed[filepath.ToSlash(filepath.Clean(oldPath))] = filepath.ToSlash(filepath.Clean(newPath))
				}
				addPhysical(newPath)
				continue
			}
			if strings.HasPrefix(status, "D") {
				if !physical {
					addBase(oldPath)
					continue
				}
				overlay.Deleted = append(overlay.Deleted, filepath.ToSlash(filepath.Clean(oldPath)))
				continue
			}
			if physical {
				addPhysical(oldPath)
			} else {
				addBase(oldPath)
			}
		}
		return nil
	}
	if err := collectDiff(diff, true); err != nil {
		return WorktreeOverlay{}, err
	}
	for _, paths := range [][]byte{untracked, physicalIgnored} {
		for _, path := range strings.Split(string(paths), "\x00") {
			addPhysical(path)
		}
	}
	if err := collectDiff(baseDiff, false); err != nil {
		return WorktreeOverlay{}, err
	}
	for _, paths := range [][]byte{baseUntracked, baseIgnored} {
		for _, path := range strings.Split(string(paths), "\x00") {
			addBase(path)
		}
	}
	sort.Strings(overlay.Changed)
	sort.Strings(overlay.Deleted)
	sort.Strings(overlay.BaseEdited)
	fingerprint, err := worktreeOverlayFingerprint(overlay)
	if err != nil {
		return WorktreeOverlay{}, err
	}
	overlay.Fingerprint = fingerprint
	return overlay, nil
}

package workspace

import (
	"context"
	"fmt"
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
	Root     string
	BaseRoot string
	Changed  []string
	Deleted  []string
	Renamed  map[string]string // old path -> new path
}

// TryResolveWorktreeOverlay distinguishes an ordinary repository from a linked worktree.
func TryResolveWorktreeOverlay(root string) (WorktreeOverlay, bool, error) {
	return TryResolveWorktreeOverlayContext(context.Background(), root)
}

func TryResolveWorktreeOverlayContext(ctx context.Context, root string) (WorktreeOverlay, bool, error) {
	physicalRoot, ok, err := gitTopLevelContext(ctx, root)
	if err != nil {
		return WorktreeOverlay{}, false, err
	}
	if !ok {
		return WorktreeOverlay{}, false, nil
	}
	linked, _, valid, err := linkedWorktreeContext(ctx, physicalRoot)
	if err != nil {
		return WorktreeOverlay{}, false, err
	}
	if !valid {
		return WorktreeOverlay{}, false, fmt.Errorf("cannot resolve git common-dir: %s", physicalRoot)
	}
	if err := ctx.Err(); err != nil {
		return WorktreeOverlay{}, false, err
	}
	if !linked {
		return WorktreeOverlay{}, false, nil
	}
	overlay, err := ResolveWorktreeOverlayContext(ctx, physicalRoot)
	if err != nil {
		return WorktreeOverlay{}, true, err
	}
	if err := ctx.Err(); err != nil {
		return WorktreeOverlay{}, false, err
	}
	return overlay, true, nil
}

func gitContextOutput(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output, err := processutil.CommandContext(ctx, "git", args...).Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return output, err
}

// ResolveWorktreeOverlay returns the committed, staged, unstaged, and
// untracked path delta for a linked worktree without registering or indexing it.
// RegisteredWorktreeBase returns the registered main checkout for this common-dir.
func RegisteredWorktreeBase(overlay WorktreeOverlay) (model.WorkspaceRegistration, bool, error) {
	return RegisteredWorktreeBaseContext(context.Background(), overlay)
}

func RegisteredWorktreeBaseContext(ctx context.Context, overlay WorktreeOverlay) (model.WorkspaceRegistration, bool, error) {
	_, commonDir, valid, err := linkedWorktreeContext(ctx, overlay.Root)
	if err != nil {
		return model.WorkspaceRegistration{}, false, err
	}
	if !valid {
		return model.WorkspaceRegistration{}, false, fmt.Errorf("cannot resolve worktree common-dir: %s", overlay.Root)
	}
	registry, err := LoadRegistryReadOnly()
	if err != nil {
		return model.WorkspaceRegistration{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return model.WorkspaceRegistration{}, false, err
	}
	registration, found, err := registeredMainWorktreeContext(ctx, commonDir, registry)
	if err != nil {
		return model.WorkspaceRegistration{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return model.WorkspaceRegistration{}, false, err
	}
	if !found || !sameFilePath(registration.Root, overlay.BaseRoot) {
		return model.WorkspaceRegistration{}, false, nil
	}
	return registration, true, nil
}

func ResolveWorktreeOverlay(root string) (WorktreeOverlay, error) {
	return ResolveWorktreeOverlayContext(context.Background(), root)
}

func ResolveWorktreeOverlayContext(ctx context.Context, root string) (WorktreeOverlay, error) {
	physicalRoot, ok, err := gitTopLevelContext(ctx, root)
	if err != nil {
		return WorktreeOverlay{}, err
	}
	if !ok {
		return WorktreeOverlay{}, fmt.Errorf("not a git worktree: %s", root)
	}
	linked, commonDir, valid, err := linkedWorktreeContext(ctx, physicalRoot)
	if err != nil {
		return WorktreeOverlay{}, err
	}
	if !valid || !linked {
		return WorktreeOverlay{}, fmt.Errorf("not a linked git worktree: %s", physicalRoot)
	}
	baseRoot := filepath.Dir(commonDir)
	baseTop, ok, err := gitTopLevelContext(ctx, baseRoot)
	if err != nil {
		return WorktreeOverlay{}, err
	}
	if !ok || filepath.Clean(baseTop) != filepath.Clean(baseRoot) {
		return WorktreeOverlay{}, fmt.Errorf("git common-dir has no verified main worktree: %s", commonDir)
	}
	baseLinked, baseCommon, baseValid, err := linkedWorktreeContext(ctx, baseRoot)
	if err != nil {
		return WorktreeOverlay{}, err
	}
	if !baseValid || baseLinked || filepath.Clean(baseCommon) != filepath.Clean(commonDir) {
		return WorktreeOverlay{}, fmt.Errorf("git common-dir mismatch for base root %s", baseRoot)
	}
	baseRevision, err := gitContextOutput(ctx, "-C", baseRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("resolve base revision: %w", err)
	}
	args := []string{"-C", physicalRoot, "diff", "--name-status", "-z", "-M", strings.TrimSpace(string(baseRevision)), "--"}
	diff, err := gitContextOutput(ctx, args...)
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read worktree delta: %w", err)
	}
	untracked, err := gitContextOutput(ctx, "-C", physicalRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read untracked worktree paths: %w", err)
	}
	baseDiff, err := gitContextOutput(ctx, "-C", baseRoot, "diff", "--name-status", "-z", "-M", strings.TrimSpace(string(baseRevision)), "--")
	if err != nil {
		return WorktreeOverlay{}, fmt.Errorf("read base worktree changes: %w", err)
	}
	baseUntracked, err := gitContextOutput(ctx, "-C", baseRoot, "ls-files", "--others", "--exclude-standard", "-z")
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
	if err := ctx.Err(); err != nil {
		return WorktreeOverlay{}, err
	}
	return overlay, nil
}

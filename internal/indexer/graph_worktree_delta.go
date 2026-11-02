package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GraphDeltaMembership returns only paths whose membership in the exact project
// is proven, including linked or deleted documents. It must not infer membership
// from directory prefixes. The observer must independently acknowledge them.
type GraphDeltaMembership func(context.Context, GraphObservationRequest, []string) ([]string, error)

type GraphDeltaOwner struct {
	Backend, RequestRoot, ProjectOrModule, RequestPath string
}

type GraphDeltaPathCoverage struct {
	WorkspacePath string
	Owners        []GraphDeltaOwner
	Config        bool
}

// GraphDeltaCoverage is populated only for selective calls. Empty Owners means
// unknown/uncovered, never permission to mask the path. On any error discard
// all owners: only successful complete observations can be consumed.
type GraphDeltaCoverage struct {
	Paths             []GraphDeltaPathCoverage
	TopologyAmbiguous bool
	// GoReceipts contain compiler-derived applied/excluded/missing/unknown paths.
	GoReceipts []GoGraphDeltaReceipt
}

func (c *GraphDeltaCoverage) reset(paths []string) {
	*c = GraphDeltaCoverage{}
	for _, p := range paths {
		ext := strings.ToLower(filepath.Ext(p))
		c.Paths = append(c.Paths, GraphDeltaPathCoverage{WorkspacePath: filepath.ToSlash(filepath.Clean(p)), Config: ext != ".go" && ext != ".cs"})
	}
}

func (c *GraphDeltaCoverage) record(workspaceRoot, requestRoot, backend, module string, paths []string) {
	for _, p := range paths {
		abs, err := filepath.Abs(filepath.Join(requestRoot, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		root, err := filepath.Abs(workspaceRoot)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		for i := range c.Paths {
			if c.Paths[i].WorkspacePath == filepath.ToSlash(rel) {
				c.Paths[i].Owners = append(c.Paths[i].Owners, GraphDeltaOwner{Backend: backend, RequestRoot: requestRoot, ProjectOrModule: module, RequestPath: p})
			}
		}
	}
}

func graphDeltaSamePaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, p := range a {
		seen[p]++
	}
	for _, p := range b {
		seen[p]--
		if seen[p] < 0 {
			return false
		}
	}
	return true
}

func graphDeltaValidateSubset(paths, original []string, workspaceRoot, requestRoot string) error {
	candidates, err := graphDeltaPathsForRoot(workspaceRoot, requestRoot, original, ".cs")
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, p := range candidates {
		allowed[p] = true
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if !allowed[p] || seen[p] || !graphDeltaSafePath(requestRoot, p) {
			return fmt.Errorf("explicit_incomplete: unverified Roslyn membership for %q", p)
		}
		seen[p] = true
	}
	return nil
}

// A missing component is valid for a deletion, while symlinks and any other
// Lstat error leave ownership indeterminate. No symlink traversal is assumed.
func graphDeltaSafePath(root, rel string) bool {
	current := root
	for _, part := range strings.Split(filepath.FromSlash(rel), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return true
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

// GraphDeltaPathsForRoot converts workspace-relative affected paths to the
// observation root's coordinates. Paths outside that root are not its mask.
// It does not resolve symlinks: deleted paths must remain representable.
func GraphDeltaPathsForRoot(workspaceRoot, observationRoot string, paths []string, extension string) ([]string, error) {
	if err := validateGraphDeltaPaths(paths); err != nil {
		return nil, err
	}
	return graphDeltaPathsForRoot(workspaceRoot, observationRoot, paths, extension)
}

func validateGraphDeltaPaths(paths []string) error {
	for _, path := range paths {
		if path == "" || strings.ContainsRune(path, 0) || strings.Contains(path, "\\") || filepath.IsAbs(path) || (len(path) >= 2 && path[1] == ':') {
			return fmt.Errorf("explicit_incomplete: invalid workspace-relative affected path %q", path)
		}
		for _, part := range strings.Split(path, "/") {
			if part == ".." {
				return fmt.Errorf("explicit_incomplete: invalid workspace-relative affected path %q", path)
			}
		}
		if filepath.Clean(path) == "." {
			return fmt.Errorf("explicit_incomplete: invalid workspace-relative affected path %q", path)
		}
	}
	return nil
}

func graphDeltaPathsForRoot(workspaceRoot, observationRoot string, paths []string, extension string) ([]string, error) {
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	rootAbs, err := filepath.Abs(observationRoot)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, path := range paths {
		if extension != "" && !strings.EqualFold(filepath.Ext(path), extension) {
			continue
		}
		rel, err := filepath.Rel(rootAbs, filepath.Join(workspaceAbs, filepath.FromSlash(path)))
		if err != nil {
			return nil, fmt.Errorf("explicit_incomplete: affected path %q: %w", path, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !seen[rel] {
			seen[rel] = true
			result = append(result, rel)
		}
	}
	return result, nil
}

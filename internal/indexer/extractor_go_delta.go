package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// ObserveGoGraphDelta observes changed module sources while retaining full package
// type context. affectedPaths are always relative to req.Root; absent/deleted paths
// are intentionally ignored so this observer cannot resurrect deleted records.
func ObserveGoGraphDelta(ctx context.Context, req GoGraphObservationRequest, affectedPaths []string) (model.GraphObservationBatch, error) {
	return observeGoGraphDeltaWithReceipt(ctx, req, affectedPaths, nil)
}

// ObserveGoGraphDeltaWithReceipt returns a compiler-context file partition.
// Receipt entries are root-relative. Missing is filesystem absence only, not
// proof of prior module ownership. Discard the receipt on error or non-ready batch.
func ObserveGoGraphDeltaWithReceipt(ctx context.Context, req GoGraphObservationRequest, affectedPaths []string) (model.GraphObservationBatch, GoGraphDeltaReceipt, error) {
	var receipt GoGraphDeltaReceipt
	batch, err := observeGoGraphDeltaWithReceipt(ctx, req, affectedPaths, &receipt)
	receipt.sync()
	return batch, receipt, err
}

func observeGoGraphDeltaWithReceipt(ctx context.Context, req GoGraphObservationRequest, affectedPaths []string, receipt *GoGraphDeltaReceipt) (model.GraphObservationBatch, error) {
	if len(affectedPaths) == 0 {
		return model.GraphObservationBatch{}, goGraphError("GPH_GO_DELTA_EMPTY", "affected_paths", "at least one affected root-relative path is required")
	}
	selected := make(map[string]struct{}, len(affectedPaths))
	for _, path := range affectedPaths {
		clean := filepath.Clean(filepath.FromSlash(path))
		if path == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return model.GraphObservationBatch{}, goGraphError("GPH_GO_DELTA_PATH_INVALID", "affected_paths", "affected paths must be non-empty root-relative paths")
		}
		selected[filepath.ToSlash(clean)] = struct{}{}
	}
	return observeGoGraph(ctx, req, selected, receipt)
}

// Applied means a selected file was actually parsed from the selected module's
// go list package. Excluded means go list explicitly reported IgnoredGoFiles.
// Missing never establishes prior ownership; Indeterminate grants no mask.
type GoGraphDeltaReceipt struct {
	Root, ProjectOrModule                     string
	ConfigFingerprint                         model.GraphDigest
	Applied, Excluded, Missing, Indeterminate []string
	states                                    map[string]string
}

func (r *GoGraphDeltaReceipt) classify(root, moduleRoot, moduleRootReal, modulePath string, affected map[string]struct{}, listed []goListPackage) {
	r.states = make(map[string]string, len(affected))
	for p := range affected {
		r.states[p] = "indeterminate"
		full := filepath.Join(root, filepath.FromSlash(p))
		if !goGraphInside(moduleRoot, full) {
			continue
		}
		if graphDeltaMissingSafe(root, p) {
			r.states[p] = "missing"
		}
	}
	for _, pkg := range listed {
		if pkg.Error != nil || pkg.Module == nil || pkg.Module.Path != modulePath || !goGraphResolvedInside(moduleRoot, moduleRootReal, pkg.Dir) || goGraphSkippedPath(pkg.Dir) {
			continue
		}
		for _, name := range pkg.IgnoredGoFiles {
			full := filepath.Join(pkg.Dir, name)
			rel, err := filepath.Rel(root, full)
			if err != nil {
				continue
			}
			p := filepath.ToSlash(rel)
			if _, ok := r.states[p]; !ok || !graphDeltaSafePath(root, p) {
				continue
			}
			info, err := os.Lstat(full)
			if err == nil && info.Mode().IsRegular() {
				r.states[p] = "excluded"
			}
		}
	}
	r.sync()
}

func (r *GoGraphDeltaReceipt) markParsed(root, full string) {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return
	}
	p := filepath.ToSlash(rel)
	if _, ok := r.states[p]; ok && graphDeltaSafePath(root, p) {
		r.states[p] = "applied"
	}
}

func (r *GoGraphDeltaReceipt) sync() {
	r.Applied, r.Excluded, r.Missing, r.Indeterminate = nil, nil, nil, nil
	for p, state := range r.states {
		switch state {
		case "applied":
			r.Applied = append(r.Applied, p)
		case "excluded":
			r.Excluded = append(r.Excluded, p)
		case "missing":
			r.Missing = append(r.Missing, p)
		default:
			r.Indeterminate = append(r.Indeterminate, p)
		}
	}
	sort.Strings(r.Applied)
	sort.Strings(r.Excluded)
	sort.Strings(r.Missing)
	sort.Strings(r.Indeterminate)
}

// Missing components are accepted only after every existing component has
// been Lstat'ed without following symlinks. Permission errors stay unknown.
func graphDeltaMissingSafe(root, path string) bool {
	current := root
	for _, part := range strings.Split(filepath.FromSlash(path), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return true
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return false
}

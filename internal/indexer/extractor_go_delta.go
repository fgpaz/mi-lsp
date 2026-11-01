package indexer

import (
	"context"
	"fmt"
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
	if err == nil && batch.ReadyForStaging() == nil {
		for _, node := range batch.Nodes {
			if node.Key.SymbolKind == "package" && strings.HasPrefix(node.Key.OwnerPath, "@package/") {
				receipt.PackageOwnerCertificates = append(receipt.PackageOwnerCertificates, GoGraphPackageCertificate{Owner: node.Key.OwnerPath, ImportPath: strings.TrimPrefix(node.Key.OwnerPath, "@package/"), Module: batch.ProjectOrModule, SourceDigest: node.SourceDigest, ScopeFingerprint: receipt.ScopeFingerprint, CompletenessDigest: receipt.List.ResultDigest, State: "applied"})
			}
		}
		if receipt.LocalScopeComplete && receipt.ScopeFingerprint != (model.GraphDigest{}) {
			listed := make(map[string]bool, len(receipt.ListedImportPaths))
			for _, importPath := range receipt.ListedImportPaths {
				listed[importPath] = true
			}
			for _, owner := range req.PreviousPackageOwners {
				importPath := strings.TrimPrefix(owner, "@package/")
				if listed[importPath] || !goGraphImportPathInside(receipt.ModulePath, importPath) || !goGraphCandidateScopeSafe(receipt.ModuleRoot, receipt.ModulePath, importPath) {
					continue
				}
				receipt.PackageOwnerCertificates = append(receipt.PackageOwnerCertificates, GoGraphPackageCertificate{Owner: owner, ImportPath: importPath, Module: batch.ProjectOrModule, ScopeFingerprint: receipt.ScopeFingerprint, CompletenessDigest: receipt.List.ResultDigest, State: "absence-confirmed"})
			}
		}
	} else {
		receipt.PackageOwnerCertificates = nil
	}
	receipt.sync()
	return batch, receipt, err
}

func observeGoGraphDeltaWithReceipt(ctx context.Context, req GoGraphObservationRequest, affectedPaths []string, receipt *GoGraphDeltaReceipt) (model.GraphObservationBatch, error) {
	if len(affectedPaths) == 0 {
		return model.GraphObservationBatch{}, goGraphError("GPH_GO_DELTA_EMPTY", "affected_paths", "at least one affected root-relative path is required")
	}
	selected := make(map[string]struct{}, len(affectedPaths))
	addPath := func(path string) error {
		clean := filepath.Clean(filepath.FromSlash(path))
		if path == "" || strings.ContainsRune(path, 0) || strings.Contains(path, "\\") || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return goGraphError("GPH_GO_DELTA_PATH_INVALID", "affected_paths", "affected paths must be non-empty root-relative paths")
		}
		selected[filepath.ToSlash(clean)] = struct{}{}
		return nil
	}
	for _, path := range affectedPaths {
		if err := addPath(path); err != nil {
			return model.GraphObservationBatch{}, err
		}
	}
	for _, path := range req.PreviousModulePaths {
		clean := filepath.Clean(filepath.FromSlash(path))
		if path == "" || strings.ContainsRune(path, 0) || strings.Contains(path, "\\") || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return model.GraphObservationBatch{}, goGraphError("GPH_GO_DELTA_PATH_INVALID", "previous_module_paths", "previous module paths must be non-empty root-relative paths")
		}
	}
	return observeGoGraph(ctx, req, selected, receipt)
}

// Applied means a selected file was actually parsed from the selected module's
// go list package. Excluded means go list explicitly reported IgnoredGoFiles.
// Missing never establishes prior ownership; Indeterminate grants no mask.
// GoGraphControlInput binds a consumed file or a confirmed absent optional input.
// Digest includes a presence tag, distinguishing absence from an empty file.
type GoGraphControlInput struct {
	Path     string
	Digest   model.GraphDigest
	Present  bool
	Consumed bool
	State    string
}

// GoGraphDeltaReceipt partitions selected and compiler-expanded root-relative
// source paths. ControlInputs are only go.mod/go.sum for this exact module;
// go.work and other files are never claimed as consumed by this backend.
// GoGraphPackageCertificate is either an applied package node or an absence proof
// bound to a complete go list invocation. Absence certificates carry no source digest.
type GoGraphPackageCertificate struct {
	Owner, ImportPath, Module string
	SourceDigest              model.GraphDigest
	ScopeFingerprint          model.GraphDigest
	CompletenessDigest        model.GraphDigest
	State                     string
}

type GoGraphCurrentPackage struct {
	ImportPath string
	Dir        string // Request-root-relative actual go list package directory.
}

type GoGraphListReceipt struct {
	Command, Dir, State string
	Args                []string
	ExitCode            int
	StdoutEOF           bool
	JSONValid           bool
	Complete            bool
	PackageCount        int
	ResultDigest        model.GraphDigest
	InvocationDigest    model.GraphDigest
}

type GoGraphDeltaReceipt struct {
	Root, ProjectOrModule, ModulePath         string
	ModuleRoot                                string
	ConfigFingerprint                         model.GraphDigest
	ScopeFingerprint                          model.GraphDigest
	LocalScopeComplete                        bool
	ListedImportPaths                         []string
	List                                      GoGraphListReceipt
	Applied, Excluded, Missing, Indeterminate []string
	ControlInputs                             []GoGraphControlInput
	PackageOwnerCertificates                  []GoGraphPackageCertificate
	CurrentPackages                           []GoGraphCurrentPackage
	ExpandedPaths                             []string
	ExpansionReason                           string
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

func (r *GoGraphDeltaReceipt) sealScope(root, moduleRoot, moduleRootReal, modulePath string, listed []goListPackage) {
	r.ModulePath, r.ModuleRoot = modulePath, moduleRoot
	r.ListedImportPaths = nil
	r.CurrentPackages = nil
	for _, pkg := range listed {
		if pkg.ImportPath != "" {
			r.ListedImportPaths = append(r.ListedImportPaths, pkg.ImportPath)
		}
	}
	sort.Strings(r.ListedImportPaths)
	r.ListedImportPaths = uniqueStrings(r.ListedImportPaths)
	r.LocalScopeComplete = r.List.Complete
	seen := map[string]bool{}
	for _, pkg := range listed {
		if pkg.ImportPath == "" || pkg.Dir == "" {
			r.LocalScopeComplete = false
			continue
		}
		abs, err := filepath.Abs(pkg.Dir)
		if err != nil {
			r.LocalScopeComplete = false
			continue
		}
		inside := goGraphInside(moduleRoot, abs)
		resolved, resolveErr := filepath.EvalSymlinks(abs)
		if goGraphImportPathInside(modulePath, pkg.ImportPath) && (pkg.Module == nil || pkg.Module.Path != modulePath || !inside) {
			r.LocalScopeComplete = false
		}
		if inside && (resolveErr != nil || !goGraphInside(moduleRootReal, resolved) || pkg.Module == nil || pkg.Module.Path != modulePath || pkg.Module.Dir == "" || pkg.Error != nil || !goGraphImportPathInside(modulePath, pkg.ImportPath)) {
			r.LocalScopeComplete = false
		}
		if pkg.Module != nil && pkg.Module.Path == modulePath {
			moduleDir, moduleDirErr := filepath.EvalSymlinks(pkg.Module.Dir)
			validPackage := inside && resolveErr == nil && goGraphInside(moduleRootReal, resolved) && moduleDirErr == nil && moduleDir == moduleRootReal && pkg.Error == nil && goGraphImportPathInside(modulePath, pkg.ImportPath) && !seen[pkg.ImportPath]
			if !validPackage {
				r.LocalScopeComplete = false
			} else {
				rel, relErr := filepath.Rel(root, abs)
				if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
					r.LocalScopeComplete = false
				} else {
					r.CurrentPackages = append(r.CurrentPackages, GoGraphCurrentPackage{ImportPath: pkg.ImportPath, Dir: filepath.ToSlash(rel)})
				}
			}
			seen[pkg.ImportPath] = true
		}
	}
	if r.LocalScopeComplete {
		parts := []string{root, r.ProjectOrModule, modulePath, r.ConfigFingerprint.String(), r.List.InvocationDigest.String(), r.List.ResultDigest.String(), fmt.Sprintf("packages=%d", r.List.PackageCount)}
		r.ScopeFingerprint = goGraphDigest([]byte(strings.Join(parts, "\x00")))
	}
}

func goGraphImportPathInside(modulePath, importPath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

func goGraphCandidateScopeSafe(moduleRoot, modulePath, importPath string) bool {
	if !goGraphImportPathInside(modulePath, importPath) {
		return false
	}
	suffix := strings.TrimPrefix(importPath, modulePath)
	suffix = strings.TrimPrefix(suffix, "/")
	if suffix == "" {
		return true
	}
	current := moduleRoot
	for _, part := range strings.Split(suffix, "/") {
		if part == "" || part == "." || part == ".." || strings.Contains(part, "!") || strings.ContainsAny(part, "\\:\x00") {
			return false
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return true
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false
		}
		_, moduleErr := os.Lstat(filepath.Join(current, "go.mod"))
		if moduleErr == nil || !os.IsNotExist(moduleErr) {
			return false
		}
	}
	return true
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

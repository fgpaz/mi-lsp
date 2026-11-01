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
type GoGraphControlCoverage struct {
	Module          string
	Inputs          []GoGraphControlInput // Workspace-relative paths.
	RawInputs       []GoGraphControlInput // Exact request-root receipt inputs.
	ExpandedPaths   []string              // Workspace-relative compiler-selected source paths.
	ExpansionReason string
}

type GraphDeltaCoverage struct {
	Paths             []GraphDeltaPathCoverage
	TopologyAmbiguous bool
	GoReceiptBridges  []GoGraphDeltaCanonicalBridge
	// GoReceipts contain compiler-derived applied/excluded/missing/unknown paths.
	GoReceipts []GoGraphDeltaReceipt
	// GoControlCertificates are projected only after the batch is ReadyForStaging.
	GoControlCertificates []GoGraphControlCoverage
	// GoPackageCertificates are package-node observations, not physical file coverage.
	GoPackageCertificates []GoGraphPackageCertificate
}

func (c *GraphDeltaCoverage) reset(paths []string) {
	*c = GraphDeltaCoverage{}
	for _, p := range paths {
		ext := strings.ToLower(filepath.Ext(p))
		c.Paths = append(c.Paths, GraphDeltaPathCoverage{WorkspacePath: filepath.ToSlash(filepath.Clean(p)), Config: ext != ".go" && ext != ".cs"})
	}
}

// GoGraphDeltaPathMapping is a receipt-bound raw-to-canonical path projection.
type GoGraphDeltaPathMapping struct {
	Field, Raw, Canonical string
}

// GoGraphDeltaCanonicalBridge retains both identities and the exact path
// projections used to rebase one native Go receipt into workspace coordinates.
type GoGraphDeltaCanonicalBridge struct {
	RawRoot, RawProjectOrModule, CanonicalRoot, CanonicalProjectOrModule, PhysicalProjectOrModule string
	WorkspaceIdentity, RepositoryIdentity                                                          string
	RawBatchDigest, CanonicalBatchDigest, RawSourceFingerprint, CanonicalSourceFingerprint, ConfigFingerprint model.GraphDigest
	ScopeFingerprint, CompletenessDigest                                                                      model.GraphDigest
	RawReceipt, CanonicalReceipt                                                                              GoGraphDeltaReceipt
	Domain                                                                                                    model.GraphWorktreeDomain
	PathMappings                                                                                              []GoGraphDeltaPathMapping
}

// CanonicalizeGoGraphDelta binds an exact raw request/receipt to the physical
// workspace root, rebases its batch and receipt, and reseals the same batch.
func CanonicalizeGoGraphDelta(workspaceRoot, requestRoot, module string, batch *model.GraphObservationBatch, receipt GoGraphDeltaReceipt) (GoGraphDeltaCanonicalBridge, error) {
	fail := func() (GoGraphDeltaCanonicalBridge, error) {
		return GoGraphDeltaCanonicalBridge{}, fmt.Errorf("explicit_incomplete: Go raw receipt cannot be canonically rebased")
	}
	if batch == nil || module == "" || receipt.Root == "" || receipt.ProjectOrModule != module || batch.ProjectOrModule != module || batch.Backend != "go" || batch.RepositoryIdentity == "" || receipt.ConfigFingerprint != batch.ConfigFingerprint {
		return fail()
	}
	bridge := GoGraphDeltaCanonicalBridge{RawRoot: receipt.Root, RawProjectOrModule: receipt.ProjectOrModule, RawBatchDigest: batch.Digest, RawSourceFingerprint: batch.SourceFingerprint, ConfigFingerprint: batch.ConfigFingerprint, ScopeFingerprint: receipt.ScopeFingerprint, CompletenessDigest: receipt.List.ResultDigest, RawReceipt: receipt}
	if receipt.ModuleRoot == "" || receipt.ModuleRoot != receipt.List.Dir {
		return fail()
	}
	if !receipt.LocalScopeComplete || receipt.ScopeFingerprint == (model.GraphDigest{}) || receipt.List.ResultDigest == (model.GraphDigest{}) || receipt.List.InvocationDigest == (model.GraphDigest{}) {
		return fail()
	}
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fail()
	}
	requestAbs, err := filepath.Abs(requestRoot)
	if err != nil {
		return fail()
	}
	expectedModuleRoot, err := filepath.Abs(filepath.Dir(filepath.Join(requestAbs, filepath.FromSlash(module))))
	if err != nil || filepath.Clean(receipt.ModuleRoot) != filepath.Clean(expectedModuleRoot) {
		return fail()
	}
	rawRoot, err := filepath.Abs(receipt.Root)
	if err != nil || filepath.Clean(rawRoot) != filepath.Clean(requestAbs) {
		return fail()
	}
	prefix, err := filepath.Rel(workspaceAbs, requestAbs)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) || filepath.IsAbs(prefix) {
		return fail()
	}
	if filepath.Clean(prefix) != "." {
		physicalPrefix := filepath.ToSlash(filepath.Clean(prefix))
		physicalModule := filepath.ToSlash(filepath.Join(filepath.FromSlash(physicalPrefix), filepath.FromSlash(module)))
		mappings, mapErr := goGraphDeltaPhysicalMappings(requestAbs, workspaceAbs, physicalPrefix, module, receipt)
		if mapErr != nil || batch.ReadyForStaging() != nil {
			return fail()
		}
		bridge.CanonicalRoot = workspaceAbs
		bridge.CanonicalProjectOrModule = module // Preserve the pinned raw NodeKey domain.
		bridge.PhysicalProjectOrModule = physicalModule
		bridge.WorkspaceIdentity = batch.WorkspaceIdentity
		bridge.RepositoryIdentity = batch.RepositoryIdentity
		bridge.CanonicalBatchDigest = batch.Digest
		bridge.CanonicalSourceFingerprint = batch.SourceFingerprint
		bridge.ConfigFingerprint = batch.ConfigFingerprint
		bridge.CanonicalReceipt = receipt
		bridge.Domain = graphWorktreeDomainSeed(physicalPrefix, physicalModule, module, batch, receipt)
		bridge.PathMappings = mappings
		return bridge, nil
	}
	moduleAbs := filepath.Join(requestAbs, filepath.FromSlash(module))
	moduleRel, err := filepath.Rel(workspaceAbs, moduleAbs)
	if err != nil || moduleRel == ".." || strings.HasPrefix(moduleRel, ".."+string(filepath.Separator)) || filepath.IsAbs(moduleRel) {
		return fail()
	}
	canonicalModule := filepath.ToSlash(filepath.Clean(moduleRel))
	canonicalizePath := func(field, raw string) (string, error) {
		if raw == "" || strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") || filepath.IsAbs(filepath.FromSlash(raw)) {
			return "", fmt.Errorf("invalid raw path")
		}
		clean := filepath.Clean(filepath.FromSlash(raw))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("invalid raw path")
		}
		abs := filepath.Join(requestAbs, clean)
		rel, relErr := filepath.Rel(workspaceAbs, abs)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", fmt.Errorf("path escapes workspace")
		}
		canonical := filepath.ToSlash(filepath.Clean(rel))
		bridge.PathMappings = append(bridge.PathMappings, GoGraphDeltaPathMapping{Field: field, Raw: filepath.ToSlash(clean), Canonical: canonical})
		return canonical, nil
	}
	canonicalReceipt := receipt
	canonicalReceipt.Applied = append([]string(nil), receipt.Applied...)
	canonicalReceipt.Excluded = append([]string(nil), receipt.Excluded...)
	canonicalReceipt.Missing = append([]string(nil), receipt.Missing...)
	canonicalReceipt.Indeterminate = append([]string(nil), receipt.Indeterminate...)
	canonicalReceipt.ExpandedPaths = append([]string(nil), receipt.ExpandedPaths...)
	canonicalReceipt.ControlInputs = append([]GoGraphControlInput(nil), receipt.ControlInputs...)
	canonicalReceipt.CurrentPackages = append([]GoGraphCurrentPackage(nil), receipt.CurrentPackages...)
	canonicalReceipt.PackageOwnerCertificates = append([]GoGraphPackageCertificate(nil), receipt.PackageOwnerCertificates...)
	canonicalReceipt.ListedImportPaths = append([]string(nil), receipt.ListedImportPaths...)
	canonicalReceipt.List.Args = append([]string(nil), receipt.List.Args...)
	canonicalReceipt.Root = workspaceAbs
	canonicalReceipt.ProjectOrModule = canonicalModule
	mapPaths := func(field string, paths []string) ([]string, error) {
		mapped := make([]string, 0, len(paths))
		for _, raw := range paths {
			canonical, mapErr := canonicalizePath(field, raw)
			if mapErr != nil {
				return nil, mapErr
			}
			mapped = append(mapped, canonical)
		}
		return mapped, nil
	}
	var mapErr error
	if canonicalReceipt.Applied, mapErr = mapPaths("applied", receipt.Applied); mapErr != nil {
		return fail()
	}
	if canonicalReceipt.Excluded, mapErr = mapPaths("excluded", receipt.Excluded); mapErr != nil {
		return fail()
	}
	if canonicalReceipt.Missing, mapErr = mapPaths("missing", receipt.Missing); mapErr != nil {
		return fail()
	}
	if canonicalReceipt.Indeterminate, mapErr = mapPaths("indeterminate", receipt.Indeterminate); mapErr != nil {
		return fail()
	}
	if canonicalReceipt.ExpandedPaths, mapErr = mapPaths("expanded", receipt.ExpandedPaths); mapErr != nil {
		return fail()
	}
	for i := range canonicalReceipt.ControlInputs {
		canonicalReceipt.ControlInputs[i].Path, mapErr = canonicalizePath("control", receipt.ControlInputs[i].Path)
		if mapErr != nil {
			return fail()
		}
	}
	for i := range canonicalReceipt.CurrentPackages {
		rawDir := receipt.CurrentPackages[i].Dir
		if rawDir == "." {
			canonicalReceipt.CurrentPackages[i].Dir = "."
			bridge.PathMappings = append(bridge.PathMappings, GoGraphDeltaPathMapping{Field: "package_dir", Raw: ".", Canonical: "."})
			continue
		}
		canonicalReceipt.CurrentPackages[i].Dir, mapErr = canonicalizePath("package_dir", rawDir)
		if mapErr != nil {
			return fail()
		}
	}
	for i := range canonicalReceipt.PackageOwnerCertificates {
		canonicalReceipt.PackageOwnerCertificates[i].Module = canonicalModule
	}
	if receipt.states != nil {
		canonicalReceipt.states = make(map[string]string, len(receipt.states))
		for raw, state := range receipt.states {
			canonical, mapErr := canonicalizePath("state", raw)
			if mapErr != nil {
				return fail()
			}
			canonicalReceipt.states[canonical] = state
		}
	}
	if err := model.SealGraphObservationBatch(batch); err != nil || batch.ProjectOrModule != canonicalModule || batch.ReadyForStaging() != nil {
		return fail()
	}
	bridge.CanonicalRoot = workspaceAbs
	bridge.CanonicalProjectOrModule = canonicalModule
	bridge.WorkspaceIdentity = batch.WorkspaceIdentity
	bridge.RepositoryIdentity = batch.RepositoryIdentity
	bridge.CanonicalSourceFingerprint = batch.SourceFingerprint
	bridge.ConfigFingerprint = batch.ConfigFingerprint
	bridge.CanonicalBatchDigest = batch.Digest
	bridge.CanonicalReceipt = canonicalReceipt
	bridge.PhysicalProjectOrModule = canonicalModule
	bridge.Domain = graphWorktreeDomainSeed(".", canonicalModule, module, batch, receipt)
	return bridge, nil
}

func graphWorktreeDomainSeed(physicalPrefix, physicalModule, module string, batch *model.GraphObservationBatch, receipt GoGraphDeltaReceipt) model.GraphWorktreeDomain {
	return model.GraphWorktreeDomain{
		WorkspaceIdentity: batch.WorkspaceIdentity, RepositoryIdentity: batch.RepositoryIdentity,
		Backend: "go", ProjectOrModule: module, PhysicalPrefix: physicalPrefix,
		PhysicalModule: physicalModule, ModulePath: receipt.ModulePath,
		SourceFingerprint: batch.SourceFingerprint, ConfigFingerprint: batch.ConfigFingerprint,
		BatchDigest: batch.Digest, ScopeFingerprint: receipt.ScopeFingerprint,
		ListInvocationDigest: receipt.List.InvocationDigest, ListResultDigest: receipt.List.ResultDigest,
	}
}

func goGraphDeltaPhysicalMappings(requestRoot, workspaceRoot, physicalPrefix, module string, receipt GoGraphDeltaReceipt) ([]GoGraphDeltaPathMapping, error) {
	var mappings []GoGraphDeltaPathMapping
	mapPath := func(field, raw string, allowDot bool) error {
		if raw == "." && allowDot {
			mappings = append(mappings, GoGraphDeltaPathMapping{Field: field, Raw: raw, Canonical: physicalPrefix})
			return nil
		}
		if raw == "" || strings.ContainsAny(raw, "\\\\\\x00") || strings.HasPrefix(raw, "/") || filepath.IsAbs(filepath.FromSlash(raw)) {
			return fmt.Errorf("invalid Go receipt path")
		}
		clean := filepath.Clean(filepath.FromSlash(raw))
		if filepath.ToSlash(clean) != raw || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("noncanonical Go receipt path")
		}
		abs := filepath.Join(requestRoot, clean)
		rel, err := filepath.Rel(workspaceRoot, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("Go receipt path escapes physical workspace")
		}
		mappings = append(mappings, GoGraphDeltaPathMapping{Field: field, Raw: raw, Canonical: filepath.ToSlash(filepath.Clean(rel))})
		return nil
	}
	if err := mapPath("module", module, false); err != nil { return nil, err }
	for _, group := range []struct{ field string; paths []string }{{"applied", receipt.Applied}, {"excluded", receipt.Excluded}, {"missing", receipt.Missing}, {"indeterminate", receipt.Indeterminate}, {"expanded", receipt.ExpandedPaths}} {
		for _, raw := range group.paths { if err := mapPath(group.field, raw, false); err != nil { return nil, err } }
	}
	for _, input := range receipt.ControlInputs { if err := mapPath("control", input.Path, false); err != nil { return nil, err } }
	for _, pkg := range receipt.CurrentPackages { if err := mapPath("package_dir", pkg.Dir, true); err != nil { return nil, err } }
	for raw := range receipt.states { if err := mapPath("state", raw, false); err != nil { return nil, err } }
	sort.Slice(mappings, func(i, j int) bool {
		a, b := mappings[i], mappings[j]
		if a.Field != b.Field { return a.Field < b.Field }
		if a.Raw != b.Raw { return a.Raw < b.Raw }
		return a.Canonical < b.Canonical
	})
	return mappings, nil
}

func graphGoControlCoverage(workspaceRoot, requestRoot, module string, receipt GoGraphDeltaReceipt) (GoGraphControlCoverage, error) {
	projected := GoGraphControlCoverage{ExpansionReason: receipt.ExpansionReason}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return projected, err
	}
	request, err := filepath.Abs(requestRoot)
	if err != nil {
		return projected, err
	}
	moduleAbs := filepath.Join(request, filepath.FromSlash(module))
	projected.Module, err = filepath.Rel(root, moduleAbs)
	if err != nil || projected.Module == ".." || strings.HasPrefix(projected.Module, ".."+string(filepath.Separator)) {
		return projected, fmt.Errorf("explicit_incomplete: Go module is outside workspace root")
	}
	projected.Module = filepath.ToSlash(projected.Module)
	for _, input := range receipt.ControlInputs {
		projected.RawInputs = append(projected.RawInputs, input)
		abs := filepath.Join(request, filepath.FromSlash(input.Path))
		rel, relErr := filepath.Rel(root, abs)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return projected, fmt.Errorf("explicit_incomplete: Go control input is outside workspace root")
		}
		input.Path = filepath.ToSlash(rel)
		projected.Inputs = append(projected.Inputs, input)
	}
	for _, path := range receipt.ExpandedPaths {
		abs := filepath.Join(request, filepath.FromSlash(path))
		rel, relErr := filepath.Rel(root, abs)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return projected, fmt.Errorf("explicit_incomplete: expanded Go source is outside workspace root")
		}
		projected.ExpandedPaths = append(projected.ExpandedPaths, filepath.ToSlash(rel))
	}
	return projected, nil
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
		workspacePath := filepath.ToSlash(rel)
		found := false
		for i := range c.Paths {
			if c.Paths[i].WorkspacePath == workspacePath {
				c.Paths[i].Owners = append(c.Paths[i].Owners, GraphDeltaOwner{Backend: backend, RequestRoot: requestRoot, ProjectOrModule: module, RequestPath: p})
				found = true
			}
		}
		if !found {
			c.Paths = append(c.Paths, GraphDeltaPathCoverage{WorkspacePath: workspacePath, Owners: []GraphDeltaOwner{{Backend: backend, RequestRoot: requestRoot, ProjectOrModule: module, RequestPath: p}}, Config: !strings.EqualFold(filepath.Ext(workspacePath), ".go") && !strings.EqualFold(filepath.Ext(workspacePath), ".cs")})
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

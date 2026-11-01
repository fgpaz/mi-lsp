package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// LayeredGraphSnapshot pins an immutable base graph generation and retains
// only the worktree's typed observations and path tombstones. It never copies
// base graph records or writes to the base database. The base snapshot is
// owned by this value and is closed with it.
type LayeredGraphSnapshot struct {
	base            *GraphQuerySnapshot
	generation      model.GraphGeneration
	fingerprint     string
	affected        []string
	affectedSet     map[string]struct{}
	batches         []model.GraphObservationBatch
	worktreeDomains []model.GraphWorktreeDomain
	groups          []graphOverlayGroup
	derivedSources  map[model.GraphDigest][]model.GraphDigest
	closed          bool
}

// NewLayeredGraphSnapshot validates and layers complete typed observations on
// a caller-pinned read-only base generation. affectedPaths are repository-
// relative source paths, including deleted paths. Edges are owned by those
// paths; nodes outside the mask are reference anchors, not replacements.
// Effective reads verify every such anchor against the pinned base identity
// and source digest. References must be closed by typed nodes in the batches.
func NewLayeredGraphSnapshot(base *GraphQuerySnapshot, affectedPaths []string, batches []model.GraphObservationBatch, branchFingerprint string) (*LayeredGraphSnapshot, error) {
	return NewLayeredGraphSnapshotWithWorktreeDomains(base, affectedPaths, batches, branchFingerprint, nil)
}

// NewLayeredGraphSnapshotWithWorktreeDomains binds Go legacy key domains to
// one pinned base without changing canonical NodeKeys.
func NewLayeredGraphSnapshotWithWorktreeDomains(base *GraphQuerySnapshot, affectedPaths []string, batches []model.GraphObservationBatch, branchFingerprint string, domains []model.GraphWorktreeDomain) (*LayeredGraphSnapshot, error) {
	if base == nil || base.tx == nil || strings.TrimSpace(branchFingerprint) == "" {
		return nil, model.ErrGraphGenerationInvalid
	}
	generation, err := base.Generation()
	if err != nil {
		return nil, err
	}
	paths, pathSet, err := normalizeGraphOverlayPaths(affectedPaths)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 && (len(batches) != 0) {
		return nil, fmt.Errorf("graph overlay batches require affected source paths")
	}

	batchCopy := make([]model.GraphObservationBatch, len(batches))
	workspaceIdentity, repositoryIdentity := "", ""
	seenNodes := map[string]string{}
	seenEdges := map[string]string{}
	knownRefs := map[string]struct{}{}
	for i := range batches {
		var batch model.GraphObservationBatch
		encoded, marshalErr := json.Marshal(batches[i])
		if marshalErr != nil {
			return nil, fmt.Errorf("encode graph overlay batch %d: %w", i, marshalErr)
		}
		if unmarshalErr := json.Unmarshal(encoded, &batch); unmarshalErr != nil {
			return nil, fmt.Errorf("copy graph overlay batch %d: %w", i, unmarshalErr)
		}
		if err := batch.ReadyForStaging(); err != nil {
			return nil, fmt.Errorf("graph overlay batch %d is not stageable: %w", i, err)
		}
		if batch.WorkspaceIdentity == "" || batch.WorkspaceIdentity != generation.WorkspaceIdentity || batch.RepositoryIdentity == "" || batch.RepositoryIdentity != generation.RepositoryIdentity {
			return nil, fmt.Errorf("graph overlay batch %d identity does not match pinned base generation", i)
		}
		if workspaceIdentity == "" {
			workspaceIdentity, repositoryIdentity = batch.WorkspaceIdentity, batch.RepositoryIdentity
		} else if workspaceIdentity != batch.WorkspaceIdentity || repositoryIdentity != batch.RepositoryIdentity {
			return nil, fmt.Errorf("graph overlay batches mix workspace or repository generations")
		}
		for _, node := range batch.Nodes {
			owner := node.Key.OwnerPath
			if _, _, err := normalizeGraphOverlayPaths([]string{owner}); err != nil {
				return nil, err
			}
			key, keyErr := model.NewNodeKey(node.Key)
			if keyErr != nil {
				return nil, keyErr
			}
			digest, keyErr := key.Hash()
			if keyErr != nil {
				return nil, keyErr
			}
			fingerprint := fmt.Sprintf("%s:%s:%s", node.DisplayName, node.ClaimStatus, node.SourceDigest)
			if previous, found := seenNodes[digest.String()]; found && previous != fingerprint {
				return nil, fmt.Errorf("graph overlay stable node id collision: %s", digest)
			}
			seenNodes[digest.String()] = fingerprint
			knownRefs[graphOverlayRef(i, node.Ref)] = struct{}{}
		}
		for _, edge := range batch.Edges {
			if _, ok := pathSet[edge.OwnerPath]; !ok && !(batch.Backend == "go" && strings.HasPrefix(edge.OwnerPath, "@package/")) {
				return nil, fmt.Errorf("graph overlay edge path %q is outside the affected-path set", edge.OwnerPath)
			}
			edgeKey := graphOverlayRef(i, strings.Join([]string{edge.FromRef, edge.ToRef, edge.Relation, edge.Scope}, "\x00"))
			value := strings.Join([]string{edge.OwnerPath, edge.Status, edge.Backend, edge.SourceDigest.String()}, "\x00")
			if previous, found := seenEdges[edgeKey]; found && previous != value {
				return nil, fmt.Errorf("graph overlay stable edge id collision")
			}
			seenEdges[edgeKey] = value
		}
		batchCopy[i] = batch
	}
	// Resolve batch-local refs without conflating identically named refs in
	// different projects or modules.
	for i := range batchCopy {
		batch := &batchCopy[i]
		for _, edge := range batch.Edges {
			if _, ok := knownRefs[graphOverlayRef(i, edge.FromRef)]; !ok {
				return nil, fmt.Errorf("graph overlay edge has unresolved from reference %q", edge.FromRef)
			}
			if _, ok := knownRefs[graphOverlayRef(i, edge.ToRef)]; !ok {
				return nil, fmt.Errorf("graph overlay edge has unresolved to reference %q", edge.ToRef)
			}
		}
	}
	for i := range batchCopy {
		if err := batchCopy[i].ReadyForStaging(); err != nil {
			return nil, fmt.Errorf("graph overlay batch %d changed during validation: %w", i, err)
		}
	}
	domainCopy, err := validateGraphWorktreeDomains(context.Background(), base, generation, batchCopy, pathSet, domains)
	if err != nil {
		return nil, err
	}
	for _, batch := range batchCopy {
		for _, edge := range batch.Edges {
			if _, physical := pathSet[edge.OwnerPath]; physical {
				continue
			}
			if batch.Backend != "go" || !graphWorktreePackageMaskCovers(domainCopy, batch.ProjectOrModule, edge.OwnerPath) {
				return nil, fmt.Errorf("graph overlay semantic owner is outside a pinned Go package mask")
			}
		}
	}
	batchDigests := make([]string, len(batchCopy))
	for i, batch := range batchCopy {
		batchDigests[i] = batch.Digest.String()
	}
	sort.Strings(batchDigests)
	identity, err := json.Marshal(struct {
		Base    string
		Branch  string
		Paths   []string
		Batches []string
		Domains []model.GraphWorktreeDomain
	}{generation.GenerationID.String(), branchFingerprint, paths, batchDigests, domainCopy})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append([]byte("milsp-layered-graph/v1\x00"), identity...))
	generation.GenerationID = model.GraphDigest(digest)
	generation.SourceFingerprint = sha256.Sum256(append([]byte("milsp-layered-source/v1\x00"), identity...))
	// This identity describes a layered view, not a canonical published
	// generation. Do not inherit base content digests or counters as if they
	// validated the effective graph; canonical validation remains on the base.
	generation.Status = "overlay"
	generation.ContentDigest = model.GraphDigest{}
	generation.BackendManifestDigest = model.GraphDigest{}
	generation.NodeCount = 0
	generation.EdgeCount = 0
	generation.EvidenceCount = 0
	generation.UnresolvedCount = 0
	generation.PreviousGenerationID = nil
	generation.CreatedAt = generation.CreatedAt.UTC()
	groups := make([]graphOverlayGroup, len(batchCopy))
	for i, batch := range batchCopy {
		groups[i] = graphOverlayNativeGroup(i, batch)
	}
	return &LayeredGraphSnapshot{base: base, generation: generation, fingerprint: branchFingerprint, affected: paths, affectedSet: pathSet, batches: batchCopy, worktreeDomains: domainCopy, groups: groups}, nil
}

func graphWorktreePackageMaskCovers(domains []model.GraphWorktreeDomain, module, owner string) bool {
	for _, domain := range domains {
		if domain.ProjectOrModule != module {
			continue
		}
		for _, mask := range domain.OwnerMasks {
			if mask.Kind == "package" && mask.OwnerPath == owner {
				return true
			}
		}
	}
	return false
}

func validateGraphWorktreeDomains(ctx context.Context, base *GraphQuerySnapshot, generation model.GraphGeneration, batches []model.GraphObservationBatch, pathSet map[string]struct{}, domains []model.GraphWorktreeDomain) ([]model.GraphWorktreeDomain, error) {
	seen := map[string]bool{}
	rawModuleRoots := map[string]string{}
	out := make([]model.GraphWorktreeDomain, 0, len(domains))
	for _, input := range domains {
		domain := input
		domain.PinnedPackageNodeKeys = append([]model.GraphDigest(nil), input.PinnedPackageNodeKeys...)
		domain.OwnerMasks = append([]model.GraphWorktreeOwnerMask(nil), input.OwnerMasks...)
		if err := domain.Validate(); err != nil || domain.PinGenerationID != generation.GenerationID || domain.WorkspaceIdentity != generation.WorkspaceIdentity || domain.RepositoryIdentity != generation.RepositoryIdentity {
			return nil, fmt.Errorf("explicit_incomplete: Go worktree domain is not bound to the pinned generation")
		}
		key := domain.ProjectOrModule + "\x00" + domain.PhysicalPrefix + "\x00" + domain.PhysicalModule
		if seen[key] {
			return nil, fmt.Errorf("explicit_incomplete: duplicate Go worktree domain")
		}
		if priorRoot, exists := rawModuleRoots[domain.ProjectOrModule]; exists && priorRoot != domain.PhysicalPrefix {
			return nil, fmt.Errorf("explicit_incomplete: pinned Go module key is ambiguous across request roots")
		}
		rawModuleRoots[domain.ProjectOrModule] = domain.PhysicalPrefix
		seen[key] = true
		var batch *model.GraphObservationBatch
		matches := 0
		for i := range batches {
			candidate := &batches[i]
			if candidate.Backend == "go" && candidate.ProjectOrModule == domain.ProjectOrModule && candidate.Digest == domain.BatchDigest && candidate.SourceFingerprint == domain.SourceFingerprint && candidate.ConfigFingerprint == domain.ConfigFingerprint && candidate.WorkspaceIdentity == domain.WorkspaceIdentity && candidate.RepositoryIdentity == domain.RepositoryIdentity {
				batch = candidate
				matches++
			}
		}
		if matches != 1 || batch == nil {
			return nil, fmt.Errorf("explicit_incomplete: Go domain is not bound to one native batch")
		}
		pinnedKeys, err := base.WorktreeGoPackageNodeKeysInDomain(ctx, domain.RepositoryIdentity, domain.ProjectOrModule)
		if err != nil {
			return nil, fmt.Errorf("explicit_incomplete: pinned Go package keys are unavailable")
		}
		pinnedSet := map[model.GraphDigest]bool{}
		for _, digest := range pinnedKeys {
			pinnedSet[digest] = true
		}
		for _, digest := range domain.PinnedPackageNodeKeys {
			if !pinnedSet[digest] {
				return nil, fmt.Errorf("explicit_incomplete: Go pinned package witness is not in the pinned key domain")
			}
		}
		for _, mask := range domain.OwnerMasks {
			if mask.Kind == "source" {
				if _, ok := pathSet[mask.WorkspacePath]; !ok {
					return nil, fmt.Errorf("Go source mask is outside physical overlay paths")
				}
				declared, err := base.WorktreeDeclaredCodeOwners(ctx, mask.OwnerPath)
				if err != nil {
					return nil, fmt.Errorf("explicit_incomplete: pinned Go source owner contexts are unavailable")
				}
				for _, owner := range declared {
					if owner.Backend == "go" && owner.ProjectOrModule != domain.ProjectOrModule {
						return nil, fmt.Errorf("explicit_incomplete: pinned Go source owner is ambiguous across modules")
					}
				}
				continue
			}
			contexts, err := base.WorktreeGoPackageOwnerContexts(ctx, []string{mask.OwnerPath})
			if err != nil {
				return nil, fmt.Errorf("explicit_incomplete: pinned Go package owner contexts are unavailable")
			}
			for _, owner := range contexts {
				if owner.ProjectOrModule != domain.ProjectOrModule {
					return nil, fmt.Errorf("explicit_incomplete: pinned Go package owner is ambiguous across modules")
				}
			}
			importPath := strings.TrimPrefix(mask.OwnerPath, "@package/")
			nodeKey, err := model.NewNodeKey(model.NodeKey{RepositoryIdentity: domain.RepositoryIdentity, BackendType: "go", Language: "go", ProjectOrModule: domain.ProjectOrModule, OwnerPath: mask.OwnerPath, SymbolKind: "package", SemanticIdentity: "pkg:" + importPath})
			if err != nil {
				return nil, fmt.Errorf("invalid Go package mask identity")
			}
			digest, err := nodeKey.Hash()
			if err != nil || !pinnedSet[digest] {
				return nil, fmt.Errorf("explicit_incomplete: Go package mask is not in the pinned key domain")
			}
		}
		out = append(out, domain)
	}
	return out, nil
}

// Observation refs are local to a batch, not globally unique across projects.
func graphOverlayRef(batch int, ref string) string {
	return fmt.Sprintf("%d\x00%s", batch, ref)
}

func normalizeGraphOverlayPaths(inputs []string) ([]string, map[string]struct{}, error) {
	set := make(map[string]struct{}, len(inputs))
	for _, raw := range inputs {
		if raw == "" || strings.ContainsAny(raw, "\\\x00") || strings.HasPrefix(raw, "/") || path.IsAbs(raw) || len(raw) >= 2 && raw[1] == ':' {
			return nil, nil, fmt.Errorf("unsafe graph overlay source path %q", raw)
		}
		clean := path.Clean(raw)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != raw {
			return nil, nil, fmt.Errorf("non-normalized graph overlay source path %q", raw)
		}
		for _, segment := range strings.Split(clean, "/") {
			if segment == ".git" || segment == ".mi-lsp" {
				return nil, nil, fmt.Errorf("reserved graph overlay source path %q", raw)
			}
		}
		set[clean] = struct{}{}
	}
	paths := make([]string, 0, len(set))
	for item := range set {
		paths = append(paths, item)
	}
	sort.Strings(paths)
	return paths, set, nil
}

// Generation returns the branch-scoped effective generation identity. Its
// content remains represented as base reference + delta, not materialized rows.
func (s *LayeredGraphSnapshot) Generation() (model.GraphGeneration, error) {
	if s == nil || s.closed {
		return model.GraphGeneration{}, model.ErrGraphGenerationInvalid
	}
	return s.generation, nil
}

// Base exposes the pinned base query snapshot for lazy reads. Callers must not
// close it independently; LayeredGraphSnapshot.Close owns its lifetime.
func (s *LayeredGraphSnapshot) Base() (*GraphQuerySnapshot, error) {
	if s == nil || s.closed || s.base == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	return s.base, nil
}

// AffectedPaths returns the sorted physical-delta path mask, including paths
// with no replacement batch (tombstones).
func (s *LayeredGraphSnapshot) AffectedPaths() ([]string, error) {
	if s == nil || s.closed {
		return nil, model.ErrGraphGenerationInvalid
	}
	return append([]string(nil), s.affected...), nil
}

// MasksPath reports whether a base component owned by path is shadowed by this
// overlay. It does not match parent/child paths: callers must use the exact
// canonical source path recorded by the component.
func (s *LayeredGraphSnapshot) MasksPath(sourcePath string) bool {
	if s == nil || s.closed {
		return false
	}
	_, ok := s.affectedSet[sourcePath]
	return ok
}

// MasksNode uses backend, repository, and raw module identity before applying
// a worktree mask; a physical path alone cannot identify a legacy Go owner.
func (s *LayeredGraphSnapshot) MasksNode(key model.NodeKeyFields) bool {
	if s == nil || s.closed {
		return false
	}
	if key.BackendType != "go" {
		return s.MasksPath(key.OwnerPath)
	}
	for _, domain := range s.worktreeDomains {
		if domain.RepositoryIdentity == key.RepositoryIdentity && domain.ProjectOrModule == key.ProjectOrModule && domain.MasksOwner(key.OwnerPath) {
			return true
		}
	}
	return false
}

// MasksSemanticOwner is for legacy records whose storage shape omits the
// module. Ambiguous raw owner coordinates are deliberately not masked.
func (s *LayeredGraphSnapshot) MasksSemanticOwner(backend, owner string) bool {
	if s == nil || s.closed {
		return false
	}
	if backend != "go" {
		return s.MasksPath(owner)
	}
	matches := 0
	for _, domain := range s.worktreeDomains {
		if domain.MasksOwner(owner) {
			matches++
		}
	}
	return matches == 1
}

func (s *LayeredGraphSnapshot) MasksSemanticOwnerInDomain(backend, module, owner string) bool {
	if s == nil || s.closed {
		return false
	}
	if backend != "go" {
		return s.MasksPath(owner)
	}
	for _, domain := range s.worktreeDomains {
		if domain.ProjectOrModule == module && domain.MasksOwner(owner) {
			return true
		}
	}
	return false
}

// ProjectGoOwner maps a native Go key to its physical worktree source path.
func (s *LayeredGraphSnapshot) ProjectGoOwner(key model.NodeKeyFields) (string, bool) {
	if s == nil || s.closed || key.BackendType != "go" {
		return key.OwnerPath, false
	}
	var projected string
	matches := 0
	for _, domain := range s.worktreeDomains {
		if domain.RepositoryIdentity != key.RepositoryIdentity || domain.ProjectOrModule != key.ProjectOrModule {
			continue
		}
		value, ok := domain.ProjectOwner(key.OwnerPath)
		if ok {
			projected = value
			matches++
		}
	}
	return projected, matches == 1
}

// MasksGraphEdge resolves the source node identity for a Go edge before
// deciding whether its legacy owner path is shadowed.
func (s *LayeredGraphSnapshot) MasksGraphEdge(ctx context.Context, edge model.GraphEdgeRecord) bool {
	if edge.SourceBackend != "go" {
		return s.MasksPath(edge.OwnerPath)
	}
	if s == nil || s.closed || ctx == nil {
		return false
	}
	node, err := s.base.Node(ctx, edge.FromNodeID)
	if err != nil {
		return s.MasksSemanticOwner("go", edge.OwnerPath)
	}
	return s.MasksNode(node.Identity)
}

// DeltaBatches returns defensive copies of the typed worktree observations.
func (s *LayeredGraphSnapshot) DeltaBatches() ([]model.GraphObservationBatch, error) {
	if s == nil || s.closed {
		return nil, model.ErrGraphGenerationInvalid
	}
	out := make([]model.GraphObservationBatch, len(s.batches))
	for i := range s.batches {
		encoded, err := json.Marshal(s.batches[i])
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(encoded, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Close releases the pinned base read transaction. It is idempotent.
func (s *LayeredGraphSnapshot) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.base != nil {
		return s.base.Close()
	}
	return nil
}

// Fingerprint returns the caller-supplied, branch-local physical snapshot id.
func (s *LayeredGraphSnapshot) Fingerprint() string {
	if s == nil || s.closed {
		return ""
	}
	return s.fingerprint
}

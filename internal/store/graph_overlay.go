package store

import (
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
	base        *GraphQuerySnapshot
	generation  model.GraphGeneration
	fingerprint string
	affected    []string
	affectedSet map[string]struct{}
	batches     []model.GraphObservationBatch
	closed      bool
}

// NewLayeredGraphSnapshot validates and layers complete typed observations on
// a caller-pinned read-only base generation. affectedPaths are repository-
// relative source paths, including deleted paths. Edges are owned by those
// paths; nodes outside the mask are reference anchors, not replacements.
// Effective reads verify every such anchor against the pinned base identity
// and source digest. References must be closed by typed nodes in the batches.
func NewLayeredGraphSnapshot(base *GraphQuerySnapshot, affectedPaths []string, batches []model.GraphObservationBatch, branchFingerprint string) (*LayeredGraphSnapshot, error) {
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
			if _, ok := pathSet[edge.OwnerPath]; !ok {
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
	}{generation.GenerationID.String(), branchFingerprint, paths, batchDigests})
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
	return &LayeredGraphSnapshot{base: base, generation: generation, fingerprint: branchFingerprint, affected: paths, affectedSet: pathSet, batches: batchCopy}, nil
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

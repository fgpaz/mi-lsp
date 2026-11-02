package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// graphOverlayGroup is the private read-layer IR. It is deliberately not a
// GraphObservationBatch: only a native batch that passed ReadyForStaging may
// be converted into one, and this representation cannot be staged or published.
type graphOverlayGroup struct {
	Index                                                      int
	SchemaVersion                                              int
	Completeness                                               string
	Capabilities                                               []model.GraphObservationCapability
	Coverage                                                   []model.GraphObservationCoverage
	ResourceStats                                              *model.GraphObservationResourceStats
	WorkspaceIdentity, RepositoryIdentity                      string
	Backend, BackendVersion, ExtractorVersion, ProjectOrModule string
	SourceFingerprint, ConfigFingerprint, Digest               model.GraphDigest
	Native                                                     bool
	Nodes                                                      []model.GraphObservationNode
	Edges                                                      []model.GraphObservationEdge
	Evidence                                                   []model.GraphObservationEvidence
	Unresolved                                                 []graphOverlayUnresolvedRow
	Omissions                                                  []model.GraphObservationOmission
}

// Keep the full canonical record alongside the observation-facing fields.
// In particular GraphUnresolved has document diagnostics that the native
// GraphObservationUnresolved contract intentionally does not carry.
type graphOverlayUnresolvedRow struct {
	Observation model.GraphObservationUnresolved
	Record      model.GraphUnresolved
}

func graphOverlayNativeGroup(index int, batch model.GraphObservationBatch) graphOverlayGroup {
	g := graphOverlayGroup{
		Index: index, SchemaVersion: batch.SchemaVersion, Completeness: batch.Completeness,
		Capabilities:      append([]model.GraphObservationCapability(nil), batch.Capabilities...),
		Coverage:          append([]model.GraphObservationCoverage(nil), batch.Coverage...),
		WorkspaceIdentity: batch.WorkspaceIdentity, RepositoryIdentity: batch.RepositoryIdentity,
		Backend: batch.Backend, BackendVersion: batch.BackendVersion, ExtractorVersion: batch.ExtractorVersion,
		ProjectOrModule: batch.ProjectOrModule, SourceFingerprint: batch.SourceFingerprint,
		ConfigFingerprint: batch.ConfigFingerprint, Digest: batch.Digest, Native: true,
		Nodes:     append([]model.GraphObservationNode(nil), batch.Nodes...),
		Edges:     append([]model.GraphObservationEdge(nil), batch.Edges...),
		Evidence:  append([]model.GraphObservationEvidence(nil), batch.Evidence...),
		Omissions: append([]model.GraphObservationOmission(nil), batch.Omissions...),
	}
	if batch.ResourceStats != nil {
		stats := *batch.ResourceStats
		g.ResourceStats = &stats
	}
	for _, u := range batch.Unresolved {
		var source *model.GraphDigest
		if u.SourceDigest != nil {
			d := *u.SourceDigest
			source = &d
		}
		obs := u
		obs.Candidates = append([]string(nil), u.Candidates...)
		record := model.GraphUnresolved{OwnerPath: u.OwnerPath, SubjectKind: u.SubjectKind, SelectorDigest: u.SelectorDigest,
			ReasonCode: u.ReasonCode, Candidates: append([]string(nil), u.Candidates...), Backend: u.Backend,
			SourceDigest: source, RecoveryHintCode: u.RecoveryHintCode}
		g.Unresolved = append(g.Unresolved, graphOverlayUnresolvedRow{Observation: obs, Record: record})
	}
	return g
}

// GraphDocumentDelta is store-owned input IR. It intentionally uses common
// model records and does not depend on indexer, avoiding the store/indexer cycle.
type GraphDocumentDelta struct {
	Bundle              model.GraphBundle
	AffectedOwnerPaths  []string
	ContextDocumentKeys []model.GraphDigest
	ContextNodeKeys     []model.GraphDigest
	DerivedNodeSources  map[model.GraphDigest][]model.GraphDigest
}

type graphOverlayDerivedSourcePair struct {
	Key     model.GraphDigest
	Sources []model.GraphDigest
}

// NewLayeredGraphSnapshotWithDocuments pins one base and combines already
// validated native observations with a private, non-publishable document IR.
// No compiler batch is synthesized for documents.
func NewLayeredGraphSnapshotWithDocuments(base *GraphQuerySnapshot, nativeAffectedPaths []string, nativeBatches []model.GraphObservationBatch, branchFingerprint string, input GraphDocumentDelta) (*LayeredGraphSnapshot, error) {
	return NewLayeredGraphSnapshotWithDocumentsAndWorktreeDomains(base, nativeAffectedPaths, nativeBatches, branchFingerprint, input, nil)
}

func NewLayeredGraphSnapshotWithDocumentsAndWorktreeDomains(base *GraphQuerySnapshot, nativeAffectedPaths []string, nativeBatches []model.GraphObservationBatch, branchFingerprint string, input GraphDocumentDelta, domains []model.GraphWorktreeDomain) (*LayeredGraphSnapshot, error) {
	docPaths, _, err := normalizeGraphOverlayPaths(input.AffectedOwnerPaths)
	if err == nil && len(docPaths) != len(input.AffectedOwnerPaths) {
		err = fmt.Errorf("duplicate document affected owner path")
	}
	if err != nil {
		return nil, fmt.Errorf("document delta paths: %w", err)
	}
	if len(docPaths) == 0 {
		return nil, fmt.Errorf("document delta requires affected owner paths")
	}
	allPaths := append([]string(nil), nativeAffectedPaths...)
	allPaths = append(allPaths, docPaths...)
	allPaths, _, err = normalizeGraphOverlayPaths(allPaths)
	if err != nil {
		return nil, err
	}
	snapshot, err := NewLayeredGraphSnapshotWithWorktreeDomains(base, allPaths, nativeBatches, branchFingerprint, domains)
	if err != nil {
		return nil, err
	}
	group, derived, identity, err := snapshot.prepareDocumentGroup(input, docPaths)
	if err != nil {
		_ = snapshot.Close()
		return nil, err
	}
	snapshot.groups = append(snapshot.groups, group)
	snapshot.derivedSources = derived
	genIdentity, err := json.Marshal(struct {
		Base, Native, Fingerprint string
		Input                     json.RawMessage
	}{snapshot.base.generation.GenerationID.String(), snapshot.generation.GenerationID.String(), snapshot.fingerprint, identity})
	if err != nil {
		_ = snapshot.Close()
		return nil, err
	}
	digest := sha256.Sum256(append([]byte("milsp-layered-graph-doc/v1\x00"), genIdentity...))
	snapshot.generation.GenerationID = model.GraphDigest(digest)
	source := sha256.Sum256(append([]byte("milsp-layered-source-doc/v1\x00"), genIdentity...))
	snapshot.generation.SourceFingerprint = model.GraphDigest(source)
	snapshot.generation.ContentDigest = model.GraphDigest{}
	snapshot.generation.ConfigFingerprint = model.GraphDigest{}
	snapshot.generation.BackendManifestDigest = model.GraphDigest{}
	snapshot.generation.NodeCount, snapshot.generation.EdgeCount = 0, 0
	snapshot.generation.EvidenceCount, snapshot.generation.UnresolvedCount = 0, 0
	snapshot.generation.Status = "overlay"
	for i := range group.Unresolved {
		group.Unresolved[i].Record.GenerationID = snapshot.generation.GenerationID
		snapshot.groups[len(snapshot.groups)-1].Unresolved[i].Record.GenerationID = snapshot.generation.GenerationID
	}
	return snapshot, nil
}

func (s *LayeredGraphSnapshot) prepareDocumentGroup(input GraphDocumentDelta, docPaths []string) (graphOverlayGroup, map[model.GraphDigest][]model.GraphDigest, json.RawMessage, error) {
	var zero model.GraphDigest
	if input.Bundle.Generation != (model.GraphGeneration{}) {
		return graphOverlayGroup{}, nil, nil, fmt.Errorf("document delta must be an unsealed private bundle")
	}
	pathSet := map[string]bool{}
	for _, p := range docPaths {
		if !graphOverlayCanonicalDocumentPath(p) {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("affected owner is not a canonical document path: %s", p)
		}
		pathSet[p] = true
	}
	contextDocs, err := graphOverlayDigestSet(input.ContextDocumentKeys)
	if err != nil {
		return graphOverlayGroup{}, nil, nil, err
	}
	contextNodes, err := graphOverlayDigestSet(input.ContextNodeKeys)
	if err != nil {
		return graphOverlayGroup{}, nil, nil, err
	}
	derivedSources := map[model.GraphDigest][]model.GraphDigest{}
	for key, sources := range input.DerivedNodeSources {
		if key == zero || len(sources) == 0 {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("derived node requires key and source anchors")
		}
		copySources := append([]model.GraphDigest(nil), sources...)
		sort.Slice(copySources, func(i, j int) bool { return copySources[i].String() < copySources[j].String() })
		for i, source := range copySources {
			if source == zero || i > 0 && source == copySources[i-1] {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid or duplicate derived source")
			}
		}
		derivedSources[key] = copySources
	}
	if len(input.Bundle.Nodes) == 0 && (len(input.Bundle.Edges)+len(input.Bundle.Evidence)+len(input.Bundle.Unresolved) > 0) {
		return graphOverlayGroup{}, nil, nil, fmt.Errorf("document bundle rows require nodes")
	}
	// The constructor runs on the pinned snapshot; all source resolution below is
	// direct and bounded by the supplied key lists, never a full graph scan.
	nativeRows, _, err := s.nodeLayerForDocuments()
	if err != nil {
		return graphOverlayGroup{}, nil, nil, err
	}
	nativeByKey := map[model.GraphDigest]model.GraphNodeRecord{}
	for _, n := range nativeRows {
		nativeByKey[n.NodeKey] = n
	}
	nodeByID := make(map[int]model.GraphNodeRecord, len(input.Bundle.Nodes))
	nodeRefs := make(map[int]string, len(input.Bundle.Nodes))
	nodeKeys := make(map[model.GraphDigest]model.GraphNodeRecord, len(input.Bundle.Nodes))
	group := graphOverlayGroup{Index: len(s.groups), WorkspaceIdentity: s.base.generation.WorkspaceIdentity, RepositoryIdentity: s.base.generation.RepositoryIdentity, Backend: "docgraph", ExtractorVersion: "docgraph/v1", ProjectOrModule: ".", Native: false}
	for i, n := range input.Bundle.Nodes {
		if n.NodeID != i || n.GenerationID != zero || n.SourceDigest == zero || model.ValidateGraphNodeRecord(n) != nil {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid local document node %d", i)
		}
		identity, err := model.NewNodeKey(n.Identity)
		if err != nil || identity != n.Identity {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("noncanonical document node identity %d", i)
		}
		if identity.RepositoryIdentity != s.base.generation.RepositoryIdentity {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("document node repository differs from pinned base")
		}
		if _, exists := nodeKeys[n.NodeKey]; exists {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("duplicate document bundle node key")
		}
		nodeByID[i], nodeKeys[n.NodeKey], nodeRefs[i] = n, n, fmt.Sprintf("doc-node:%d", i)
	}
	// Every non-document node must be explicitly classified as a base/native
	// context anchor or as a narrowly-derived file node.
	for key := range contextDocs {
		if _, ok := nodeKeys[key]; !ok {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("context document key is absent from bundle")
		}
	}
	for key := range contextNodes {
		if _, ok := nodeKeys[key]; !ok {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("context code key is absent from bundle")
		}
	}
	for key := range derivedSources {
		if _, ok := nodeKeys[key]; !ok {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("derived file key is absent from bundle")
		}
	}
	usedDocs, usedNodes, usedDerived := map[model.GraphDigest]bool{}, map[model.GraphDigest]bool{}, map[model.GraphDigest]bool{}
	documentNodes := map[string]model.GraphNodeRecord{}
	for _, n := range input.Bundle.Nodes {
		id := n.Identity
		if id.BackendType == "docgraph" {
			if id.Language != "markdown" || id.SymbolKind != "document" || id.ProjectOrModule != graphOverlayDocumentModule(id.OwnerPath) || !graphOverlayCanonicalDocumentPath(id.OwnerPath) {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("noncanonical document node identity")
			}
			if contextDocs[n.NodeKey] {
				if pathSet[id.OwnerPath] || id.SymbolKind != "document" {
					return graphOverlayGroup{}, nil, nil, fmt.Errorf("context document must be an unmasked document node")
				}
				base, e := s.baseNodeByKey(context.Background(), n.NodeKey)
				if e != nil || s.MasksPath(base.Identity.OwnerPath) || !graphOverlayNodeCompatible(base, n) {
					return graphOverlayGroup{}, nil, nil, fmt.Errorf("context document does not match pinned base")
				}
				usedDocs[n.NodeKey] = true
			} else if !pathSet[id.OwnerPath] {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document-owned node is outside affected document paths")
			} else {
				if _, exists := documentNodes[id.OwnerPath]; exists {
					return graphOverlayGroup{}, nil, nil, fmt.Errorf("duplicate owned document node path")
				}
				documentNodes[id.OwnerPath] = n
			}
		} else if contextNodes[n.NodeKey] {
			if id.SymbolKind == "document" {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("code context key points at document node")
			}
			actual, ok := nativeByKey[n.NodeKey]
			if !ok {
				actual, _ = s.nativeBaseNode(n.NodeKey)
			}
			if actual.NodeKey != n.NodeKey || !graphOverlayNodeCompatible(actual, n) {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("code context node does not match effective native view")
			}
			usedNodes[n.NodeKey] = true
		} else if sources, ok := derivedSources[n.NodeKey]; ok {
			if pathSet[id.OwnerPath] {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("derived code file cannot be owned by a changed document path")
			}
			if err := s.validateDerivedFile(n, sources, nativeByKey); err != nil {
				return graphOverlayGroup{}, nil, nil, err
			}
			usedDerived[n.NodeKey] = true
		} else {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("unclassified non-document node in document bundle")
		}
		obs := model.GraphObservationNode{Ref: nodeRefs[n.NodeID], Key: n.Identity, DisplayName: n.DisplayName, SourceDigest: n.SourceDigest, ClaimStatus: n.ClaimStatus}
		if id.BackendType == "docgraph" {
			obs.Resolution = "docgraph"
		} else {
			if native, ok := findNativeObservation(s.groups, n.NodeKey); ok {
				obs.Resolution = native.Resolution
			}
		}
		group.Nodes = append(group.Nodes, obs)
	}
	if len(usedDocs) != len(contextDocs) || len(usedNodes) != len(contextNodes) || len(usedDerived) != len(derivedSources) {
		return graphOverlayGroup{}, nil, nil, fmt.Errorf("unconsumed document context or derived metadata")
	}
	// Edges use local bundle IDs only as ref wiring. Their identity and source
	// digest are independently recomputed from canonical endpoints/evidence.
	edgeByID := map[int]model.GraphEdgeRecord{}
	for i, e := range input.Bundle.Edges {
		if e.EdgeID != i || e.GenerationID != zero || e.FromNodeID < 0 || e.ToNodeID < 0 || e.OwnerPath == "" || !pathSet[e.OwnerPath] || e.SourceBackend != "docgraph" {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid document-owned edge %d", i)
		}
		from, fok := nodeByID[e.FromNodeID]
		to, tok := nodeByID[e.ToNodeID]
		if !fok || !tok || model.ValidateGraphEdgeRecord(e, from.NodeKey, to.NodeKey) != nil {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid document edge identity %d", i)
		}
		edgeByID[i] = e
	}
	evidenceByEdge := map[int][]model.GraphEvidence{}
	for i, e := range input.Bundle.Evidence {
		if e.EvidenceID != i || e.GenerationID != zero {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid local document evidence id")
		}
		var subject model.GraphDigest
		if e.NodeID != nil {
			n, ok := nodeByID[*e.NodeID]
			if !ok {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document evidence node missing")
			}
			subject = n.NodeKey
		} else if e.EdgeID != nil {
			edge, ok := edgeByID[*e.EdgeID]
			if !ok {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document evidence edge missing")
			}
			subject = edge.EdgeKey
			evidenceByEdge[*e.EdgeID] = append(evidenceByEdge[*e.EdgeID], e)
		} else {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("document evidence lacks subject")
		}
		if model.ValidateGraphEvidence(e, subject) != nil {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid canonical document evidence")
		}
		// Native evidence copied into the private producer bundle is already
		// represented by native groups; do not duplicate it in the document layer.
		if e.Backend != "docgraph" {
			continue
		}
		if e.NodeID != nil {
			n := nodeByID[*e.NodeID]
			if n.Identity.BackendType != "docgraph" || !pathSet[n.Identity.OwnerPath] || e.SourceURI != n.Identity.OwnerPath || e.SourceDigest != n.SourceDigest || e.ObservedClaimDigest != n.SourceDigest || e.ClaimKind != "document" {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document node evidence owner/digest mismatch")
			}
		} else {
			edge := edgeByID[*e.EdgeID]
			if e.SourceURI != edge.OwnerPath {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document edge evidence owner mismatch")
			}
		}
		obs := graphEvidenceObservation(e, fmt.Sprintf("doc-evidence:%d", i))
		if e.NodeID != nil {
			obs.NodeRef = nodeRefs[*e.NodeID]
		} else {
			obs.EdgeRef = fmt.Sprintf("doc-edge:%d", *e.EdgeID)
		}
		group.Evidence = append(group.Evidence, obs)
		if e.NodeID != nil {
			n := nodeByID[*e.NodeID]
			owner, ok := documentNodes[n.Identity.OwnerPath]
			if !ok || owner.NodeID != n.NodeID {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document node evidence has no owned node")
			}
		}
	}
	for path, n := range documentNodes {
		found := false
		for _, ev := range input.Bundle.Evidence {
			if ev.NodeID != nil && *ev.NodeID == n.NodeID && ev.Backend == "docgraph" {
				found = true
				break
			}
		}
		if !found {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("document node %s lacks canonical source evidence", path)
		}
	}
	for i, e := range input.Bundle.Edges {
		evidence := evidenceByEdge[i]
		if len(evidence) == 0 {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("document edge lacks source evidence")
		}
		var source model.GraphDigest
		ownerNode, ownerOK := documentNodes[e.OwnerPath]
		if !ownerOK {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("document edge owner is not an affected document node")
		}
		for _, ev := range evidence {
			if ev.Backend != "docgraph" || ev.SourceURI != e.OwnerPath || ev.SourceDigest != ownerNode.SourceDigest || ev.SourceDigest == zero || ev.ClaimKind != e.Relation || ev.ClaimStatus != e.ClaimStatus || ev.ObservedClaimDigest == zero {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document edge evidence does not prove its owner/source")
			}
			if source != zero && source != ev.SourceDigest {
				return graphOverlayGroup{}, nil, nil, fmt.Errorf("document edge has conflicting evidence source digests")
			}
			source = ev.SourceDigest
		}
		group.Edges = append(group.Edges, model.GraphObservationEdge{Ref: fmt.Sprintf("doc-edge:%d", i), FromRef: nodeRefs[e.FromNodeID], ToRef: nodeRefs[e.ToNodeID], Relation: e.Relation, Scope: e.ClaimScope, Status: e.ClaimStatus, OwnerPath: e.OwnerPath, Backend: e.SourceBackend, Resolution: "docgraph", SourceDigest: source})
	}
	seenUnresolved := map[model.GraphDigest]bool{}
	for i, u := range input.Bundle.Unresolved {
		if u.UnresolvedID != i || u.GenerationID != zero || !pathSet[u.OwnerPath] || u.Backend != "docgraph" || model.ValidateGraphUnresolved(u) != nil {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("invalid document unresolved record %d", i)
		}
		ownerNode, ownerOK := documentNodes[u.OwnerPath]
		if u.SourceDocument == "" || u.SourceDocument != u.OwnerPath || !ownerOK || u.SourceDigest == nil || *u.SourceDigest != ownerNode.SourceDigest {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("document unresolved source document/digest must match its affected owner")
		}
		key := model.GraphUnresolvedKey(u)
		if seenUnresolved[key] {
			return graphOverlayGroup{}, nil, nil, fmt.Errorf("duplicate document unresolved identity")
		}
		seenUnresolved[key] = true
		copyRecord := u
		copyRecord.Candidates = append([]string(nil), u.Candidates...)
		if u.SourceDigest != nil {
			d := *u.SourceDigest
			copyRecord.SourceDigest = &d
		}
		obs := model.GraphObservationUnresolved{Ref: fmt.Sprintf("doc-unresolved:%d", i), OwnerPath: u.OwnerPath, SubjectKind: u.SubjectKind, SelectorDigest: u.SelectorDigest, ReasonCode: u.ReasonCode, Candidates: append([]string(nil), u.Candidates...), Backend: u.Backend, SourceDigest: copyRecord.SourceDigest, RecoveryHintCode: u.RecoveryHintCode}
		group.Unresolved = append(group.Unresolved, graphOverlayUnresolvedRow{Observation: obs, Record: copyRecord})
	}
	group.SourceFingerprint = input.Bundle.Generation.SourceFingerprint
	// The producer intentionally supplies an empty generation. Hash the entire
	// canonical IR, including masks, contexts and derived-source declarations.
	derivedSourcePairs := make([]graphOverlayDerivedSourcePair, 0, len(derivedSources))
	for key, sources := range derivedSources {
		derivedSourcePairs = append(derivedSourcePairs, graphOverlayDerivedSourcePair{
			Key:     key,
			Sources: append([]model.GraphDigest(nil), sources...),
		})
	}
	sort.Slice(derivedSourcePairs, func(i, j int) bool {
		return derivedSourcePairs[i].Key.String() < derivedSourcePairs[j].Key.String()
	})
	canonical := struct {
		Bundle              model.GraphBundle
		AffectedOwnerPaths  []string
		ContextDocumentKeys []model.GraphDigest
		ContextNodeKeys     []model.GraphDigest
		DerivedNodeSources  []graphOverlayDerivedSourcePair
	}{
		Bundle:              input.Bundle,
		AffectedOwnerPaths:  append([]string(nil), docPaths...),
		ContextDocumentKeys: graphOverlaySortedDigests(contextDocs),
		ContextNodeKeys:     graphOverlaySortedDigests(contextNodes),
		DerivedNodeSources:  derivedSourcePairs,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return graphOverlayGroup{}, nil, nil, err
	}
	group.Digest = model.GraphDigest(sha256.Sum256(append([]byte("milsp-doc-delta-ir/v1\x00"), encoded...)))
	group.SourceFingerprint = group.Digest
	return group, derivedSources, json.RawMessage(encoded), nil
}

func graphOverlayDigestSet(in []model.GraphDigest) (map[model.GraphDigest]bool, error) {
	out := map[model.GraphDigest]bool{}
	for _, d := range in {
		if d == (model.GraphDigest{}) || out[d] {
			return nil, fmt.Errorf("zero or duplicate document context key")
		}
		out[d] = true
	}
	return out, nil
}
func graphOverlaySortedDigests(in map[model.GraphDigest]bool) []model.GraphDigest {
	out := make([]model.GraphDigest, 0, len(in))
	for d := range in {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].String(), out[j].String()) < 0 })
	return out
}
func graphOverlayNodeCompatible(actual model.GraphNodeRecord, expected model.GraphNodeRecord) bool {
	return actual.NodeKey == expected.NodeKey && actual.Identity == expected.Identity && actual.IdentitySchema == expected.IdentitySchema && actual.SourceDigest == expected.SourceDigest && actual.ClaimStatus == expected.ClaimStatus && actual.DisplayName == expected.DisplayName
}
func (s *LayeredGraphSnapshot) nativeBaseNode(key model.GraphDigest) (model.GraphNodeRecord, error) {
	n, err := s.baseNodeByKey(context.Background(), key)
	if err != nil || s.MasksPath(n.Identity.OwnerPath) || n.Identity.BackendType == "docgraph" {
		return model.GraphNodeRecord{}, fmt.Errorf("native anchor is absent from effective view")
	}
	return n, nil
}
func (s *LayeredGraphSnapshot) nodeLayerForDocuments() (map[int]model.GraphNodeRecord, map[string]int, error) {
	return s.nodeLayer(context.Background())
}
func findNativeObservation(groups []graphOverlayGroup, key model.GraphDigest) (model.GraphObservationNode, bool) {
	for _, g := range groups {
		if !g.Native {
			continue
		}
		for _, n := range g.Nodes {
			id, err := model.NewNodeKey(n.Key)
			if err != nil {
				continue
			}
			d, err := id.Hash()
			if err == nil && d == key {
				return n, true
			}
		}
	}
	return model.GraphObservationNode{}, false
}
func graphEvidenceObservation(e model.GraphEvidence, ref string) model.GraphObservationEvidence {
	obs := model.GraphObservationEvidence{Ref: ref, SourceURI: e.SourceURI, Backend: e.Backend, ExtractorVersion: e.ExtractorVersion, SourceDigest: e.SourceDigest, ObservedDigest: e.ObservedClaimDigest, ClaimKind: e.ClaimKind, Status: e.ClaimStatus}
	if e.StartLine != nil && e.StartColumn != nil && e.EndLine != nil && e.EndColumn != nil {
		obs.Range = &model.GraphObservationRange{StartLine: *e.StartLine, StartColumn: *e.StartColumn, EndLine: *e.EndLine, EndColumn: *e.EndColumn}
	}
	return obs
}
func (s *LayeredGraphSnapshot) validateDerivedFile(file model.GraphNodeRecord, sourceKeys []model.GraphDigest, native map[model.GraphDigest]model.GraphNodeRecord) error {
	id := file.Identity
	if id.BackendType == "docgraph" || id.SymbolKind != "file" || id.SemanticIdentity != id.OwnerPath || len(sourceKeys) == 0 {
		return fmt.Errorf("derived node is not a canonical code file")
	}
	var first *model.GraphNodeRecord
	for _, key := range sourceKeys {
		src, ok := native[key]
		if !ok {
			var err error
			src, err = s.nativeBaseNode(key)
			if err != nil {
				return fmt.Errorf("derived file source %s is not an effective native node", key)
			}
		}
		si := src.Identity
		if si.BackendType == "docgraph" || si.OwnerPath != id.OwnerPath || si.RepositoryIdentity != id.RepositoryIdentity || si.BackendType != id.BackendType || si.Language != id.Language || si.ProjectOrModule != id.ProjectOrModule || src.SourceDigest != file.SourceDigest {
			return fmt.Errorf("derived file source identity/digest mismatch")
		}
		if first == nil {
			copy := src
			first = &copy
		}
	}
	if first == nil || file.ClaimStatus != model.GraphRecordExtracted {
		return fmt.Errorf("derived file claim is incompatible")
	}
	return nil
}

// UnresolvedRecords returns complete effective unresolved records, including
// document diagnostic fields. Overlay IDs are deterministic and view-local;
// they are not publishable canonical IDs.
func (s *LayeredGraphSnapshot) UnresolvedRecords(ctx context.Context, max int) ([]model.GraphUnresolved, error) {
	if s == nil || s.closed || s.base == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if max < 1 {
		return []model.GraphUnresolved{}, nil
	}
	if max > 1000 {
		max = 1000
	}
	mask, args := s.baseUnresolvedUnmaskedPredicate()
	rows, err := s.base.query(ctx, `SELECT unresolved_id,unresolved_key,owner_path,subject_kind,selector_digest,reason_code,candidates_json,backend,source_digest,cross_rid,recovery_hint_code,source_document,source_block,target_kind,target_value FROM graph_unresolved WHERE generation_id=?`+mask+` ORDER BY unresolved_id`, append([]any{digestArg(s.base.generation.GenerationID)}, args...)...)
	if err != nil {
		return nil, err
	}
	byKey := map[model.GraphDigest]model.GraphUnresolved{}
	maxID := -1
	if err := s.base.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(unresolved_id),-1) FROM graph_unresolved WHERE generation_id=?`, digestArg(s.base.generation.GenerationID)).Scan(&maxID); err != nil {
		rows.Close()
		return nil, err
	}
	for rows.Next() {
		var r model.GraphUnresolved
		var keyRaw, selectorRaw, sourceRaw []byte
		var candidates []byte
		var recovery, sourceDoc, sourceBlock, targetKind, targetValue sql.NullString
		if err = rows.Scan(&r.UnresolvedID, &keyRaw, &r.OwnerPath, &r.SubjectKind, &selectorRaw, &r.ReasonCode, &candidates, &r.Backend, &sourceRaw, &r.CrossRID, &recovery, &sourceDoc, &sourceBlock, &targetKind, &targetValue); err != nil {
			rows.Close()
			return nil, err
		}
		r.GenerationID = s.base.generation.GenerationID
		if r.UnresolvedKey, err = scanDigest(keyRaw); err != nil {
			rows.Close()
			return nil, err
		}
		if r.SelectorDigest, err = scanDigest(selectorRaw); err != nil {
			rows.Close()
			return nil, err
		}
		if len(sourceRaw) > 0 {
			d, e := scanDigest(sourceRaw)
			if e != nil {
				rows.Close()
				return nil, e
			}
			r.SourceDigest = &d
		}
		if err = json.Unmarshal(candidates, &r.Candidates); err != nil {
			rows.Close()
			return nil, err
		}
		if recovery.Valid {
			r.RecoveryHintCode = recovery.String
		}
		if sourceDoc.Valid {
			r.SourceDocument = sourceDoc.String
		}
		if sourceBlock.Valid {
			r.SourceBlock = sourceBlock.String
		}
		if targetKind.Valid {
			r.TargetKind = targetKind.String
		}
		if targetValue.Valid {
			r.TargetValue = targetValue.String
		}
		byKey[r.UnresolvedKey] = r
		if r.UnresolvedID > maxID {
			maxID = r.UnresolvedID
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	overlay := map[model.GraphDigest]model.GraphUnresolved{}
	for _, g := range s.groups {
		for _, row := range g.Unresolved {
			r := row.Record
			r.Candidates = append([]string(nil), row.Record.Candidates...)
			if row.Record.SourceDigest != nil {
				d := *row.Record.SourceDigest
				r.SourceDigest = &d
			}
			if !s.MasksSemanticOwnerInDomain(r.Backend, g.ProjectOrModule, r.OwnerPath) {
				continue
			}
			key := model.GraphUnresolvedKey(r)
			if old, ok := overlay[key]; ok && !graphOverlayUnresolvedRecordEqual(old, r) {
				return nil, fmt.Errorf("overlay unresolved identity has conflicting diagnostic metadata")
			}
			overlay[key] = r
		}
	}
	keys := make([]model.GraphDigest, 0, len(overlay))
	for key := range overlay {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, key := range keys {
		r := overlay[key]
		if old, ok := byKey[key]; ok {
			if !graphOverlayUnresolvedRecordEqual(old, r) {
				return nil, fmt.Errorf("overlay unresolved conflicts with visible base identity")
			}
			continue
		}
		maxID++
		r.GenerationID = s.generation.GenerationID
		r.UnresolvedID = maxID
		r.UnresolvedKey = key
		r.CrossRID = model.UnresolvedRID(key)
		byKey[key] = r
	}
	result := make([]model.GraphUnresolved, 0, len(byKey))
	for _, r := range byKey {
		r.GenerationID = s.generation.GenerationID
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].OwnerPath != result[j].OwnerPath {
			return result[i].OwnerPath < result[j].OwnerPath
		}
		return result[i].UnresolvedKey.String() < result[j].UnresolvedKey.String()
	})
	if len(result) > max {
		result = result[:max]
	}
	return result, nil
}

func graphOverlayUnresolvedRecordEqual(a, b model.GraphUnresolved) bool {
	return a.OwnerPath == b.OwnerPath && a.SubjectKind == b.SubjectKind && a.SelectorDigest == b.SelectorDigest && a.ReasonCode == b.ReasonCode && strings.Join(a.Candidates, "\x00") == strings.Join(b.Candidates, "\x00") && a.Backend == b.Backend && graphOverlayDigestPtrEqual(a.SourceDigest, b.SourceDigest) && a.RecoveryHintCode == b.RecoveryHintCode && a.SourceDocument == b.SourceDocument && a.SourceBlock == b.SourceBlock && a.TargetKind == b.TargetKind && a.TargetValue == b.TargetValue
}
func graphOverlayDigestPtrEqual(a, b *model.GraphDigest) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func graphOverlayCanonicalDocumentPath(path string) bool {
	lower := strings.ToLower(path)
	for _, excluded := range []string{".docs/raw/", ".docs/auditoria/", "/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"} {
		if strings.Contains("/"+lower, excluded) {
			return false
		}
	}
	return (strings.HasPrefix(lower, ".docs/wiki/") || strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "wiki/") || strings.HasPrefix(lower, "bibliotecas/") || strings.HasPrefix(lower, "readme")) && strings.HasSuffix(lower, ".md")
}
func graphOverlayDocumentModule(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasPrefix(lower, "wiki/"):
		return "wiki"
	case strings.HasPrefix(lower, "bibliotecas/"):
		return "bibliotecas"
	case strings.HasPrefix(lower, "docs/"):
		return "docs"
	default:
		return ".docs/wiki"
	}
}

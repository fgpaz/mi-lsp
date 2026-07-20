package indexer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sort"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

var (
	errGraphAssemblyInvalid  = errors.New("invalid graph observation assembly request")
	errGraphAssemblyConflict = errors.New("conflicting graph observation assembly claim")
)

// GraphAssemblyRequest is the deterministic input to graph assembly.
type GraphAssemblyRequest struct {
	Batches   []model.GraphObservationBatch
	CreatedAt time.Time
}

type graphDigestBuilder struct{ h hash.Hash }

func newGraphDigestBuilder(prefix string) *graphDigestBuilder {
	b := &graphDigestBuilder{h: sha256.New()}
	_, _ = b.h.Write([]byte(prefix))
	return b
}
func (b *graphDigestBuilder) frame(tag byte, value []byte) {
	_, _ = b.h.Write([]byte{tag})
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(value)))
	_, _ = b.h.Write(n[:])
	_, _ = b.h.Write(value)
}
func (b *graphDigestBuilder) text(tag byte, value string)              { b.frame(tag, []byte(value)) }
func (b *graphDigestBuilder) digest(tag byte, value model.GraphDigest) { b.frame(tag, value[:]) }
func (b *graphDigestBuilder) sum() model.GraphDigest {
	var d model.GraphDigest
	copy(d[:], b.h.Sum(nil))
	return d
}

func aggregateGraphDigest(prefix string, batches []model.GraphObservationBatch, kind byte) model.GraphDigest {
	b := newGraphDigestBuilder(prefix)
	b.frame(240, []byte{kind})
	for _, batch := range batches {
		b.text(1, batch.Backend)
		b.text(2, batch.ProjectOrModule)
		if kind == 1 {
			b.digest(3, batch.SourceFingerprint)
		} else if kind == 2 {
			b.digest(3, batch.ConfigFingerprint)
		} else {
			b.text(3, batch.BackendVersion)
			b.text(4, batch.ExtractorVersion)
			b.digest(5, batch.Digest)
			for _, capability := range batch.Capabilities {
				b.text(6, capability.Backend)
				b.text(7, capability.Capability)
				b.text(8, capability.State)
			}
			for _, coverage := range batch.Coverage {
				b.text(9, coverage.Backend)
				b.text(10, coverage.Capability)
				var n [32]byte
				binary.BigEndian.PutUint64(n[:8], uint64(coverage.Eligible))
				binary.BigEndian.PutUint64(n[8:16], uint64(coverage.Observed))
				binary.BigEndian.PutUint64(n[16:24], uint64(coverage.Unresolved))
				binary.BigEndian.PutUint64(n[24:32], uint64(coverage.Omitted))
				b.frame(11, n[:])
			}
		}
	}
	return b.sum()
}

func graphBatchLess(a, b model.GraphObservationBatch) bool {
	if a.Backend != b.Backend {
		return a.Backend < b.Backend
	}
	if a.ProjectOrModule != b.ProjectOrModule {
		return a.ProjectOrModule < b.ProjectOrModule
	}
	return a.Digest.String() < b.Digest.String()
}

func prepareGraphBatches(req GraphAssemblyRequest) ([]model.GraphObservationBatch, time.Time, error) {
	if len(req.Batches) == 0 || req.CreatedAt.IsZero() {
		return nil, time.Time{}, errGraphAssemblyInvalid
	}
	createdAt := req.CreatedAt.Round(0).UTC()
	batches := append([]model.GraphObservationBatch(nil), req.Batches...)
	var workspaceIdentity, repositoryIdentity string
	for i := range batches {
		if err := batches[i].Validate(); err != nil {
			return nil, time.Time{}, fmt.Errorf("batch %d validation: %w", i, err)
		}
		if err := batches[i].ReadyForStaging(); err != nil {
			return nil, time.Time{}, fmt.Errorf("batch %d staging gate: %w", i, err)
		}
		if i == 0 {
			workspaceIdentity, repositoryIdentity = batches[i].WorkspaceIdentity, batches[i].RepositoryIdentity
		} else if batches[i].WorkspaceIdentity != workspaceIdentity || batches[i].RepositoryIdentity != repositoryIdentity {
			return nil, time.Time{}, fmt.Errorf("%w: workspace or repository differs", errGraphAssemblyInvalid)
		}
	}
	sort.Slice(batches, func(i, j int) bool { return graphBatchLess(batches[i], batches[j]) })
	unique := make([]model.GraphObservationBatch, 0, len(batches))
	for _, batch := range batches {
		if len(unique) == 0 {
			unique = append(unique, batch)
			continue
		}
		last := unique[len(unique)-1]
		if last.Backend == batch.Backend && last.ProjectOrModule == batch.ProjectOrModule {
			if last.Digest != batch.Digest {
				return nil, time.Time{}, fmt.Errorf("%w: backend=%s module=%s", errGraphAssemblyConflict, batch.Backend, batch.ProjectOrModule)
			}
			continue
		}
		unique = append(unique, batch)
	}
	return unique, createdAt, nil
}

type graphScopedRef struct {
	batch int
	ref   string
}

type graphNodeCandidate struct {
	key      model.GraphDigest
	identity model.NodeKey
	node     model.GraphObservationNode
}

type graphEdgeCandidate struct {
	key      model.GraphDigest
	from, to model.GraphDigest
	edge     model.GraphObservationEdge
}

type graphEvidenceCandidate struct {
	subject  model.GraphDigest
	edgeKey  model.GraphDigest
	evidence model.GraphObservationEvidence
}

type graphUnresolvedCandidate struct {
	key        model.GraphDigest
	unresolved model.GraphUnresolved
	ref        string
}

func graphNodeEquivalent(a, b graphNodeCandidate) bool {
	return a.identity == b.identity && a.node.DisplayName == b.node.DisplayName && a.node.SourceDigest == b.node.SourceDigest && a.node.ClaimStatus == b.node.ClaimStatus && a.node.Resolution == b.node.Resolution
}
func graphEdgeEquivalent(a, b graphEdgeCandidate) bool {
	return a.from == b.from && a.to == b.to && a.edge.Relation == b.edge.Relation && a.edge.Scope == b.edge.Scope && a.edge.Status == b.edge.Status && a.edge.OwnerPath == b.edge.OwnerPath && a.edge.Backend == b.edge.Backend && a.edge.Resolution == b.edge.Resolution && a.edge.SourceDigest == b.edge.SourceDigest
}

func graphDigestLess(a, b model.GraphDigest) bool { return a.String() < b.String() }
func graphStringSliceLess(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func graphRangeLess(a, b *model.GraphObservationRange) bool {
	if a == nil || b == nil {
		return a == nil && b != nil
	}
	if a.StartLine != b.StartLine {
		return a.StartLine < b.StartLine
	}
	if a.StartColumn != b.StartColumn {
		return a.StartColumn < b.StartColumn
	}
	if a.EndLine != b.EndLine {
		return a.EndLine < b.EndLine
	}
	return a.EndColumn < b.EndColumn
}

func assembleGraphBundle(batches []model.GraphObservationBatch, createdAt time.Time) (model.GraphBundle, error) {
	nodeRefs := make(map[graphScopedRef]model.GraphDigest)
	nodesByKey := make(map[model.GraphDigest]graphNodeCandidate)
	for batchIndex, batch := range batches {
		for _, observed := range batch.Nodes {
			identity, err := model.NewNodeKey(observed.Key)
			if err != nil {
				return model.GraphBundle{}, err
			}
			key, err := identity.Hash()
			if err != nil {
				return model.GraphBundle{}, err
			}
			candidate := graphNodeCandidate{key: key, identity: identity, node: observed}
			if old, ok := nodesByKey[key]; ok && !graphNodeEquivalent(old, candidate) {
				return model.GraphBundle{}, fmt.Errorf("%w: node %s", errGraphAssemblyConflict, key.String())
			}
			nodesByKey[key] = candidate
			nodeRefs[graphScopedRef{batch: batchIndex, ref: observed.Ref}] = key
		}
	}

	nodeKeys := make([]model.GraphDigest, 0, len(nodesByKey))
	for key := range nodesByKey {
		nodeKeys = append(nodeKeys, key)
	}
	sort.Slice(nodeKeys, func(i, j int) bool {
		if nodeKeys[i] != nodeKeys[j] {
			return graphDigestLess(nodeKeys[i], nodeKeys[j])
		}
		return nodesByKey[nodeKeys[i]].identity.SemanticIdentity < nodesByKey[nodeKeys[j]].identity.SemanticIdentity
	})
	nodeIDs := make(map[model.GraphDigest]int, len(nodeKeys))
	nodes := make([]model.GraphNodeRecord, len(nodeKeys))
	for i, key := range nodeKeys {
		nodeIDs[key] = i
		candidate := nodesByKey[key]
		nodes[i] = model.GraphNodeRecord{NodeID: i, NodeKey: key, Identity: candidate.identity, IdentitySchema: "milsp-node-key/v1", DisplayName: candidate.node.DisplayName, SourceDigest: candidate.node.SourceDigest, ClaimStatus: candidate.node.ClaimStatus, CrossRID: model.NodeRID(key), SortKey: key.String()}
	}

	edgeRefs := make(map[graphScopedRef]model.GraphDigest)
	edgesByKey := make(map[model.GraphDigest]graphEdgeCandidate)
	for batchIndex, batch := range batches {
		for _, observed := range batch.Edges {
			from, fromOK := nodeRefs[graphScopedRef{batch: batchIndex, ref: observed.FromRef}]
			to, toOK := nodeRefs[graphScopedRef{batch: batchIndex, ref: observed.ToRef}]
			if !fromOK || !toOK {
				return model.GraphBundle{}, fmt.Errorf("%w: edge endpoint", errGraphAssemblyInvalid)
			}
			key := model.EdgeKey(from, to, observed.Relation, observed.Scope)
			candidate := graphEdgeCandidate{key: key, from: from, to: to, edge: observed}
			if old, ok := edgesByKey[key]; ok && !graphEdgeEquivalent(old, candidate) {
				return model.GraphBundle{}, fmt.Errorf("%w: edge %s", errGraphAssemblyConflict, key.String())
			}
			edgesByKey[key] = candidate
			edgeRefs[graphScopedRef{batch: batchIndex, ref: observed.Ref}] = key
		}
	}
	edgeKeys := make([]model.GraphDigest, 0, len(edgesByKey))
	for key := range edgesByKey {
		edgeKeys = append(edgeKeys, key)
	}
	sort.Slice(edgeKeys, func(i, j int) bool { return graphDigestLess(edgeKeys[i], edgeKeys[j]) })
	edgeIDs := make(map[model.GraphDigest]int, len(edgeKeys))
	edges := make([]model.GraphEdgeRecord, len(edgeKeys))
	for i, key := range edgeKeys {
		edgeIDs[key] = i
		candidate := edgesByKey[key]
		edges[i] = model.GraphEdgeRecord{EdgeID: i, EdgeKey: key, FromNodeID: nodeIDs[candidate.from], ToNodeID: nodeIDs[candidate.to], Relation: candidate.edge.Relation, ClaimScope: candidate.edge.Scope, ClaimStatus: candidate.edge.Status, OwnerPath: candidate.edge.OwnerPath, SourceBackend: candidate.edge.Backend, CrossRID: model.EdgeRID(key)}
	}

	evidence := make([]graphEvidenceCandidate, 0)
	for batchIndex, batch := range batches {
		for _, observed := range batch.Evidence {
			candidate := graphEvidenceCandidate{evidence: observed}
			if observed.NodeRef != "" {
				key, ok := nodeRefs[graphScopedRef{batch: batchIndex, ref: observed.NodeRef}]
				if !ok {
					return model.GraphBundle{}, fmt.Errorf("%w: evidence node", errGraphAssemblyInvalid)
				}
				candidate.subject = key
			} else {
				key, ok := edgeRefs[graphScopedRef{batch: batchIndex, ref: observed.EdgeRef}]
				if !ok {
					return model.GraphBundle{}, fmt.Errorf("%w: evidence edge", errGraphAssemblyInvalid)
				}
				candidate.subject, candidate.edgeKey = key, key
			}
			evidence = append(evidence, candidate)
		}
	}
	sort.Slice(evidence, func(i, j int) bool {
		a, b := evidence[i], evidence[j]
		if a.subject != b.subject {
			return graphDigestLess(a.subject, b.subject)
		}
		if a.evidence.SourceURI != b.evidence.SourceURI {
			return a.evidence.SourceURI < b.evidence.SourceURI
		}
		if a.evidence.Range != nil && b.evidence.Range != nil {
			if graphRangeLess(a.evidence.Range, b.evidence.Range) {
				return true
			}
			if graphRangeLess(b.evidence.Range, a.evidence.Range) {
				return false
			}
		}
		if a.evidence.ObservedDigest != b.evidence.ObservedDigest {
			return graphDigestLess(a.evidence.ObservedDigest, b.evidence.ObservedDigest)
		}
		if a.evidence.ClaimKind != b.evidence.ClaimKind {
			return a.evidence.ClaimKind < b.evidence.ClaimKind
		}
		if a.evidence.Backend != b.evidence.Backend {
			return a.evidence.Backend < b.evidence.Backend
		}
		if a.evidence.ExtractorVersion != b.evidence.ExtractorVersion {
			return a.evidence.ExtractorVersion < b.evidence.ExtractorVersion
		}
		return a.evidence.Ref < b.evidence.Ref
	})
	evidenceRecords := make([]model.GraphEvidence, len(evidence))
	for i, candidate := range evidence {
		observed := candidate.evidence
		startLine, startColumn, endLine, endColumn := 0, 0, 0, 0
		if observed.Range != nil {
			startLine, startColumn, endLine, endColumn = observed.Range.StartLine, observed.Range.StartColumn, observed.Range.EndLine, observed.Range.EndColumn
		}
		digest := model.EvidenceDigest(observed.SourceDigest, observed.ObservedDigest, observed.SourceURI, observed.ClaimKind, observed.Backend, observed.ExtractorVersion, startLine, startColumn, endLine, endColumn)
		record := model.GraphEvidence{EvidenceID: i, EvidenceKey: model.EvidenceKey(candidate.subject, digest, i), EvidenceDigest: digest, SourceURI: observed.SourceURI, Backend: observed.Backend, ExtractorVersion: observed.ExtractorVersion, SourceDigest: observed.SourceDigest, ClaimKind: observed.ClaimKind, ObservedClaimDigest: observed.ObservedDigest, ClaimStatus: observed.Status, CrossRID: model.EvidenceRID(model.EvidenceKey(candidate.subject, digest, i))}
		if observed.NodeRef != "" {
			id := nodeIDs[candidate.subject]
			record.SubjectKind, record.NodeID = "node", &id
		} else {
			id := edgeIDs[candidate.edgeKey]
			record.SubjectKind, record.EdgeID = "edge", &id
		}
		if observed.Range != nil {
			record.StartLine, record.StartColumn, record.EndLine, record.EndColumn = &startLine, &startColumn, &endLine, &endColumn
		}
		evidenceRecords[i] = record
	}

	unresolved := make([]graphUnresolvedCandidate, 0)
	for _, batch := range batches {
		for _, observed := range batch.Unresolved {
			var sourceDigest *model.GraphDigest
			if observed.SourceDigest != nil {
				d := *observed.SourceDigest
				sourceDigest = &d
			}
			u := model.GraphUnresolved{OwnerPath: observed.OwnerPath, SubjectKind: observed.SubjectKind, SelectorDigest: observed.SelectorDigest, ReasonCode: observed.ReasonCode, Candidates: append([]string(nil), observed.Candidates...), Backend: observed.Backend, SourceDigest: sourceDigest, RecoveryHintCode: observed.RecoveryHintCode}
			unresolved = append(unresolved, graphUnresolvedCandidate{key: model.GraphUnresolvedKey(u), unresolved: u, ref: observed.Ref})
		}
	}
	sort.Slice(unresolved, func(i, j int) bool {
		a, b := unresolved[i], unresolved[j]
		if a.key != b.key {
			return graphDigestLess(a.key, b.key)
		}
		if a.unresolved.OwnerPath != b.unresolved.OwnerPath {
			return a.unresolved.OwnerPath < b.unresolved.OwnerPath
		}
		if a.unresolved.SubjectKind != b.unresolved.SubjectKind {
			return a.unresolved.SubjectKind < b.unresolved.SubjectKind
		}
		if a.unresolved.ReasonCode != b.unresolved.ReasonCode {
			return a.unresolved.ReasonCode < b.unresolved.ReasonCode
		}
		if !graphStringSliceLess(a.unresolved.Candidates, b.unresolved.Candidates) && !graphStringSliceLess(b.unresolved.Candidates, a.unresolved.Candidates) {
			return a.ref < b.ref
		}
		return graphStringSliceLess(a.unresolved.Candidates, b.unresolved.Candidates)
	})
	unresolvedRecords := make([]model.GraphUnresolved, len(unresolved))
	for i, candidate := range unresolved {
		candidate.unresolved.UnresolvedID, candidate.unresolved.UnresolvedKey, candidate.unresolved.CrossRID = i, candidate.key, model.UnresolvedRID(candidate.key)
		unresolvedRecords[i] = candidate.unresolved
	}

	repository := batches[0].RepositoryIdentity
	bundle := model.GraphBundle{Generation: model.GraphGeneration{SchemaVersion: 1, WorkspaceIdentity: batches[0].WorkspaceIdentity, RepositoryIdentity: repository, SourceFingerprint: aggregateGraphDigest("MILSP-G3-SOURCE/v1", batches, 1), ConfigFingerprint: aggregateGraphDigest("MILSP-G3-CONFIG/v1", batches, 2), BackendManifestDigest: aggregateGraphDigest("MILSP-G3-BACKEND-MANIFEST/v1", batches, 3), Status: model.GraphGenerationStaged, NodeCount: len(nodes), EdgeCount: len(edges), EvidenceCount: len(evidenceRecords), UnresolvedCount: len(unresolvedRecords), CreatedAt: createdAt}, Nodes: nodes, Edges: edges, Evidence: evidenceRecords, Unresolved: unresolvedRecords}
	if err := bundle.SealIDs(); err != nil {
		return model.GraphBundle{}, err
	}
	if err := bundle.Validate(); err != nil {
		return model.GraphBundle{}, err
	}
	return bundle, nil
}

// AssembleGraphObservationBatches validates and deterministically assembles sealed observations.
func AssembleGraphObservationBatches(req GraphAssemblyRequest) (model.GraphBundle, error) {
	batches, createdAt, err := prepareGraphBatches(req)
	if err != nil {
		return model.GraphBundle{}, err
	}
	return assembleGraphBundle(batches, createdAt)
}

// StageGraphObservationBatches assembles once and persists one staged generation.
func StageGraphObservationBatches(ctx context.Context, db *sql.DB, req GraphAssemblyRequest) (model.GraphGeneration, error) {
	var empty model.GraphGeneration
	if ctx == nil || db == nil {
		return empty, model.ErrGraphGenerationInvalid
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	bundle, err := AssembleGraphObservationBatches(req)
	if err != nil {
		return empty, err
	}
	if err := store.StageGraphGeneration(ctx, db, &bundle); err != nil {
		return empty, err
	}
	return bundle.Generation, nil
}

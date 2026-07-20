package indexer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"path/filepath"
	"sort"
	"strings"
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
	Batches            []model.GraphObservationBatch
	Docs               []model.DocRecord
	DocEdges           []model.DocEdge
	DocMentions        []model.DocMention
	WorkspaceIdentity  string
	RepositoryIdentity string
	CreatedAt          time.Time
}

type graphAssemblyInput struct {
	batches            []model.GraphObservationBatch
	docs               []model.DocRecord
	docEdges           []model.DocEdge
	docMentions        []model.DocMention
	workspaceIdentity  string
	repositoryIdentity string
	createdAt          time.Time
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

func aggregateGraphDigest(prefix string, batches []model.GraphObservationBatch, kind byte, docs []model.DocRecord, docEdges []model.DocEdge, docMentions []model.DocMention) model.GraphDigest {
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
	if len(docs) != 0 {
		b.digest(250, graphDocFactsDigest(docs, docEdges, docMentions))
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

func isCanonicalGraphDoc(path string, snapshot bool) bool {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if snapshot || path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\\\") || (len(path) > 1 && path[1] == ':') || strings.Contains(path, "\\") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	lower := strings.ToLower(path)
	for _, excluded := range []string{".docs/raw/", ".docs/auditoria/", "/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"} {
		if strings.Contains("/"+lower, excluded) {
			return false
		}
	}
	if strings.HasPrefix(lower, ".docs/wiki/") || strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "readme") {
		return strings.HasSuffix(lower, ".md")
	}
	return false
}

func graphDocSourceDigest(doc model.DocRecord) model.GraphDigest {
	if d, err := model.ParseGraphDigest(strings.ToLower(strings.TrimSpace(doc.ContentHash))); err == nil {
		return d
	}
	b := newGraphDigestBuilder("MILSP-DOC-SOURCE/v1")
	b.text(1, doc.Path)
	b.text(2, doc.DocID)
	b.text(3, doc.Title)
	b.text(4, doc.Layer)
	b.text(5, doc.Family)
	b.text(6, doc.Snippet)
	b.text(7, doc.SearchText)
	b.text(8, doc.ContentHash)
	return b.sum()
}

func graphDocFactsDigest(docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention) model.GraphDigest {
	b := newGraphDigestBuilder("MILSP-DOC-FACTS/v1")
	for _, doc := range docs {
		b.text(1, doc.Path)
		b.text(2, doc.DocID)
		b.digest(3, graphDocSourceDigest(doc))
	}
	for _, edge := range edges {
		b.text(4, edge.FromPath)
		b.text(5, edge.ToPath)
		b.text(6, edge.ToDocID)
		b.text(7, edge.Kind)
		b.text(8, edge.Label)
	}
	for _, mention := range mentions {
		b.text(9, mention.DocPath)
		b.text(10, mention.MentionType)
		b.text(11, mention.MentionValue)
	}
	return b.sum()
}

func prepareGraphInput(req GraphAssemblyRequest) (graphAssemblyInput, error) {
	if req.CreatedAt.IsZero() {
		return graphAssemblyInput{}, errGraphAssemblyInvalid
	}
	createdAt := req.CreatedAt.Round(0).UTC()
	batches := append([]model.GraphObservationBatch(nil), req.Batches...)
	workspaceIdentity, repositoryIdentity := "", ""
	for i := range batches {
		if err := batches[i].Validate(); err != nil {
			return graphAssemblyInput{}, fmt.Errorf("batch %d validation: %w", i, err)
		}
		if err := batches[i].ReadyForStaging(); err != nil {
			return graphAssemblyInput{}, fmt.Errorf("batch %d staging gate: %w", i, err)
		}
		if i == 0 {
			workspaceIdentity, repositoryIdentity = batches[i].WorkspaceIdentity, batches[i].RepositoryIdentity
		} else if batches[i].WorkspaceIdentity != workspaceIdentity || batches[i].RepositoryIdentity != repositoryIdentity {
			return graphAssemblyInput{}, fmt.Errorf("%w: workspace or repository differs", errGraphAssemblyInvalid)
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
				return graphAssemblyInput{}, fmt.Errorf("%w: backend=%s module=%s", errGraphAssemblyConflict, batch.Backend, batch.ProjectOrModule)
			}
			continue
		}
		unique = append(unique, batch)
	}
	if len(unique) == 0 {
		repositoryIdentity = strings.TrimSpace(req.RepositoryIdentity)
		if repositoryIdentity == "" {
			return graphAssemblyInput{}, fmt.Errorf("%w: doc-only assembly requires explicit repository identity", errGraphAssemblyInvalid)
		}
		var err error
		repositoryIdentity, err = model.NormalizeRepositoryIdentity(repositoryIdentity)
		if err != nil {
			return graphAssemblyInput{}, err
		}
		workspaceIdentity = strings.TrimSpace(req.WorkspaceIdentity)
		if workspaceIdentity == "" {
			workspaceIdentity = repositoryIdentity
		}
		if workspaceIdentity != repositoryIdentity {
			return graphAssemblyInput{}, fmt.Errorf("%w: workspace identity must equal repository identity", errGraphAssemblyInvalid)
		}
	}

	docByPath := make(map[string]model.DocRecord, len(req.Docs))
	for _, doc := range req.Docs {
		doc.Path = filepath.ToSlash(strings.TrimSpace(doc.Path))
		if !isCanonicalGraphDoc(doc.Path, doc.IsSnapshot) {
			continue
		}
		if old, ok := docByPath[doc.Path]; ok && graphDocSourceDigest(old) != graphDocSourceDigest(doc) {
			return graphAssemblyInput{}, fmt.Errorf("%w: document path=%s", errGraphAssemblyConflict, doc.Path)
		}
		docByPath[doc.Path] = doc
	}
	docs := make([]model.DocRecord, 0, len(docByPath))
	for _, doc := range docByPath {
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	if len(unique) == 0 && len(docs) == 0 {
		return graphAssemblyInput{}, errGraphAssemblyInvalid
	}
	docPaths := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		docPaths[doc.Path] = struct{}{}
	}
	filterEdges := make([]model.DocEdge, 0, len(req.DocEdges))
	for _, edge := range req.DocEdges {
		edge.FromPath, edge.ToPath = filepath.ToSlash(strings.TrimSpace(edge.FromPath)), filepath.ToSlash(strings.TrimSpace(edge.ToPath))
		if _, ok := docPaths[edge.FromPath]; ok {
			filterEdges = append(filterEdges, edge)
		}
	}
	filterMentions := make([]model.DocMention, 0, len(req.DocMentions))
	for _, mention := range req.DocMentions {
		mention.DocPath = filepath.ToSlash(strings.TrimSpace(mention.DocPath))
		if _, ok := docPaths[mention.DocPath]; ok {
			filterMentions = append(filterMentions, mention)
		}
	}
	sort.Slice(filterEdges, func(i, j int) bool {
		a, b := filterEdges[i], filterEdges[j]
		for _, pair := range [][2]string{{a.FromPath, b.FromPath}, {a.ToPath, b.ToPath}, {a.ToDocID, b.ToDocID}, {a.Kind, b.Kind}, {a.Label, b.Label}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})
	sort.Slice(filterMentions, func(i, j int) bool {
		a, b := filterMentions[i], filterMentions[j]
		for _, pair := range [][2]string{{a.DocPath, b.DocPath}, {a.MentionType, b.MentionType}, {a.MentionValue, b.MentionValue}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})
	return graphAssemblyInput{batches: unique, docs: docs, docEdges: filterEdges, docMentions: filterMentions, workspaceIdentity: workspaceIdentity, repositoryIdentity: repositoryIdentity, createdAt: createdAt}, nil
}

func prepareGraphBatches(req GraphAssemblyRequest) ([]model.GraphObservationBatch, time.Time, error) {
	input, err := prepareGraphInput(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	return input.batches, input.createdAt, nil
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

func firstNonEmptyGraph(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func graphDocClaimDigest(kind, from, to, label string, source model.GraphDigest) model.GraphDigest {
	b := newGraphDigestBuilder("MILSP-DOC-CLAIM/v1")
	b.text(1, kind)
	b.text(2, from)
	b.text(3, to)
	b.text(4, label)
	b.digest(5, source)
	return b.sum()
}

func graphDocUnresolved(ref, owner, kind, value, reason string, candidates []string, source model.GraphDigest) graphUnresolvedCandidate {
	u := model.GraphUnresolved{OwnerPath: owner, SubjectKind: "document", SelectorDigest: graphDocClaimDigest(kind, owner, value, reason, source), ReasonCode: reason, Candidates: append([]string(nil), candidates...), Backend: "docgraph", SourceDigest: &source, RecoveryHintCode: "inspect_doc_graph_reference"}
	return graphUnresolvedCandidate{key: model.GraphUnresolvedKey(u), unresolved: u, ref: ref}
}

func assembleGraphBundle(input graphAssemblyInput) (model.GraphBundle, error) {
	batches, createdAt := input.batches, input.createdAt
	nodeRefs := make(map[graphScopedRef]model.GraphDigest)
	nodesByKey := make(map[model.GraphDigest]graphNodeCandidate)
	docKeys := make(map[string]model.GraphDigest, len(input.docs))
	docSources := make(map[string]model.GraphDigest, len(input.docs))
	docIDs := make(map[string][]string)
	for _, doc := range input.docs {
		identity, err := model.NewNodeKey(model.NodeKeyFields{RepositoryIdentity: input.repositoryIdentity, BackendType: "docgraph", Language: "markdown", ProjectOrModule: ".docs/wiki", OwnerPath: doc.Path, SymbolKind: "document", SemanticIdentity: firstNonEmptyGraph(doc.DocID, doc.Path)})
		if err != nil {
			return model.GraphBundle{}, err
		}
		key, err := identity.Hash()
		if err != nil {
			return model.GraphBundle{}, err
		}
		claim := model.GraphRecordExact
		candidate := graphNodeCandidate{key: key, identity: identity, node: model.GraphObservationNode{Ref: doc.Path, Key: identity, DisplayName: firstNonEmptyGraph(doc.Title, doc.Path), SourceDigest: graphDocSourceDigest(doc), ClaimStatus: claim, Resolution: "docgraph"}}
		if old, ok := nodesByKey[key]; ok && !graphNodeEquivalent(old, candidate) {
			return model.GraphBundle{}, fmt.Errorf("%w: document %s", errGraphAssemblyConflict, doc.Path)
		}
		nodesByKey[key] = candidate
		docKeys[doc.Path] = key
		docSources[doc.Path] = candidate.node.SourceDigest
		if doc.DocID != "" {
			docIDs[doc.DocID] = append(docIDs[doc.DocID], doc.Path)
		}
		nodeRefs[graphScopedRef{batch: -1, ref: doc.Path}] = key
	}
	docUnresolved := make([]graphUnresolvedCandidate, 0)
	nodeRefsByOwner := make(map[string][]model.GraphDigest)
	nodeRefsBySemantic := make(map[string][]model.GraphDigest)
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
	for key, candidate := range nodesByKey {
		if candidate.identity.BackendType == "docgraph" {
			continue
		}
		nodeRefsByOwner[candidate.identity.OwnerPath] = append(nodeRefsByOwner[candidate.identity.OwnerPath], key)
		nodeRefsBySemantic[candidate.identity.SemanticIdentity] = append(nodeRefsBySemantic[candidate.identity.SemanticIdentity], key)
	}
	for path := range nodeRefsByOwner {
		sort.Slice(nodeRefsByOwner[path], func(i, j int) bool { return graphDigestLess(nodeRefsByOwner[path][i], nodeRefsByOwner[path][j]) })
	}
	for semantic := range nodeRefsBySemantic {
		sort.Slice(nodeRefsBySemantic[semantic], func(i, j int) bool {
			return graphDigestLess(nodeRefsBySemantic[semantic][i], nodeRefsBySemantic[semantic][j])
		})
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

	docEvidence := make([]graphEvidenceCandidate, 0)
	addDocEdge := func(ref string, from, to model.GraphDigest, owner, claim string, source model.GraphDigest, observed model.GraphDigest) {
		observation := model.GraphObservationEdge{Ref: ref, Relation: "doc_mentions", Scope: "document", Status: claim, OwnerPath: owner, Backend: "docgraph", Resolution: "docgraph", SourceDigest: source}
		key := model.EdgeKey(from, to, observation.Relation, observation.Scope)
		candidate := graphEdgeCandidate{key: key, from: from, to: to, edge: observation}
		if old, ok := edgesByKey[key]; ok {
			if !graphEdgeEquivalent(old, candidate) {
				return
			}
		} else {
			edgesByKey[key] = candidate
		}
		edgeRefs[graphScopedRef{batch: -1, ref: ref}] = key
		docEvidence = append(docEvidence, graphEvidenceCandidate{subject: key, edgeKey: key, evidence: model.GraphObservationEvidence{Ref: ref, EdgeRef: ref, SourceURI: owner, Backend: "docgraph", ExtractorVersion: "docgraph/v1", SourceDigest: source, ObservedDigest: observed, ClaimKind: "doc_mentions", Status: claim}})
	}
	for index, edge := range input.docEdges {
		from, ok := docKeys[edge.FromPath]
		if !ok {
			continue
		}
		var targetPath string
		if edge.ToPath != "" {
			if _, exists := docKeys[edge.ToPath]; exists {
				targetPath = edge.ToPath
			}
		} else if edge.ToDocID != "" {
			paths := docIDs[edge.ToDocID]
			if len(paths) == 1 {
				targetPath = paths[0]
			} else {
				candidates := append([]string(nil), paths...)
				docUnresolved = append(docUnresolved, graphDocUnresolved(fmt.Sprintf("doc-edge:%d", index), edge.FromPath, "doc_id", edge.ToDocID, map[bool]string{true: "ambiguous_doc_target", false: "missing_doc_target"}[len(paths) > 1], candidates, docSources[edge.FromPath]))
				continue
			}
		}
		if targetPath == "" {
			docUnresolved = append(docUnresolved, graphDocUnresolved(fmt.Sprintf("doc-edge:%d", index), edge.FromPath, "doc_path", firstNonEmptyGraph(edge.ToPath, edge.ToDocID), "missing_doc_target", nil, docSources[edge.FromPath]))
			continue
		}
		to := docKeys[targetPath]
		addDocEdge(fmt.Sprintf("doc-edge:%d", index), from, to, edge.FromPath, model.GraphRecordExact, docSources[edge.FromPath], graphDocClaimDigest(edge.Kind, edge.FromPath, targetPath, edge.Label, docSources[edge.FromPath]))
	}
	for index, mention := range input.docMentions {
		from, ok := docKeys[mention.DocPath]
		if !ok {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(mention.MentionType))
		value := strings.TrimSpace(mention.MentionValue)
		var candidates []model.GraphDigest
		var candidateNames []string
		switch kind {
		case "file_path", "implements", "test_file":
			candidates = append(candidates, nodeRefsByOwner[value]...)
			candidateNames = append(candidateNames, value)
		case "semantic_identity", "typed_semantic_identity":
			candidates = append(candidates, nodeRefsBySemantic[value]...)
			candidateNames = append(candidateNames, value)
		case "symbol":
			if !strings.Contains(value, ":") {
				// Untyped inline prose is intentionally non-positive.
				continue
			}
			candidates = append(candidates, nodeRefsBySemantic[value]...)
			candidateNames = append(candidateNames, value)
		default:
			// Free text, commands, and untyped symbols are intentionally non-positive.
			continue
		}
		if len(candidates) != 1 {
			reason := "missing_code_target"
			if len(candidates) > 1 {
				reason = "ambiguous_code_target"
			}
			if len(candidates) > 0 {
				candidateNames = candidateNames[:0]
				for _, key := range candidates {
					candidateNames = append(candidateNames, nodesByKey[key].identity.OwnerPath)
				}
			}
			docUnresolved = append(docUnresolved, graphDocUnresolved(fmt.Sprintf("doc-mention:%d", index), mention.DocPath, kind, value, reason, candidateNames, docSources[mention.DocPath]))
			continue
		}
		addDocEdge(fmt.Sprintf("doc-mention:%d", index), from, candidates[0], mention.DocPath, model.GraphRecordExtracted, docSources[mention.DocPath], graphDocClaimDigest(kind, mention.DocPath, value, "", docSources[mention.DocPath]))
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
	for _, doc := range input.docs {
		key := docKeys[doc.Path]
		evidence = append(evidence, graphEvidenceCandidate{subject: key, evidence: model.GraphObservationEvidence{Ref: "doc-node:" + doc.Path, NodeRef: doc.Path, SourceURI: doc.Path, Backend: "docgraph", ExtractorVersion: "docgraph/v1", SourceDigest: docSources[doc.Path], ObservedDigest: docSources[doc.Path], ClaimKind: "document", Status: model.GraphRecordExact}})
	}
	evidence = append(evidence, docEvidence...)
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
	unresolved = append(unresolved, docUnresolved...)
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

	repository := input.repositoryIdentity
	bundle := model.GraphBundle{Generation: model.GraphGeneration{SchemaVersion: 1, WorkspaceIdentity: input.workspaceIdentity, RepositoryIdentity: repository, SourceFingerprint: aggregateGraphDigest("MILSP-G3-SOURCE/v1", batches, 1, input.docs, input.docEdges, input.docMentions), ConfigFingerprint: aggregateGraphDigest("MILSP-G3-CONFIG/v1", batches, 2, input.docs, input.docEdges, input.docMentions), BackendManifestDigest: aggregateGraphDigest("MILSP-G3-BACKEND-MANIFEST/v1", batches, 3, input.docs, input.docEdges, input.docMentions), Status: model.GraphGenerationStaged, NodeCount: len(nodes), EdgeCount: len(edges), EvidenceCount: len(evidenceRecords), UnresolvedCount: len(unresolvedRecords), CreatedAt: createdAt}, Nodes: nodes, Edges: edges, Evidence: evidenceRecords, Unresolved: unresolvedRecords}
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
	input, err := prepareGraphInput(req)
	if err != nil {
		return model.GraphBundle{}, err
	}
	return assembleGraphBundle(input)
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

// PublishGraphObservationBatches composes the explicit stage and publish
// operations. StageGraphObservationBatches deliberately never activates a
// generation; a pointer conflict leaves the staged generation invisible.
func PublishGraphObservationBatches(ctx context.Context, db *sql.DB, req GraphAssemblyRequest, expectedPrior *model.GraphDigest) (model.GraphGeneration, error) {
	generation, err := StageGraphObservationBatches(ctx, db, req)
	if err != nil {
		return generation, err
	}
	if err := store.ActivateGraphGenerationAt(ctx, db, generation.GenerationID, expectedPrior, req.CreatedAt); err != nil {
		return generation, err
	}
	return generation, nil
}

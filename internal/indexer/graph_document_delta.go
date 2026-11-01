package indexer

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// GraphDocumentDeltaRequest contains already parsed affected documents and a
// caller-selected, complete lookup context from a pinned read-only base.
// ContextComplete asserts that all candidates for each lookup were supplied;
// the producer cannot establish that assertion without access to the base.
type GraphDocumentDeltaRequest struct {
	Docs               []model.DocRecord
	DocEdges           []model.DocEdge
	DocMentions        []model.DocMention
	AffectedOwnerPaths []string
	ContextDocs        []model.DocRecord
	ContextNodes       []model.GraphNodeRecord
	ContextComplete    bool
	WorkspaceIdentity  string
	RepositoryIdentity string
	CreatedAt          time.Time
}

// GraphDocumentDelta is private assembly IR, not a publishable generation.
// Local IDs connect its rows only; the generation and per-row generation IDs
// are deliberately empty. Context and derived keys require base verification.
type GraphDocumentDelta struct {
	Bundle              model.GraphBundle
	AffectedOwnerPaths  []string
	ContextDocumentKeys []model.GraphDigest
	ContextNodeKeys     []model.GraphDigest
	DerivedNodeSources  map[model.GraphDigest][]model.GraphDigest
}

type graphDocumentAssemblyContext struct {
	nodes    []graphNodeCandidate
	docPaths map[string]bool
	derived  map[model.GraphDigest][]model.GraphDigest
}

// WorktreeGraphDocumentPaths applies the same canonical profile collector used
// by incremental indexing, then returns only physical, graphable Markdown paths.
// The caller's limit bounds the overlay domain; exceeding it is incomplete.
func WorktreeGraphDocumentPaths(ctx context.Context, root string, profile model.DocsReadProfile, extraIgnorePatterns []string, limit int) ([]string, error) {
	if ctx == nil || limit < 1 {
		return nil, fmt.Errorf("explicit_incomplete: invalid canonical document scan bound")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	matcher, err := workspace.LoadIgnoreMatcher(root, extraIgnorePatterns)
	if err != nil {
		return nil, err
	}
	files, _, err := collectCanonicalDocFilesBounded(ctx, root, profile, matcher, defaultCanonicalDocScanLimits())
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if !strings.EqualFold(filepath.Ext(file.relativePath), ".md") || !isCanonicalGraphDoc(file.relativePath, false) {
			continue
		}
		physical, err := filepath.Abs(file.path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, physical)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("explicit_incomplete: canonical document source escapes the worktree")
		}
		rel = filepath.ToSlash(rel)
		if rel != filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.relativePath))) {
			return nil, fmt.Errorf("explicit_incomplete: canonical document path does not match its physical owner")
		}
		paths = append(paths, rel)
		if len(paths) > limit {
			return nil, fmt.Errorf("explicit_incomplete: canonical document domain exceeds bound %d", limit)
		}
	}
	sort.Strings(paths)
	unique := paths[:0]
	for _, path := range paths {
		if len(unique) == 0 || unique[len(unique)-1] != path {
			unique = append(unique, path)
		}
	}
	return unique, nil
}

// AssembleGraphDocumentDelta assembles only the caller's delta and lookup
// anchors. It neither reads a workspace nor proves base membership.
func AssembleGraphDocumentDelta(req GraphDocumentDeltaRequest) (GraphDocumentDelta, error) {
	var out GraphDocumentDelta
	if !req.ContextComplete {
		return out, fmt.Errorf("%w: incomplete document lookup context", errGraphAssemblyInvalid)
	}
	if req.CreatedAt.IsZero() || len(req.AffectedOwnerPaths) == 0 {
		return out, fmt.Errorf("%w: missing time or affected paths", errGraphAssemblyInvalid)
	}
	masks := make(map[string]bool, len(req.AffectedOwnerPaths))
	for _, p := range req.AffectedOwnerPaths {
		if p != strings.TrimSpace(p) || !isCanonicalGraphDoc(p, false) || masks[p] {
			return out, fmt.Errorf("%w: invalid or duplicate affected path %q", errGraphAssemblyInvalid, p)
		}
		masks[p] = true
		out.AffectedOwnerPaths = append(out.AffectedOwnerPaths, p)
	}
	sort.Strings(out.AffectedOwnerPaths)
	ctx := &graphDocumentAssemblyContext{docPaths: map[string]bool{}, derived: map[model.GraphDigest][]model.GraphDigest{}}
	seen := map[string]bool{}
	docs := make([]model.DocRecord, 0, len(req.Docs)+len(req.ContextDocs))
	for _, d := range req.Docs {
		if d.Path != strings.TrimSpace(d.Path) || !masks[d.Path] || !isCanonicalGraphDoc(d.Path, d.IsSnapshot) || seen[d.Path] {
			return out, fmt.Errorf("%w: invalid changed document %q", errGraphAssemblyInvalid, d.Path)
		}
		seen[d.Path] = true
		docs = append(docs, d)
	}
	for _, d := range req.ContextDocs {
		if d.Path != strings.TrimSpace(d.Path) || masks[d.Path] || !isCanonicalGraphDoc(d.Path, d.IsSnapshot) || seen[d.Path] {
			return out, fmt.Errorf("%w: invalid context document %q", errGraphAssemblyInvalid, d.Path)
		}
		seen[d.Path] = true
		ctx.docPaths[d.Path] = true
		docs = append(docs, d)
	}
	for _, e := range req.DocEdges {
		if !masks[e.FromPath] || !seen[e.FromPath] {
			return out, fmt.Errorf("%w: edge source %q", errGraphAssemblyInvalid, e.FromPath)
		}
	}
	for _, m := range req.DocMentions {
		if !masks[m.DocPath] || !seen[m.DocPath] {
			return out, fmt.Errorf("%w: mention source %q", errGraphAssemblyInvalid, m.DocPath)
		}
	}
	repository, err := model.NormalizeRepositoryIdentity(req.RepositoryIdentity)
	if err != nil {
		return out, err
	}
	if req.WorkspaceIdentity != repository {
		return out, fmt.Errorf("%w: workspace identity", errGraphAssemblyInvalid)
	}
	nodeKeys := map[model.GraphDigest]bool{}
	for _, n := range req.ContextNodes {
		identity, err := model.NewNodeKey(n.Identity)
		if err != nil {
			return out, err
		}
		key, err := identity.Hash()
		if err != nil {
			return out, err
		}
		if identity != n.Identity || identity.RepositoryIdentity != repository || identity.BackendType == "docgraph" || masks[identity.OwnerPath] || n.NodeKey != key || n.IdentitySchema != "milsp-node-key/v1" || n.SourceDigest == (model.GraphDigest{}) || (n.ClaimStatus != model.GraphRecordExact && n.ClaimStatus != model.GraphRecordExtracted) || nodeKeys[key] {
			return out, fmt.Errorf("%w: invalid or duplicate context node %s", errGraphAssemblyInvalid, key.String())
		}
		nodeKeys[key] = true
		ctx.nodes = append(ctx.nodes, graphNodeCandidate{key: key, identity: identity, node: model.GraphObservationNode{Key: identity, DisplayName: n.DisplayName, SourceDigest: n.SourceDigest, ClaimStatus: n.ClaimStatus}})
		out.ContextNodeKeys = append(out.ContextNodeKeys, key)
	}
	// A derived file identity cannot silently select among conflicting source claims.
	fileSources := map[model.GraphDigest]model.GraphDigest{}
	for _, candidate := range ctx.nodes {
		id := candidate.identity
		if id.SymbolKind == "file" {
			continue
		}
		id.SymbolKind, id.SemanticIdentity = "file", id.OwnerPath
		key, err := id.Hash()
		if err != nil {
			return out, err
		}
		if source, ok := fileSources[key]; ok && source != candidate.node.SourceDigest {
			return out, fmt.Errorf("%w: conflicting derived file source %s", errGraphAssemblyConflict, key.String())
		}
		fileSources[key] = candidate.node.SourceDigest
	}
	var input graphAssemblyInput
	if len(docs) > 0 {
		input, err = prepareGraphInput(GraphAssemblyRequest{Docs: docs, DocEdges: req.DocEdges, DocMentions: req.DocMentions, WorkspaceIdentity: req.WorkspaceIdentity, RepositoryIdentity: repository, CreatedAt: req.CreatedAt})
		if err != nil {
			return out, err
		}
	} else {
		input = graphAssemblyInput{workspaceIdentity: req.WorkspaceIdentity, repositoryIdentity: repository, createdAt: req.CreatedAt.Round(0).UTC()}
	}
	bundle, err := assembleGraphBundleWithContext(input, ctx)
	if err != nil {
		return out, err
	}
	for _, n := range bundle.Nodes {
		if ctx.docPaths[n.Identity.OwnerPath] && n.Identity.BackendType == "docgraph" {
			out.ContextDocumentKeys = append(out.ContextDocumentKeys, n.NodeKey)
		}
	}
	for key, sources := range ctx.derived {
		if nodeKeys[key] {
			continue
		}
		sort.Slice(sources, func(i, j int) bool { return graphDigestLess(sources[i], sources[j]) })
		ctx.derived[key] = sources
	}
	for key := range nodeKeys {
		delete(ctx.derived, key)
	}
	sort.Slice(out.ContextNodeKeys, func(i, j int) bool { return graphDigestLess(out.ContextNodeKeys[i], out.ContextNodeKeys[j]) })
	sort.Slice(out.ContextDocumentKeys, func(i, j int) bool { return graphDigestLess(out.ContextDocumentKeys[i], out.ContextDocumentKeys[j]) })
	out.Bundle = bundle
	out.DerivedNodeSources = ctx.derived
	return out, nil
}

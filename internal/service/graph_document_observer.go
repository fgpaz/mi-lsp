package service

import (
	"context"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// observeWorktreeGraphDocuments builds private document IR from exact physical
// selectors and exact pinned lookup queries. The caller must first close the
// affected-owner frontier (including incoming document dependencies), supply a
// validated profile and native masks, and recheck the physical fingerprint.
// This helper neither publishes a graph nor installs a public overlay route.
func observeWorktreeGraphDocuments(ctx context.Context, base *store.GraphQuerySnapshot, root string, paths, nativeMasks []string, batches []model.GraphObservationBatch, profileInput graphDocumentProfileInput, createdAt time.Time) (store.GraphDocumentDelta, error) {
	var empty store.GraphDocumentDelta
	generation, err := base.Generation()
	if err != nil {
		return empty, err
	}
	if !base.SnapshotFreshness(generation.GenerationID.String()).AllowsExactClaims() {
		return empty, overlayGraphUnavailable("document lookup base is not exact")
	}
	if err := profileInput.recheck(ctx, root); err != nil {
		return empty, err
	}
	parsed, err := parseWorktreeGraphDocuments(ctx, root, paths, profileInput.Profile)
	if err != nil {
		return empty, err
	}
	docPaths, docIDs, codePaths, semanticIDs := append([]string(nil), paths...), []string{}, []string{}, []string{}
	for _, doc := range parsed.Docs {
		if doc.DocID != "" {
			docIDs = append(docIDs, doc.DocID)
		}
	}
	for _, edge := range parsed.Edges {
		if edge.Kind == "code" {
			if edge.ToPath != "" {
				codePaths = append(codePaths, edge.ToPath)
			}
			continue
		}
		if edge.ToDocID != "" {
			docIDs = append(docIDs, edge.ToDocID)
		}
	}
	for _, mention := range parsed.Mentions {
		value := strings.TrimSpace(mention.MentionValue)
		if value == "" {
			continue
		}
		// TP IDs and implements-document bindings are resolved by the canonical
		// assembler. Query every exact mention value rather than guessing its ID grammar.
		docIDs = append(docIDs, value)
		switch strings.ToLower(strings.TrimSpace(mention.MentionType)) {
		case "file_path", "implements", "test_file":
			codePaths = append(codePaths, value)
			docPaths = append(docPaths, value)
		case "semantic_identity", "typed_semantic_identity":
			semanticIDs = append(semanticIDs, value)
		case "symbol":
			if strings.Contains(value, ":") {
				semanticIDs = append(semanticIDs, value)
			}
		}
	}
	// Resolve raw relative/root/basename paths with the canonical resolver,
	// not by interpreting an exact-query miss as a missing document.
	basenames, fallbackIDs := docgraph.DocumentEdgeLookupPlan(parsed.Edges)
	candidatePaths, err := base.WorktreeDocumentBasenamePaths(ctx, basenames, 4096)
	if err != nil {
		return empty, err
	}
	docPaths = graphDeltaPathUnion(docPaths, candidatePaths)
	docIDs = graphDeltaPathUnion(docIDs, fallbackIDs)
	baseDocs, err := base.WorktreeDocumentContextNodes(ctx, docPaths, docIDs, 4096)
	if err != nil {
		return empty, err
	}
	masked, oldDocs := map[string]bool{}, map[string]bool{}
	for _, path := range paths {
		masked[path] = true
	}
	var contextDocs []model.DocRecord
	for _, node := range baseDocs {
		oldDocs[node.Identity.OwnerPath] = true
		if masked[node.Identity.OwnerPath] {
			continue
		}
		contextDocs = append(contextDocs, model.DocRecord{Path: node.Identity.OwnerPath, DocID: node.Identity.SemanticIdentity, Title: node.DisplayName, ContentHash: node.SourceDigest.String()})
	}
	for _, path := range parsed.MissingPaths {
		if !oldDocs[path] {
			return empty, overlayGraphUnavailable("missing document has no pinned document-owner proof")
		}
	}
	resolutionDocs := append(append([]model.DocRecord(nil), contextDocs...), parsed.Docs...)
	parsed.Edges = docgraph.ResolveDocEdges(resolutionDocs, parsed.Edges, profileInput.Profile)
	baseCode, err := base.WorktreeCodeContextNodes(ctx, graphDeltaPathUnion(codePaths), graphDeltaPathUnion(semanticIDs), 4096)
	if err != nil {
		return empty, err
	}
	for _, path := range nativeMasks {
		masked[path] = true
	}
	contextNodes := []model.GraphNodeRecord{}
	seen := map[model.GraphDigest]model.GraphNodeRecord{}
	add := func(node model.GraphNodeRecord) error {
		if previous, ok := seen[node.NodeKey]; ok {
			if previous.Identity != node.Identity || previous.SourceDigest != node.SourceDigest || previous.ClaimStatus != node.ClaimStatus || previous.DisplayName != node.DisplayName {
				return overlayGraphUnavailable("document lookup has conflicting native identities")
			}
			return nil
		}
		seen[node.NodeKey] = node
		contextNodes = append(contextNodes, node)
		return nil
	}
	for _, node := range baseCode {
		if !masked[node.Identity.OwnerPath] {
			if err := add(node); err != nil {
				return empty, err
			}
		}
	}
	wantedPaths, wantedSemantics := map[string]bool{}, map[string]bool{}
	for _, path := range codePaths {
		wantedPaths[path] = true
	}
	for _, semantic := range semanticIDs {
		wantedSemantics[semantic] = true
	}
	for _, batch := range batches {
		if err := batch.ReadyForStaging(); err != nil {
			return empty, err
		}
		for _, observed := range batch.Nodes {
			if !wantedPaths[observed.Key.OwnerPath] && !wantedSemantics[observed.Key.SemanticIdentity] {
				continue
			}
			identity, err := model.NewNodeKey(observed.Key)
			if err != nil {
				return empty, err
			}
			key, err := identity.Hash()
			if err != nil {
				return empty, err
			}
			node := model.GraphNodeRecord{NodeKey: key, IdentitySchema: "milsp-node-key/v1", Identity: identity, DisplayName: observed.DisplayName, SourceDigest: observed.SourceDigest, ClaimStatus: observed.ClaimStatus}
			if err := add(node); err != nil {
				return empty, err
			}
		}
	}
	ir, err := indexer.AssembleGraphDocumentDelta(indexer.GraphDocumentDeltaRequest{
		WorkspaceIdentity: generation.WorkspaceIdentity, RepositoryIdentity: generation.RepositoryIdentity,
		CreatedAt: createdAt, AffectedOwnerPaths: paths, Docs: parsed.Docs, DocEdges: parsed.Edges, DocMentions: parsed.Mentions,
		ContextDocs: contextDocs, ContextNodes: contextNodes, ContextComplete: true,
	})
	if err != nil {
		return empty, err
	}
	if err := profileInput.recheck(ctx, root); err != nil {
		return empty, err
	}
	return store.GraphDocumentDelta{Bundle: ir.Bundle, AffectedOwnerPaths: ir.AffectedOwnerPaths, ContextDocumentKeys: ir.ContextDocumentKeys, ContextNodeKeys: ir.ContextNodeKeys, DerivedNodeSources: ir.DerivedNodeSources}, nil
}

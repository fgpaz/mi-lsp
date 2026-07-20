package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// BuildWikiCodeContext builds a deterministic, query-only wiki/code context.
// The caller must provide the already-selected primary document; this function
// never ranks or substitutes a primary document.
func BuildWikiCodeContext(ctx context.Context, db *sql.DB, primary model.DocRecord, tokenBudget int) (model.WikiCodeContext, error) {
	if ctx == nil || db == nil {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_BACKEND_UNAVAILABLE", "workspace database is unavailable")
	}
	if tokenBudget < 1 {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_BUDGET_EXCEEDED", "token budget must be positive")
	}
	if strings.TrimSpace(primary.Path) == "" || !model.CanonicalWikiAuthority(primary.Path) || primary.IsSnapshot {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_PRIMARY_INVALID", "primary document must be an active canonical wiki document")
	}

	generation, _, _ := store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveDocsGeneration)
	ctxResult := model.WikiCodeContext{
		PrimaryDoc:      primary,
		DocGenerationID: generation,
		TokenBudget:     tokenBudget,
		AuthorityChain:  make([]model.WikiCodeAuthorityEntry, 0, 3),
		CodeEvidence:    make([]model.WikiCodeEvidence, 0),
		GraphPaths:      make([]model.WikiCodeGraphPath, 0),
		Drift:           make([]model.WikiCodeDrift, 0),
		Omissions:       make([]model.WikiCodeContextOmission, 0),
		Provenance:      model.WikiCodeProvenance{Backend: "sqlite-direct", DocsGeneration: generation, QueryOnly: true},
	}

	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return model.WikiCodeContext{}, err
	}
	mentions, _ := store.DocMentionsForPath(ctx, db, primary.Path)

	byPath := make(map[string]model.DocRecord, len(docs))
	byID := make(map[string][]model.DocRecord, len(docs))
	for _, doc := range docs {
		byPath[doc.Path] = doc
		if doc.DocID != "" {
			byID[strings.ToUpper(doc.DocID)] = append(byID[strings.ToUpper(doc.DocID)], doc)
		}
	}

	// Governance is always first; the explicit primary is always retained and
	// is never replaced by a connected document.
	if governance, ok := governanceDoc(docs); ok && governance.Path != primary.Path {
		ctxResult.AuthorityChain = append(ctxResult.AuthorityChain, authorityEntry(governance, "governance"))
	}
	ctxResult.AuthorityChain = append(ctxResult.AuthorityChain, authorityEntry(primary, "primary"))

	snapshot, graphErr := store.BeginGraphQuerySnapshot(ctx, db, "")
	if graphErr != nil {
		code := "GPH_WIKI_GRAPH_STALE"
		if strings.Contains(strings.ToLower(graphErr.Error()), "not found") || strings.Contains(strings.ToLower(graphErr.Error()), "generation_not_found") {
			code = "GPH_WIKI_GRAPH_UNAVAILABLE"
		}
		ctxResult.Omissions = append(ctxResult.Omissions, model.WikiCodeContextOmission{Code: code, Source: primary.Path, Reason: "docs-first context retained; active code graph is unavailable"})
	} else {
		defer snapshot.Close()
		graphGeneration, err := snapshot.Generation()
		if err != nil {
			return model.WikiCodeContext{}, err
		}
		ctxResult.CodeGenerationID = graphGeneration.GenerationID.String()
		ctxResult.Provenance.GraphGeneration = ctxResult.CodeGenerationID
		primaryNode, err := resolvePrimaryGraphNode(ctx, snapshot, primary)
		if err != nil {
			return model.WikiCodeContext{}, err
		}
		if primaryNode == nil {
			ctxResult.Omissions = append(ctxResult.Omissions, model.WikiCodeContextOmission{Code: "GPH_WIKI_PRIMARY_GRAPH_MISSING", Source: primary.Path, Reason: "primary document is canonical but has no graph node"})
		} else {
			edges, err := snapshot.Edges(ctx, []int{primaryNode.NodeID}, "out", []string{"doc_mentions"}, 128)
			if err != nil {
				return model.WikiCodeContext{}, err
			}
			seenTargets := make(map[string]struct{})
			for _, edge := range edges {
				target, err := snapshot.Node(ctx, edge.ToNodeID)
				if err != nil {
					return model.WikiCodeContext{}, err
				}
				refs, err := snapshot.EvidenceRefs(ctx, nil, &edge.EdgeID, 16)
				if err != nil {
					return model.WikiCodeContext{}, err
				}
				from := primary.Path
				to := target.Identity.OwnerPath
				if target.Identity.SymbolKind == "document" {
					doc := byPath[to]
					if doc.Path == "" && target.Identity.SemanticIdentity != "" {
						matches := byID[strings.ToUpper(target.Identity.SemanticIdentity)]
						if len(matches) == 1 {
							doc = matches[0]
						}
					}
					if doc.Path == "" {
						ctxResult.Omissions = append(ctxResult.Omissions, model.WikiCodeContextOmission{Code: "GPH_WIKI_CODE_AMBIGUOUS", Source: primary.Path, Reason: "document target is not uniquely resolvable", Candidates: []string{to}})
					} else if model.CanonicalWikiAuthority(doc.Path) && !doc.IsSnapshot && doc.Path != primary.Path {
						if _, ok := seenTargets[doc.Path]; !ok {
							seenTargets[doc.Path] = struct{}{}
							ctxResult.AuthorityChain = append(ctxResult.AuthorityChain, authorityEntry(doc, "related"))
						}
					}
					continue
				}
				ctxResult.GraphPaths = append(ctxResult.GraphPaths, model.WikiCodeGraphPath{From: from, To: to, Relation: edge.Relation, ClaimStatus: edge.ClaimStatus, EdgeRef: edge.CrossRID, EvidenceRefs: refs})
				ctxResult.CodeEvidence = append(ctxResult.CodeEvidence, model.WikiCodeEvidence{Path: target.Identity.OwnerPath, Symbol: target.Identity.SemanticIdentity, Kind: target.Identity.SymbolKind, Language: target.Identity.Language, ClaimStatus: target.ClaimStatus, SourceDigest: target.SourceDigest.String(), EvidenceRefs: refs})
			}
			appendUnresolvedMentionOmissions(ctx, primary, mentions, snapshot, &ctxResult)
		}
	}

	model.SortWikiCodeContext(&ctxResult)
	applyWikiCodeBudget(&ctxResult)
	ctxResult.DeterminismDigest = model.WikiCodeContextDigest(ctxResult)
	return ctxResult, nil
}

// NewWikiCodeContext is a compatibility-oriented constructor alias for callers
// that prefer constructor naming; it has the same query-only contract.
func NewWikiCodeContext(ctx context.Context, db *sql.DB, primary model.DocRecord, tokenBudget int) (model.WikiCodeContext, error) {
	return BuildWikiCodeContext(ctx, db, primary, tokenBudget)
}

// BuildWikiCodeContextForPath resolves only the explicit path supplied by the
// caller. It does not rank or substitute another document.
func BuildWikiCodeContextForPath(ctx context.Context, db *sql.DB, path string, tokenBudget int) (model.WikiCodeContext, error) {
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return model.WikiCodeContext{}, err
	}
	for _, doc := range docs {
		if doc.Path == path {
			return BuildWikiCodeContext(ctx, db, doc, tokenBudget)
		}
	}
	return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_PRIMARY_NOT_FOUND", "explicit primary document path is not indexed")
}

func authorityEntry(doc model.DocRecord, role string) model.WikiCodeAuthorityEntry {
	return model.WikiCodeAuthorityEntry{DocID: doc.DocID, Path: doc.Path, Layer: doc.Layer, Role: role, ContentHash: doc.ContentHash}
}

func governanceDoc(docs []model.DocRecord) (model.DocRecord, bool) {
	for _, doc := range docs {
		if doc.Path == ".docs/wiki/00_gobierno_documental.md" && model.CanonicalWikiAuthority(doc.Path) && !doc.IsSnapshot {
			return doc, true
		}
	}
	for _, doc := range docs {
		if doc.Layer == "00" && model.CanonicalWikiAuthority(doc.Path) && !doc.IsSnapshot {
			return doc, true
		}
	}
	return model.DocRecord{}, false
}

func resolvePrimaryGraphNode(ctx context.Context, snapshot *store.GraphQuerySnapshot, primary model.DocRecord) (*model.GraphNodeRecord, error) {
	selectors := []string{primary.DocID, primary.Path}
	for _, selector := range selectors {
		if strings.TrimSpace(selector) == "" {
			continue
		}
		nodes, _, err := snapshot.ResolveGraphSelector(ctx, selector)
		if err != nil {
			return nil, err
		}
		if len(nodes) > 1 {
			return nil, model.NewWikiCodeContextError("GPH_WIKI_PRIMARY_AMBIGUOUS", "primary document graph selector is ambiguous")
		}
		if len(nodes) == 1 {
			return &nodes[0], nil
		}
	}
	return nil, nil
}

func appendUnresolvedMentionOmissions(ctx context.Context, primary model.DocRecord, mentions []model.DocMention, snapshot *store.GraphQuerySnapshot, result *model.WikiCodeContext) {
	for _, mention := range mentions {
		if !isCodeMention(mention.MentionType) || wikiCodeMentionResolved(result.CodeEvidence, mention) {
			continue
		}
		nodes, _, _ := snapshot.ResolveGraphSelector(ctx, mention.MentionValue)
		if len(nodes) <= 1 {
			if len(nodes) == 0 {
				result.Omissions = append(result.Omissions, model.WikiCodeContextOmission{Code: "GPH_WIKI_CODE_MISSING", Source: mention.MentionValue, Reason: "typed code mention has no resolved graph target"})
			}
			continue
		}
		candidates := make([]string, 0, len(nodes))
		for _, node := range nodes {
			candidates = append(candidates, node.Identity.OwnerPath)
		}
		result.Omissions = append(result.Omissions, model.WikiCodeContextOmission{Code: "GPH_WIKI_CODE_AMBIGUOUS", Source: mention.MentionValue, Reason: "typed code mention has multiple graph targets", Candidates: candidates})
	}
}

func wikiCodeMentionResolved(evidence []model.WikiCodeEvidence, mention model.DocMention) bool {
	value := strings.TrimSpace(mention.MentionValue)
	for _, item := range evidence {
		if item.Path == value || item.Symbol == value {
			return true
		}
	}
	return false
}

func isCodeMention(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "file_path", "implements", "test_file", "semantic_identity", "typed_semantic_identity", "symbol":
		return true
	default:
		return false
	}
}

func applyWikiCodeBudget(c *model.WikiCodeContext) {
	if c == nil {
		return
	}
	mandatoryAuthority := make([]model.WikiCodeAuthorityEntry, 0, len(c.AuthorityChain))
	optionalAuthority := make([]model.WikiCodeAuthorityEntry, 0, len(c.AuthorityChain))
	for _, entry := range c.AuthorityChain {
		if entry.Role == "related" {
			optionalAuthority = append(optionalAuthority, entry)
		} else {
			mandatoryAuthority = append(mandatoryAuthority, entry)
		}
	}
	mandatory := struct {
		PrimaryDoc model.DocRecord                `json:"primary_doc"`
		Authority  []model.WikiCodeAuthorityEntry `json:"authority_chain"`
	}{c.PrimaryDoc, mandatoryAuthority}
	mandatoryBytes, _ := json.Marshal(mandatory)
	mandatoryTokens := (len(mandatoryBytes) + 3) / 4
	if mandatoryTokens > c.TokenBudget {
		c.Truncated = true
		c.TokenUsed = mandatoryTokens
		c.Omissions = append(c.Omissions, model.WikiCodeContextOmission{Code: "GPH_WIKI_BUDGET_EXCEEDED", Source: c.PrimaryDoc.Path, Reason: fmt.Sprintf("BLOCKED: mandatory authority chain requires %d tokens", mandatoryTokens)})
		c.AuthorityChain = mandatoryAuthority
		return
	}
	remaining := c.TokenBudget - mandatoryTokens
	selectedAuthority := make([]model.WikiCodeAuthorityEntry, 0, len(optionalAuthority))
	for _, entry := range optionalAuthority {
		b, _ := json.Marshal(entry)
		cost := (len(b) + 3) / 4
		if cost > remaining {
			c.Truncated = true
			break
		}
		remaining -= cost
		selectedAuthority = append(selectedAuthority, entry)
	}
	c.AuthorityChain = append(append([]model.WikiCodeAuthorityEntry(nil), mandatoryAuthority...), selectedAuthority...)
	for i := 0; i < len(c.CodeEvidence); {
		b, _ := json.Marshal(c.CodeEvidence[i])
		cost := (len(b) + 3) / 4
		if cost > remaining {
			c.CodeEvidence = c.CodeEvidence[:i]
			c.Truncated = true
			break
		}
		remaining -= cost
		i++
	}
	for i := 0; i < len(c.GraphPaths); {
		b, _ := json.Marshal(c.GraphPaths[i])
		cost := (len(b) + 3) / 4
		if cost > remaining {
			c.GraphPaths = c.GraphPaths[:i]
			c.Truncated = true
			break
		}
		remaining -= cost
		i++
	}
	c.TokenUsed = c.TokenBudget - remaining
	if c.Truncated {
		c.Omissions = append(c.Omissions, model.WikiCodeContextOmission{Code: "GPH_WIKI_BUDGET_EXCEEDED", Source: c.PrimaryDoc.Path, Reason: "optional graph evidence omitted after mandatory authority"})
	}
}

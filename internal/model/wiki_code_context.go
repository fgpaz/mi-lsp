package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// WikiCodeContext is the bounded, docs-first projection of one canonical wiki
// document and its explicitly resolved graph evidence. It intentionally has no
// log or raw-artifact fields.
type WikiCodeContext struct {
	PrimaryDoc        DocRecord                 `json:"primary_doc"`
	AuthorityChain    []WikiCodeAuthorityEntry  `json:"authority_chain"`
	CodeEvidence      []WikiCodeEvidence        `json:"code_evidence"`
	GraphPaths        []WikiCodeGraphPath       `json:"graph_paths"`
	Drift             []WikiCodeDrift           `json:"drift"`
	Omissions         []WikiCodeContextOmission `json:"omissions"`
	// DirectCode and Tests are populated by the shared bidirectional resolver.
	// They remain separate from CodeEvidence, which is the compatibility field
	// used by the original docs-first projection.
	DirectCode      []WikiCodeEvidence        `json:"direct_code,omitempty"`
	Tests           []WikiCodeEvidence        `json:"tests,omitempty"`
	SupportingCode  []WikiCodeEvidence        `json:"supporting_code,omitempty"`
	Candidates      []WikiCodeEvidence        `json:"candidates,omitempty"`
	WikiContext     []WikiCodeWikiContextItem `json:"wiki_context,omitempty"`
	Direction       string                     `json:"direction,omitempty"`
	Freshness       WikiCodeFreshness          `json:"freshness"`
	OverlayDigest   string                     `json:"overlay_digest,omitempty"`
	Cost            WikiCodeResolveCost        `json:"cost,omitempty"`
	Classification  string                     `json:"classification,omitempty"`
	Classifications []string                   `json:"classifications,omitempty"`
	NextQueries     []string                   `json:"next_queries,omitempty"`
	NextCursor      string                     `json:"next_cursor,omitempty"`
	Continuation    *WikiCodeContextContinuation `json:"continuation,omitempty"`
	DocGenerationID   string                   `json:"doc_generation_id,omitempty"`
	CodeGenerationID  string                   `json:"code_generation_id,omitempty"`
	Provenance        WikiCodeProvenance        `json:"provenance"`
	TokenBudget       int                       `json:"token_budget"`
	TokenUsed         int                       `json:"token_used"`
	Truncated         bool                      `json:"truncated"`
	DeterminismDigest string                    `json:"determinism_digest"`
}

type WikiCodeAuthorityEntry struct {
	DocID       string `json:"doc_id,omitempty"`
	Path        string `json:"path"`
	Layer       string `json:"layer,omitempty"`
	Role        string `json:"role"`
	ContentHash string `json:"content_hash,omitempty"`
}

type WikiCodeEvidence struct {
	Path         string   `json:"path,omitempty"`
	Symbol       string   `json:"symbol,omitempty"`
	Kind         string   `json:"kind,omitempty"`
	Language     string   `json:"language,omitempty"`
	ClaimStatus  string   `json:"claim_status,omitempty"`
	SourceDigest string   `json:"source_digest,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	// The following fields are additive bridge metadata. The original graph
	// projection above remains valid when these fields are empty.
	Origin            string `json:"origin,omitempty"`
	AuthoringOrigin   string `json:"authoring_origin,omitempty"`
	ObservedOrigin    string `json:"observed_origin,omitempty"`
	Status            string `json:"status,omitempty"`
	ResolutionStatus  string `json:"resolution_status,omitempty"`
	DocID             string `json:"doc_id,omitempty"`
	DocPath           string `json:"doc_path,omitempty"`
	BlockID           string `json:"block_id,omitempty"`
	Relation          string `json:"relation,omitempty"`
	Role              string `json:"role,omitempty"`
	BindingRef        string `json:"binding_ref,omitempty"`
	TargetKind        string `json:"target_kind,omitempty"`
	StartLine         int    `json:"start_line,omitempty"`
	EndLine           int    `json:"end_line,omitempty"`
	// Source* aliases match the bridge contract while Start/EndLine preserve
	// the existing range vocabulary.
	SourceDoc         string `json:"source_doc,omitempty"`
	SourceBlock       string `json:"source_block,omitempty"`
	SourceLine        int    `json:"source_line,omitempty"`
	TargetHash        string `json:"target_hash,omitempty"`
	Classification    string `json:"classification,omitempty"`
}

type WikiCodeGraphPath struct {
	From         string   `json:"from"`
	To           string   `json:"to"`
	Relation     string   `json:"relation"`
	ClaimStatus  string   `json:"claim_status,omitempty"`
	EdgeRef      string   `json:"edge_ref,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type WikiCodeDrift struct {
	Code        string `json:"code"`
	Source      string `json:"source"`
	Target      string `json:"target,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Description string `json:"description,omitempty"`
}

type WikiCodeContextOmission struct {
	Code       string   `json:"code"`
	Source     string   `json:"source,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Status     string   `json:"status,omitempty"`
	DocID      string   `json:"doc_id,omitempty"`
	DocPath    string   `json:"doc_path,omitempty"`
	BlockID    string   `json:"block_id,omitempty"`
	BindingRef string   `json:"binding_ref,omitempty"`
}

// WikiCodeWikiContextItem is the canonical reverse code→wiki item. Parents
// are documentary links already present in the doc graph; they are not
// inferred from code or lexical similarity.
type WikiCodeWikiContextItem struct {
	DocID        string                     `json:"doc_id,omitempty"`
	Path         string                     `json:"path"`
	BlockID      string                     `json:"block_id,omitempty"`
	Relation     string                     `json:"relation,omitempty"`
	Role         string                     `json:"role,omitempty"`
	BindingRef   string                     `json:"binding_ref,omitempty"`
	Status       string                     `json:"status,omitempty"`
	SupersededBy string                     `json:"superseded_by,omitempty"`
	StartLine    int                        `json:"start_line,omitempty"`
	EndLine      int                        `json:"end_line,omitempty"`
	Origin         string                     `json:"origin,omitempty"`
	AuthoringOrigin string                    `json:"authoring_origin,omitempty"`
	Classification string                     `json:"classification,omitempty"`
	Parents      []WikiCodeWikiContextItem  `json:"parents,omitempty"`
}

// Compatibility aliases keep the bridge item discoverable to callers that use
// the more general context/resolution vocabulary.
type WikiCodeContextItem = WikiCodeWikiContextItem
type WikiCodeReverseContextItem = WikiCodeWikiContextItem
type WikiCodeBindingResolution = WikiCodeEvidence

// WikiCodeResolveCost is diagnostic telemetry. It is deliberately excluded
// from WikiCodeContextDigest because filesystem/catalog work can vary while
// the canonical result remains the same.
type WikiCodeResolveCost struct {
	BindingsExamined  int   `json:"bindings_examined,omitempty"`
	FilesChecked      int   `json:"files_checked,omitempty"`
	FilesHashed       int   `json:"files_hashed,omitempty"`
	FilesParsed       int   `json:"files_parsed,omitempty"`
	MetadataChecked   int   `json:"metadata_checked,omitempty"`
	UnchangedReused   int   `json:"unchanged_reused,omitempty"`
	BytesRead         int64 `json:"bytes_read,omitempty"`
	SymbolsChecked    int   `json:"symbols_checked,omitempty"`
	CatalogQueries    int   `json:"catalog_queries,omitempty"`
	GraphNodesVisited int   `json:"graph_nodes_visited,omitempty"`
	GraphEdgesVisited int   `json:"graph_edges_visited,omitempty"`
}

type WikiCodeContextContinuation struct {
	Direction string `json:"direction"`
	Cursor    string `json:"cursor"`
	Remaining int    `json:"remaining,omitempty"`
}

// Stable resolver directions.
const (
	WikiCodeDirectionWikiToCode = "wiki_to_code"
	WikiCodeDirectionCodeToWiki = "code_to_wiki"
)

// Resolution statuses are intentionally closed. They describe resolution
// state only; they are not an AE verdict.
const (
	WikiCodeStatusResolvedSymbol   = "resolved_symbol"
	WikiCodeStatusResolvedFile     = "resolved_file"
	WikiCodeStatusMissingPath      = "missing_path"
	WikiCodeStatusMissingSymbol    = "missing_symbol"
	WikiCodeStatusAmbiguousSymbol  = "ambiguous_symbol"
	WikiCodeStatusCatalogUnavailable = "catalog_unavailable"
	WikiCodeStatusGraphUnavailable = "graph_unavailable"
	WikiCodeStatusGraphStale       = "graph_stale"
	WikiCodeStatusUnsafeTarget     = "unsafe_target"
	WikiCodeStatusConcurrentChange = "concurrent_change"
	WikiCodeStatusSelfEdgeRejected = "self_edge_rejected"
)

// Closed AE classification labels exposed as data for downstream consumers.
const (
	WikiCodeClassificationDirectSDDBinding    = "direct_sdd_binding"
	WikiCodeClassificationSupportingCode      = "supporting_code"
	WikiCodeClassificationSharedTechnical     = "shared_technical_binding"
	WikiCodeClassificationGeneratedOrVendor   = "generated_or_vendor"
	WikiCodeClassificationMechanicalNoImpact  = "mechanical_no_sdd_impact"
	WikiCodeClassificationUnmappedChangedCode = "unmapped_changed_code"

	// Descriptive aliases used by downstream adapters.
	WikiCodeClassificationSharedTechnicalBinding = WikiCodeClassificationSharedTechnical
	WikiCodeClassificationMechanicalNoSDDImpact  = WikiCodeClassificationMechanicalNoImpact
)

type WikiCodeProvenance struct {
	Backend         string `json:"backend"`
	DocsGeneration  string `json:"docs_generation,omitempty"`
	GraphGeneration string `json:"graph_generation,omitempty"`
	OverlayDigest   string `json:"overlay_digest,omitempty"`
	QueryOnly       bool   `json:"query_only"`
}

var ErrWikiCodeContextInvalid = errors.New("invalid wiki-code context")

type WikiCodeContextError struct {
	Code    string
	Message string
}

func (e *WikiCodeContextError) Error() string { return e.Code + ": " + e.Message }

func NewWikiCodeContextError(code, message string) error {
	return &WikiCodeContextError{Code: code, Message: message}
}

// WikiCodeContextDigest returns the stable v1 digest. Budget accounting,
// continuation state, cost telemetry, and elapsed time are deliberately
// excluded from the digest. New bridge collections are canonicalized before
// hashing so equivalent input order produces one digest.
func WikiCodeContextDigest(c WikiCodeContext) string {
	copyContext := cloneWikiCodeContext(c)
	SortWikiCodeContext(&copyContext)
	copyContext.TokenBudget = 0
	copyContext.TokenUsed = 0
	copyContext.Truncated = false
	copyContext.DeterminismDigest = ""
	copyContext.Cost = WikiCodeResolveCost{}
	copyContext.NextCursor = ""
	copyContext.Continuation = nil
	b, _ := json.Marshal(copyContext)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func cloneWikiCodeContext(c WikiCodeContext) WikiCodeContext {
	copyContext := c
	copyContext.AuthorityChain = append([]WikiCodeAuthorityEntry(nil), c.AuthorityChain...)
	copyContext.CodeEvidence = cloneWikiCodeEvidence(c.CodeEvidence)
	copyContext.DirectCode = cloneWikiCodeEvidence(c.DirectCode)
	copyContext.Tests = cloneWikiCodeEvidence(c.Tests)
	copyContext.SupportingCode = cloneWikiCodeEvidence(c.SupportingCode)
	copyContext.Candidates = cloneWikiCodeEvidence(c.Candidates)
	copyContext.GraphPaths = append([]WikiCodeGraphPath(nil), c.GraphPaths...)
	for i := range copyContext.GraphPaths {
		copyContext.GraphPaths[i].EvidenceRefs = append([]string(nil), c.GraphPaths[i].EvidenceRefs...)
	}
	copyContext.Drift = append([]WikiCodeDrift(nil), c.Drift...)
	copyContext.Omissions = append([]WikiCodeContextOmission(nil), c.Omissions...)
	for i := range copyContext.Omissions {
		copyContext.Omissions[i].Candidates = append([]string(nil), c.Omissions[i].Candidates...)
	}
	copyContext.WikiContext = cloneWikiCodeWikiContext(c.WikiContext)
	if c.Classifications != nil {
		copyContext.Classifications = append([]string(nil), c.Classifications...)
	}
	if c.NextQueries != nil {
		copyContext.NextQueries = append([]string(nil), c.NextQueries...)
	}
	return copyContext
}

func cloneWikiCodeEvidence(items []WikiCodeEvidence) []WikiCodeEvidence {
	if items == nil {
		return nil
	}
	out := append([]WikiCodeEvidence(nil), items...)
	for i := range out {
		out[i].EvidenceRefs = append([]string(nil), items[i].EvidenceRefs...)
	}
	return out
}

func cloneWikiCodeWikiContext(items []WikiCodeWikiContextItem) []WikiCodeWikiContextItem {
	if items == nil {
		return nil
	}
	out := append([]WikiCodeWikiContextItem(nil), items...)
	for i := range out {
		out[i].Parents = cloneWikiCodeWikiContext(items[i].Parents)
	}
	return out
}

func CanonicalWikiAuthority(path string) bool {
	p := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	return strings.HasPrefix(p, ".docs/wiki/") &&
		!strings.HasPrefix(p, ".docs/raw/") &&
		!strings.HasPrefix(p, ".docs/auditoria/") &&
		!strings.Contains(strings.ToLower(p), "snapshot")
}

// WikiCodeRelationOrder is the Graph v1 semantic ordering used by bridge
// materialization. Folder/layer numbering is intentionally not consulted.
func WikiCodeRelationOrder(relation string) int {
	switch strings.ToLower(strings.TrimSpace(relation)) {
	case RelationImplements:
		return 10
	case RelationTests:
		return 20
	case RelationConfigures:
		return 30
	case RelationOperates:
		return 40
	case "contains":
		return 50
	case "imports":
		return 60
	case "references":
		return 70
	case "calls":
		return 80
	default:
		return 100
	}
}

func compareWikiCodeStrings(left, right []string) int {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func wikiCodeEvidenceKey(item WikiCodeEvidence) []string {
	return []string{
		fmt.Sprintf("%03d", WikiCodeRelationOrder(item.Relation)), item.Path, item.Symbol,
		item.Origin, item.AuthoringOrigin, item.ObservedOrigin, item.Status, item.ResolutionStatus,
		item.DocPath, item.DocID, item.BlockID, item.SourceDoc, item.SourceBlock,
		fmt.Sprintf("%09d", item.SourceLine), item.Role, item.BindingRef,
		item.TargetKind, item.Kind, item.Language, item.ClaimStatus,
		item.SourceDigest, item.TargetHash, item.Classification, strings.Join(item.EvidenceRefs, "\x00"),
	}
}

func wikiCodeWikiContextKey(item WikiCodeWikiContextItem) []string {
	return []string{
		fmt.Sprintf("%03d", WikiCodeRelationOrder(item.Relation)), item.Path, item.DocID, item.BlockID, item.Relation, item.Role,
		item.BindingRef, item.Status, item.SupersededBy, item.Origin, item.AuthoringOrigin,
		item.Classification, fmt.Sprintf("%09d", item.StartLine), fmt.Sprintf("%09d", item.EndLine),
	}
}

func SortWikiCodeContext(c *WikiCodeContext) {
	if c == nil {
		return
	}
	sort.SliceStable(c.AuthorityChain, func(i, j int) bool {
		left := []string{c.AuthorityChain[i].Role, c.AuthorityChain[i].Path, c.AuthorityChain[i].DocID, c.AuthorityChain[i].Layer, c.AuthorityChain[i].ContentHash}
		right := []string{c.AuthorityChain[j].Role, c.AuthorityChain[j].Path, c.AuthorityChain[j].DocID, c.AuthorityChain[j].Layer, c.AuthorityChain[j].ContentHash}
		return compareWikiCodeStrings(left, right) < 0
	})
	for _, items := range [][]WikiCodeEvidence{c.CodeEvidence, c.DirectCode, c.Tests, c.SupportingCode, c.Candidates} {
		for i := range items {
			sort.Strings(items[i].EvidenceRefs)
		}
		sort.SliceStable(items, func(i, j int) bool {
			return compareWikiCodeStrings(wikiCodeEvidenceKey(items[i]), wikiCodeEvidenceKey(items[j])) < 0
		})
	}
	for i := range c.GraphPaths {
		sort.Strings(c.GraphPaths[i].EvidenceRefs)
	}
	sort.SliceStable(c.GraphPaths, func(i, j int) bool {
		left := []string{fmt.Sprintf("%03d", WikiCodeRelationOrder(c.GraphPaths[i].Relation)), c.GraphPaths[i].From, c.GraphPaths[i].To, c.GraphPaths[i].EdgeRef, c.GraphPaths[i].ClaimStatus, strings.Join(c.GraphPaths[i].EvidenceRefs, "\x00")}
		right := []string{fmt.Sprintf("%03d", WikiCodeRelationOrder(c.GraphPaths[j].Relation)), c.GraphPaths[j].From, c.GraphPaths[j].To, c.GraphPaths[j].EdgeRef, c.GraphPaths[j].ClaimStatus, strings.Join(c.GraphPaths[j].EvidenceRefs, "\x00")}
		return compareWikiCodeStrings(left, right) < 0
	})
	sort.SliceStable(c.Drift, func(i, j int) bool {
		left := []string{c.Drift[i].Code, c.Drift[i].Source, c.Drift[i].Target, c.Drift[i].Owner, c.Drift[i].Description}
		right := []string{c.Drift[j].Code, c.Drift[j].Source, c.Drift[j].Target, c.Drift[j].Owner, c.Drift[j].Description}
		return compareWikiCodeStrings(left, right) < 0
	})
	for i := range c.Omissions {
		sort.Strings(c.Omissions[i].Candidates)
	}
	sort.SliceStable(c.Omissions, func(i, j int) bool {
		left := []string{c.Omissions[i].Code, c.Omissions[i].Source, c.Omissions[i].DocPath, c.Omissions[i].DocID, c.Omissions[i].BlockID, c.Omissions[i].BindingRef, c.Omissions[i].Reason, c.Omissions[i].Owner, c.Omissions[i].Status, strings.Join(c.Omissions[i].Candidates, "\x00")}
		right := []string{c.Omissions[j].Code, c.Omissions[j].Source, c.Omissions[j].DocPath, c.Omissions[j].DocID, c.Omissions[j].BlockID, c.Omissions[j].BindingRef, c.Omissions[j].Reason, c.Omissions[j].Owner, c.Omissions[j].Status, strings.Join(c.Omissions[j].Candidates, "\x00")}
		return compareWikiCodeStrings(left, right) < 0
	})
	sort.SliceStable(c.WikiContext, func(i, j int) bool {
		return compareWikiCodeStrings(wikiCodeWikiContextKey(c.WikiContext[i]), wikiCodeWikiContextKey(c.WikiContext[j])) < 0
	})
	for i := range c.WikiContext {
		sortWikiCodeContextItem(&c.WikiContext[i])
	}
	sort.SliceStable(c.Classifications, func(i, j int) bool {
		return wikiCodeClassificationOrder(c.Classifications[i]) < wikiCodeClassificationOrder(c.Classifications[j])
	})
	sort.Strings(c.NextQueries)
}

func sortWikiCodeContextItem(item *WikiCodeWikiContextItem) {
	if item == nil {
		return
	}
	for i := range item.Parents {
		sortWikiCodeContextItem(&item.Parents[i])
	}
	sort.SliceStable(item.Parents, func(i, j int) bool {
		return compareWikiCodeStrings(wikiCodeWikiContextKey(item.Parents[i]), wikiCodeWikiContextKey(item.Parents[j])) < 0
	})
}

func wikiCodeClassificationOrder(value string) int {
	switch value {
	case WikiCodeClassificationDirectSDDBinding:
		return 10
	case WikiCodeClassificationSupportingCode:
		return 20
	case WikiCodeClassificationSharedTechnical:
		return 30
	case WikiCodeClassificationGeneratedOrVendor:
		return 40
	case WikiCodeClassificationMechanicalNoImpact:
		return 50
	case WikiCodeClassificationUnmappedChangedCode:
		return 60
	default:
		return 100
	}
}

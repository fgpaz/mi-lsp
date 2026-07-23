package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	DocGenerationID   string                    `json:"doc_generation_id,omitempty"`
	CodeGenerationID  string                    `json:"code_generation_id,omitempty"`
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
}

type WikiCodeProvenance struct {
	Backend         string `json:"backend"`
	DocsGeneration  string `json:"docs_generation,omitempty"`
	GraphGeneration string `json:"graph_generation,omitempty"`
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

// WikiCodeContextDigest returns the stable v1 digest. Budget accounting and
// elapsed time are deliberately excluded from the digest.
func WikiCodeContextDigest(c WikiCodeContext) string {
	copyContext := c
	copyContext.TokenBudget = 0
	copyContext.TokenUsed = 0
	copyContext.Truncated = false
	copyContext.DeterminismDigest = ""
	b, _ := json.Marshal(copyContext)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func CanonicalWikiAuthority(path string) bool {
	p := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	return strings.HasPrefix(p, ".docs/wiki/") &&
		!strings.HasPrefix(p, ".docs/raw/") &&
		!strings.HasPrefix(p, ".docs/auditoria/") &&
		!strings.Contains(strings.ToLower(p), "snapshot")
}

func SortWikiCodeContext(c *WikiCodeContext) {
	if c == nil {
		return
	}
	sort.SliceStable(c.AuthorityChain, func(i, j int) bool {
		return c.AuthorityChain[i].Role < c.AuthorityChain[j].Role || (c.AuthorityChain[i].Role == c.AuthorityChain[j].Role && c.AuthorityChain[i].Path < c.AuthorityChain[j].Path)
	})
	sort.Slice(c.CodeEvidence, func(i, j int) bool {
		return c.CodeEvidence[i].Path+c.CodeEvidence[i].Symbol < c.CodeEvidence[j].Path+c.CodeEvidence[j].Symbol
	})
	sort.Slice(c.GraphPaths, func(i, j int) bool {
		return c.GraphPaths[i].From+c.GraphPaths[i].To+c.GraphPaths[i].EdgeRef < c.GraphPaths[j].From+c.GraphPaths[j].To+c.GraphPaths[j].EdgeRef
	})
	sort.Slice(c.Drift, func(i, j int) bool {
		return c.Drift[i].Code+c.Drift[i].Source+c.Drift[i].Target < c.Drift[j].Code+c.Drift[j].Source+c.Drift[j].Target
	})
	sort.Slice(c.Omissions, func(i, j int) bool {
		return c.Omissions[i].Code+c.Omissions[i].Source < c.Omissions[j].Code+c.Omissions[j].Source
	})
}

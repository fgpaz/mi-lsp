package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	GraphFreshnessCurrent = "current"
	GraphFreshnessLagging = "lagging"
	GraphFreshnessStale   = "stale"
	GraphFreshnessInvalid = "invalid"
	GraphFreshnessUnknown = "unknown"

	GraphAnalysisAlgorithm = "bounded-deterministic-v1"
	GraphAnalysisVersion   = "1"
	GraphAnalysisProfile   = "exact-extracted-only"
	GraphAnalysisMaxNodes  = 10000
	GraphAnalysisMaxEdges  = 50000
	GraphAnalysisMaxBytes  = 64 * 1024

	UtilitySignalContinuationFollowed       = "continuation_followed"
	UtilitySignalFeedbackPositive           = "feedback_positive"
	UtilitySignalFeedbackNegative           = "feedback_negative"
	UtilitySignalResultSelected             = "result_selected"
	UtilityMaxEventsPerCandidate            = 4096
	UtilityMaxEventsPerScopeIntentOperation = 4096
	// UtilityMaxEventsPerScope is retained as a compatibility alias for the
	// candidate-scoped cap; new code should use the explicit constant above.
	UtilityMaxEventsPerScope       = UtilityMaxEventsPerCandidate
	UtilityMaxIntentLength         = 64
	UtilityMaxWorkspaceScopeLength = 256
)

// GraphFreshness is an additive claim gate. Exact graph claims are only
// authoritative when State is current; other states must be surfaced rather
// than silently treated as current.
type GraphFreshness struct {
	State              string `json:"state"`
	GenerationID       string `json:"generation_id,omitempty"`
	ObservedGeneration string `json:"observed_generation_id,omitempty"`
	ReasonCode         string `json:"reason_code,omitempty"`
}

func (f GraphFreshness) ValidState() bool {
	switch f.State {
	case GraphFreshnessCurrent, GraphFreshnessLagging, GraphFreshnessStale, GraphFreshnessInvalid, GraphFreshnessUnknown:
		return true
	default:
		return false
	}
}

func (f GraphFreshness) AllowsExactClaims() bool { return f.State == GraphFreshnessCurrent }

// ClassifyGraphFreshness is deliberately pure so freshness negative cases are
// easy to test without SQLite. runtimeState is the graph runtime marker;
// activeGeneration is the published generation and requestedGeneration is the
// generation a cache or query wants to serve.
func ClassifyGraphFreshness(runtimeState, activeGeneration, requestedGeneration string, generationValid bool) GraphFreshness {
	activeGeneration = strings.TrimSpace(activeGeneration)
	requestedGeneration = strings.TrimSpace(requestedGeneration)
	result := GraphFreshness{GenerationID: activeGeneration, ObservedGeneration: requestedGeneration}
	if !generationValid {
		result.State = GraphFreshnessInvalid
		result.ReasonCode = "generation_validation_failed"
		return result
	}
	if activeGeneration == "" {
		result.State = GraphFreshnessUnknown
		result.ReasonCode = "active_generation_missing"
		return result
	}
	switch strings.ToLower(strings.TrimSpace(runtimeState)) {
	case "stale":
		result.State = GraphFreshnessStale
		result.ReasonCode = "runtime_marked_stale"
		return result
	case "fresh", "current", "":
		// Continue with generation comparison.
	default:
		result.State = GraphFreshnessUnknown
		result.ReasonCode = "runtime_state_unknown"
		return result
	}
	if requestedGeneration != "" && requestedGeneration != activeGeneration {
		result.State = GraphFreshnessLagging
		result.ReasonCode = "generation_mismatch"
		return result
	}
	result.State = GraphFreshnessCurrent
	result.ReasonCode = "generation_matches_active"
	return result
}

type GraphAnalysisRequest struct {
	GenerationID     string `json:"generation_id"`
	Algorithm        string `json:"algorithm"`
	AlgorithmVersion string `json:"algorithm_version"`
	Profile          string `json:"profile"`
	ParametersDigest string `json:"parameters_digest"`
	AuthorityProfile string `json:"authority_profile"`
	MaxNodes         int    `json:"max_nodes"`
	MaxEdges         int    `json:"max_edges"`
	Limit            int    `json:"limit"`
}

func (r GraphAnalysisRequest) Normalize() GraphAnalysisRequest {
	r.GenerationID = strings.TrimSpace(r.GenerationID)
	r.Algorithm = strings.TrimSpace(r.Algorithm)
	if r.Algorithm == "" {
		r.Algorithm = GraphAnalysisAlgorithm
	}
	r.AlgorithmVersion = strings.TrimSpace(r.AlgorithmVersion)
	if r.AlgorithmVersion == "" {
		r.AlgorithmVersion = GraphAnalysisVersion
	}
	r.Profile = strings.TrimSpace(r.Profile)
	if r.Profile == "" {
		r.Profile = GraphAnalysisProfile
	}
	if r.MaxNodes <= 0 || r.MaxNodes > GraphAnalysisMaxNodes {
		r.MaxNodes = GraphAnalysisMaxNodes
	}
	if r.MaxEdges <= 0 || r.MaxEdges > GraphAnalysisMaxEdges {
		r.MaxEdges = GraphAnalysisMaxEdges
	}
	if r.Limit <= 0 || r.Limit > GraphQueryMaxLimit {
		r.Limit = GraphQueryMaxLimit
	}
	return r
}

type GraphRank struct {
	NodeKey           string  `json:"node_key,omitempty"`
	CrossRID          string  `json:"cross_rid,omitempty"`
	Display           string  `json:"display,omitempty"`
	OwnerPath         string  `json:"owner_path,omitempty"`
	CommunityID       string  `json:"community_id,omitempty"`
	RankReason        string  `json:"rank_reason,omitempty"`
	AlgorithmVersion  string  `json:"algorithm_version"`
	DeterminismDigest string  `json:"determinism_digest"`
	Score             float64 `json:"score"`
	Authority         float64 `json:"authority"`
	Impact            float64 `json:"impact"`
	Centrality        float64 `json:"centrality"`
	Boundary          float64 `json:"boundary"`
	Utility           float64 `json:"utility,omitempty"`
	ClaimStatus       string  `json:"claim_status,omitempty"`
	HeuristicExcluded bool    `json:"heuristic_excluded,omitempty"`
}

type GraphRankEnvelope struct {
	Ok                bool           `json:"ok"`
	GenerationID      string         `json:"generation_id"`
	GraphFreshness    GraphFreshness `json:"graph_freshness"`
	Algorithm         string         `json:"algorithm"`
	AlgorithmVersion  string         `json:"algorithm_version"`
	Profile           string         `json:"profile"`
	DeterminismDigest string         `json:"determinism_digest"`
	Items             []GraphRank    `json:"items"`
	Truncated         bool           `json:"truncated,omitempty"`
	Warnings          []string       `json:"warnings,omitempty"`
}

// GraphRankDeterminismDigest hashes only canonical, sorted output. Map order,
// timestamps, and utility event arrival order therefore cannot change it.
func GraphRankDeterminismDigest(generation, algorithm, version, profile string, ranks []GraphRank) string {
	canonical := append([]GraphRank(nil), ranks...)
	sort.SliceStable(canonical, func(i, j int) bool {
		if canonical[i].NodeKey != canonical[j].NodeKey {
			return canonical[i].NodeKey < canonical[j].NodeKey
		}
		return canonical[i].CrossRID < canonical[j].CrossRID
	})
	payload := struct {
		Generation string
		Algorithm  string
		Version    string
		Profile    string
		Ranks      []GraphRank
	}{generation, algorithm, version, profile, canonical}
	b, _ := json.Marshal(payload)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func CommunityID(nodeKeys []string) string {
	keys := append([]string(nil), nodeKeys...)
	sort.Strings(keys)
	b, _ := json.Marshal(keys)
	d := sha256.Sum256(append([]byte("milsp-community-v1\x00"), b...))
	return "community:" + hex.EncodeToString(d[:])[:16]
}

// UtilityEvent intentionally contains no query, prompt, argv, snippet, or
// content field. It is safe to persist after normalization.
type UtilityEvent struct {
	OccurredAt       time.Time `json:"occurred_at"`
	WorkspaceScope   string    `json:"workspace_scope"`
	Intent           string    `json:"intent"`
	Operation        string    `json:"operation"`
	CandidateNodeKey string    `json:"candidate_node_key"`
	Signal           string    `json:"signal"`
	Value            float64   `json:"value"`
	GenerationID     string    `json:"generation_id,omitempty"`
}

func (e UtilityEvent) Normalize() (UtilityEvent, bool) {
	e.WorkspaceScope = normalizeUtilityWorkspaceScope(e.WorkspaceScope)
	e.Intent = SanitizeUtilityIntent(e.Intent)
	e.Operation = sanitizeUtilityToken(e.Operation, 96)
	e.GenerationID = strings.ToLower(strings.TrimSpace(e.GenerationID))
	e.CandidateNodeKey = strings.ToLower(strings.TrimSpace(e.CandidateNodeKey))
	if e.CandidateNodeKey != "" {
		if _, err := ParseGraphDigest(e.CandidateNodeKey); err != nil {
			return UtilityEvent{}, false
		}
	}
	if e.GenerationID != "" {
		if _, err := ParseGraphDigest(e.GenerationID); err != nil {
			return UtilityEvent{}, false
		}
	}
	switch e.Signal {
	case UtilitySignalContinuationFollowed, UtilitySignalFeedbackPositive, UtilitySignalFeedbackNegative, UtilitySignalResultSelected:
	default:
		return UtilityEvent{}, false
	}
	if e.WorkspaceScope == "" || e.Intent == "" || e.Operation == "" || math.IsNaN(e.Value) || math.IsInf(e.Value, 0) {
		return UtilityEvent{}, false
	}
	if e.Value > 1 {
		e.Value = 1
	}
	if e.Value < -1 {
		e.Value = -1
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	return e, true
}

func SanitizeUtilityIntent(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "unknown"
	}
	allowed := map[string]bool{
		"callers": true, "callees": true, "affected-change": true, "path-between": true,
		"explain-edge": true, "neighborhood": true, "explain-change": true,
		"workspace-map": true, "related": true, "graph": true, "rank": true, "unknown": true,
	}
	if !allowed[raw] {
		return "unknown"
	}
	return raw
}

func normalizeUtilityWorkspaceScope(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || len(raw) > UtilityMaxWorkspaceScopeLength {
		return ""
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return raw
}

func sanitizeUtilityToken(raw string, max int) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if len(raw) > max {
		raw = raw[:max]
	}
	for _, r := range raw {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return ""
		}
	}
	return raw
}

type UtilitySignal struct {
	WorkspaceScope   string  `json:"workspace_scope"`
	Intent           string  `json:"intent"`
	Operation        string  `json:"operation"`
	CandidateNodeKey string  `json:"candidate_node_key,omitempty"`
	Score            float64 `json:"score"`
	Samples          int     `json:"samples"`
}

func UtilityScore(events []UtilityEvent, workspace, intent, operation string, now time.Time) UtilitySignal {
	return utilityScore(events, workspace, intent, operation, "", now)
}

// UtilityScoreForCandidate is the only candidate-aware utility path. The
// caller supplies a NodeKey, never a prompt, snippet, or display label.
func UtilityScoreForCandidate(events []UtilityEvent, workspace, intent, operation, candidateNodeKey string, now time.Time) UtilitySignal {
	return utilityScore(events, workspace, intent, operation, strings.TrimSpace(candidateNodeKey), now)
}

func utilityScore(events []UtilityEvent, workspace, intent, operation, candidateNodeKey string, now time.Time) UtilitySignal {
	workspace = strings.TrimSpace(workspace)
	intent = SanitizeUtilityIntent(intent)
	operation = sanitizeUtilityToken(operation, 96)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	const halfLife = 7 * 24 * time.Hour
	var score float64
	samples := 0
	for _, raw := range events {
		e, ok := raw.Normalize()
		if !ok || e.WorkspaceScope != workspace || e.Intent != intent || e.Operation != operation || (candidateNodeKey != "" && e.CandidateNodeKey != candidateNodeKey) {
			continue
		}
		age := now.Sub(e.OccurredAt)
		if age < 0 {
			age = 0
		}
		weight := math.Exp(-math.Ln2 * age.Hours() / halfLife.Hours())
		score += e.Value * weight
		samples++
	}
	if score > 1 {
		score = 1
	}
	if score < -1 {
		score = -1
	}
	return UtilitySignal{WorkspaceScope: workspace, Intent: intent, Operation: operation, CandidateNodeKey: candidateNodeKey, Score: score, Samples: samples}
}

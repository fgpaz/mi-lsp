package model

import (
	"container/heap"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

const (
	GraphImpactModeDirect     = "direct"
	GraphImpactModeTransitive = "transitive"
	MaxImpactSeeds            = 2048
)

var graphImpactRelations = map[string]GraphImpactRelation{
	"calls":            {Relation: "calls", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: true},
	"imports":          {Relation: "imports", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: true},
	"implements":       {Relation: "implements", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: true},
	"extends":          {Relation: "extends", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: true},
	"tests":            {Relation: "tests", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
	"route_to_handler": {Relation: "route_to_handler", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
	"publishes":        {Relation: "publishes", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
	"consumes":         {Relation: "consumes", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
	"reads":            {Relation: "reads", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
	"writes":           {Relation: "writes", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
	"doc_mentions":     {Relation: "doc_mentions", Direction: "in", ClaimStatuses: []string{GraphRecordExact, GraphRecordExtracted, GraphRecordInferred}, Cost: 1, Transitive: false},
}

type GraphImpactRelation struct {
	Relation      string   `json:"relation"`
	Direction     string   `json:"direction"`
	ClaimStatuses []string `json:"claim_statuses"`
	Cost          int      `json:"cost"`
	Transitive    bool     `json:"transitive"`
}

func GraphImpactRelationSemantics(relation string) (GraphImpactRelation, bool) {
	r, ok := graphImpactRelations[strings.ToLower(strings.TrimSpace(relation))]
	return r, ok
}

func GraphImpactRelations() []GraphImpactRelation {
	out := make([]GraphImpactRelation, 0, len(graphImpactRelations))
	for _, r := range graphImpactRelations {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Relation < out[j].Relation })
	return out
}

type GraphImpactRequest struct {
	Generation   string
	Paths        []string
	ChangedPaths []string
	Mode         string
	Depth        int
	Limit        int
	TokenBudget  int
	Direction    string
	Relations    []string
	IncludeTests bool
	IncludeDocs  bool
	Cursor       string
	Omissions    []GraphImpactOmission
	Warnings     []string
}

func (q GraphImpactRequest) Normalize() (GraphImpactRequest, error) {
	q.Generation = strings.TrimSpace(q.Generation)
	q.Direction = strings.ToLower(strings.TrimSpace(q.Direction))
	if q.Direction == "" {
		q.Direction = "in"
	}
	if q.Direction != "in" {
		return q, &GraphQueryError{Code: "GPH_IMPACT_RELATION_UNSUPPORTED", Field: "direction", Message: "impact semantics only support inbound dependency edges"}
	}
	q.Mode = strings.ToLower(strings.TrimSpace(q.Mode))
	if q.Mode == "" {
		q.Mode = GraphImpactModeDirect
	}
	if q.Mode != GraphImpactModeDirect && q.Mode != GraphImpactModeTransitive {
		return q, &GraphQueryError{Code: "GPH_IMPACT_BUDGET_INVALID", Field: "mode", Message: "mode must be direct or transitive"}
	}
	if q.Depth == 0 {
		q.Depth = GraphQueryDefaultDepth
	}
	if q.Limit == 0 {
		q.Limit = GraphQueryDefaultLimit
	}
	if q.TokenBudget == 0 {
		q.TokenBudget = GraphQueryDefaultToken
	}
	maxDepth := GraphQueryMaxDepth
	if q.Depth < 1 || q.Depth > maxDepth {
		return q, &GraphQueryError{Code: "GPH_IMPACT_BUDGET_INVALID", Field: "depth", Message: "depth is outside the allowed range"}
	}
	if q.Limit < 1 || q.Limit > GraphQueryMaxLimit {
		return q, &GraphQueryError{Code: "GPH_IMPACT_BUDGET_INVALID", Field: "limit", Message: "limit is outside the allowed range"}
	}
	if q.TokenBudget < 1 || q.TokenBudget > GraphQueryMaxToken {
		return q, &GraphQueryError{Code: "GPH_IMPACT_BUDGET_INVALID", Field: "token_budget", Message: "token budget is outside the allowed range"}
	}
	paths, seedOverflow := boundedImpactSeedPaths(q.Paths, q.ChangedPaths)
	q.Omissions = nil
	q.Warnings = nil
	if seedOverflow {
		q.Omissions = append(q.Omissions, GraphImpactOmission{
			Reason: "normalized seed input exceeded the explicit bounded seed limit before ordering",
			Code:   "GPH_IMPACT_SEED_BUDGET_EXCEEDED",
		})
		q.Warnings = append(q.Warnings, "impact seed input was bounded before ordering; omitted seed paths were not traversed")
	}
	q.Paths = paths
	q.ChangedPaths = append([]string(nil), paths...)
	unique := map[string]struct{}{}
	relations := make([]string, 0, len(q.Relations))
	for _, relation := range q.Relations {
		relation = strings.ToLower(strings.TrimSpace(relation))
		if relation == "" {
			continue
		}
		if _, ok := GraphImpactRelationSemantics(relation); !ok {
			return q, &GraphQueryError{Code: "GPH_IMPACT_RELATION_UNSUPPORTED", Field: "edge", Message: "relation has no impact semantics"}
		}
		if _, ok := unique[relation]; !ok {
			unique[relation] = struct{}{}
			relations = append(relations, relation)
		}
	}
	if len(relations) == 0 {
		for _, r := range GraphImpactRelations() {
			if r.Relation == "doc_mentions" && !q.IncludeDocs {
				continue
			}
			if r.Relation == "tests" && !q.IncludeTests {
				continue
			}
			relations = append(relations, r.Relation)
		}
	}
	sort.Strings(relations)
	q.Relations = relations
	return q, nil
}

type GraphImpactPathStep struct {
	EdgeCrossRID string   `json:"edge_cross_rid"`
	Relation     string   `json:"relation"`
	Status       string   `json:"status"`
	FromCrossRID string   `json:"from_cross_rid"`
	ToCrossRID   string   `json:"to_cross_rid"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type GraphImpactItem struct {
	Kind            string                `json:"kind"`
	Path            string                `json:"path"`
	GenerationID    string                `json:"generation_id"`
	CrossRID        string                `json:"cross_rid"`
	NodeKey         string                `json:"node_key,omitempty"`
	Display         string                `json:"display,omitempty"`
	OwnerPath       string                `json:"owner_path,omitempty"`
	SymbolKind      string                `json:"symbol_kind,omitempty"`
	Status          string                `json:"status,omitempty"`
	ConfidenceClass string                `json:"confidence_class"`
	Distance        int                   `json:"distance"`
	Relation        string                `json:"relation,omitempty"`
	Reason          string                `json:"reason"`
	TriggerPath     string                `json:"trigger_path"`
	ChangeType      string                `json:"change_type,omitempty"`
	NodeID          int                   `json:"node_id,omitempty"`
	EvidencePath    []GraphImpactPathStep `json:"evidence_path,omitempty"`
	EvidenceRefs    []string              `json:"evidence_refs,omitempty"`
	RankScore       float64               `json:"rank_score,omitempty"`
	CommunityID     string                `json:"community_id,omitempty"`
}

type GraphImpactOmission struct {
	Path       string   `json:"path,omitempty"`
	Reason     string   `json:"reason"`
	Code       string   `json:"code"`
	Candidates []string `json:"candidates,omitempty"`
}

type GraphImpactStats struct {
	Seeds        int `json:"seeds"`
	Visited      int `json:"visited"`
	Frontier     int `json:"frontier"`
	Returned     int `json:"returned"`
	Inferred     int `json:"inferred"`
	DepthReached int `json:"depth_reached"`
	TokenUnits   int `json:"token_units"`
	Unresolved   int `json:"unresolved"`
}

type GraphImpactEnvelope struct {
	Ok                 bool                  `json:"ok"`
	Backend            string                `json:"backend"`
	GenerationID       string                `json:"generation_id"`
	GraphSchemaVersion int                   `json:"graph_schema_version"`
	Mode               string                `json:"mode"`
	PrecisionScope     string                `json:"precision_scope"`
	Relations          []string              `json:"relations"`
	Items              []GraphImpactItem     `json:"items"`
	Inferred           []GraphImpactItem     `json:"inferred,omitempty"`
	Omissions          []GraphImpactOmission `json:"omissions,omitempty"`
	Stats              GraphImpactStats      `json:"stats"`
	Truncated          bool                  `json:"truncated"`
	Warnings           []string              `json:"warnings,omitempty"`
	DeterminismDigest  string                `json:"determinism_digest"`
	GraphFreshness     GraphFreshness        `json:"graph_freshness"`
	Continuation       *Continuation         `json:"continuation,omitempty"`
}

func GraphImpactDeterminismDigest(q GraphImpactRequest, generation GraphDigest, items, inferred []GraphImpactItem, stats GraphImpactStats) string {
	payload := struct {
		Request    GraphImpactRequest
		Generation string
		Items      []GraphImpactItem
		Inferred   []GraphImpactItem
		Stats      GraphImpactStats
	}{q, generation.String(), items, inferred, stats}
	b, _ := json.Marshal(payload)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

type impactSeedMaxHeap []string

func (h impactSeedMaxHeap) Len() int           { return len(h) }
func (h impactSeedMaxHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h impactSeedMaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *impactSeedMaxHeap) Push(value any)    { *h = append(*h, value.(string)) }
func (h *impactSeedMaxHeap) Pop() any {
	old := *h
	value := old[len(old)-1]
	*h = old[:len(old)-1]
	return value
}

func boundedImpactSeedPaths(sources ...[]string) ([]string, bool) {
	h := &impactSeedMaxHeap{}
	heap.Init(h)
	accepted := make(map[string]struct{}, MaxImpactSeeds)
	overflow := false
	for _, source := range sources {
		for _, raw := range source {
			path := strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "./")
			if path == "" {
				continue
			}
			if _, ok := accepted[path]; ok {
				continue
			}
			if h.Len() < MaxImpactSeeds {
				heap.Push(h, path)
				accepted[path] = struct{}{}
				continue
			}
			overflow = true
			if path >= (*h)[0] {
				continue
			}
			removed := heap.Pop(h).(string)
			delete(accepted, removed)
			heap.Push(h, path)
			accepted[path] = struct{}{}
		}
	}
	paths := append([]string(nil), (*h)...)
	sort.Strings(paths)
	return paths, overflow
}

// ImpactRequest and ImpactItem are concise compatibility aliases for core callers.
type ImpactRequest = GraphImpactRequest
type ImpactItem = GraphImpactItem

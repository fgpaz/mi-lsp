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

const (
	GraphQueryDefaultDepth        = 1
	GraphQueryMaxDepth            = 6
	GraphQueryPathMaxDepth        = 12
	GraphQueryPathExpansionBudget = 1000
	GraphQueryDefaultLimit        = 50
	GraphQueryMaxLimit            = 500
	GraphQueryDefaultToken        = 4000
	GraphQueryMaxToken            = 20000
)

var (
	ErrGraphQueryBudgetInvalid = errors.New("GPH_QUERY_BUDGET_INVALID")
	ErrGraphQueryCursorStale   = errors.New("GPH_QUERY_CURSOR_STALE")
	ErrGraphQuerySelector      = errors.New("GPH_QUERY_SELECTOR_INVALID")
	ErrGraphQueryGeneration    = errors.New("GPH_QUERY_GENERATION_INVALID")
)

type GraphQueryError struct {
	Code       string           `json:"code"`
	Field      string           `json:"field,omitempty"`
	Message    string           `json:"message"`
	Hint       string           `json:"hint,omitempty"`
	Candidates []GraphQueryItem `json:"candidates,omitempty"`
}

func (e *GraphQueryError) Error() string { return e.Code + ": " + e.Message }

func (e *GraphQueryError) Unwrap() error {
	switch e.Code {
	case "GPH_QUERY_BUDGET_INVALID":
		return ErrGraphQueryBudgetInvalid
	case "GPH_QUERY_CURSOR_STALE":
		return ErrGraphQueryCursorStale
	case "GPH_QUERY_SELECTOR_INVALID":
		return ErrGraphQuerySelector
	default:
		return ErrGraphQueryGeneration
	}
}

type GraphQueryRequest struct {
	Operation        string
	Selector         string
	From             string
	To               string
	Generation       string
	Depth            int
	Limit            int
	TokenBudget      int
	Direction        string
	Relations        []string
	Cursor           string
	UtilitySignal    string
	CandidateNodeKey string
	UtilityIntent    string
	Utility          []UtilitySignal
	CachedRanks      []GraphRank
	CachedDigest     string
}

func (q GraphQueryRequest) Normalize() (GraphQueryRequest, error) {
	q.Operation = strings.TrimSpace(q.Operation)
	if q.Operation != "nav.neighbors" && q.Operation != "nav.callers" && q.Operation != "nav.callees" && q.Operation != "nav.path" && q.Operation != "nav.explain" && q.Operation != "nav.graph.stats" && q.Operation != "nav.graph.status" && q.Operation != "nav.graph.rank" && q.Operation != "nav.graph.validate" {
		return q, &GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Field: "operation", Message: "operation is not a graph query"}
	}
	q.Selector = strings.TrimSpace(q.Selector)
	q.From = strings.TrimSpace(q.From)
	q.To = strings.TrimSpace(q.To)
	q.Generation = strings.TrimSpace(q.Generation)
	q.Direction = strings.ToLower(strings.TrimSpace(q.Direction))
	if q.Direction == "" {
		q.Direction = "both"
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
	if q.Operation == "nav.path" {
		maxDepth = GraphQueryPathMaxDepth
	}
	if q.Depth < 1 || q.Depth > maxDepth {
		return q, &GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "depth", Message: "depth is outside the allowed range"}
	}
	if q.Limit < 1 || q.Limit > GraphQueryMaxLimit {
		return q, &GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "limit", Message: "limit is outside the allowed range"}
	}
	if q.TokenBudget < 1 || q.TokenBudget > GraphQueryMaxToken {
		return q, &GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "token_budget", Message: "token budget is outside the allowed range"}
	}
	if q.Direction != "in" && q.Direction != "out" && q.Direction != "both" {
		return q, &GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "direction", Message: "direction must be in, out, or both"}
	}
	// Normalize, validate, de-duplicate, and sort relation filters.
	unique := make(map[string]struct{}, len(q.Relations))
	for _, relation := range q.Relations {
		relation = strings.ToLower(strings.TrimSpace(relation))
		if _, ok := registeredGraphValues["relation"][relation]; !ok {
			return q, &GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "edge", Message: "relation is not registered"}
		}
		unique[relation] = struct{}{}
	}
	q.Relations = q.Relations[:0]
	for relation := range unique {
		q.Relations = append(q.Relations, relation)
	}
	sort.Strings(q.Relations)
	return q, nil
}

type GraphQueryItem struct {
	Kind            string   `json:"kind"`
	CrossRID        string   `json:"cross_rid"`
	Display         string   `json:"display"`
	Status          string   `json:"status"`
	Distance        int      `json:"distance"`
	EvidenceRefs    []string `json:"evidence_refs,omitempty"`
	NodeKey         string   `json:"node_key,omitempty"`
	EdgeKey         string   `json:"edge_key,omitempty"`
	EdgeCrossRID    string   `json:"edge_cross_rid,omitempty"`
	ConfidenceClass string   `json:"confidence_class,omitempty"`
	NodeID          int      `json:"node_id,omitempty"`
	EdgeID          int      `json:"edge_id,omitempty"`
	Relation        string   `json:"relation,omitempty"`
	FromNodeKey     string   `json:"from_node_key,omitempty"`
	ToNodeKey       string   `json:"to_node_key,omitempty"`
	FromCrossRID    string   `json:"from_cross_rid,omitempty"`
	ToCrossRID      string   `json:"to_cross_rid,omitempty"`
	SymbolKind      string   `json:"symbol_kind,omitempty"`
	OwnerPath       string   `json:"owner_path,omitempty"`
}

type GraphQueryStats struct {
	Visited               int  `json:"visited"`
	Frontier              int  `json:"frontier"`
	Returned              int  `json:"returned"`
	Depth                 int  `json:"depth"`
	TokenUnits            int  `json:"token_units"`
	DepthReached          int  `json:"depth_reached"`
	Unresolved            int  `json:"unresolved"`
	SearchBudgetExhausted bool `json:"search_budget_exhausted,omitempty"`
}

type GraphQueryMetadata struct {
	Operation         string          `json:"operation"`
	GenerationID      string          `json:"generation_id"`
	Schema            int             `json:"schema"`
	DeterminismDigest string          `json:"determinism_digest"`
	Stats             GraphQueryStats `json:"stats"`
	GraphFreshness    GraphFreshness  `json:"graph_freshness"`
	NextCursor        string          `json:"next_cursor,omitempty"`
}

func DeterminismDigest(operation string, generation GraphDigest, items []GraphQueryItem, stats GraphQueryStats) string {
	payload := struct {
		Operation  string
		Generation string
		Items      []GraphQueryItem
		Stats      GraphQueryStats
	}{operation, generation.String(), items, stats}
	b, _ := json.Marshal(payload)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func (q GraphQueryRequest) String() string {
	return fmt.Sprintf("%s:%s", q.Operation, q.Selector)
}

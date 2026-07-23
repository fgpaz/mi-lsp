package model

import (
	"errors"
	"testing"
)

func TestGraphQueryRequestNormalizeDefaultsAndRelations(t *testing.T) {
	q, err := (GraphQueryRequest{Operation: " nav.neighbors ", Relations: []string{"calls", " CALLS ", "references"}}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if q.Depth != GraphQueryDefaultDepth || q.Limit != GraphQueryDefaultLimit || q.TokenBudget != GraphQueryDefaultToken || q.Direction != "both" {
		t.Fatalf("defaults=%+v", q)
	}
	if len(q.Relations) != 2 || q.Relations[0] != "calls" || q.Relations[1] != "references" {
		t.Fatalf("relations=%v", q.Relations)
	}
}

func TestGraphQueryRequestNormalizeRejectsBudgetsAndRelations(t *testing.T) {
	cases := []GraphQueryRequest{
		{Operation: "nav.neighbors", Depth: GraphQueryMaxDepth + 1},
		{Operation: "nav.path", Depth: GraphQueryPathMaxDepth + 1},
		{Operation: "nav.neighbors", Limit: GraphQueryMaxLimit + 1},
		{Operation: "nav.neighbors", TokenBudget: GraphQueryMaxToken + 1},
		{Operation: "nav.neighbors", Direction: "sideways"},
		{Operation: "nav.neighbors", Relations: []string{"calls'); DROP TABLE graph_edges;--"}},
	}
	for _, input := range cases {
		if _, err := input.Normalize(); !errors.Is(err, ErrGraphQueryBudgetInvalid) {
			t.Fatalf("%+v: %v", input, err)
		}
	}
}

func TestDeterminismDigestExcludesElapsedTime(t *testing.T) {
	generation := digestBytes([]byte("generation"))
	items := []GraphQueryItem{{Kind: "node", CrossRID: "rid", Display: "name", Distance: 1}}
	a := DeterminismDigest("nav.neighbors", generation, items, GraphQueryStats{Visited: 2, Returned: 1, Depth: 1})
	b := DeterminismDigest("nav.neighbors", generation, items, GraphQueryStats{Visited: 2, Returned: 1, Depth: 1})
	if a != b {
		t.Fatalf("digest must be stable: %s != %s", a, b)
	}
}

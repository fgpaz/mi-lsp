package service

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestGraphRequestFromPayloadDefaultsAndOperationOverrides(t *testing.T) {
	request := model.CommandRequest{Operation: "nav.callers", Context: model.QueryOptions{TokenBudget: 0}, Payload: map[string]any{
		"selector":     "pkg.Widget",
		"depth":        2,
		"limit":        7,
		"token_budget": 4000,
		"direction":    "out",
		"edge":         []any{"references"},
	}}
	q, err := graphRequestFromPayload(request)
	if err != nil {
		t.Fatal(err)
	}
	if q.Direction != "in" || len(q.Relations) != 1 || q.Relations[0] != "calls" {
		t.Fatalf("callers must force calls/in: %+v", q)
	}
	if q.Depth != 2 || q.Limit != 7 || q.TokenBudget != 4000 {
		t.Fatalf("payload shape lost: %+v", q)
	}
}

func TestGraphQueryRejectsBudgetBeforeWorkspaceResolution(t *testing.T) {
	app := New(t.TempDir(), nil)
	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.neighbors",
		Context:   model.QueryOptions{Workspace: "invalid-workspace-sentinel"},
		Payload:   map[string]any{"selector": "x", "token_budget": model.GraphQueryMaxToken + 1},
	})
	if err == nil {
		t.Fatal("expected budget validation error")
	}
	graphErr, ok := err.(*model.GraphQueryError)
	if !ok || graphErr.Code != "GPH_QUERY_BUDGET_INVALID" {
		t.Fatalf("error = %#v, want pre-resolution budget error", err)
	}
}

func TestGraphRequestRejectsInvalidBudget(t *testing.T) {
	_, err := graphRequestFromPayload(model.CommandRequest{Operation: "nav.graph.stats", Payload: map[string]any{"token_budget": -1}})
	if err == nil {
		t.Fatal("expected invalid budget")
	}
	if graphErr, ok := err.(*model.GraphQueryError); !ok || graphErr.Code != "GPH_QUERY_BUDGET_INVALID" {
		t.Fatalf("error = %#v, want GPH_QUERY_BUDGET_INVALID", err)
	}
}

func TestFinalizeGraphItemsAlwaysAdvancesCursorAtTinyBudget(t *testing.T) {
	q := model.GraphQueryRequest{Operation: "nav.neighbors", Selector: "root", Depth: 1, Limit: 2, TokenBudget: 1, Direction: "both"}
	items := []model.GraphQueryItem{
		{Kind: "node", CrossRID: "n1", Display: "first", Status: model.GraphRecordExact},
		{Kind: "node", CrossRID: "n2", Display: "second", Status: model.GraphRecordExact},
	}
	env := finalizeGraphItems(q, model.GraphGeneration{SchemaVersion: 1}, nil, items, 2, 0, 0)
	got := env.Items.([]model.GraphQueryItem)
	if len(got) != 1 || !env.Truncated || env.Graph.NextCursor == "" {
		t.Fatalf("tiny budget must return one item and an advancing cursor: %#v", env)
	}
	cursor, err := decodeGraphCursor(env.Graph.NextCursor)
	if err != nil || cursor.Offset != 1 {
		t.Fatalf("cursor=%+v err=%v, want offset 1", cursor, err)
	}
}

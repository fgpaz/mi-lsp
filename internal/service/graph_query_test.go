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

func testCanonicalPathKey(edge, node model.GraphDigest) string {
	return "calls\x00" + edge.String() + "\x00" + node.String()
}

func TestGraphPathPrefersCanonicalEqualLengthPath(t *testing.T) {
	ctx := context.Background()
	db := impactTestDB(t)
	bundle := impactTestBundle(t)
	altKey, err := model.NewNodeKey(model.NodeKeyFields{RepositoryIdentity: "https://example.com/repo", BackendType: "go", Language: "go", ProjectOrModule: "core", OwnerPath: "src/alternate.go", SymbolKind: "function", SemanticIdentity: "alternate"})
	if err != nil {
		t.Fatal(err)
	}
	altDigest, _ := altKey.Hash()
	generation := bundle.Generation.GenerationID
	if _, err := db.Exec(`INSERT INTO graph_nodes (generation_id,node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, generation[:], 100, altDigest[:], "milsp-node-key/v1", altKey.RepositoryIdentity, altKey.BackendType, altKey.Language, altKey.ProjectOrModule, altKey.OwnerPath, altKey.SymbolKind, altKey.SemanticIdentity, "src/alternate.go", altDigest[:], model.GraphRecordExact, model.NodeRID(altDigest), altKey.OwnerPath); err != nil {
		t.Fatal(err)
	}
	altFirst := model.EdgeKey(bundle.Nodes[2].NodeKey, altDigest, "calls", "workspace")
	altSecond := model.EdgeKey(altDigest, bundle.Nodes[0].NodeKey, "calls", "workspace")
	for _, edge := range []struct {
		id   int
		key  model.GraphDigest
		from int
		to   int
	}{
		{100, altFirst, 2, 100},
		{101, altSecond, 100, 0},
	} {
		if _, err := db.Exec(`INSERT INTO graph_edges (generation_id,edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, generation[:], edge.id, edge.key[:], edge.from, edge.to, "calls", "workspace", model.GraphRecordExact, "src/alternate.go", "go", model.EdgeRID(edge.key)); err != nil {
			t.Fatal(err)
		}
	}
	env, err := GraphQuery(ctx, db, model.GraphQueryRequest{Operation: "nav.path", From: "src/upstream.go", To: "src/target.go", Direction: "out", Relations: []string{"calls"}, Depth: 2, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := env.Items.([]model.GraphQueryItem)
	if !ok || len(items) != 2 {
		t.Fatalf("path items=%#v", env.Items)
	}
	callerFirst := testCanonicalPathKey(model.EdgeKey(bundle.Nodes[2].NodeKey, bundle.Nodes[1].NodeKey, "calls", "workspace"), bundle.Nodes[1].NodeKey)
	callerFirst += testCanonicalPathKey(model.EdgeKey(bundle.Nodes[1].NodeKey, bundle.Nodes[0].NodeKey, "calls", "workspace"), bundle.Nodes[0].NodeKey)
	altPath := testCanonicalPathKey(altFirst, altDigest)
	altPath += testCanonicalPathKey(altSecond, bundle.Nodes[0].NodeKey)
	want := bundle.Nodes[1].CrossRID
	if altPath < callerFirst {
		want = model.NodeRID(altDigest)
	}
	if items[0].ToCrossRID != want {
		t.Fatalf("first hop=%q want=%q callerKey=%q alternateKey=%q", items[0].ToCrossRID, want, callerFirst, altPath)
	}
}

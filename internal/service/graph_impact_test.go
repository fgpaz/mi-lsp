package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestGraphImpactRejectsInvalidBudgetBeforeStoreRead(t *testing.T) {
	_, err := GraphImpact(context.Background(), nil, model.GraphImpactRequest{Depth: model.GraphQueryMaxDepth + 1})
	if err == nil {
		t.Fatal("expected backend boundary error")
	}
	graphErr, ok := err.(*model.GraphQueryError)
	if !ok || graphErr.Code != "GPH_QUERY_BACKEND_UNAVAILABLE" {
		t.Fatalf("error=%v", err)
	}
}

func TestGraphImpactRequiresReadOnlyInboundSemantics(t *testing.T) {
	_, err := GraphImpactRequestForTest(model.GraphImpactRequest{Direction: "out"})
	if err == nil {
		t.Fatal("expected outbound impact rejection")
	}
	_, err = GraphImpactRequestForTest(model.GraphImpactRequest{Relations: []string{"references"}})
	if err == nil {
		t.Fatal("expected non-impact relation rejection")
	}
}

func TestGraphImpactDirectInboundSeparatesInferredAndIncludesEvidence(t *testing.T) {
	db := impactTestDB(t)
	before := graphGenerationCount(t, db)
	env, err := GraphImpact(context.Background(), db, model.GraphImpactRequest{
		Paths:     []string{"./src/target.go", "src/target.go"},
		Relations: []string{"calls"},
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := graphGenerationCount(t, db); got != before {
		t.Fatalf("query_only violated: generations=%d want %d", got, before)
	}
	if len(env.Items) != 1 || env.Items[0].Path != "src/caller.go" || env.Items[0].ConfidenceClass != "exact" {
		t.Fatalf("primary=%+v", env.Items)
	}
	item := env.Items[0]
	if item.TriggerPath != "src/target.go" || len(item.EvidencePath) != 1 || item.EvidencePath[0].Relation != "calls" || len(item.EvidencePath[0].EvidenceRefs) != 1 || len(item.EvidenceRefs) != 1 {
		t.Fatalf("missing explainable evidence: %+v", item)
	}
	if len(env.Inferred) != 1 || env.Inferred[0].Path != "src/guessed.go" || env.Inferred[0].ConfidenceClass != "inferred" {
		t.Fatalf("inferred=%+v", env.Inferred)
	}
	if env.Stats.Seeds != 1 || env.Stats.Returned != 1 {
		t.Fatalf("stats=%+v", env.Stats)
	}
}

func TestGraphImpactTransitiveGatesTestsAndReportsDepthAndBudgetTruncation(t *testing.T) {
	db := impactTestDB(t)
	ctx := context.Background()
	withoutTests, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Mode: model.GraphImpactModeTransitive, Depth: 2, Relations: []string{"calls"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutTests.Items) != 2 || withoutTests.Items[0].Path != "src/caller.go" || withoutTests.Items[1].Path != "src/upstream.go" || withoutTests.Items[1].Distance != 2 {
		t.Fatalf("transitive=%+v", withoutTests.Items)
	}
	withTests, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Relations: []string{"tests"}, IncludeTests: true, Limit: 10})
	if err != nil || len(withTests.Items) != 1 || withTests.Items[0].Path != "src/target_test.go" {
		t.Fatalf("tests=%+v err=%v", withTests.Items, err)
	}
	depthLimited, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Mode: model.GraphImpactModeTransitive, Depth: 1, Relations: []string{"calls"}, Limit: 10})
	if err != nil || !depthLimited.Truncated || len(depthLimited.Items) != 1 || depthLimited.Stats.Frontier != 1 {
		t.Fatalf("depth truncation=%+v err=%v", depthLimited, err)
	}
	limited, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Relations: []string{"calls"}, Limit: 1, TokenBudget: 1})
	if err != nil || !limited.Truncated || len(limited.Items)+len(limited.Inferred) != 1 || limited.Continuation == nil || limited.Continuation.Next.Query == "" {
		t.Fatalf("budget truncation=%+v err=%v", limited, err)
	}
	resumed, err := GraphImpact(ctx, db, model.GraphImpactRequest{Cursor: limited.Continuation.Next.Query})
	if err != nil || len(resumed.Items)+len(resumed.Inferred) != 1 || resumed.GenerationID != limited.GenerationID {
		t.Fatalf("cursor-only continuation=%+v err=%v", resumed, err)
	}
}

func TestGraphImpactContinuationRejectsAlteredRequestAndGeneration(t *testing.T) {
	db := impactTestDB(t)
	first, err := GraphImpact(context.Background(), db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Relations: []string{"calls"}, Limit: 1, TokenBudget: 1})
	if err != nil || first.Continuation == nil {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	cursor := first.Continuation.Next.Query
	if _, err := GraphImpact(context.Background(), db, model.GraphImpactRequest{Cursor: cursor, Paths: []string{"src/caller.go"}}); graphErrorCode(err) != "GPH_QUERY_CURSOR_STALE" {
		t.Fatalf("altered paths error=%v", err)
	}
	if _, err := GraphImpact(context.Background(), db, model.GraphImpactRequest{Cursor: cursor, Generation: "other-generation"}); graphErrorCode(err) != "GPH_QUERY_CURSOR_STALE" {
		t.Fatalf("altered generation error=%v", err)
	}
	decoded, err := decodeGraphImpactCursor(cursor)
	if err != nil || len(decoded.Paths) != 1 || decoded.Paths[0] != "src/target.go" {
		t.Fatalf("cursor=%+v err=%v", decoded, err)
	}
}

func TestGraphImpactAppContinuationUsesOnlyOperationAndCursor(t *testing.T) {
	root, alias := impactAppWorkspace(t)
	app := New(root, nil)
	first, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.graph-impact",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"paths":        []string{"./src\\target.go"},
			"mode":         "transitive",
			"depth":        2,
			"relations":    []string{"calls"},
			"limit":        1,
			"token_budget": 1,
		},
	})
	if err != nil {
		t.Fatalf("first app request: %v", err)
	}
	if first.Continuation == nil || first.Continuation.Next.Op != "nav.graph-impact" || first.Continuation.Next.Query == "" {
		t.Fatalf("first continuation=%+v", first.Continuation)
	}
	second, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: first.Continuation.Next.Op,
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"cursor": first.Continuation.Next.Query},
	})
	if err != nil {
		t.Fatalf("cursor-only app request: %v", err)
	}
	items, ok := second.Items.([]model.GraphImpactItem)
	if !ok || len(items) != 1 || items[0].Path != "src/upstream.go" {
		t.Fatalf("second items=%T %+v", second.Items, second.Items)
	}
}

func graphErrorCode(err error) string {
	graphErr, ok := err.(*model.GraphQueryError)
	if !ok {
		return ""
	}
	return graphErr.Code
}

func impactAppWorkspace(t *testing.T) (string, string) {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "graph-impact-" + filepath.Base(root)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	bundle := impactTestBundle(t)
	if err := store.StageGraphGeneration(context.Background(), db, &bundle); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := store.ActivateGraphGeneration(context.Background(), db, bundle.Generation.GenerationID, nil); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	return root, alias
}

func TestGraphImpactTransitiveStopsAtNonTransitiveRelations(t *testing.T) {
	db := impactTestDB(t)
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		testsOnly, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Mode: model.GraphImpactModeTransitive, Depth: 3, Relations: []string{"tests"}, Limit: 10})
		if err != nil || len(testsOnly.Items) != 1 || testsOnly.Items[0].Path != "src/target_test.go" {
			t.Fatalf("run=%d tests-only=%+v err=%v", i, testsOnly.Items, err)
		}
		mixed, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/target.go"}, Mode: model.GraphImpactModeTransitive, Depth: 3, Relations: []string{"calls", "tests"}, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		paths := map[string]bool{}
		for _, item := range mixed.Items {
			paths[item.Path] = true
		}
		if !paths["src/caller.go"] || !paths["src/upstream.go"] || !paths["src/test_consumer.go"] || paths["src/test_upstream.go"] {
			t.Fatalf("run=%d mixed paths=%v", i, paths)
		}
	}
}

func TestGraphImpactUnresolvedAndDigestAreDeterministic(t *testing.T) {
	db := impactTestDB(t)
	ctx := context.Background()
	missing, err := GraphImpact(ctx, db, model.GraphImpactRequest{Paths: []string{"src/missing.go", "src/target.go"}, Relations: []string{"calls"}, Limit: 10})
	if err != nil || len(missing.Omissions) != 1 || len(missing.Items) != 1 {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	request := model.GraphImpactRequest{ChangedPaths: []string{"src/target.go", "./src/target.go"}, Relations: []string{"calls"}, Limit: 10}
	first, err := GraphImpact(ctx, db, request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		next, runErr := GraphImpact(ctx, db, request)
		if runErr != nil || next.DeterminismDigest == "" || next.DeterminismDigest != first.DeterminismDigest {
			t.Fatalf("run=%d digest=%q first=%q err=%v", i, next.DeterminismDigest, first.DeterminismDigest, runErr)
		}
	}
}

func GraphImpactRequestForTest(request model.GraphImpactRequest) (model.GraphImpactRequest, error) {
	return request.Normalize()
}

func impactTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	bundle := impactTestBundle(t)
	if err := store.StageGraphGeneration(context.Background(), db, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateGraphGeneration(context.Background(), db, bundle.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	return db
}

func graphGenerationCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM graph_generations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func impactTestBundle(t *testing.T) model.GraphBundle {
	t.Helper()
	paths := []string{"src/target.go", "src/caller.go", "src/upstream.go", "src/guessed.go", "src/target_test.go", "src/test_consumer.go", "src/test_upstream.go", "src/test_chain.go"}
	nodes := make([]model.GraphNodeRecord, len(paths))
	for i, path := range paths {
		key, err := model.NewNodeKey(model.NodeKeyFields{RepositoryIdentity: "https://example.com/repo", BackendType: "go", Language: "go", ProjectOrModule: "core", OwnerPath: path, SymbolKind: "function", SemanticIdentity: fmt.Sprintf("n%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		digest, _ := key.Hash()
		nodes[i] = model.GraphNodeRecord{NodeID: i, Identity: key, IdentitySchema: "milsp-node-key/v1", NodeKey: digest, DisplayName: path, SourceDigest: digest, ClaimStatus: model.GraphRecordExact, CrossRID: model.NodeRID(digest), SortKey: path}
	}
	edge := func(id, from, to int, relation, status string) model.GraphEdgeRecord {
		key := model.EdgeKey(nodes[from].NodeKey, nodes[to].NodeKey, relation, "workspace")
		return model.GraphEdgeRecord{EdgeID: id, EdgeKey: key, FromNodeID: from, ToNodeID: to, Relation: relation, ClaimScope: "workspace", ClaimStatus: status, OwnerPath: nodes[from].Identity.OwnerPath, SourceBackend: "go", CrossRID: model.EdgeRID(key)}
	}
	edges := []model.GraphEdgeRecord{
		edge(0, 1, 0, "calls", model.GraphRecordExact),
		edge(1, 2, 1, "calls", model.GraphRecordExtracted),
		edge(2, 3, 0, "calls", model.GraphRecordInferred),
		edge(3, 4, 0, "tests", model.GraphRecordExact),
		edge(4, 2, 0, "references", model.GraphRecordExact),
		edge(5, 5, 1, "tests", model.GraphRecordExact),
		edge(6, 6, 5, "calls", model.GraphRecordExact),
		edge(7, 7, 4, "tests", model.GraphRecordExtracted),
	}
	source := nodes[1].SourceDigest
	evidence := make([]model.GraphEvidence, len(edges))
	for i, edge := range edges {
		edgeID := i
		digest := model.EvidenceDigest(nodes[edge.FromNodeID].SourceDigest, edge.EdgeKey, edge.OwnerPath, edge.Relation, "go", "test/v1", 1, 1, 1, 8)
		key := model.EvidenceKey(edge.EdgeKey, digest, i)
		evidence[i] = model.GraphEvidence{EvidenceID: i, EvidenceKey: key, EvidenceDigest: digest, SubjectKind: "edge", EdgeID: &edgeID, SourceURI: edge.OwnerPath, StartLine: intRef(1), StartColumn: intRef(1), EndLine: intRef(1), EndColumn: intRef(8), Backend: "go", ExtractorVersion: "test/v1", SourceDigest: nodes[edge.FromNodeID].SourceDigest, ClaimKind: edge.Relation, ObservedClaimDigest: edge.EdgeKey, ClaimStatus: edge.ClaimStatus, CrossRID: model.EvidenceRID(key)}
	}
	bundle := model.GraphBundle{
		Generation: model.GraphGeneration{SchemaVersion: 1, WorkspaceIdentity: "example.com/repo", RepositoryIdentity: "https://example.com/repo", SourceFingerprint: source, ConfigFingerprint: source, BackendManifestDigest: source, Status: model.GraphGenerationStaged, NodeCount: len(nodes), EdgeCount: len(edges), EvidenceCount: len(evidence)},
		Nodes:      nodes,
		Edges:      edges,
		Evidence:   evidence,
	}
	if err := bundle.SealIDs(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func intRef(value int) *int { return &value }

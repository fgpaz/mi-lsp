package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestBeginGraphQuerySnapshotRejectsCatalogGenerationMismatch(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()
	bundle := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(ctx, db, bundle.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetGraphRuntimeState(ctx, db, GraphRuntimeFresh, "catalog-a"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertWorkspaceMeta(ctx, db, WorkspaceMetaActiveCatalogGeneration, "catalog-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginGraphQuerySnapshot(ctx, db, ""); err == nil {
		t.Fatal("expected stale graph rejection")
	}
	if err := SetGraphRuntimeState(ctx, db, GraphRuntimeFresh, ""); err != nil {
		t.Fatal(err)
	}
	if _, bound, err := WorkspaceMetaValue(ctx, db, GraphCatalogGenerationMeta); err != nil || bound {
		t.Fatalf("binding not cleared: bound=%v err=%v", bound, err)
	}
	if snapshot, err := BeginGraphQuerySnapshot(ctx, db, ""); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot.Close()
	}
}

func TestBeginGraphQuerySnapshotAcceptsActiveAndRetiredOnly(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &b); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginGraphQuerySnapshot(ctx, db, b.Generation.GenerationID.String()); !errors.Is(err, ErrGraphQueryGenerationStatus) {
		t.Fatalf("staged error=%v", err)
	}
	if err := ActivateGraphGeneration(ctx, db, b.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	s, err := BeginGraphQuerySnapshot(ctx, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Generation(); err != nil || got.GenerationID != b.Generation.GenerationID {
		t.Fatalf("generation=%+v err=%v", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE graph_generations SET status=? WHERE generation_id=?`, model.GraphGenerationRetired, b.Generation.GenerationID[:]); err != nil {
		t.Fatal(err)
	}
	retired, err := BeginGraphQuerySnapshot(ctx, db, b.Generation.GenerationID.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = retired.Close()
	if _, err := BeginGraphQuerySnapshot(ctx, db, "not-a-digest"); !errors.Is(err, ErrGraphQueryGenerationMissing) {
		t.Fatalf("missing error=%v", err)
	}
}

func TestGraphQuerySnapshotRejectsInvalidAndCancelledQueries(t *testing.T) {
	var nilSnapshot *GraphQuerySnapshot
	if _, err := nilSnapshot.EdgesFromFrontier(context.Background(), []int{0}, "out", nil, 1); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("nil snapshot error=%v", err)
	}
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &b); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(context.Background(), db, b.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	s, err := BeginGraphQuerySnapshot(context.Background(), db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.EdgesFromFrontier(context.Background(), []int{0}, "injected", nil, 1); err == nil {
		t.Fatal("invalid direction accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.EdgesFromFrontier(ctx, []int{0}, "out", []string{"calls'); DROP TABLE graph_edges;--"}, 1); err == nil {
		t.Fatal("cancelled query succeeded")
	}
	if _, err := s.EdgesFromFrontier(context.Background(), []int{-1}, "out", nil, 1); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("negative frontier error=%v", err)
	}
}

func TestSanitizeGraphQueryErrorDoesNotLeakDatabaseDetail(t *testing.T) {
	err := SanitizeGraphQueryError(errors.New("SQLITE_ERROR: sensitive schema detail"))
	if err == nil || err.Error() != "GPH_QUERY_BACKEND_UNAVAILABLE: graph backend is unavailable" {
		t.Fatalf("sanitized error=%v", err)
	}
}

func TestGraphSelectorIndexesAreGenerationScopedAndUsable(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	for _, name := range []string{"idx_graph_nodes_generation_semantic_identity", "idx_graph_nodes_generation_display_name"} {
		var found string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&found); err != nil || found != name {
			t.Fatalf("index %q missing: found=%q err=%v", name, found, err)
		}
	}
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT node_id FROM graph_nodes WHERE generation_id=? AND semantic_identity=?`, make([]byte, 32), "subject")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var detail string
	for rows.Next() {
		var id, parent, notUsed int
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "idx_graph_nodes_generation_semantic_identity") {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("semantic selector did not use the generation-scoped index")
}

func TestGraphEdgesBothUsesSharedFairBudget(t *testing.T) {
	out := []model.GraphEdgeRecord{{EdgeID: 1}, {EdgeID: 2}, {EdgeID: 3}}
	in := []model.GraphEdgeRecord{{EdgeID: 1}, {EdgeID: 11}, {EdgeID: 12}}
	edges := interleaveGraphEdges(out, in, 4)
	if len(edges) != 4 {
		t.Fatalf("edges=%+v", edges)
	}
	seen := map[int]bool{}
	for _, edge := range edges {
		if seen[edge.EdgeID] {
			t.Fatalf("duplicate edge survived both-direction merge: %+v", edges)
		}
		seen[edge.EdgeID] = true
	}
	for _, id := range []int{1, 2, 11, 12} {
		if !seen[id] {
			t.Fatalf("both-direction merge lost fair candidates: %+v", edges)
		}
	}
}

func TestGraphBatchHydrationReturnsNodesAndEvidenceWithoutPerItemCalls(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()
	bundle := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(ctx, db, bundle.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	node := bundle.Nodes[0]
	edgeKey := model.EdgeKey(node.NodeKey, node.NodeKey, "calls", "workspace")
	if _, err := db.Exec(`INSERT INTO graph_edges (generation_id,edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, bundle.Generation.GenerationID[:], 0, edgeKey[:], 0, 0, "calls", "workspace", model.GraphRecordExact, node.Identity.OwnerPath, "go", model.EdgeRID(edgeKey)); err != nil {
		t.Fatal(err)
	}
	evidenceDigest := model.EvidenceDigest(node.SourceDigest, edgeKey, node.Identity.OwnerPath, "calls", "go", "test/v1", 1, 1, 1, 8)
	evidenceKey := model.EvidenceKey(edgeKey, evidenceDigest, 0)
	if _, err := db.Exec(`INSERT INTO graph_evidence (generation_id,evidence_id,evidence_key,subject_kind,node_id,edge_id,source_uri,start_line,start_column,end_line,end_column,backend,extractor_version,source_digest,claim_kind,observed_claim_digest,claim_status,cross_rid) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, bundle.Generation.GenerationID[:], 0, evidenceKey[:], "edge", nil, 0, node.Identity.OwnerPath, 1, 1, 1, 8, "go", "test/v1", node.SourceDigest[:], "calls", edgeKey[:], model.GraphRecordExact, model.EvidenceRID(evidenceKey)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BeginGraphQuerySnapshot(ctx, db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	nodes, err := snapshot.NodesByIDs(ctx, []int{0, 0})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes=%+v err=%v", nodes, err)
	}
	evidence, err := snapshot.EvidenceRefsByEdges(ctx, []int{0, 0}, 32)
	if err != nil || len(evidence[0]) != 1 {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

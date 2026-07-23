package store

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestResolveImpactSeedsUsesExactOwnerPathsOnly(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	bundle := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(ctx, db, bundle.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BeginGraphQuerySnapshot(ctx, db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()

	resolved, err := snapshot.ResolveImpactSeeds(ctx, []string{"./main.go", "main.go"}, 10)
	if err != nil || len(resolved.Nodes) != 1 || len(resolved.Omissions) != 0 {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if resolved.Nodes[0].ClaimStatus != model.GraphRecordExtracted {
		t.Fatalf("seed=%+v", resolved.Nodes[0])
	}

	missing, err := snapshot.ResolveImpactSeeds(ctx, []string{"src/nearby.go"}, 10)
	if err != nil || len(missing.Nodes) != 0 || len(missing.Omissions) != 1 || missing.Omissions[0].Code != "GPH_IMPACT_SEED_UNRESOLVED" {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
}

func TestImpactEdgesRejectsInvalidDirectionAndReturnsEmptyFrontier(t *testing.T) {
	db, _ := seedTestDB(t)
	bundle := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(context.Background(), db, bundle.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BeginGraphQuerySnapshot(context.Background(), db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if _, err := snapshot.ImpactEdges(context.Background(), []int{0}, "both", []string{"calls"}, []string{model.GraphRecordExact}, 10); err == nil {
		t.Fatal("expected invalid direction")
	}
	edges, err := snapshot.ImpactEdges(context.Background(), nil, "in", []string{"calls"}, []string{model.GraphRecordExact}, 10)
	if err != nil || len(edges) != 0 {
		t.Fatalf("edges=%+v err=%v", edges, err)
	}
}

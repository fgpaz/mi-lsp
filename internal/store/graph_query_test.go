package store

import (
	"context"
	"errors"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

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

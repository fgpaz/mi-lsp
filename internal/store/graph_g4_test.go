package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func graphG4Digest(seed byte) model.GraphDigest {
	var d model.GraphDigest
	for i := range d {
		d[i] = seed
	}
	return d
}

func graphG4Migration(id string, from, to int, started time.Time) model.GraphMigration {
	return model.GraphMigration{
		MigrationID:     id,
		FromVersion:     from,
		ToVersion:       to,
		Status:          "prepared",
		PreflightDigest: graphG4Digest(1),
		BackupDigest:    graphG4Digest(2),
		StartedAt:       started,
	}
}

func TestPrepareGraphMigrationBootstrapsAbsentSchemaAtomically(t *testing.T) {
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	started := time.Unix(100, 0).UTC()
	m := graphG4Migration("bootstrap", 0, 1, started)
	if err := PrepareGraphMigration(context.Background(), db, m); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := PrepareGraphMigration(context.Background(), db, m); err != nil {
		t.Fatalf("idempotent bootstrap: %v", err)
	}
	for _, bad := range []model.GraphMigration{
		graphG4Migration("bad-negative", -1, 1, started),
		graphG4Migration("bad-target", 0, 2, started),
		graphG4Migration("bad-equal", 1, 1, started),
	} {
		if err := PrepareGraphMigration(context.Background(), db, bad); !errors.Is(err, ErrGraphMigrationTransition) {
			t.Fatalf("%s error=%v", bad.MigrationID, err)
		}
	}
}

func TestGraphCompatibilityCASAndRollback(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	if got, err := GetGraphCompatibilityState(ctx, db); err != nil || got != GraphCompatibilityLegacyPreservedNoDualWrite {
		t.Fatalf("initial state=%q err=%v", got, err)
	}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityLegacyPreservedNoDualWrite, GraphCompatibilityDualReadWrite); err != nil {
		t.Fatal(err)
	}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityLegacyPreservedNoDualWrite, GraphCompatibilityDualReadWrite); !errors.Is(err, ErrGraphCompatibilityConflict) {
		t.Fatalf("stale transition=%v", err)
	}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityDualReadWrite, GraphCompatibilityGraphAuthoritative); err != nil {
		t.Fatal(err)
	}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityGraphAuthoritative, GraphCompatibilityLegacyPreservedNoDualWrite); !errors.Is(err, ErrGraphMigrationTransition) {
		t.Fatalf("skipped rollback=%v", err)
	}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityGraphAuthoritative, GraphCompatibilityDualReadWrite); !errors.Is(err, ErrGraphMigrationTransition) {
		t.Fatalf("skipped reverse=%v", err)
	}
}

func TestGraphCleanupPreservesActivePreviousAndReferences(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	prior := testGraphBundle(t)
	prior.Generation.CreatedAt = time.Unix(100, 0).UTC()
	if err := prior.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &prior); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, prior.Generation.GenerationID, nil, time.Unix(101, 0)); err != nil {
		t.Fatal(err)
	}
	current := testGraphBundle(t)
	current.Generation.SourceFingerprint = graphG4Digest(9)
	current.Generation.ConfigFingerprint = graphG4Digest(9)
	current.Generation.BackendManifestDigest = graphG4Digest(9)
	current.Generation.CreatedAt = time.Unix(200, 0).UTC()
	if err := current.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &current); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, current.Generation.GenerationID, &prior.Generation.GenerationID, time.Unix(201, 0)); err != nil {
		t.Fatal(err)
	}
	if err := CleanupGraphGenerations(ctx, db, "example.com/repo", time.Unix(300, 0)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM graph_generations WHERE generation_id=?", prior.Generation.GenerationID[:]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("previous generation was deleted")
	}
}

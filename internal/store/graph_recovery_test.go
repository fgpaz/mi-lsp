package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRecoverGraphStateRejectsMalformedActiveMetadataWithoutPointerMutation(t *testing.T) {
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &b); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(context.Background(), db, b.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE graph_generations SET created_at='not-a-timestamp' WHERE generation_id=?", b.Generation.GenerationID[:]); err != nil {
		t.Fatal(err)
	}
	if err := RecoverGraphState(context.Background(), db, "example.com/repo", time.Now().UTC()); !errors.Is(err, ErrGraphCrashRecoveryRequired) {
		t.Fatalf("RecoverGraphState error=%v", err)
	}
	var after []byte
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("active pointer changed: before=%x after=%x", before, after)
	}
}

func TestRecoverGraphStateRestoresValidatedPreviousGeneration(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	prior := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &prior); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(ctx, db, prior.Generation.GenerationID, nil); err != nil {
		t.Fatal(err)
	}
	current := testGraphBundle(t)
	current.Generation.SourceFingerprint = prior.Generation.GenerationID
	current.Generation.ConfigFingerprint = prior.Generation.GenerationID
	current.Generation.BackendManifestDigest = prior.Generation.GenerationID
	if err := current.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &current); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGeneration(ctx, db, current.Generation.GenerationID, &prior.Generation.GenerationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE graph_generations SET created_at='not-a-timestamp' WHERE generation_id=?", current.Generation.GenerationID[:]); err != nil {
		t.Fatal(err)
	}
	if err := RecoverGraphState(ctx, db, "example.com/repo", time.Now().UTC()); err != nil {
		t.Fatalf("RecoverGraphState error=%v", err)
	}
	var active, previous []byte
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if string(active) != string(prior.Generation.GenerationID[:]) {
		t.Fatalf("active pointer=%x, want=%x", active, prior.Generation.GenerationID)
	}
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previous); err != nil {
		t.Fatal(err)
	}
	if len(previous) != 0 {
		t.Fatalf("previous pointer=%x", previous)
	}
	var currentStatus, priorStatus string
	if err := db.QueryRow("SELECT status FROM graph_generations WHERE generation_id=?", current.Generation.GenerationID[:]).Scan(&currentStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT status FROM graph_generations WHERE generation_id=?", prior.Generation.GenerationID[:]).Scan(&priorStatus); err != nil {
		t.Fatal(err)
	}
	if currentStatus != model.GraphGenerationInvalid || priorStatus != model.GraphGenerationActive {
		t.Fatalf("statuses current=%q prior=%q", currentStatus, priorStatus)
	}
	var activeRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if activeRows != 1 {
		t.Fatalf("active rows=%d", activeRows)
	}
	if err := RecoverGraphState(ctx, db, "example.com/repo", time.Now().UTC()); err != nil {
		t.Fatalf("second RecoverGraphState error=%v", err)
	}
	var activeAfter, previousAfter []byte
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&activeAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previousAfter); err != nil {
		t.Fatal(err)
	}
	if string(activeAfter) != string(prior.Generation.GenerationID[:]) || len(previousAfter) != 0 {
		t.Fatalf("second recovery pointers active=%x previous=%x", activeAfter, previousAfter)
	}
}

func TestRecoverGraphGenerationsInvalidatesAbandonedStaging(t *testing.T) {
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &b); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO graph_migrations(migration_id,from_version,to_version,status,preflight_digest,backup_digest,started_at) VALUES('recovery-test',1,2,'applying',zeroblob(32),zeroblob(32),?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	err := RecoverGraphState(context.Background(), db, "example.com/repo", time.Now().UTC())
	if err != ErrGraphCrashRecoveryRequired {
		t.Fatalf("err=%v", err)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM graph_generations WHERE generation_id=?", b.Generation.GenerationID[:]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != model.GraphGenerationInvalid {
		t.Fatalf("status=%q", status)
	}
	for _, table := range []string{"graph_nodes", "graph_edges", "graph_evidence", "graph_unresolved"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE generation_id=?", b.Generation.GenerationID[:]).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s rows=%d", table, count)
		}
	}
	var migrationStatus string
	if err := db.QueryRow("SELECT status FROM graph_migrations WHERE migration_id='recovery-test'").Scan(&migrationStatus); err != nil {
		t.Fatal(err)
	}
	if migrationStatus != "rolled_back" && migrationStatus != "failed" {
		t.Fatalf("migration status=%q", migrationStatus)
	}
	var active, previous []byte
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active); err == nil && len(active) != 0 {
		t.Fatalf("active pointer=%x", active)
	}
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previous); err == nil && len(previous) != 0 {
		t.Fatalf("previous pointer=%x", previous)
	}
}

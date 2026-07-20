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

func TestGraphCompatibilityCoexistsWithLegacyAndRollbackHidesGraph(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "legacy", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	files := []model.FileRecord{{FilePath: "src/Foo.go", RepoID: "main", RepoName: "main", Language: "go"}}
	symbols := []model.SymbolRecord{{FilePath: "src/Foo.go", RepoID: "main", RepoName: "main", Name: "Foo", Kind: "function", QualifiedName: "Foo", Language: "go"}}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityLegacyPreservedNoDualWrite, GraphCompatibilityDualReadWrite); err != nil {
		t.Fatal(err)
	}
	job, err := CreateIndexJob(ctx, db, "legacy", t.TempDir(), IndexModeFull, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReplaceWorkspaceCatalog(ctx, db, job.GenerationID, project, files, symbols); err != nil {
		t.Fatal(err)
	}
	legacy, err := FindSymbols(ctx, db, "Foo", "", true, 10, 0)
	if err != nil || len(legacy) != 1 || legacy[0].Name != "Foo" {
		t.Fatalf("legacy query=%v err=%v", legacy, err)
	}
	bundle := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, bundle.Generation.GenerationID, nil, time.Unix(10, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateGraphGeneration(ctx, db, bundle.Generation.GenerationID); err != nil {
		t.Fatal(err)
	}
	if _, active, err := ActiveGraphGeneration(ctx, db); err != nil || !active {
		t.Fatalf("graph active=%v err=%v", active, err)
	}
	if err := TransitionGraphCompatibilityState(ctx, db, GraphCompatibilityDualReadWrite, GraphCompatibilityLegacyPreservedNoDualWrite); err != nil {
		t.Fatal(err)
	}
	if _, active, err := ActiveGraphGeneration(ctx, db); err != nil || active {
		t.Fatalf("graph remained active=%v err=%v", active, err)
	}
	legacy, err = FindSymbols(ctx, db, "Foo", "", true, 10, 0)
	if err != nil || len(legacy) != 1 {
		t.Fatalf("legacy rollback query=%v err=%v", legacy, err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM graph_generations WHERE generation_id=?", bundle.Generation.GenerationID[:]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != model.GraphGenerationRetired {
		t.Fatalf("graph status=%q, want retired", status)
	}
}

func TestGraphReadSnapshotPinsPointerAcrossActivation(t *testing.T) {
	ctx := context.Background()
	db, root := seedTestDB(t)
	prior := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &prior); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, prior.Generation.GenerationID, nil, time.Unix(20, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	next := testGraphBundle(t)
	next.Generation.SourceFingerprint = graphG4Digest(8)
	next.Generation.ConfigFingerprint = graphG4Digest(8)
	next.Generation.BackendManifestDigest = graphG4Digest(8)
	if err := next.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, writer, &next); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BeginGraphReadSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	published := make(chan error, 1)
	go func() {
		published <- ActivateGraphGenerationAt(ctx, writer, next.Generation.GenerationID, &prior.Generation.GenerationID, time.Unix(21, 0).UTC())
	}()
	select {
	case err := <-published:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writer activation timed out behind read snapshot")
	}
	old, active, err := snapshot.ActiveGraphGeneration()
	if err != nil || !active || old.GenerationID != prior.Generation.GenerationID {
		t.Fatalf("snapshot active=%+v active=%v err=%v", old, active, err)
	}
	if _, err := snapshot.ValidateGraphGeneration(ctx, prior.Generation.GenerationID); err != nil {
		t.Fatalf("snapshot old validation: %v", err)
	}
	fresh, active, err := ActiveGraphGeneration(ctx, writer)
	if err != nil || !active || fresh != next.Generation.GenerationID {
		t.Fatalf("fresh active=%x active=%v err=%v", fresh, active, err)
	}
	if _, err := ValidateGraphGeneration(ctx, writer, next.Generation.GenerationID); err != nil {
		t.Fatalf("fresh validation: %v", err)
	}
}

func TestGraphMigrationLifecycleAndIllegalCASAreDurable(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	started := time.Unix(30, 0).UTC()
	m := graphG4Migration("lifecycle", 1, 2, started)
	if err := PrepareGraphMigration(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	if err := PrepareGraphMigration(ctx, db, m); err != nil {
		t.Fatalf("idempotent prepare: %v", err)
	}
	if err := TransitionGraphMigration(ctx, db, m.MigrationID, "prepared", "applying", "", started.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := TransitionGraphMigration(ctx, db, m.MigrationID, "applying", "validated", "", started.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := TransitionGraphMigration(ctx, db, m.MigrationID, "validated", "committed", "", started.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := TransitionGraphMigration(ctx, db, m.MigrationID, "validated", "rolled_back", "", started.Add(4*time.Second)); !errors.Is(err, ErrGraphMigrationTransition) {
		t.Fatalf("terminal CAS=%v", err)
	}
	var status, completed, errorCode string
	if err := db.QueryRowContext(ctx, "SELECT status,completed_at,error_code FROM graph_migrations WHERE migration_id=?", m.MigrationID).Scan(&status, &completed, &errorCode); err != nil {
		t.Fatal(err)
	}
	if status != "committed" || completed == "" || errorCode != "" {
		t.Fatalf("migration terminal state status=%q completed=%q error=%q", status, completed, errorCode)
	}
	if err := TransitionGraphMigration(nil, db, "id", "prepared", "applying", "", started); !errors.Is(err, ErrGraphMigrationTransition) {
		t.Fatalf("nil context error=%v", err)
	}
}

func TestGraphMigrationRollbackIsAllowedFromEveryNonterminalState(t *testing.T) {
	ctx := context.Background()
	for _, state := range []string{"prepared", "applying", "validated"} {
		t.Run(state, func(t *testing.T) {
			db, _ := seedTestDB(t)
			started := time.Unix(60, 0).UTC()
			m := graphG4Migration("rollback-"+state, 1, 2, started)
			if err := PrepareGraphMigration(ctx, db, m); err != nil {
				t.Fatal(err)
			}
			if state != "prepared" {
				if err := TransitionGraphMigration(ctx, db, m.MigrationID, "prepared", "applying", "", started.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if state == "validated" {
				if err := TransitionGraphMigration(ctx, db, m.MigrationID, "applying", "validated", "", started.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if err := RollbackGraphMigration(ctx, db, m.MigrationID, state, "GPH_TEST_ROLLBACK", started.Add(3*time.Second)); err != nil {
				t.Fatal(err)
			}
			var got, code, completed string
			if err := db.QueryRowContext(ctx, "SELECT status,error_code,completed_at FROM graph_migrations WHERE migration_id=?", m.MigrationID).Scan(&got, &code, &completed); err != nil {
				t.Fatal(err)
			}
			if got != "rolled_back" || code != "GPH_TEST_ROLLBACK" || completed == "" {
				t.Fatalf("rollback state=%q code=%q completed=%q", got, code, completed)
			}
		})
	}
}

func TestRecoverGraphMigrationCrashWindowsPreserveActiveGeneration(t *testing.T) {
	ctx := context.Background()
	for _, state := range []string{"prepared", "applying", "validated"} {
		t.Run(state, func(t *testing.T) {
			db, _ := seedTestDB(t)
			prior := testGraphBundle(t)
			if err := StageGraphGeneration(ctx, db, &prior); err != nil {
				t.Fatal(err)
			}
			if err := ActivateGraphGeneration(ctx, db, prior.Generation.GenerationID, nil); err != nil {
				t.Fatal(err)
			}
			current := testGraphBundle(t)
			current.Generation.SourceFingerprint = graphG4Digest(70)
			current.Generation.ConfigFingerprint = graphG4Digest(70)
			current.Generation.BackendManifestDigest = graphG4Digest(70)
			if err := current.SealIDs(); err != nil {
				t.Fatal(err)
			}
			if err := StageGraphGeneration(ctx, db, &current); err != nil {
				t.Fatal(err)
			}
			if err := ActivateGraphGeneration(ctx, db, current.Generation.GenerationID, &prior.Generation.GenerationID); err != nil {
				t.Fatal(err)
			}
			m := graphG4Migration("recovery-"+state, 1, 2, time.Unix(70, 0).UTC())
			m.PriorActiveGenerationID = &prior.Generation.GenerationID
			if err := PrepareGraphMigration(ctx, db, m); err != nil {
				t.Fatal(err)
			}
			if state != "prepared" {
				if err := TransitionGraphMigration(ctx, db, m.MigrationID, "prepared", "applying", "", time.Unix(71, 0).UTC()); err != nil {
					t.Fatal(err)
				}
			}
			if state == "validated" {
				if err := TransitionGraphMigration(ctx, db, m.MigrationID, "applying", "validated", "", time.Unix(72, 0).UTC()); err != nil {
					t.Fatal(err)
				}
			}
			if err := RecoverGraphState(ctx, db, "example.com/repo", time.Unix(73, 0).UTC()); err != nil {
				t.Fatalf("recovery: %v", err)
			}
			active, ok, err := ActiveGraphGeneration(ctx, db)
			if err != nil || !ok || active != current.Generation.GenerationID {
				t.Fatalf("active=%x ok=%v err=%v", active, ok, err)
			}
			var got, code string
			if err := db.QueryRowContext(ctx, "SELECT status,error_code FROM graph_migrations WHERE migration_id=?", m.MigrationID).Scan(&got, &code); err != nil {
				t.Fatal(err)
			}
			if got != "rolled_back" || code != "GPH_CRASH_RECOVERY_ROLLBACK" {
				t.Fatalf("recovery terminal status=%q code=%q", got, code)
			}
		})
	}
}

func TestCleanupGraphGenerationsPropagatesProtectedScanErrors(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &b); err != nil {
		t.Fatal(err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, b.Generation.GenerationID, nil, time.Unix(40, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE workspace_meta SET value=? WHERE key=?", []byte{1}, graphActiveMeta); err != nil {
		t.Fatal(err)
	}
	if err := CleanupGraphGenerations(ctx, db, "example.com/repo", time.Unix(50, 0).UTC()); err == nil {
		t.Fatal("malformed active pointer accepted")
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

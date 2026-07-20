package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func testGraphBundle(t *testing.T) model.GraphBundle {
	t.Helper()
	key, err := model.NewNodeKey(model.NodeKeyFields{RepositoryIdentity: "https://example.com/repo", BackendType: "go", Language: "go", ProjectOrModule: "cmd", OwnerPath: "main.go", SymbolKind: "function", SemanticIdentity: "main"})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := key.Hash()
	b := model.GraphBundle{Generation: model.GraphGeneration{SchemaVersion: 1, WorkspaceIdentity: "example.com/repo", RepositoryIdentity: key.RepositoryIdentity, SourceFingerprint: d, ConfigFingerprint: d, BackendManifestDigest: d, Status: model.GraphGenerationStaged, NodeCount: 1, EdgeCount: 0, EvidenceCount: 0, UnresolvedCount: 0}, Nodes: []model.GraphNodeRecord{{NodeID: 0, Identity: key, IdentitySchema: "milsp-node-key/v1", NodeKey: d, DisplayName: "main", SourceDigest: d, ClaimStatus: model.GraphRecordExtracted, CrossRID: model.NodeRID(d), SortKey: "main"}}}
	if err := b.SealIDs(); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStageGraphGenerationRejectsNilBoundaries(t *testing.T) {
	b := testGraphBundle(t)
	if err := StageGraphGeneration(nil, nil, &b); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("nil context/db error=%v", err)
	}
	db, _ := seedTestDB(t)
	if err := StageGraphGeneration(context.Background(), nil, &b); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("nil db error=%v", err)
	}
	if _, err := beginGraphImmediate(nil, db); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("nil context transaction error=%v", err)
	}
}

func TestStageGraphGenerationIsAtomicAndInitiallyInvisible(t *testing.T) {
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &b); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM graph_generations WHERE generation_id=?", b.Generation.GenerationID[:]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != model.GraphGenerationStaged {
		t.Fatalf("status=%q", status)
	}
	var active []byte
	if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active); err == nil && len(active) != 0 {
		t.Fatal("staged generation became active")
	}
}

func TestValidateGraphGenerationStreamsPersistedContent(t *testing.T) {
	db, _ := seedTestDB(t)
	b := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &b); err != nil {
		t.Fatal(err)
	}
	g, err := ValidateGraphGeneration(context.Background(), db, b.Generation.GenerationID)
	if err != nil || g.ContentDigest != b.Generation.ContentDigest {
		t.Fatalf("validation=%+v err=%v", g, err)
	}
}

func TestLoadGenerationRejectsMalformedRequiredMetadata(t *testing.T) {
	for _, tc := range []struct {
		column, value string
	}{
		{"source_fingerprint", "NULL"},
		{"created_at", "NULL"},
		{"previous_generation_id", "zeroblob(31)"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			db, err := sql.Open(driverName, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err = db.Exec(`CREATE TABLE graph_generations (generation_id BLOB, schema_version INTEGER, workspace_identity TEXT, source_fingerprint BLOB, config_fingerprint BLOB, backend_manifest_digest BLOB, content_digest BLOB, status TEXT, node_count INTEGER, edge_count INTEGER, evidence_count INTEGER, unresolved_count INTEGER, previous_generation_id BLOB, created_at TEXT, published_at TEXT, error_code TEXT)`); err != nil {
				t.Fatal(err)
			}
			b := testGraphBundle(t)
			g := b.Generation
			if _, err = db.Exec(`INSERT INTO graph_generations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, g.GenerationID[:], g.SchemaVersion, g.WorkspaceIdentity, g.SourceFingerprint[:], g.ConfigFingerprint[:], g.BackendManifestDigest[:], g.ContentDigest[:], g.Status, g.NodeCount, g.EdgeCount, g.EvidenceCount, g.UnresolvedCount, nil, g.CreatedAt.UTC().Format(time.RFC3339Nano), nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("UPDATE graph_generations SET "+tc.column+"="+tc.value+" WHERE generation_id=?", g.GenerationID[:]); err != nil {
				t.Fatal(err)
			}
			if _, err = loadGeneration(context.Background(), db, g.GenerationID); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
				t.Fatalf("loadGeneration error=%v", err)
			}
		})
	}
}

func TestGraphGenerationRejectsMalformedPersistedMetadataWithoutActivation(t *testing.T) {
	cases := []struct {
		name, column, value string
	}{
		{"source digest", "source_fingerprint", "zeroblob(31)"},
		{"config digest", "config_fingerprint", "zeroblob(31)"},
		{"backend digest", "backend_manifest_digest", "zeroblob(31)"},
		{"content digest", "content_digest", "zeroblob(31)"},
		{"created timestamp", "created_at", "'not-a-timestamp'"},
		{"published timestamp", "published_at", "'not-a-timestamp'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := seedTestDB(t)
			b := testGraphBundle(t)
			if err := StageGraphGeneration(context.Background(), db, &b); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("PRAGMA ignore_check_constraints=ON"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE graph_generations SET "+tc.column+"="+tc.value+" WHERE generation_id=?", b.Generation.GenerationID[:]); err != nil {
				t.Fatal(err)
			}
			if _, err := ValidateGraphGeneration(context.Background(), db, b.Generation.GenerationID); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
				t.Fatalf("ValidateGraphGeneration error=%v", err)
			}
			if err := ActivateGraphGeneration(context.Background(), db, b.Generation.GenerationID, nil); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
				t.Fatalf("ActivateGraphGeneration error=%v", err)
			}
			var active []byte
			if err := db.QueryRow("SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active); err == nil && len(active) != 0 {
				t.Fatalf("active pointer mutated: %x", active)
			}
		})
	}
}

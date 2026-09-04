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
func seedRetiredGraphPairForAncestryTest(t *testing.T, ctx context.Context, db *sql.DB) (model.GraphBundle, model.GraphBundle) {
	t.Helper()
	first := testGraphBundle(t)
	first.Generation.CreatedAt = time.Unix(1, 0).UTC()
	if err := StageGraphGeneration(ctx, db, &first); err != nil {
		t.Fatalf("stage first graph: %v", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, first.Generation.GenerationID, nil, time.Unix(2, 0).UTC()); err != nil {
		t.Fatalf("activate first graph: %v", err)
	}

	second := testGraphBundle(t)
	second.Generation.CreatedAt = time.Unix(3, 0).UTC()
	second.Nodes[0].DisplayName = "candidate"
	if err := second.SealIDs(); err != nil {
		t.Fatalf("seal second graph: %v", err)
	}
	if err := StageGraphGeneration(ctx, db, &second); err != nil {
		t.Fatalf("stage second graph: %v", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, second.Generation.GenerationID, &first.Generation.GenerationID, time.Unix(4, 0).UTC()); err != nil {
		t.Fatalf("activate second graph: %v", err)
	}
	return first, second
}

func stageCrossWorkspaceRetiredGraphForAncestryTest(t *testing.T, ctx context.Context, db *sql.DB) model.GraphBundle {
	t.Helper()
	cross := testGraphBundle(t)
	identity, err := model.NormalizeRepositoryIdentity("https://other.example/repo")
	if err != nil {
		t.Fatal(err)
	}
	key := cross.Nodes[0].Identity
	key.RepositoryIdentity = identity
	nodeKey, err := key.Hash()
	if err != nil {
		t.Fatal(err)
	}
	cross.Generation.WorkspaceIdentity = identity
	cross.Generation.RepositoryIdentity = identity
	cross.Generation.SourceFingerprint = nodeKey
	cross.Generation.ConfigFingerprint = nodeKey
	cross.Generation.BackendManifestDigest = nodeKey
	cross.Generation.CreatedAt = time.Unix(5, 0).UTC()
	cross.Nodes[0].Identity = key
	cross.Nodes[0].NodeKey = nodeKey
	cross.Nodes[0].SourceDigest = nodeKey
	cross.Nodes[0].CrossRID = model.NodeRID(nodeKey)
	if err := cross.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &cross); err != nil {
		t.Fatalf("stage cross-workspace graph: %v", err)
	}
	if _, err := db.Exec("UPDATE graph_generations SET status=?,published_at=? WHERE generation_id=?", model.GraphGenerationRetired, cross.Generation.CreatedAt.Add(time.Second).Format(time.RFC3339Nano), digestArg(cross.Generation.GenerationID)); err != nil {
		t.Fatalf("retire cross-workspace graph: %v", err)
	}
	return cross
}

func setGraphPreviousForAncestryTest(t *testing.T, db *sql.DB, target model.GraphDigest, previous []byte, disableForeignKeys bool) {
	t.Helper()
	if disableForeignKeys {
		if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
			t.Fatalf("disable graph foreign keys: %v", err)
		}
	}
	if _, err := db.Exec("UPDATE graph_generations SET previous_generation_id=? WHERE generation_id=?", previous, digestArg(target)); err != nil {
		t.Fatalf("corrupt graph ancestry: %v", err)
	}
	if disableForeignKeys {
		if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
			t.Fatalf("restore graph foreign keys: %v", err)
		}
	}
}

func TestActivateGraphGenerationAtRejectsRetiredTargetAncestryCorruption(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"self_cycle", "multi_node_cycle", "dangling_predecessor", "zero_predecessor", "cross_workspace"} {
		t.Run(kind, func(t *testing.T) {
			db, _ := seedTestDB(t)
			first, second := seedRetiredGraphPairForAncestryTest(t, ctx, db)
			var previous []byte
			disableForeignKeys := false
			switch kind {
			case "self_cycle":
				previous = digestArg(first.Generation.GenerationID)
			case "multi_node_cycle":
				previous = digestArg(second.Generation.GenerationID)
			case "dangling_predecessor":
				previous = digestArg(model.GraphDigest{0x7f})
				disableForeignKeys = true
			case "zero_predecessor":
				previous = make([]byte, 32)
				disableForeignKeys = true
			case "cross_workspace":
				cross := stageCrossWorkspaceRetiredGraphForAncestryTest(t, ctx, db)
				previous = digestArg(cross.Generation.GenerationID)
			}
			setGraphPreviousForAncestryTest(t, db, first.Generation.GenerationID, previous, disableForeignKeys)

			err := ActivateGraphGenerationAt(ctx, db, first.Generation.GenerationID, &second.Generation.GenerationID, time.Unix(5, 0).UTC())
			if !errors.Is(err, model.ErrGraphGenerationCorrupt) {
				t.Fatalf("retired ancestry %s error=%v, want graph corruption", kind, err)
			}
			if got, ok, err := ActiveGraphGeneration(ctx, db); err != nil || !ok || got != second.Generation.GenerationID {
				t.Fatalf("active pointer after %s = %x ok=%v err=%v, want %x", kind, got, ok, err, second.Generation.GenerationID)
			}
			if got := graphStatusForTest(t, db, first.Generation.GenerationID); got != model.GraphGenerationRetired {
				t.Fatalf("retired target status after %s=%q", kind, got)
			}
			if got := graphStatusForTest(t, db, second.Generation.GenerationID); got != model.GraphGenerationActive {
				t.Fatalf("active prior status after %s=%q", kind, got)
			}
		})
	}
}

func TestActivateGraphGenerationAtRejectsCorruptActivePriorAncestry(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"active_self_cycle", "active_cross_workspace"} {
		t.Run(kind, func(t *testing.T) {
			db, _ := seedTestDB(t)
			first, second := seedRetiredGraphPairForAncestryTest(t, ctx, db)
			switch kind {
			case "active_self_cycle":
				setGraphPreviousForAncestryTest(t, db, second.Generation.GenerationID, digestArg(second.Generation.GenerationID), false)
			case "active_cross_workspace":
				cross := stageCrossWorkspaceRetiredGraphForAncestryTest(t, ctx, db)
				setGraphPreviousForAncestryTest(t, db, second.Generation.GenerationID, digestArg(cross.Generation.GenerationID), false)
			}
			err := ActivateGraphGenerationAt(ctx, db, first.Generation.GenerationID, &second.Generation.GenerationID, time.Unix(5, 0).UTC())
			if !errors.Is(err, model.ErrGraphGenerationCorrupt) {
				t.Fatalf("active prior ancestry %s error=%v, want graph corruption", kind, err)
			}
			if got, ok, err := ActiveGraphGeneration(ctx, db); err != nil || !ok || got != second.Generation.GenerationID {
				t.Fatalf("active pointer after %s = %x ok=%v err=%v, want %x", kind, got, ok, err, second.Generation.GenerationID)
			}
			if got := graphStatusForTest(t, db, first.Generation.GenerationID); got != model.GraphGenerationRetired {
				t.Fatalf("retired target status after %s=%q", kind, got)
			}
			if got := graphStatusForTest(t, db, second.Generation.GenerationID); got != model.GraphGenerationActive {
				t.Fatalf("active prior status after %s=%q", kind, got)
			}
		})
	}
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
func TestGraphUnresolvedContextRoundTripsAndKeepsEmptyFieldsNullable(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()
	bundle := testGraphBundle(t)
	bundle.Unresolved = []model.GraphUnresolved{
		{
			GenerationID:     bundle.Generation.GenerationID,
			UnresolvedID:     0,
			OwnerPath:        "docs/RF-SYNTHETIC.md",
			SubjectKind:      "document",
			SelectorDigest:   model.GraphDigest{1},
			ReasonCode:       "missing_doc_target",
			Backend:          "docgraph",
			SourceDocument:   "docs/RF-SYNTHETIC.md",
			SourceBlock:      "frontmatter",
			TargetKind:       "document",
			TargetValue:      "TP-SYNTHETIC-001",
			RecoveryHintCode: "inspect_doc_graph_reference",
		},
		{
			GenerationID:   bundle.Generation.GenerationID,
			UnresolvedID:   1,
			OwnerPath:      "docs/RF-SYNTHETIC.md",
			SubjectKind:    "document",
			SelectorDigest: model.GraphDigest{2},
			ReasonCode:     "missing_code_target",
			Backend:        "docgraph",
		},
	}
	for i := range bundle.Unresolved {
		bundle.Unresolved[i].UnresolvedKey = model.GraphUnresolvedKey(bundle.Unresolved[i])
		bundle.Unresolved[i].CrossRID = model.UnresolvedRID(bundle.Unresolved[i].UnresolvedKey)
	}
	bundle.Generation.UnresolvedCount = len(bundle.Unresolved)
	if err := bundle.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		t.Fatal(err)
	}
	var gotSourceDocument, gotSourceBlock, gotTargetKind, gotTargetValue sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT source_document,source_block,target_kind,target_value FROM graph_unresolved WHERE generation_id=? AND unresolved_id=0`, bundle.Generation.GenerationID[:]).Scan(&gotSourceDocument, &gotSourceBlock, &gotTargetKind, &gotTargetValue); err != nil {
		t.Fatal(err)
	}
	if gotSourceDocument.String != "docs/RF-SYNTHETIC.md" || gotSourceBlock.String != "frontmatter" || gotTargetKind.String != "document" || gotTargetValue.String != "TP-SYNTHETIC-001" {
		t.Fatalf("persisted diagnostic context = %#v %#v %#v %#v", gotSourceDocument, gotSourceBlock, gotTargetKind, gotTargetValue)
	}
	var nullSourceDocument, nullSourceBlock, nullTargetKind, nullTargetValue sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT source_document,source_block,target_kind,target_value FROM graph_unresolved WHERE generation_id=? AND unresolved_id=1`, bundle.Generation.GenerationID[:]).Scan(&nullSourceDocument, &nullSourceBlock, &nullTargetKind, &nullTargetValue); err != nil {
		t.Fatal(err)
	}
	if nullSourceDocument.Valid || nullSourceBlock.Valid || nullTargetKind.Valid || nullTargetValue.Valid {
		t.Fatalf("empty diagnostic context was not stored as NULL: %#v %#v %#v %#v", nullSourceDocument, nullSourceBlock, nullTargetKind, nullTargetValue)
	}
	if _, err := ValidateGraphGeneration(ctx, db, bundle.Generation.GenerationID); err != nil {
		t.Fatalf("persisted diagnostic context invalidated graph: %v", err)
	}
}
func TestGraphUnresolvedContextWhitespaceCanonicalizesAcrossSealAndReadback(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()

	bundle := testGraphBundle(t)
	bundle.Unresolved = []model.GraphUnresolved{{
		GenerationID:     bundle.Generation.GenerationID,
		UnresolvedID:     0,
		OwnerPath:        "docs/RF-SYNTHETIC.md",
		SubjectKind:      "document",
		SelectorDigest:   model.GraphDigest{1},
		ReasonCode:       "missing_doc_target",
		Backend:          "docgraph",
		SourceDocument:   " docs/RF-SYNTHETIC.md ",
		SourceBlock:      " frontmatter ",
		TargetKind:       " document ",
		TargetValue:      " TP-SYNTHETIC-001 ",
		RecoveryHintCode: "inspect_doc_graph_reference",
	}}
	bundle.Generation.UnresolvedCount = len(bundle.Unresolved)
	for i := range bundle.Unresolved {
		bundle.Unresolved[i].UnresolvedKey = model.GraphUnresolvedKey(bundle.Unresolved[i])
		bundle.Unresolved[i].CrossRID = model.UnresolvedRID(bundle.Unresolved[i].UnresolvedKey)
	}
	if err := bundle.SealIDs(); err != nil {
		t.Fatalf("seal bundle: %v", err)
	}
	got := bundle.Unresolved[0]
	if got.SourceDocument != "docs/RF-SYNTHETIC.md" || got.SourceBlock != "frontmatter" || got.TargetKind != "document" || got.TargetValue != "TP-SYNTHETIC-001" {
		t.Fatalf("sealed diagnostic context = %+v", got)
	}
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		t.Fatalf("stage bundle: %v", err)
	}
	var sourceDocument, sourceBlock, targetKind, targetValue sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT source_document,source_block,target_kind,target_value FROM graph_unresolved WHERE generation_id=? AND unresolved_id=0`, bundle.Generation.GenerationID[:]).Scan(&sourceDocument, &sourceBlock, &targetKind, &targetValue); err != nil {
		t.Fatal(err)
	}
	if sourceDocument.String != "docs/RF-SYNTHETIC.md" || sourceBlock.String != "frontmatter" || targetKind.String != "document" || targetValue.String != "TP-SYNTHETIC-001" {
		t.Fatalf("persisted diagnostic context = %#v %#v %#v %#v", sourceDocument, sourceBlock, targetKind, targetValue)
	}
	if _, err := ValidateGraphGeneration(ctx, db, bundle.Generation.GenerationID); err != nil {
		t.Fatalf("persisted whitespace-normalized graph invalidated: %v", err)
	}
}

func TestDocMentionSourceBlockSurvivesFullAndIncrementalRoundTrips(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()
	doc := model.DocRecord{Path: "docs/RF-SYNTHETIC.md", Title: "Synthetic", DocID: "RF-SYNTHETIC", Layer: "04", Family: "functional", ContentHash: "hash-1"}
	mention := model.DocMention{DocPath: doc.Path, MentionType: "test_file", MentionValue: "TP-SYNTHETIC-001", SourceBlock: "frontmatter"}
	if err := ReplaceDocsWithSources(ctx, db, []model.DocRecord{doc}, nil, []model.DocMention{mention}, nil, nil, nil); err != nil {
		t.Fatalf("full document replacement: %v", err)
	}
	assertMention := func(label string) {
		t.Helper()
		mentions, err := ListDocMentions(ctx, db)
		if err != nil {
			t.Fatalf("%s list mentions: %v", label, err)
		}
		if len(mentions) != 1 || mentions[0].SourceBlock != "frontmatter" {
			t.Fatalf("%s list mentions = %#v", label, mentions)
		}
		mentions, err = DocMentionsForPath(ctx, db, doc.Path)
		if err != nil {
			t.Fatalf("%s path mentions: %v", label, err)
		}
		if len(mentions) != 1 || mentions[0].SourceBlock != "frontmatter" {
			t.Fatalf("%s path mentions = %#v", label, mentions)
		}
	}
	assertMention("full")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyIncrementalDocChangesTx(ctx, tx, []IncrementalDocChange{{
		Action:   "replace",
		Path:     doc.Path,
		Doc:      &doc,
		Mentions: []model.DocMention{mention},
		State:    &model.DocArtifactState{Path: doc.Path, ContentSHA256: "sha-1", Lifecycle: model.DocLifecycleActive},
	}})
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("incremental document replacement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertMention("incremental")
}

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// legacyGraphDigestWriter reproduces the immutable graph content framing used
// before unresolved provenance fields were added to AddUnresolved. The fixture
// deliberately uses this framing instead of changing the production row data.
type legacyGraphDigestWriter struct{ h hash.Hash }

func newLegacyGraphDigestWriter() *legacyGraphDigestWriter {
	w := &legacyGraphDigestWriter{h: sha256.New()}
	w.raw("MILSP-GRAPH-CONTENT/v2")
	return w
}

func (w *legacyGraphDigestWriter) raw(value string) { _, _ = w.h.Write([]byte(value)) }
func (w *legacyGraphDigestWriter) frame(tag byte, value []byte) {
	_, _ = w.h.Write([]byte{tag})
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(value)))
	_, _ = w.h.Write(n[:])
	_, _ = w.h.Write(value)
}
func (w *legacyGraphDigestWriter) text(tag byte, value string) { w.frame(tag, []byte(value)) }
func (w *legacyGraphDigestWriter) digest(tag byte, value model.GraphDigest) {
	w.frame(tag, value[:])
}
func (w *legacyGraphDigestWriter) integer(tag byte, value int) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(value))
	w.frame(tag, n[:])
}
func (w *legacyGraphDigestWriter) section(index, count int) {
	sections := [...]string{
		"MILSP-GRAPH-NODES/v2",
		"MILSP-GRAPH-EDGES/v2",
		"MILSP-GRAPH-EVIDENCE/v2",
		"MILSP-GRAPH-UNRESOLVED/v2",
	}
	w.text(240, sections[index])
	w.integer(241, count)
}
func (w *legacyGraphDigestWriter) record() { w.frame(242, nil) }
func (w *legacyGraphDigestWriter) sum() model.GraphDigest {
	var digest model.GraphDigest
	copy(digest[:], w.h.Sum(nil))
	return digest
}

func legacyGraphContentDigestForTest(t *testing.T, b model.GraphBundle) model.GraphDigest {
	t.Helper()
	w := newLegacyGraphDigestWriter()
	w.section(0, len(b.Nodes))
	for _, n := range b.Nodes {
		w.record()
		w.integer(1, n.NodeID)
		w.digest(2, n.NodeKey)
		w.text(3, n.IdentitySchema)
		w.text(4, n.Identity.RepositoryIdentity)
		w.text(5, n.Identity.BackendType)
		w.text(6, n.Identity.Language)
		w.text(7, n.Identity.ProjectOrModule)
		w.text(8, n.Identity.OwnerPath)
		w.text(9, n.Identity.SymbolKind)
		w.text(10, n.Identity.SemanticIdentity)
		w.text(11, n.DisplayName)
		w.digest(12, n.SourceDigest)
		w.text(13, n.ClaimStatus)
		w.text(14, n.CrossRID)
		w.text(15, n.SortKey)
	}
	w.section(1, len(b.Edges))
	w.section(2, len(b.Evidence))
	w.section(3, len(b.Unresolved))
	for _, u := range b.Unresolved {
		w.record()
		w.integer(1, u.UnresolvedID)
		w.digest(2, u.UnresolvedKey)
		w.text(3, u.OwnerPath)
		w.text(4, u.SubjectKind)
		w.digest(5, u.SelectorDigest)
		w.text(6, u.ReasonCode)
		for _, candidate := range u.Candidates {
			w.text(7, candidate)
		}
		w.text(8, u.Backend)
		if u.SourceDigest != nil {
			w.digest(9, *u.SourceDigest)
		}
		w.text(10, u.CrossRID)
		w.text(11, u.RecoveryHintCode)
	}
	return w.sum()
}

func legacyGraphBundleForTest(t *testing.T) model.GraphBundle {
	t.Helper()
	bundle := testGraphBundle(t)
	bundle.Generation.CreatedAt = time.Unix(1, 0).UTC()
	bundle.Unresolved = []model.GraphUnresolved{{
		UnresolvedID:     0,
		OwnerPath:        "docs/legacy.md",
		SubjectKind:      "document",
		SelectorDigest:   model.GraphDigest{9},
		ReasonCode:       "missing_doc_target",
		Candidates:       []string{"TP-LEGACY-001"},
		Backend:          "docgraph",
		RecoveryHintCode: "inspect_doc_graph_reference",
		SourceDocument:   "docs/legacy.md",
		SourceBlock:      "frontmatter",
		TargetKind:       "document",
		TargetValue:      "TP-LEGACY-001",
	}}
	bundle.Generation.UnresolvedCount = len(bundle.Unresolved)
	for i := range bundle.Unresolved {
		bundle.Unresolved[i].GenerationID = bundle.Generation.GenerationID
		bundle.Unresolved[i].UnresolvedKey = model.GraphUnresolvedKey(bundle.Unresolved[i])
		bundle.Unresolved[i].CrossRID = model.UnresolvedRID(bundle.Unresolved[i].UnresolvedKey)
	}
	if err := bundle.SealIDs(); err != nil {
		t.Fatal(err)
	}

	// Replace only the persisted identity fields for this isolated fixture. The
	// row data remains exactly the same and is inserted without production
	// validation so it represents a pre-upgrade sealed generation.
	bundle.Generation.ContentDigest = legacyGraphContentDigestForTest(t, bundle)
	bundle.Generation.GenerationID = model.DeriveGenerationID(bundle.Generation.SchemaVersion, bundle.Generation.WorkspaceIdentity, bundle.Generation.SourceFingerprint, bundle.Generation.ConfigFingerprint, bundle.Generation.BackendManifestDigest, bundle.Generation.ContentDigest)
	bundle.Generation.Status = model.GraphGenerationActive
	for i := range bundle.Nodes {
		bundle.Nodes[i].GenerationID = bundle.Generation.GenerationID
	}
	for i := range bundle.Unresolved {
		bundle.Unresolved[i].GenerationID = bundle.Generation.GenerationID
	}
	return bundle
}

func insertLegacyActiveGraphForTest(t *testing.T, db *sql.DB, bundle model.GraphBundle) {
	t.Helper()
	g := bundle.Generation
	if _, err := db.Exec(`INSERT INTO graph_generations(generation_id,schema_version,workspace_identity,source_fingerprint,config_fingerprint,backend_manifest_digest,content_digest,status,node_count,edge_count,evidence_count,unresolved_count,previous_generation_id,created_at,published_at,error_code) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), g.SchemaVersion, g.WorkspaceIdentity, digestArg(g.SourceFingerprint), digestArg(g.ConfigFingerprint), digestArg(g.BackendManifestDigest), digestArg(g.ContentDigest), g.Status, g.NodeCount, g.EdgeCount, g.EvidenceCount, g.UnresolvedCount, digestOrNil(g.PreviousGenerationID), g.CreatedAt.UTC().Format(time.RFC3339Nano), g.CreatedAt.Add(time.Second).UTC().Format(time.RFC3339Nano), nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range bundle.Nodes {
		if _, err := db.Exec(`INSERT INTO graph_nodes(generation_id,node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), n.NodeID, digestArg(n.NodeKey), n.IdentitySchema, n.Identity.RepositoryIdentity, n.Identity.BackendType, n.Identity.Language, n.Identity.ProjectOrModule, n.Identity.OwnerPath, n.Identity.SymbolKind, n.Identity.SemanticIdentity, n.DisplayName, digestArg(n.SourceDigest), n.ClaimStatus, n.CrossRID, n.SortKey); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range bundle.Unresolved {
		candidates, err := json.Marshal(u.Candidates)
		if err != nil {
			t.Fatal(err)
		}
		var sourceDigest any
		if u.SourceDigest != nil {
			sourceDigest = digestArg(*u.SourceDigest)
		}
		if _, err := db.Exec(`INSERT INTO graph_unresolved(generation_id,unresolved_id,unresolved_key,owner_path,subject_kind,selector_digest,reason_code,candidates_json,backend,source_digest,cross_rid,recovery_hint_code,source_document,source_block,target_kind,target_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), u.UnresolvedID, digestArg(u.UnresolvedKey), u.OwnerPath, u.SubjectKind, digestArg(u.SelectorDigest), u.ReasonCode, string(candidates), u.Backend, sourceDigest, u.CrossRID, u.RecoveryHintCode, u.SourceDocument, u.SourceBlock, u.TargetKind, u.TargetValue); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, graphActiveMeta, digestArg(g.GenerationID)); err != nil {
		t.Fatal(err)
	}
}

func graphStatusForTest(t *testing.T, db *sql.DB, id model.GraphDigest) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM graph_generations WHERE generation_id=?`, digestArg(id)).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func graphPointerForTest(t *testing.T, db *sql.DB, key string) []byte {
	t.Helper()
	var pointer []byte
	if err := db.QueryRow(`SELECT value FROM workspace_meta WHERE key=?`, key).Scan(&pointer); err != nil {
		t.Fatal(err)
	}
	return pointer
}

func stageCurrentReplacementForTest(t *testing.T, ctx context.Context, db *sql.DB) model.GraphBundle {
	t.Helper()
	next := testGraphBundle(t)
	next.Generation.CreatedAt = time.Unix(2, 0).UTC()
	if err := next.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &next); err != nil {
		t.Fatal(err)
	}
	return next
}

func TestActivateGraphGenerationAtAcceptsLegacyActiveDigestForReplacement(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	next := stageCurrentReplacementForTest(t, ctx, db)

	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(3, 0).UTC()); err != nil {
		t.Fatalf("legacy replacement: %v", err)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationRetired {
		t.Fatalf("legacy status=%q, want retired", got)
	}
	if got := graphStatusForTest(t, db, next.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("replacement status=%q, want active", got)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(next.Generation.GenerationID[:]) {
		t.Fatalf("active pointer=%x, want %x", got, next.Generation.GenerationID)
	}
	if got := graphPointerForTest(t, db, graphPreviousMeta); string(got) != string(legacy.Generation.GenerationID[:]) {
		t.Fatalf("previous pointer=%x, want %x", got, legacy.Generation.GenerationID)
	}
	var historicalRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_unresolved WHERE generation_id=?`, digestArg(legacy.Generation.GenerationID)).Scan(&historicalRows); err != nil {
		t.Fatal(err)
	}
	if historicalRows != 1 {
		t.Fatalf("retired unresolved rows=%d, want 1", historicalRows)
	}
	if _, err := ValidateGraphGeneration(ctx, db, next.Generation.GenerationID); err != nil {
		t.Fatalf("current replacement validation: %v", err)
	}

	// A retry with the now-current pointer is idempotent and does not create a
	// second active row or alter the immutable generation.
	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &next.Generation.GenerationID, time.Unix(4, 0).UTC()); err != nil {
		t.Fatalf("idempotent replacement retry: %v", err)
	}
	var activeRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE status=?`, model.GraphGenerationActive).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if activeRows != 1 {
		t.Fatalf("active rows=%d, want 1", activeRows)
	}
}

func TestLegacyGraphReplacementRejectsWrongCASAndKeepsRows(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	next := stageCurrentReplacementForTest(t, ctx, db)
	wrong := model.GraphDigest{0xff}

	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &wrong, time.Unix(3, 0).UTC()); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("wrong expected prior error=%v, want pointer conflict", err)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("legacy status after CAS conflict=%q, want active", got)
	}
	if got := graphStatusForTest(t, db, next.Generation.GenerationID); got != model.GraphGenerationStaged {
		t.Fatalf("replacement status after CAS conflict=%q, want staged", got)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(legacy.Generation.GenerationID[:]) {
		t.Fatalf("active pointer after CAS conflict=%x, want %x", got, legacy.Generation.GenerationID)
	}

	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(4, 0).UTC()); err != nil {
		t.Fatalf("correct expected prior after conflict: %v", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(5, 0).UTC()); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("stale repeated expected prior error=%v, want pointer conflict", err)
	}
	if got := graphStatusForTest(t, db, next.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("replacement status after stale retry=%q, want active", got)
	}
}

func TestLegacyGraphStructuralCorruptionBlocksReplacement(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	next := stageCurrentReplacementForTest(t, ctx, db)
	if _, err := db.ExecContext(ctx, `UPDATE graph_unresolved SET source_document=? WHERE generation_id=? AND unresolved_id=0`, "/absolute/forbidden.md", digestArg(legacy.Generation.GenerationID)); err != nil {
		t.Fatal(err)
	}

	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(3, 0).UTC()); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("corrupt legacy replacement error=%v, want pointer conflict", err)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("corrupt legacy status=%q, want active", got)
	}
	if got := graphStatusForTest(t, db, next.Generation.GenerationID); got != model.GraphGenerationStaged {
		t.Fatalf("replacement status after corruption=%q, want staged", got)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(legacy.Generation.GenerationID[:]) {
		t.Fatalf("active pointer after corruption=%x, want %x", got, legacy.Generation.GenerationID)
	}
}

func TestLegacyGraphUnknownDigestMismatchBlocksReplacement(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	next := stageCurrentReplacementForTest(t, ctx, db)
	if _, err := db.ExecContext(ctx, `UPDATE graph_generations SET content_digest=? WHERE generation_id=?`, make([]byte, 32), digestArg(legacy.Generation.GenerationID)); err != nil {
		t.Fatal(err)
	}

	if err := ActivateGraphGenerationAt(ctx, db, next.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(3, 0).UTC()); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("unknown legacy digest error=%v, want pointer conflict", err)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("legacy status after digest mismatch=%q, want active", got)
	}
	if got := graphStatusForTest(t, db, next.Generation.GenerationID); got != model.GraphGenerationStaged {
		t.Fatalf("replacement status after digest mismatch=%q, want staged", got)
	}
}

func TestFencedGraphPublicationUsesLegacyReplacementPolicy(t *testing.T) {
	ctx := context.Background()
	db, root := seedTestDB(t)
	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	next := stageCurrentReplacementForTest(t, ctx, db)
	job, err := CreateIndexJob(ctx, db, "legacy-graph-publication", root, IndexModeFull, false)
	if err != nil {
		t.Fatal(err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatal(err)
	}
	graphID := next.Generation.GenerationID
	if err := PublishIncrementalGenerationForJob(ctx, db, job.JobID, job.GenerationID, 0, 0, 0, fence, &IndexJobGraphPublication{
		GenerationID:  &graphID,
		ExpectedPrior: &legacy.Generation.GenerationID,
		PublishedAt:   time.Unix(3, 0).UTC(),
		GraphCurrent:  true,
	}); err != nil {
		t.Fatalf("fenced legacy replacement: %v", err)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationRetired {
		t.Fatalf("legacy status=%q, want retired", got)
	}
	if got := graphStatusForTest(t, db, next.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("replacement status=%q, want active", got)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(next.Generation.GenerationID[:]) {
		t.Fatalf("active pointer=%x, want %x", got, next.Generation.GenerationID)
	}
	final, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok || final.Status != IndexJobSucceeded {
		t.Fatalf("job status=%q ok=%v err=%v, want succeeded", final.Status, ok, err)
	}
	// The transaction-local idempotent branch uses the same active-prior policy
	// and compares the expected prior with the current pointer, not history.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := activateGraphGenerationTx(ctx, tx, next.Generation.GenerationID, &next.Generation.GenerationID, time.Unix(4, 0).UTC()); err != nil {
		_ = tx.Rollback()
		t.Fatalf("idempotent fenced retry: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func graphRowsForGenerationTest(t *testing.T, db *sql.DB, id model.GraphDigest) [4]int {
	t.Helper()
	var counts [4]int
	for i, table := range []string{"graph_nodes", "graph_edges", "graph_evidence", "graph_unresolved"} {
		if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE generation_id=?", digestArg(id)).Scan(&counts[i]); err != nil {
			t.Fatalf("count %s rows: %v", table, err)
		}
	}
	return counts
}

func graphBundleWithEdgeEvidenceForTest(t *testing.T) model.GraphBundle {
	t.Helper()
	bundle := testGraphBundle(t)
	node := bundle.Nodes[0]
	edgeKey := model.EdgeKey(node.NodeKey, node.NodeKey, "calls", "workspace")
	bundle.Edges = []model.GraphEdgeRecord{{
		GenerationID:  bundle.Generation.GenerationID,
		EdgeID:        0,
		EdgeKey:       edgeKey,
		FromNodeID:    0,
		ToNodeID:      0,
		Relation:      "calls",
		ClaimScope:    "workspace",
		ClaimStatus:   model.GraphRecordExact,
		OwnerPath:     node.Identity.OwnerPath,
		SourceBackend: "go",
		CrossRID:      model.EdgeRID(edgeKey),
	}}
	edgeID := 0
	startLine, startColumn, endLine, endColumn := 1, 1, 1, 8
	evidenceDigest := model.EvidenceDigest(node.SourceDigest, edgeKey, node.Identity.OwnerPath, "calls", "go", "test/v1", startLine, startColumn, endLine, endColumn)
	evidenceKey := model.EvidenceKey(edgeKey, evidenceDigest, 0)
	bundle.Evidence = []model.GraphEvidence{{
		GenerationID:        bundle.Generation.GenerationID,
		EvidenceID:          0,
		EvidenceKey:         evidenceKey,
		EvidenceDigest:      evidenceDigest,
		SubjectKind:         "edge",
		EdgeID:              &edgeID,
		SourceURI:           node.Identity.OwnerPath,
		StartLine:           &startLine,
		StartColumn:         &startColumn,
		EndLine:             &endLine,
		EndColumn:           &endColumn,
		Backend:             "go",
		ExtractorVersion:    "test/v1",
		SourceDigest:        node.SourceDigest,
		ClaimKind:           "calls",
		ObservedClaimDigest: edgeKey,
		ClaimStatus:         model.GraphRecordExact,
		CrossRID:            model.EvidenceRID(evidenceKey),
	}}
	bundle.Generation.EdgeCount = len(bundle.Edges)
	bundle.Generation.EvidenceCount = len(bundle.Evidence)
	if err := bundle.SealIDs(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestLegacyChangedGraphWithEdgeEvidenceRestagesIdempotently(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()

	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	candidate := graphBundleWithEdgeEvidenceForTest(t)
	candidate.Generation.CreatedAt = time.Unix(2, 0).UTC()
	if err := StageGraphGeneration(ctx, db, &candidate); err != nil {
		t.Fatalf("stage changed graph with edge evidence: %v", err)
	}
	if got := graphRowsForGenerationTest(t, db, candidate.Generation.GenerationID); got != [4]int{1, 1, 1, 0} {
		t.Fatalf("candidate graph rows=%v, want nodes/edges/evidence/unresolved 1/1/1/0", got)
	}

	wrongPrior := model.GraphDigest{0xff}
	if err := ActivateGraphGenerationAt(ctx, db, candidate.Generation.GenerationID, &wrongPrior, time.Unix(3, 0).UTC()); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("wrong expected prior error=%v, want pointer conflict", err)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationActive {
		t.Fatalf("legacy status after CAS conflict=%q, want active", got)
	}
	if got := graphStatusForTest(t, db, candidate.Generation.GenerationID); got != model.GraphGenerationStaged {
		t.Fatalf("candidate status after CAS conflict=%q, want staged", got)
	}

	if err := ActivateGraphGenerationAt(ctx, db, candidate.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(4, 0).UTC()); err != nil {
		t.Fatalf("activate changed graph: %v", err)
	}
	candidateRows := graphRowsForGenerationTest(t, db, candidate.Generation.GenerationID)
	legacyRows := graphRowsForGenerationTest(t, db, legacy.Generation.GenerationID)
	if candidateRows != [4]int{1, 1, 1, 0} || legacyRows != [4]int{1, 0, 0, 1} {
		t.Fatalf("graph rows after activation candidate=%v legacy=%v", candidateRows, legacyRows)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(candidate.Generation.GenerationID[:]) {
		t.Fatalf("active pointer=%x, want %x", got, candidate.Generation.GenerationID)
	}
	if got := graphPointerForTest(t, db, graphPreviousMeta); string(got) != string(legacy.Generation.GenerationID[:]) {
		t.Fatalf("previous pointer=%x, want %x", got, legacy.Generation.GenerationID)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationRetired {
		t.Fatalf("legacy status=%q, want retired", got)
	}

	retry := candidate
	retry.Generation.CreatedAt = time.Unix(5, 0).UTC()
	if err := StageGraphGeneration(ctx, db, &retry); err != nil {
		t.Fatalf("unchanged repeat staging: %v", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, retry.Generation.GenerationID, &candidate.Generation.GenerationID, time.Unix(6, 0).UTC()); err != nil {
		t.Fatalf("unchanged repeat activation: %v", err)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(candidate.Generation.GenerationID[:]) {
		t.Fatalf("active pointer changed after repeat: got=%x want=%x", got, candidate.Generation.GenerationID)
	}
	if got := graphPointerForTest(t, db, graphPreviousMeta); string(got) != string(legacy.Generation.GenerationID[:]) {
		t.Fatalf("previous pointer changed after repeat: got=%x want=%x", got, legacy.Generation.GenerationID)
	}

	if got := graphRowsForGenerationTest(t, db, candidate.Generation.GenerationID); got != candidateRows {
		t.Fatalf("candidate graph rows changed after repeat: got=%v want=%v", got, candidateRows)
	}
	if got := graphRowsForGenerationTest(t, db, legacy.Generation.GenerationID); got != legacyRows {
		t.Fatalf("legacy graph rows changed after repeat: got=%v want=%v", got, legacyRows)
	}
	var generations, activeRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if generations != 2 || activeRows != 1 {
		t.Fatalf("graph generations=%d active=%d, want 2/1", generations, activeRows)
	}
	if _, err := ValidateGraphGeneration(ctx, db, candidate.Generation.GenerationID); err != nil {
		t.Fatalf("candidate validation after repeat: %v", err)
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID, parent, foreignKeyID any
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("foreign key violation table=%q row=%v parent=%v fk=%v", table, rowID, parent, foreignKeyID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyForegroundReplacementAllowsSameGenerationRestage(t *testing.T) {
	ctx := context.Background()
	db, _ := seedTestDB(t)
	defer db.Close()

	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	candidate := testGraphBundle(t)
	candidate.Generation.CreatedAt = time.Unix(2, 0).UTC()
	if err := StageGraphGeneration(ctx, db, &candidate); err != nil {
		t.Fatalf("stage replacement: %v", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, candidate.Generation.GenerationID, &legacy.Generation.GenerationID, time.Unix(3, 0).UTC()); err != nil {
		t.Fatalf("activate replacement: %v", err)
	}

	candidateRows := graphRowsForGenerationTest(t, db, candidate.Generation.GenerationID)
	legacyRows := graphRowsForGenerationTest(t, db, legacy.Generation.GenerationID)
	activeBefore := graphPointerForTest(t, db, graphActiveMeta)
	previousBefore := graphPointerForTest(t, db, graphPreviousMeta)
	retry := candidate
	retry.Generation.CreatedAt = time.Unix(4, 0).UTC()
	if err := StageGraphGeneration(ctx, db, &retry); err != nil {
		t.Fatalf("restage active equivalent generation: %v", err)
	}
	wrongPrior := model.GraphDigest{0xff}
	if err := ActivateGraphGenerationAt(ctx, db, retry.Generation.GenerationID, &wrongPrior, time.Unix(5, 0).UTC()); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("wrong expected prior error=%v, want pointer conflict", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, retry.Generation.GenerationID, &retry.Generation.GenerationID, time.Unix(6, 0).UTC()); err != nil {
		t.Fatalf("activate restaged active equivalent generation: %v", err)
	}
	if got := graphRowsForGenerationTest(t, db, candidate.Generation.GenerationID); got != candidateRows {
		t.Fatalf("candidate graph rows changed after retry: got=%v want=%v", got, candidateRows)
	}
	if got := graphRowsForGenerationTest(t, db, legacy.Generation.GenerationID); got != legacyRows {
		t.Fatalf("legacy graph rows changed after retry: got=%v want=%v", got, legacyRows)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(activeBefore) {
		t.Fatalf("active pointer changed after retry: got=%x want=%x", got, activeBefore)
	}
	if got := graphPointerForTest(t, db, graphPreviousMeta); string(got) != string(previousBefore) {
		t.Fatalf("previous pointer changed after retry: got=%x want=%x", got, previousBefore)
	}
	var generations, activeRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if generations != 2 || activeRows != 1 {
		t.Fatalf("graph generations=%d active=%d, want 2/1", generations, activeRows)
	}
	if got := graphStatusForTest(t, db, legacy.Generation.GenerationID); got != model.GraphGenerationRetired {
		t.Fatalf("legacy status=%q, want retired", got)
	}
	if _, err := ValidateGraphGeneration(ctx, db, candidate.Generation.GenerationID); err != nil {
		t.Fatalf("candidate validation after retry: %v", err)
	}

	mismatch := retry
	mismatch.Generation.ErrorCode = "unexpected-metadata"
	if err := StageGraphGeneration(ctx, db, &mismatch); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
		t.Fatalf("metadata mismatch error=%v, want graph corruption", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE graph_nodes SET display_name=? WHERE generation_id=? AND node_id=0", "corrupt", digestArg(candidate.Generation.GenerationID)); err != nil {
		t.Fatal(err)
	}
	if err := StageGraphGeneration(ctx, db, &retry); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
		t.Fatalf("corrupt active row error=%v, want graph corruption", err)
	}
}

func TestLegacyFencedReplacementAllowsSameGenerationRestage(t *testing.T) {
	ctx := context.Background()
	db, root := seedTestDB(t)
	defer db.Close()

	legacy := legacyGraphBundleForTest(t)
	insertLegacyActiveGraphForTest(t, db, legacy)
	candidate := graphBundleWithEdgeEvidenceForTest(t)
	candidate.Generation.CreatedAt = time.Unix(2, 0).UTC()
	candidateID := candidate.Generation.GenerationID
	first, err := CreateIndexJob(ctx, db, "legacy-fenced-first", root, IndexModeFull, true)
	if err != nil {
		t.Fatalf("CreateIndexJob first: %v", err)
	}
	firstFence := IndexJobFence{OwnerToken: first.OwnerToken, FencingToken: first.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, first.JobID, 0, "indexing", firstFence); err != nil {
		t.Fatalf("MarkIndexJobRunning first: %v", err)
	}
	if err := ReplaceWorkspaceIndexForJob(ctx, db, first.JobID, first.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Unix(3, 0).UTC()}, firstFence, &IndexJobGraphPublication{
		GenerationID:  &candidateID,
		ExpectedPrior: &legacy.Generation.GenerationID,
		PublishedAt:   time.Unix(4, 0).UTC(),
		GraphCurrent:  true,
		GraphBundle:   &candidate,
	}); err != nil {
		t.Fatalf("first fenced replacement: %v", err)
	}
	candidateRows := graphRowsForGenerationTest(t, db, candidateID)
	legacyRows := graphRowsForGenerationTest(t, db, legacy.Generation.GenerationID)
	activeBefore := graphPointerForTest(t, db, graphActiveMeta)
	previousBefore := graphPointerForTest(t, db, graphPreviousMeta)

	retry := candidate
	retry.Generation.CreatedAt = time.Unix(5, 0).UTC()
	second, err := CreateIndexJob(ctx, db, "legacy-fenced-second", root, IndexModeFull, true)
	if err != nil {
		t.Fatalf("CreateIndexJob second: %v", err)
	}
	secondFence := IndexJobFence{OwnerToken: second.OwnerToken, FencingToken: second.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, second.JobID, 0, "indexing", secondFence); err != nil {
		t.Fatalf("MarkIndexJobRunning second: %v", err)
	}
	if err := ReplaceWorkspaceIndexForJob(ctx, db, second.JobID, second.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Unix(6, 0).UTC()}, secondFence, &IndexJobGraphPublication{
		GenerationID:  &candidateID,
		ExpectedPrior: &candidateID,
		PublishedAt:   time.Unix(7, 0).UTC(),
		GraphCurrent:  true,
		GraphBundle:   &retry,
	}); err != nil {
		t.Fatalf("same-generation fenced retry: %v", err)
	}
	if got := graphRowsForGenerationTest(t, db, candidateID); got != candidateRows {
		t.Fatalf("candidate graph rows changed after fenced retry: got=%v want=%v", got, candidateRows)
	}
	if got := graphRowsForGenerationTest(t, db, legacy.Generation.GenerationID); got != legacyRows {
		t.Fatalf("legacy graph rows changed after fenced retry: got=%v want=%v", got, legacyRows)
	}
	if got := graphPointerForTest(t, db, graphActiveMeta); string(got) != string(activeBefore) {
		t.Fatalf("active pointer changed after fenced retry: got=%x want=%x", got, activeBefore)
	}
	if got := graphPointerForTest(t, db, graphPreviousMeta); string(got) != string(previousBefore) {
		t.Fatalf("previous pointer changed after fenced retry: got=%x want=%x", got, previousBefore)
	}
	var generations, activeRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if generations != 2 || activeRows != 1 {
		t.Fatalf("fenced graph generations=%d active=%d, want 2/1", generations, activeRows)
	}
	firstFinal, ok, err := GetIndexJob(ctx, db, first.JobID)
	if err != nil || !ok || firstFinal.Status != IndexJobSucceeded {
		t.Fatalf("first job status=%q ok=%v err=%v, want succeeded", firstFinal.Status, ok, err)
	}
	secondFinal, ok, err := GetIndexJob(ctx, db, second.JobID)
	if err != nil || !ok || secondFinal.Status != IndexJobSucceeded {
		t.Fatalf("second job status=%q ok=%v err=%v, want succeeded", secondFinal.Status, ok, err)
	}
	if _, err := ValidateGraphGeneration(ctx, db, candidateID); err != nil {
		t.Fatalf("fenced candidate validation after retry: %v", err)
	}
}

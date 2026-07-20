package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func stagingDigest(s string) model.GraphDigest { return model.GraphDigest(sha256.Sum256([]byte(s))) }

func stagingDoc(path, id string) model.DocRecord {
	return model.DocRecord{Path: path, DocID: id, Title: id, Layer: "technical", Family: "technical", ContentHash: stagingDigest(path + id).String()}
}

func stagingBatch(backend, module string, withCall bool) model.GraphObservationBatch {
	resolution, claim, language, identity := "roslyn", model.GraphRecordExact, "csharp", "Acme.Widget"
	if backend == "go" {
		resolution, claim, language, identity = "go/ast", model.GraphRecordExtracted, "go", "acme.Widget"
	}
	key := model.NodeKeyFields{RepositoryIdentity: "github.com/acme/repo", BackendType: backend, Language: language, ProjectOrModule: module, OwnerPath: module + "/main.go", SymbolKind: "type", SemanticIdentity: identity}
	if backend == "roslyn" {
		key.OwnerPath = module + "/main.cs"
	}
	node := model.GraphObservationNode{Ref: "N1", Key: key, DisplayName: "Widget", SourceDigest: stagingDigest(backend + module + "node"), ClaimStatus: claim, Resolution: resolution}
	batch := model.GraphObservationBatch{SchemaVersion: 1, WorkspaceIdentity: "github.com/acme/repo", RepositoryIdentity: "https://github.com/acme/repo.git", Backend: backend, BackendVersion: "1", ExtractorVersion: "extractor-1", ProjectOrModule: module, SourceFingerprint: stagingDigest(backend + module + "source"), ConfigFingerprint: stagingDigest(backend + module + "config"), Completeness: model.GraphCompletenessComplete, Capabilities: []model.GraphObservationCapability{{Backend: backend, Capability: "declarations", State: model.GraphObservationStatusStable}}, Coverage: []model.GraphObservationCoverage{{Backend: backend, Capability: "declarations", Eligible: 1, Observed: 1}}, Nodes: []model.GraphObservationNode{node}, Evidence: []model.GraphObservationEvidence{{Ref: "EV1", NodeRef: "N1", SourceURI: key.OwnerPath, Range: &model.GraphObservationRange{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2}, Backend: backend, ExtractorVersion: "extractor-1", SourceDigest: node.SourceDigest, ObservedDigest: stagingDigest(backend + module + "observed"), ClaimKind: "declaration", Status: claim}}}
	if withCall {
		edge := model.GraphObservationEdge{Ref: "E1", FromRef: "N1", ToRef: "N1", Relation: "calls", Scope: "symbol", Status: claim, OwnerPath: key.OwnerPath, Backend: backend, Resolution: resolution, SourceDigest: stagingDigest(backend + module + "edge")}
		if backend == "go" {
			edge.Status, edge.Resolution = model.GraphRecordExact, "go/types"
		}
		batch.Edges = append(batch.Edges, edge)
		batch.Evidence = append(batch.Evidence, model.GraphObservationEvidence{Ref: "EV2", EdgeRef: "E1", SourceURI: key.OwnerPath, Range: &model.GraphObservationRange{StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 2}, Backend: backend, ExtractorVersion: "extractor-1", SourceDigest: edge.SourceDigest, ObservedDigest: stagingDigest(backend + module + "edge-observed"), ClaimKind: "calls", Status: edge.Status})
		batch.Capabilities = append(batch.Capabilities, model.GraphObservationCapability{Backend: backend, Capability: "calls", State: model.GraphObservationStatusStable})
		batch.Coverage = append(batch.Coverage, model.GraphObservationCoverage{Backend: backend, Capability: "calls", Eligible: 1, Observed: 1})
	}
	return batch
}

func TestAssembleCanonicalDocumentationSupplement(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	docs := []model.DocRecord{stagingDoc(".docs/wiki/target.md", "DOC-TARGET"), stagingDoc(".docs/wiki/guide.md", "DOC-GUIDE"), stagingDoc(".docs/raw/draft.md", "DOC-RAW"), stagingDoc(".docs/auditoria/report.md", "DOC-AUDIT"), stagingDoc(".docs/wiki/archive/old.md", "DOC-OLD")}
	edges := []model.DocEdge{{FromPath: ".docs/wiki/guide.md", ToDocID: "DOC-TARGET", Kind: "doc_id", Label: "DOC-TARGET"}}
	mentions := []model.DocMention{{DocPath: ".docs/wiki/guide.md", MentionType: "file_path", MentionValue: "src/go/main.go"}, {DocPath: ".docs/wiki/guide.md", MentionType: "symbol", MentionValue: "Widget"}}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: docs, DocEdges: edges, DocMentions: mentions, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Nodes) != 3 || len(bundle.Edges) != 2 || len(bundle.Evidence) != 5 || len(bundle.Unresolved) != 0 {
		t.Fatalf("docs graph counts: nodes=%d edges=%d evidence=%d unresolved=%d", len(bundle.Nodes), len(bundle.Edges), len(bundle.Evidence), len(bundle.Unresolved))
	}
	for _, node := range bundle.Nodes {
		if node.Identity.OwnerPath == ".docs/raw/draft.md" || node.Identity.OwnerPath == ".docs/auditoria/report.md" || node.Identity.OwnerPath == ".docs/wiki/archive/old.md" {
			t.Fatalf("excluded document became node: %s", node.Identity.OwnerPath)
		}
	}
	for _, edge := range bundle.Edges {
		if edge.Relation == "doc_mentions" && edge.SourceBackend != "docgraph" {
			t.Fatalf("doc edge backend=%q", edge.SourceBackend)
		}
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		permuted, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: []model.DocRecord{docs[(i+1)%2], docs[i%2]}, DocEdges: edges, DocMentions: mentions, CreatedAt: time.Unix(1, 0)})
		if err != nil || !reflect.DeepEqual(bundle, permuted) {
			t.Fatalf("document permutation %d changed graph: %v", i, err)
		}
		moved := docs[1]
		moved.Path = ".docs/wiki/moved.md"
		relocated, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: []model.DocRecord{docs[0], moved}, CreatedAt: time.Unix(1, 0)})
		if err != nil {
			t.Fatal(err)
		}
		if relocated.Generation.GenerationID == bundle.Generation.GenerationID || relocated.Generation.SourceFingerprint == bundle.Generation.SourceFingerprint || relocated.Generation.ConfigFingerprint == bundle.Generation.ConfigFingerprint || relocated.Generation.BackendManifestDigest == bundle.Generation.BackendManifestDigest {
			t.Fatal("document relocation did not change all generation identity inputs")
		}
	}
	codeOnly, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	excludedOnly, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: docs[2:], CreatedAt: time.Unix(1, 0)})
	if err != nil || !reflect.DeepEqual(codeOnly, excludedOnly) {
		t.Fatalf("excluded docs changed code-only graph: %v", err)
	}
}

func TestAssembleDocumentationMentionsFailsClosed(t *testing.T) {
	first := stagingBatch("go", "src/a", false)
	second := stagingBatch("go", "src/b", false)
	second.Nodes[0].Key.OwnerPath = first.Nodes[0].Key.OwnerPath
	second.Nodes[0].Key.SemanticIdentity = "acme.Other"
	second.Evidence[0].SourceURI = first.Nodes[0].Key.OwnerPath
	for _, batch := range []*model.GraphObservationBatch{&first, &second} {
		if err := model.SealGraphObservationBatch(batch); err != nil {
			t.Fatal(err)
		}
	}
	doc := stagingDoc(".docs/wiki/guide.md", "DOC-GUIDE")
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{first, second}, Docs: []model.DocRecord{doc}, DocMentions: []model.DocMention{{DocPath: doc.Path, MentionType: "file_path", MentionValue: first.Nodes[0].Key.OwnerPath}, {DocPath: doc.Path, MentionType: "file_path", MentionValue: "src/missing.go"}, {DocPath: doc.Path, MentionType: "symbol", MentionValue: "Widget"}}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Edges) != 0 || len(bundle.Unresolved) != 2 {
		t.Fatalf("fail-closed mentions: edges=%d unresolved=%d", len(bundle.Edges), len(bundle.Unresolved))
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDocOnlyAssemblyRequiresExplicitRepositoryIdentity(t *testing.T) {
	doc := stagingDoc(".docs/wiki/guide.md", "")
	if _, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Docs: []model.DocRecord{doc}, CreatedAt: time.Unix(1, 0)}); err == nil {
		t.Fatal("doc-only assembly accepted without repository identity")
	}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Docs: []model.DocRecord{doc}, RepositoryIdentity: "https://example.com/docs", CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Nodes) != 1 || len(bundle.Edges) != 0 || len(bundle.Evidence) != 1 {
		t.Fatalf("doc-only graph counts: nodes=%d edges=%d evidence=%d", len(bundle.Nodes), len(bundle.Edges), len(bundle.Evidence))
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleGraphObservationBatchesIsPermutationInvariantAndScoped(t *testing.T) {
	roslyn, goBatch := stagingBatch("roslyn", "src/roslyn", true), stagingBatch("go", "src/go", true)
	if err := model.SealGraphObservationBatch(&roslyn); err != nil {
		t.Fatal(err)
	}
	if err := model.SealGraphObservationBatch(&goBatch); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 7, 20, 1, 2, 3, 456000000, time.FixedZone("local", 3600))
	beforeRoslyn, beforeGo := roslyn, goBatch
	a, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{roslyn, goBatch}, CreatedAt: created})
	if err != nil {
		t.Fatal(err)
	}
	b, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{goBatch, roslyn}, CreatedAt: created})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("batch permutation changed assembled graph")
	}
	if a.Generation.CreatedAt.Location() != time.UTC || !a.Generation.CreatedAt.Equal(created.UTC()) {
		t.Fatal("created_at was not canonicalized")
	}
	if len(a.Nodes) != 2 || len(a.Edges) != 2 || len(a.Evidence) != 4 {
		t.Fatalf("counts: nodes=%d edges=%d evidence=%d", len(a.Nodes), len(a.Edges), len(a.Evidence))
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roslyn, beforeRoslyn) || !reflect.DeepEqual(goBatch, beforeGo) {
		t.Fatal("assembly mutated input batches")
	}
}

func TestAssembleGraphObservationBatchesDeduplicatesAndRejectsConflicts(t *testing.T) {
	batch := stagingBatch("roslyn", "src/app", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	one, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch, batch}, CreatedAt: time.Unix(1, 0)})
	if err != nil || len(one.Nodes) != 1 {
		t.Fatalf("duplicate batch: %v counts=%d", err, len(one.Nodes))
	}
	conflict := batch
	conflict.SourceFingerprint = stagingDigest("different")
	if err := model.SealGraphObservationBatch(&conflict); err != nil {
		t.Fatal(err)
	}
	if _, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch, conflict}, CreatedAt: time.Unix(1, 0)}); !errors.Is(err, errGraphAssemblyConflict) {
		t.Fatalf("conflict error=%v", err)
	}
}

func TestAssembleGraphObservationBatchesRejectsBeforeStoreAndTracksManifestOmissions(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	base, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	withOmission := batch
	withOmission.Capabilities = append(withOmission.Capabilities, model.GraphObservationCapability{Backend: "go", Capability: "calls", State: model.GraphObservationStatusStable})
	withOmission.Coverage = append(withOmission.Coverage, model.GraphObservationCoverage{Backend: "go", Capability: "calls", Eligible: 1, Omitted: 1})
	withOmission.Omissions = append(withOmission.Omissions, model.GraphObservationOmission{Ref: "O1", OwnerPath: "src/go/main.go", SubjectKind: "file", Backend: "go", Capability: "calls", ReasonCode: "not_observed", RecoveryHintCode: "retry"})
	if err := model.SealGraphObservationBatch(&withOmission); err != nil {
		t.Fatal(err)
	}
	changed, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{withOmission}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if base.Generation.ContentDigest != changed.Generation.ContentDigest {
		t.Fatal("omission changed graph facts")
	}
	if base.Generation.BackendManifestDigest == changed.Generation.BackendManifestDigest || base.Generation.GenerationID == changed.Generation.GenerationID {
		t.Fatal("omission did not change generation identity")
	}
	bad := batch
	bad.Digest = model.GraphDigest{}
	if _, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{bad}, CreatedAt: time.Unix(1, 0)}); err == nil {
		t.Fatal("unsealed batch accepted")
	}
	if _, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: nil, CreatedAt: time.Unix(1, 0)}); err == nil {
		t.Fatal("empty request accepted")
	}
	if _, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Time{}}); err == nil {
		t.Fatal("zero created_at accepted")
	}
}

func TestAssembleGraphObservationBatchesConvertsUnresolved(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	batch.Completeness = model.GraphCompletenessComplete
	batch.Coverage[0].Eligible = 2
	batch.Unresolved = append(batch.Unresolved, model.GraphObservationUnresolved{Ref: "U1", OwnerPath: "src/go/main.go", SubjectKind: "file", Capability: "declarations", SelectorDigest: stagingDigest("selector"), ReasonCode: "missing_symbol", Candidates: []string{"Widget"}, Backend: "go", RecoveryHintCode: "retry"})
	batch.Coverage[0].Unresolved = 1
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Unresolved) != 1 || bundle.Unresolved[0].UnresolvedID != 0 || bundle.Unresolved[0].CrossRID != model.UnresolvedRID(bundle.Unresolved[0].UnresolvedKey) || bundle.Unresolved[0].ReasonCode != "missing_symbol" {
		t.Fatalf("unresolved conversion=%+v", bundle.Unresolved)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStageGraphObservationBatchesRejectsNilBoundariesBeforeAssembly(t *testing.T) {
	req := GraphAssemblyRequest{}
	if _, err := StageGraphObservationBatches(nil, nil, req); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("nil context/db error=%v", err)
	}
	if _, err := StageGraphObservationBatches(context.Background(), nil, req); !errors.Is(err, model.ErrGraphGenerationInvalid) {
		t.Fatalf("nil db error=%v", err)
	}
}

func TestStageGraphObservationBatchesRejectsInvalidRequestBeforeWrite(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := StageGraphObservationBatches(context.Background(), db, GraphAssemblyRequest{}); err == nil {
		t.Fatal("invalid request accepted")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM graph_generations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("generation rows=%d err=%v", count, err)
	}
	if _, active, err := store.ActiveGraphGeneration(context.Background(), db); err != nil || active {
		t.Fatalf("active pointer: active=%v err=%v", active, err)
	}
}

func TestPublishGraphObservationBatchesConflictLeavesInvisibleStageForRetry(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	firstBatch := stagingBatch("go", "src/first", false)
	if err := model.SealGraphObservationBatch(&firstBatch); err != nil {
		t.Fatal(err)
	}
	firstReq := GraphAssemblyRequest{Batches: []model.GraphObservationBatch{firstBatch}, CreatedAt: time.Unix(100, 0).UTC()}
	first, err := PublishGraphObservationBatches(ctx, db, firstReq, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondBatch := stagingBatch("go", "src/second", false)
	if err := model.SealGraphObservationBatch(&secondBatch); err != nil {
		t.Fatal(err)
	}
	secondReq := GraphAssemblyRequest{Batches: []model.GraphObservationBatch{secondBatch}, CreatedAt: time.Unix(101, 0).UTC()}
	wrongPrior := stagingDigest("wrong-prior")
	staged, err := PublishGraphObservationBatches(ctx, db, secondReq, &wrongPrior)
	if !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("publish conflict=%v", err)
	}
	if staged.Status != model.GraphGenerationStaged {
		t.Fatalf("conflicted stage status=%q", staged.Status)
	}
	active, ok, err := store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != first.GenerationID {
		t.Fatalf("active after conflict=%x ok=%v err=%v", active, ok, err)
	}
	if _, err := store.ValidateGraphGeneration(ctx, db, staged.GenerationID); err != nil {
		t.Fatalf("staged generation was not complete: %v", err)
	}
	if err := store.ActivateGraphGenerationAt(ctx, db, staged.GenerationID, &first.GenerationID, secondReq.CreatedAt); err != nil {
		t.Fatalf("retry activation: %v", err)
	}
	if err := store.ActivateGraphGenerationAt(ctx, db, staged.GenerationID, &staged.GenerationID, secondReq.CreatedAt); err != nil {
		t.Fatalf("idempotent activation retry: %v", err)
	}
	active, ok, err = store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != staged.GenerationID {
		t.Fatalf("active after retry=%x ok=%v err=%v", active, ok, err)
	}
}

func TestStageGraphObservationBatchesIsStagedIdempotent(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	batch := stagingBatch("roslyn", "src/app", true)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	req := GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(100, 0)}
	first, err := StageGraphObservationBatches(context.Background(), db, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StageGraphObservationBatches(context.Background(), db, req)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Status != model.GraphGenerationStaged {
		t.Fatalf("idempotency/status: %v %v", first, second)
	}
	if _, active, err := store.ActiveGraphGeneration(context.Background(), db); err != nil || active {
		t.Fatalf("active pointer: active=%v err=%v", active, err)
	}
	if _, err := store.ValidateGraphGeneration(context.Background(), db, first.GenerationID); err != nil {
		t.Fatal(err)
	}
}

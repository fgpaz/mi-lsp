package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
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
func TestAssembleGraphWhitespaceCanonicalizationPreservesIdentityAndTPMiss(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	batch.Capabilities = append(batch.Capabilities, model.GraphObservationCapability{Backend: "go", Capability: "calls", State: model.GraphObservationStatusStable})
	batch.Coverage = append(batch.Coverage, model.GraphObservationCoverage{Backend: "go", Capability: "calls", Eligible: 1, Unresolved: 1})
	batch.Unresolved = append(batch.Unresolved, model.GraphObservationUnresolved{Ref: "U1", OwnerPath: "src/go/main.go", SubjectKind: "file", Capability: "calls", SelectorDigest: stagingDigest("batch-unresolved"), ReasonCode: "missing_symbol", Candidates: []string{"Widget"}, Backend: "go", RecoveryHintCode: "retry"})
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	rawBatch := batch
	rawBatch.WorkspaceIdentity = " " + rawBatch.WorkspaceIdentity + " "
	rawBatch.RepositoryIdentity = "\t" + rawBatch.RepositoryIdentity + "\t"
	rawBatch.BackendVersion = " " + rawBatch.BackendVersion + " "
	rawBatch.ProjectOrModule = " " + rawBatch.ProjectOrModule + " "
	rawBatch.Nodes = append([]model.GraphObservationNode(nil), batch.Nodes...)
	rawBatch.Nodes[0].DisplayName = " " + rawBatch.Nodes[0].DisplayName + " "
	rawBatch.Unresolved = append([]model.GraphObservationUnresolved(nil), batch.Unresolved...)
	rawBatch.Unresolved[0].Candidates = append([]string(nil), batch.Unresolved[0].Candidates...)
	rawBatch.Unresolved[0].Ref = " U1 "
	rawBatch.Unresolved[0].OwnerPath = " src/go/main.go "
	rawBatch.Unresolved[0].SubjectKind = " file "
	rawBatch.Unresolved[0].Capability = " calls "
	rawBatch.Unresolved[0].ReasonCode = " missing_symbol "
	rawBatch.Unresolved[0].Backend = " GO "
	rawBatch.Unresolved[0].RecoveryHintCode = " retry "
	rawBatch.Unresolved[0].Candidates[0] = " Widget "
	digestBatch := cloneGraphObservationBatchForAssembly(rawBatch)
	if err := model.SealGraphObservationBatch(&digestBatch); err != nil {
		t.Fatalf("seal canonical digest copy: %v", err)
	}
	rawBatch.Digest = digestBatch.Digest

	source := stagingDoc(".docs/wiki/source.md", "DOC-SOURCE")
	clean := GraphAssemblyRequest{
		Batches: []model.GraphObservationBatch{batch},
		Docs:    []model.DocRecord{source},
		DocEdges: []model.DocEdge{{
			FromPath: source.Path,
			ToDocID:  "TP-MISSING-001",
			Kind:     "doc_id",
			Label:    "TP-MISSING-001",
		}},
		DocMentions: []model.DocMention{{
			DocPath:      source.Path,
			MentionType:  "test_file",
			MentionValue: "TP-MISSING-001",
			SourceBlock:  "frontmatter",
		}},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	raw := clean
	raw.Batches = []model.GraphObservationBatch{rawBatch}
	raw.DocEdges = []model.DocEdge{{
		FromPath:         " " + source.Path + " ",
		ToDocID:          " \tTP-MISSING-001\t ",
		Kind:             " doc_id ",
		Label:            " \tTP-MISSING-001 ",
		UnresolvedReason: " ",
	}}
	raw.DocMentions = []model.DocMention{{
		DocPath:      " " + source.Path + " ",
		MentionType:  " test_file ",
		MentionValue: " \tTP-MISSING-001\t ",
		SourceBlock:  " frontmatter ",
	}}

	canonical, err := AssembleGraphObservationBatches(clean)
	if err != nil {
		t.Fatalf("canonical assembly: %v", err)
	}
	whitespace, err := AssembleGraphObservationBatches(raw)
	if err != nil {
		t.Fatalf("whitespace assembly: %v", err)
	}
	if !reflect.DeepEqual(canonical, whitespace) {
		t.Fatalf("whitespace-equivalent request changed assembled graph:\ncanonical=%+v\nwhitespace=%+v", canonical, whitespace)
	}
	if canonical.Generation.SourceFingerprint != whitespace.Generation.SourceFingerprint ||
		canonical.Generation.ConfigFingerprint != whitespace.Generation.ConfigFingerprint ||
		canonical.Generation.BackendManifestDigest != whitespace.Generation.BackendManifestDigest ||
		canonical.Generation.ContentDigest != whitespace.Generation.ContentDigest ||
		canonical.Generation.GenerationID != whitespace.Generation.GenerationID {
		t.Fatal("whitespace-equivalent request changed a generation identity digest")
	}
	if len(whitespace.Unresolved) != 3 {
		t.Fatalf("unresolved count=%d, want batch, edge, and mention claims", len(whitespace.Unresolved))
	}
	tpMissCount := 0
	foundBatchClaim := false
	for _, unresolved := range whitespace.Unresolved {
		if unresolved.UnresolvedKey == (model.GraphDigest{}) || unresolved.CrossRID != model.UnresolvedRID(unresolved.UnresolvedKey) {
			t.Fatalf("unresolved identity is not stable: %+v", unresolved)
		}
		if unresolved.ReasonCode == "missing_doc_target" && unresolved.TargetValue == "TP-MISSING-001" {
			tpMissCount++
		}
		if unresolved.ReasonCode == "missing_symbol" {
			if unresolved.OwnerPath != "src/go/main.go" || unresolved.Backend != "go" || unresolved.RecoveryHintCode != "retry" || len(unresolved.Candidates) != 1 || unresolved.Candidates[0] != "Widget" {
				t.Fatalf("batch unresolved claim was not normalized canonically: %+v", unresolved)
			}
			foundBatchClaim = true
		}
	}
	if tpMissCount != 2 {
		t.Fatalf("TP misses=%d, want edge and mention unresolved claims", tpMissCount)
	}
	if !foundBatchClaim {
		t.Fatalf("missing canonical batch unresolved claim in %+v", whitespace.Unresolved)
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

func TestAssembleDocumentationMentionDeduplicatesAndBoundsCandidates(t *testing.T) {
	batches := make([]model.GraphObservationBatch, 0, 70)
	for i := 0; i < 70; i++ {
		batch := stagingBatch("go", fmt.Sprintf("src/module-%02d", i), false)
		if err := model.SealGraphObservationBatch(&batch); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, batch)
	}
	doc := stagingDoc(".docs/wiki/guide.md", "DOC-GUIDE")
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{
		Batches:     batches,
		Docs:        []model.DocRecord{doc},
		DocMentions: []model.DocMention{{DocPath: doc.Path, MentionType: "semantic_identity", MentionValue: "acme.Widget"}},
		CreatedAt:   time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Edges) != 0 || len(bundle.Unresolved) != 1 {
		t.Fatalf("ambiguous semantic mention: edges=%d unresolved=%d", len(bundle.Edges), len(bundle.Unresolved))
	}
	got := bundle.Unresolved[0].Candidates
	if len(got) != graphDocMaxCandidates {
		t.Fatalf("distinct candidate list was not capped: got=%d", len(got))
	}
	if got[0] != "src/module-00/main.go" || got[len(got)-1] != "src/module-63/main.go" {
		t.Fatalf("candidate cap was not deterministic: first=%q last=%q", got[0], got[len(got)-1])
	}
	if total := graphDocCandidateJSONBytes(got); total > graphDocMaxCandidateBytes {
		t.Fatalf("candidate payload exceeded budget: %d", total)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
}

func graphDocCandidateJSONBytes(candidates []string) int {
	serialized, err := json.Marshal(candidates)
	if err != nil {
		panic(err)
	}
	return len(serialized)
}

func graphDocCandidateRawBytes(candidates []string) int {
	total := 0
	for _, candidate := range candidates {
		total += len(candidate)
	}
	return total
}

func TestCanonicalGraphDocCandidates(t *testing.T) {
	t.Run("more than 64 distinct candidates is capped", func(t *testing.T) {
		input := make([]string, 0, graphDocMaxCandidates+6)
		for i := 0; i < graphDocMaxCandidates+6; i++ {
			input = append(input, fmt.Sprintf("candidate-%03d", i))
		}
		got := canonicalGraphDocCandidates(input)
		if len(got) != graphDocMaxCandidates {
			t.Fatalf("candidate count=%d, want %d", len(got), graphDocMaxCandidates)
		}
		for i, want := range input[:graphDocMaxCandidates] {
			if got[i] != want {
				t.Fatalf("candidate[%d]=%q, want %q", i, got[i], want)
			}
		}
	})

	t.Run("serialized JSON payload is capped at 4096 bytes", func(t *testing.T) {
		input := []string{strings.Repeat("a", 2048), strings.Repeat("b", 2049)}
		got := canonicalGraphDocCandidates(input)
		if total := graphDocCandidateJSONBytes(got); total > graphDocMaxCandidateBytes {
			t.Fatalf("candidate JSON payload=%d, want <= %d", total, graphDocMaxCandidateBytes)
		}
		if !reflect.DeepEqual(got, input[:1]) {
			t.Fatalf("JSON payload overflow was not bounded deterministically: got lengths=%v", candidateLengths(got))
		}
	})

	t.Run("later sorted candidates are considered after an oversized candidate", func(t *testing.T) {
		input := []string{strings.Repeat("a", graphDocMaxCandidateBytes), "z-fit", "y-fit"}
		got := canonicalGraphDocCandidates(input)
		want := []string{"y-fit", "z-fit"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("later fitting candidates were discarded: got=%#v want=%#v", got, want)
		}
		if total := graphDocCandidateJSONBytes(got); total > graphDocMaxCandidateBytes {
			t.Fatalf("candidate JSON payload=%d, want <= %d", total, graphDocMaxCandidateBytes)
		}
	})

	t.Run("trim and slash normalization deduplicate candidates", func(t *testing.T) {
		canonical := "src/shared.go"
		native := filepath.FromSlash(canonical)
		got := canonicalGraphDocCandidates([]string{"  " + canonical + "  ", native, "\t" + canonical + "\n"})
		if !reflect.DeepEqual(got, []string{canonical}) {
			t.Fatalf("normalized duplicates=%#v, want %#v", got, []string{canonical})
		}
	})

	t.Run("serialized JSON exact boundary is accepted", func(t *testing.T) {
		input := []string{strings.Repeat("x", graphDocMaxCandidateBytes-4)}
		if total := graphDocCandidateJSONBytes(input); total != graphDocMaxCandidateBytes {
			t.Fatalf("test input JSON payload=%d, want exact %d", total, graphDocMaxCandidateBytes)
		}
		got := canonicalGraphDocCandidates(input)
		if !reflect.DeepEqual(got, input) {
			t.Fatalf("exact-boundary candidates changed: got lengths=%v", candidateLengths(got))
		}
	})

	t.Run("quotes and commas overhead rejects a raw-sum fit", func(t *testing.T) {
		input := []string{strings.Repeat("a", 2046), strings.Repeat("b", 2046)}
		if raw := graphDocCandidateRawBytes(input); raw > graphDocMaxCandidateBytes {
			t.Fatalf("test input raw payload=%d, want <= %d", raw, graphDocMaxCandidateBytes)
		}
		if serialized := graphDocCandidateJSONBytes(input); serialized <= graphDocMaxCandidateBytes {
			t.Fatalf("test input JSON payload=%d, want > %d", serialized, graphDocMaxCandidateBytes)
		}
		got := canonicalGraphDocCandidates(input)
		if !reflect.DeepEqual(got, input[:1]) {
			t.Fatalf("quote/comma overhead was not bounded deterministically: got lengths=%v", candidateLengths(got))
		}
	})

	t.Run("near-boundary controls and escapes use real JSON size", func(t *testing.T) {
		input := []string{
			strings.Repeat("a", 2040),
			strings.Repeat("b", 2042) + "\x00\"",
		}
		if raw := graphDocCandidateRawBytes(input); raw != 4084 {
			t.Fatalf("test input raw payload=%d, want 4084", raw)
		}
		serialized := graphDocCandidateJSONBytes(input)
		if serialized != 4097 || serialized <= graphDocMaxCandidateBytes {
			t.Fatalf("test input JSON payload=%d, want exact overflow of 4097", serialized)
		}
		got := canonicalGraphDocCandidates(input)
		if !reflect.DeepEqual(got, input[:1]) {
			t.Fatalf("control/escape overhead was not bounded deterministically: got lengths=%v", candidateLengths(got))
		}
		if total := graphDocCandidateJSONBytes(got); total > graphDocMaxCandidateBytes {
			t.Fatalf("bounded JSON payload=%d, want <= %d", total, graphDocMaxCandidateBytes)
		}
	})

	t.Run("an oversized individual candidate is excluded", func(t *testing.T) {
		got := canonicalGraphDocCandidates([]string{strings.Repeat("x", graphDocMaxCandidateBytes+1)})
		if len(got) != 0 || graphDocCandidateJSONBytes(got) > graphDocMaxCandidateBytes {
			t.Fatalf("oversized candidate was not safely excluded: count=%d payload=%d", len(got), graphDocCandidateJSONBytes(got))
		}
	})

	t.Run("adversarial characters are preserved without exceeding bounds", func(t *testing.T) {
		control := "\x00../candidate"
		internalNewline := "line\nbreak"
		got := canonicalGraphDocCandidates([]string{"  " + control + "  ", control, internalNewline, "\xff"})
		if got == nil || len(got) > graphDocMaxCandidates || graphDocCandidateJSONBytes(got) > graphDocMaxCandidateBytes {
			t.Fatalf("adversarial input produced an unbounded result: %#v", got)
		}
		if !containsString(got, control) || !containsString(got, internalNewline) || !containsString(got, "\xff") {
			t.Fatalf("canonicalization invented sanitization: %#v", got)
		}
	})
}

func TestStageGraphObservationBatchesPersistsSerializedCandidateBoundary(t *testing.T) {
	batches := make([]model.GraphObservationBatch, 0, graphDocMaxCandidates)
	for i := 0; i < graphDocMaxCandidates; i++ {
		padding := 46
		if i == graphDocMaxCandidates-1 {
			padding--
		}
		batch := stagingBatch("go", fmt.Sprintf("src/%02d-%s", i, strings.Repeat("x", padding)), false)
		if err := model.SealGraphObservationBatch(&batch); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, batch)
	}
	doc := stagingDoc(".docs/wiki/guide.md", "DOC-GUIDE")
	req := GraphAssemblyRequest{
		Batches:     batches,
		Docs:        []model.DocRecord{doc},
		DocMentions: []model.DocMention{{DocPath: doc.Path, MentionType: "semantic_identity", MentionValue: "acme.Widget"}},
		CreatedAt:   time.Unix(1, 0),
	}
	bundle, err := AssembleGraphObservationBatches(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Unresolved) != 1 {
		t.Fatalf("assembled unresolved claims=%d, want 1", len(bundle.Unresolved))
	}
	candidates := bundle.Unresolved[0].Candidates
	if len(candidates) != graphDocMaxCandidates || graphDocCandidateJSONBytes(candidates) != graphDocMaxCandidateBytes {
		t.Fatalf("assembled candidate boundary: count=%d JSON bytes=%d", len(candidates), graphDocCandidateJSONBytes(candidates))
	}

	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generation, err := StageGraphObservationBatches(ctx, db, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateGraphGeneration(ctx, db, generation.GenerationID); err != nil {
		t.Fatalf("staged generation validation failed: %v", err)
	}
	var persistedJSON string
	if err := db.QueryRowContext(ctx, "SELECT candidates_json FROM graph_unresolved WHERE generation_id = ? AND unresolved_id = 0", generation.GenerationID[:]).Scan(&persistedJSON); err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if persistedJSON != string(serialized) {
		t.Fatalf("persisted candidates differ: got bytes=%d want bytes=%d", len(persistedJSON), len(serialized))
	}
}

func TestStageGraphGenerationRollsBackEscapedCandidateOverflow(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	batch := stagingBatch("go", "src/go", false)
	batch.Unresolved = append(batch.Unresolved, model.GraphObservationUnresolved{
		Ref:              "U1",
		OwnerPath:        "src/go/main.go",
		SubjectKind:      "file",
		Capability:       "declarations",
		SelectorDigest:   stagingDigest("selector"),
		ReasonCode:       "missing_symbol",
		Candidates:       []string{"placeholder"},
		Backend:          "go",
		RecoveryHintCode: "retry",
	})
	batch.Coverage[0].Eligible = 2
	batch.Coverage[0].Unresolved = 1
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}

	candidates := make([]string, 16)
	for i := range candidates {
		candidates[i] = fmt.Sprintf("%02d%s", i, strings.Repeat("\\", 254))
	}
	bundle.Unresolved[0].Candidates = candidates
	bundle.Unresolved[0].UnresolvedKey = model.GraphUnresolvedKey(bundle.Unresolved[0])
	bundle.Unresolved[0].CrossRID = model.UnresolvedRID(bundle.Unresolved[0].UnresolvedKey)
	if err := bundle.SealIDs(); err != nil {
		t.Fatal(err)
	}
	if raw := graphDocCandidateRawBytes(candidates); raw != graphDocMaxCandidateBytes {
		t.Fatalf("manual bundle raw candidates=%d, want %d", raw, graphDocMaxCandidateBytes)
	}
	serialized := graphDocCandidateJSONBytes(candidates)
	if serialized <= graphDocMaxCandidateBytes {
		t.Fatalf("manual bundle escaped JSON=%d, want > %d", serialized, graphDocMaxCandidateBytes)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("manual bundle should pass model validation before store JSON validation: %v", err)
	}

	if err := store.StageGraphGeneration(ctx, db, &bundle); !errors.Is(err, model.ErrGraphUnresolved) {
		t.Fatalf("escaped candidate overflow error=%v, want %v", err, model.ErrGraphUnresolved)
	}
	for _, table := range []string{"graph_generations", "graph_nodes", "graph_unresolved"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("rollback left %d rows in %s", count, table)
		}
	}
}

func candidateLengths(candidates []string) []int {
	lengths := make([]int, len(candidates))
	for i, candidate := range candidates {
		lengths[i] = len(candidate)
	}
	return lengths
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestStageGraphObservationBatchesDeduplicatesDuplicateDocumentationUnresolved(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	doc := stagingDoc(".docs/wiki/guide.md", "DOC-GUIDE")
	edge := model.DocEdge{FromPath: doc.Path, ToDocID: "DOC-MISSING", Kind: "doc_id", Label: "DOC-MISSING"}
	req := GraphAssemblyRequest{
		Batches:   []model.GraphObservationBatch{batch},
		Docs:      []model.DocRecord{doc},
		DocEdges:  []model.DocEdge{edge, edge},
		CreatedAt: time.Unix(1, 0),
	}
	bundle, err := AssembleGraphObservationBatches(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Unresolved) != 1 {
		t.Fatalf("duplicate documentation unresolved claims: got=%d", len(bundle.Unresolved))
	}
	claim := bundle.Unresolved[0]
	if claim.ReasonCode != "missing_doc_target" || len(claim.Candidates) != 0 {
		t.Fatalf("deduplication changed semantic fields: reason=%q candidates=%#v", claim.ReasonCode, claim.Candidates)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	generation, err := StageGraphObservationBatches(ctx, db, req)
	if err != nil {
		t.Fatal(err)
	}
	if generation.UnresolvedCount != 1 {
		t.Fatalf("persisted unresolved count=%d", generation.UnresolvedCount)
	}
	var persisted int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_unresolved WHERE generation_id = ?", generation.GenerationID[:]).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != 1 {
		t.Fatalf("persisted unresolved rows=%d", persisted)
	}
	if _, err := store.ValidateGraphGeneration(ctx, db, generation.GenerationID); err != nil {
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

func TestStageGraphObservationBatchesPreservesDistinctUnresolvedClaimsAfterDedupe(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	batch := stagingBatch("go", "src/go", true)
	edge := batch.Edges[0]
	selector := stagingDigest("same-edge-selector")
	source := edge.SourceDigest
	claims := []model.GraphObservationUnresolved{
		{Ref: "U1", OwnerPath: edge.OwnerPath, SubjectKind: "method", Capability: "calls", SelectorDigest: selector, ReasonCode: "missing_symbol", Candidates: []string{"pkg.Target"}, Backend: "go", SourceDigest: &source, RecoveryHintCode: "retry"},
		{Ref: "U2", OwnerPath: edge.OwnerPath, SubjectKind: "method", Capability: "calls", SelectorDigest: selector, ReasonCode: "ambiguous_symbol", Candidates: []string{"pkg.TargetA", "pkg.TargetB"}, Backend: "go", SourceDigest: &source, RecoveryHintCode: "retry"},
	}
	duplicate := claims[0]
	duplicate.Ref = "U3"
	batch.Unresolved = append(batch.Unresolved, claims[0], claims[1], duplicate)
	for i := range batch.Coverage {
		if batch.Coverage[i].Capability == "calls" {
			batch.Coverage[i].Eligible = 4
			batch.Coverage[i].Observed = 1
			batch.Coverage[i].Unresolved = 3
		}
	}
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	req := GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(1, 0)}
	bundle, err := AssembleGraphObservationBatches(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Unresolved) != 2 {
		t.Fatalf("unresolved claims after exact-duplicate dedupe=%d, want 2", len(bundle.Unresolved))
	}
	if bundle.Unresolved[0].UnresolvedID != 0 || bundle.Unresolved[1].UnresolvedID != 1 {
		t.Fatalf("unresolved IDs were not assigned after dedupe: %+v", bundle.Unresolved)
	}

	byReason := make(map[string]model.GraphUnresolved, len(bundle.Unresolved))
	keys := make(map[model.GraphDigest]struct{}, len(bundle.Unresolved))
	for _, got := range bundle.Unresolved {
		if got.UnresolvedKey == (model.GraphDigest{}) || got.CrossRID != model.UnresolvedRID(got.UnresolvedKey) {
			t.Fatalf("unresolved identity was not assigned canonically: %+v", got)
		}
		if _, duplicate := keys[got.UnresolvedKey]; duplicate {
			t.Fatalf("distinct unresolved claims share key: %+v", got)
		}
		keys[got.UnresolvedKey] = struct{}{}
		byReason[got.ReasonCode] = got
	}
	for _, want := range claims[:2] {
		got, ok := byReason[want.ReasonCode]
		if !ok {
			t.Fatalf("unresolved reason %q did not survive: %+v", want.ReasonCode, bundle.Unresolved)
		}
		if got.OwnerPath != want.OwnerPath || got.SubjectKind != want.SubjectKind || got.SelectorDigest != want.SelectorDigest || got.Backend != want.Backend || got.RecoveryHintCode != want.RecoveryHintCode || !reflect.DeepEqual(got.Candidates, want.Candidates) {
			t.Fatalf("unresolved claim fields changed for %q: got=%+v want=%+v", want.ReasonCode, got, want)
		}
		if got.SourceDigest == nil || *got.SourceDigest != source {
			t.Fatalf("unresolved source digest changed for %q: %+v", want.ReasonCode, got)
		}
		if err := model.ValidateGraphUnresolved(got); err != nil {
			t.Fatalf("assembled unresolved %q is invalid: %v", want.ReasonCode, err)
		}
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}

	generation, err := StageGraphObservationBatches(ctx, db, req)
	if err != nil {
		t.Fatal(err)
	}
	if generation.UnresolvedCount != 2 {
		t.Fatalf("staged unresolved count=%d, want 2", generation.UnresolvedCount)
	}
	if _, err := store.ValidateGraphGeneration(ctx, db, generation.GenerationID); err != nil {
		t.Fatalf("staged generation validation failed: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT unresolved_id, unresolved_key, reason_code, candidates_json, cross_rid FROM graph_unresolved WHERE generation_id = ? ORDER BY unresolved_id", generation.GenerationID[:])
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seenPersistedKeys := make(map[string]struct{}, len(bundle.Unresolved))
	persisted := 0
	for rows.Next() {
		var id int
		var key []byte
		var reason, candidatesJSON, crossRID string
		if err := rows.Scan(&id, &key, &reason, &candidatesJSON, &crossRID); err != nil {
			t.Fatal(err)
		}
		persisted++
		if id != persisted-1 {
			t.Fatalf("persisted unresolved ID=%d at row %d, want post-dedupe ID %d", id, persisted, persisted-1)
		}
		if len(key) == 0 {
			t.Fatalf("persisted unresolved %q has no key", reason)
		}
		if _, duplicate := seenPersistedKeys[string(key)]; duplicate {
			t.Fatalf("persisted unresolved claims share a key: reason=%q", reason)
		}
		seenPersistedKeys[string(key)] = struct{}{}
		var candidates []string
		if err := json.Unmarshal([]byte(candidatesJSON), &candidates); err != nil {
			t.Fatalf("decode persisted candidates for %q: %v", reason, err)
		}
		want, ok := byReason[reason]
		if !ok || !reflect.DeepEqual(candidates, want.Candidates) || crossRID != want.CrossRID {
			t.Fatalf("persisted unresolved changed: reason=%q candidates=%#v cross_rid=%q", reason, candidates, crossRID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if persisted != 2 {
		t.Fatalf("persisted unresolved rows=%d, want 2", persisted)
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

func TestPublishGraphObservationBatchesIsIdempotentForActiveContent(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	batch := stagingBatch("go", "src/app", true)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	first, err := PublishGraphObservationBatches(ctx, db, GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(100, 0).UTC()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PublishGraphObservationBatches(ctx, db, GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(101, 0).UTC()}, &first.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if !graphGenerationImmutableEqual(first, second) || second.Status != model.GraphGenerationActive {
		t.Fatalf("idempotent generation changed: first=%+v second=%+v", first, second)
	}
	var generations, active int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations WHERE status = ?", model.GraphGenerationActive).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if generations != 1 || active != 1 {
		t.Fatalf("generation rows=%d active=%d, want one active generation", generations, active)
	}
	if persisted, err := store.ValidateGraphGeneration(ctx, db, first.GenerationID); err != nil {
		t.Fatalf("persisted content validation: %v", err)
	} else if !graphGenerationImmutableEqual(persisted, second) {
		t.Fatalf("persisted generation differs from returned generation: persisted=%+v returned=%+v", persisted, second)
	}
}

func TestPublishGraphObservationBatchesRejectsCorruptActiveGeneration(t *testing.T) {
	for _, test := range []struct {
		name   string
		update string
	}{
		{name: "metadata", update: "UPDATE graph_generations SET source_fingerprint = zeroblob(32) WHERE generation_id = ?"},
		{name: "content", update: "UPDATE graph_nodes SET display_name = 'corrupt' WHERE generation_id = ? AND node_id = 0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			batch := stagingBatch("go", "src/app", false)
			if err := model.SealGraphObservationBatch(&batch); err != nil {
				t.Fatal(err)
			}
			req := GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(100, 0).UTC()}
			first, err := PublishGraphObservationBatches(ctx, db, req, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, test.update, first.GenerationID[:]); err != nil {
				t.Fatal(err)
			}
			if _, err := PublishGraphObservationBatches(ctx, db, GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(101, 0).UTC()}, &first.GenerationID); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
				t.Fatalf("corruption error=%v, want %v", err, model.ErrGraphGenerationCorrupt)
			}
		})
	}
}

func TestPublishGraphObservationBatchesIdempotentPathPreservesPointerCAS(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	batch := stagingBatch("go", "src/app", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	first, err := PublishGraphObservationBatches(ctx, db, GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(100, 0).UTC()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wrongPrior := stagingDigest("wrong-prior")
	got, err := PublishGraphObservationBatches(ctx, db, GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, CreatedAt: time.Unix(101, 0).UTC()}, &wrongPrior)
	if !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("pointer conflict=%v, want %v", err, model.ErrGraphPointerConflict)
	}
	if got.GenerationID != first.GenerationID {
		t.Fatalf("idempotent conflict returned generation=%+v, want %+v", got, first)
	}
	var generations int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if generations != 1 {
		t.Fatalf("generation rows=%d, want 1", generations)
	}
}

func TestDocumentGraphPreservesRelationKinds(t *testing.T) {
	docs := []model.DocRecord{
		{Path: "wiki/source.md", DocID: "SOURCE", ContentHash: stagingDigest("source").String()},
		{Path: "wiki/target.md", DocID: "TARGET", ContentHash: stagingDigest("target").String()},
	}
	kinds := []string{"doc_wikilink", "doc_embed", "doc_markdown_link", "doc_id", "doc_hierarchy"}
	edges := make([]model.DocEdge, 0, len(kinds))
	for _, kind := range kinds {
		edges = append(edges, model.DocEdge{FromPath: "wiki/source.md", ToPath: "wiki/target.md", Kind: kind, Label: kind})
	}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{
		Docs: docs, DocEdges: edges, RepositoryIdentity: "https://example.com/docs", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, edge := range bundle.Edges {
		if edge.SourceBackend == "docgraph" {
			got[edge.Relation] = true
		}
	}
	for _, kind := range kinds {
		if !got[kind] {
			t.Fatalf("relation %q missing from %v", kind, got)
		}
	}
}

func TestDocumentGraphReportsBoundedAmbiguousAndMissingTargets(t *testing.T) {
	docs := []model.DocRecord{
		{Path: "wiki/source.md", ContentHash: stagingDigest("source").String()},
		{Path: "wiki/a/target.md", DocID: "DUP", ContentHash: stagingDigest("a").String()},
		{Path: "wiki/b/target.md", DocID: "DUP", ContentHash: stagingDigest("b").String()},
	}
	edges := []model.DocEdge{
		{FromPath: "wiki/source.md", ToPath: "target.md", Kind: "doc_wikilink", Label: "target", Candidates: []string{"wiki/b/target.md", "wiki/a/target.md"}, UnresolvedReason: "ambiguous_doc_target"},
		{FromPath: "wiki/source.md", ToPath: "missing.md", Kind: "doc_wikilink", Label: "missing", UnresolvedReason: "missing_doc_target"},
		{FromPath: "wiki/source.md", ToDocID: "DUP", Kind: "doc_id", Label: "DUP"},
	}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{
		Docs: docs, DocEdges: edges, RepositoryIdentity: "https://example.com/docs", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, missing := 0, 0
	for _, unresolved := range bundle.Unresolved {
		switch unresolved.ReasonCode {
		case "ambiguous_doc_target":
			ambiguous++
			if len(unresolved.Candidates) != 2 || unresolved.Candidates[0] != "wiki/a/target.md" || unresolved.Candidates[1] != "wiki/b/target.md" {
				t.Fatalf("candidates=%v", unresolved.Candidates)
			}
		case "missing_doc_target":
			missing++
		}
	}
	if ambiguous != 2 || missing != 1 {
		t.Fatalf("ambiguous=%d missing=%d unresolved=%#v", ambiguous, missing, bundle.Unresolved)
	}
}

func TestDocumentGraphAssemblyRemainsDeterministicAndSkipsSelfEdges(t *testing.T) {
	docs := []model.DocRecord{
		{Path: "wiki/source.md", DocID: "SOURCE", ContentHash: stagingDigest("source").String()},
		{Path: "wiki/target.md", DocID: "TARGET", ContentHash: stagingDigest("target").String()},
	}
	edges := []model.DocEdge{
		{FromPath: "wiki/source.md", ToPath: "wiki/source.md", Kind: "doc_wikilink", Label: "#self"},
		{FromPath: "wiki/source.md", ToPath: "wiki/target.md", Kind: "doc_wikilink", Label: "target"},
	}
	request := GraphAssemblyRequest{Docs: docs, DocEdges: edges, RepositoryIdentity: "https://example.com/docs", CreatedAt: time.Unix(1, 0)}
	first, err := AssembleGraphObservationBatches(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Docs[0], request.Docs[1] = request.Docs[1], request.Docs[0]
	second, err := AssembleGraphObservationBatches(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation.GenerationID != second.Generation.GenerationID {
		t.Fatalf("generation changed: %s != %s", first.Generation.GenerationID, second.Generation.GenerationID)
	}
	for _, edge := range first.Edges {
		if edge.FromNodeID == edge.ToNodeID && edge.SourceBackend == "docgraph" {
			t.Fatalf("self edge published: %#v", edge)
		}
	}
}
func TestAssembleGraphResolvesCanonicalTPDocumentMention(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	source := stagingDoc(".docs/wiki/RF-SYNTHETIC.md", "RF-SYNTHETIC")
	target := stagingDoc(".docs/wiki/TP-SYNTHETIC.md", "TP-SYNTHETIC-001")
	mention := model.DocMention{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-SYNTHETIC-001"}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: []model.DocRecord{source, target}, DocMentions: []model.DocMention{mention}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	var sourceID, targetID int
	for _, node := range bundle.Nodes {
		switch node.Identity.OwnerPath {
		case source.Path:
			sourceID = node.NodeID
		case target.Path:
			targetID = node.NodeID
		}
	}
	found := false
	for _, edge := range bundle.Edges {
		if edge.Relation == "doc_mentions" && edge.FromNodeID == sourceID && edge.ToNodeID == targetID {
			found = true
		}
	}
	if !found {
		t.Fatalf("canonical TP mention did not resolve to document node: %+v", bundle.Edges)
	}
	if len(bundle.Unresolved) != 0 || mention.MentionValue != "TP-SYNTHETIC-001" {
		t.Fatalf("canonical TP mention changed or remained unresolved: value=%q unresolved=%+v", mention.MentionValue, bundle.Unresolved)
	}
}
func TestResolvedDocumentMentionSourceBlockChangesAllIdentityDigests(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	source := stagingDoc(".docs/wiki/RF-SYNTHETIC.md", "RF-SYNTHETIC")
	targetA := stagingDoc(".docs/wiki/TP-A.md", "TP-SYNTHETIC-A")
	targetB := stagingDoc(".docs/wiki/TP-B.md", "TP-SYNTHETIC-B")
	canonicalMentions := []model.DocMention{
		{DocPath: source.Path, MentionType: "test_file", MentionValue: targetA.DocID, SourceBlock: "frontmatter"},
		{DocPath: source.Path, MentionType: "test_file", MentionValue: targetB.DocID, SourceBlock: "requirements"},
		{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-MISSING-001", SourceBlock: "missing-target"},
	}
	request := func(docs []model.DocRecord, mentions []model.DocMention) GraphAssemblyRequest {
		return GraphAssemblyRequest{
			Batches:     []model.GraphObservationBatch{batch},
			Docs:        docs,
			DocMentions: mentions,
			CreatedAt:   time.Unix(1, 0).UTC(),
		}
	}
	canonical, err := AssembleGraphObservationBatches(request(
		[]model.DocRecord{source, targetA, targetB},
		canonicalMentions,
	))
	if err != nil {
		t.Fatal(err)
	}
	whitespaceAndReordered, err := AssembleGraphObservationBatches(request(
		[]model.DocRecord{targetB, source, targetA},
		[]model.DocMention{
			{DocPath: " " + source.Path + " ", MentionType: " test_file ", MentionValue: " TP-MISSING-001 ", SourceBlock: " missing-target "},
			{DocPath: " " + source.Path + " ", MentionType: " test_file ", MentionValue: " " + targetB.DocID + " ", SourceBlock: " requirements "},
			{DocPath: " " + source.Path + " ", MentionType: " test_file ", MentionValue: " " + targetA.DocID + " ", SourceBlock: " frontmatter "},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonical, whitespaceAndReordered) {
		t.Fatalf("canonical-equivalent mention input changed assembled graph:\ncanonical=%+v\nwhitespace/reordered=%+v", canonical, whitespaceAndReordered)
	}
	changedMentions := append([]model.DocMention(nil), canonicalMentions...)
	changedMentions[0].SourceBlock = "appendix"
	changed, err := AssembleGraphObservationBatches(request(
		[]model.DocRecord{source, targetA, targetB},
		changedMentions,
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical.Edges) != 2 || len(changed.Edges) != 2 || len(canonical.Unresolved) != 1 || len(changed.Unresolved) != 1 {
		t.Fatalf("mention resolution changed unexpectedly: canonical edges/unresolved=%d/%d changed=%d/%d", len(canonical.Edges), len(canonical.Unresolved), len(changed.Edges), len(changed.Unresolved))
	}
	beforeFacts := graphDocFactsDigest([]model.DocRecord{source, targetA, targetB}, nil, canonicalMentions, canonical.Unresolved)
	afterFacts := graphDocFactsDigest([]model.DocRecord{source, targetA, targetB}, nil, changedMentions, changed.Unresolved)
	if beforeFacts == afterFacts {
		t.Fatal("resolved mention SourceBlock did not alter document facts digest")
	}
	if canonical.Generation.ContentDigest == changed.Generation.ContentDigest ||
		canonical.Generation.SourceFingerprint == changed.Generation.SourceFingerprint ||
		canonical.Generation.ConfigFingerprint == changed.Generation.ConfigFingerprint ||
		canonical.Generation.BackendManifestDigest == changed.Generation.BackendManifestDigest ||
		canonical.Generation.GenerationID == changed.Generation.GenerationID {
		t.Fatal("resolved mention SourceBlock did not alter the complete generation identity chain")
	}
	var canonicalMissing, changedMissing model.GraphUnresolved
	for _, unresolved := range canonical.Unresolved {
		if unresolved.TargetValue == "TP-MISSING-001" {
			canonicalMissing = unresolved
		}
	}
	for _, unresolved := range changed.Unresolved {
		if unresolved.TargetValue == "TP-MISSING-001" {
			changedMissing = unresolved
		}
	}
	if canonicalMissing.UnresolvedKey == (model.GraphDigest{}) || canonicalMissing.UnresolvedKey != changedMissing.UnresolvedKey ||
		canonicalMissing.CrossRID != changedMissing.CrossRID ||
		canonicalMissing.CrossRID != model.UnresolvedRID(canonicalMissing.UnresolvedKey) {
		t.Fatalf("unrelated unresolved identity changed: canonical=%+v changed=%+v", canonicalMissing, changedMissing)
	}
}

func TestAssembleGraphReportsMissingAndAmbiguousCanonicalTPDocuments(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	source := stagingDoc(".docs/wiki/RF-SYNTHETIC.md", "RF-SYNTHETIC")
	missing := model.DocMention{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-MISSING-001"}
	missingBundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: []model.DocRecord{source}, DocMentions: []model.DocMention{missing}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(missingBundle.Unresolved) != 1 || missingBundle.Unresolved[0].ReasonCode != "missing_doc_target" || missingBundle.Unresolved[0].TargetKind != "document" || missingBundle.Unresolved[0].TargetValue != "TP-MISSING-001" || len(missingBundle.Unresolved[0].Candidates) != 0 {
		t.Fatalf("missing canonical TP diagnostic = %+v", missingBundle.Unresolved)
	}
	duplicateA := stagingDoc(".docs/wiki/TP-a.md", "TP-DUPLICATE-001")
	duplicateB := stagingDoc(".docs/wiki/TP-b.md", "TP-DUPLICATE-001")
	ambiguous := model.DocMention{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-DUPLICATE-001"}
	ambiguousBundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: []model.DocRecord{source, duplicateB, duplicateA}, DocMentions: []model.DocMention{ambiguous}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(ambiguousBundle.Unresolved) != 1 {
		t.Fatalf("ambiguous canonical TP unresolved = %+v", ambiguousBundle.Unresolved)
	}
	u := ambiguousBundle.Unresolved[0]
	if u.ReasonCode != "ambiguous_doc_target" || u.TargetKind != "document" || u.TargetValue != "TP-DUPLICATE-001" || !reflect.DeepEqual(u.Candidates, []string{duplicateA.Path, duplicateB.Path}) {
		t.Fatalf("ambiguous canonical TP diagnostic = %+v", u)
	}
}
func TestAssembleGraphCarriesDocumentMentionBlockAttribution(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	source := stagingDoc(".docs/wiki/RF-SYNTHETIC.md", "RF-SYNTHETIC")
	mention := model.DocMention{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-MISSING-001", SourceBlock: "frontmatter"}
	bundle, err := AssembleGraphObservationBatches(GraphAssemblyRequest{Batches: []model.GraphObservationBatch{batch}, Docs: []model.DocRecord{source}, DocMentions: []model.DocMention{mention}, CreatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Unresolved) != 1 || bundle.Unresolved[0].OwnerPath != source.Path || bundle.Unresolved[0].SourceDocument != source.Path || bundle.Unresolved[0].SourceBlock != "frontmatter" {
		t.Fatalf("document mention attribution = %+v", bundle.Unresolved)
	}
}
func TestGraphUnresolvedProvenanceChangesFactsContentAndGeneration(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	source := stagingDoc(".docs/wiki/RF-SYNTHETIC.md", "RF-SYNTHETIC")
	mention := model.DocMention{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-MISSING-001", SourceBlock: "frontmatter"}
	request := GraphAssemblyRequest{
		Batches:     []model.GraphObservationBatch{batch},
		Docs:        []model.DocRecord{source},
		DocMentions: []model.DocMention{mention},
		CreatedAt:   time.Unix(1, 0),
	}
	before, err := AssembleGraphObservationBatches(request)
	if err != nil {
		t.Fatal(err)
	}
	mention.SourceBlock = "requirements"
	request.DocMentions = []model.DocMention{mention}
	after, err := AssembleGraphObservationBatches(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Unresolved) != 1 || len(after.Unresolved) != 1 {
		t.Fatalf("unresolved counts: before=%d after=%d", len(before.Unresolved), len(after.Unresolved))
	}
	if before.Unresolved[0].UnresolvedKey != after.Unresolved[0].UnresolvedKey || before.Unresolved[0].CrossRID != after.Unresolved[0].CrossRID {
		t.Fatal("provenance-only change altered unresolved identity")
	}
	if before.Unresolved[0].SourceBlock != "frontmatter" || after.Unresolved[0].SourceBlock != "requirements" {
		t.Fatalf("provenance was not preserved: before=%+v after=%+v", before.Unresolved[0], after.Unresolved[0])
	}
	beforeFacts := graphDocFactsDigest(request.Docs, nil, []model.DocMention{{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-MISSING-001", SourceBlock: "frontmatter"}}, before.Unresolved)
	afterFacts := graphDocFactsDigest(request.Docs, nil, request.DocMentions, after.Unresolved)
	if beforeFacts == afterFacts {
		t.Fatal("provenance-only change did not alter document facts digest")
	}
	if before.Generation.ContentDigest == after.Generation.ContentDigest {
		t.Fatal("provenance-only change did not alter content digest")
	}
	if before.Generation.SourceFingerprint == after.Generation.SourceFingerprint || before.Generation.ConfigFingerprint == after.Generation.ConfigFingerprint || before.Generation.BackendManifestDigest == after.Generation.BackendManifestDigest || before.Generation.GenerationID == after.Generation.GenerationID {
		t.Fatal("provenance-only change did not alter generation fingerprints")
	}
}

func TestGraphUnresolvedDuplicateProvenanceIsDeterministicAndPrefersSourceBlock(t *testing.T) {
	batch := stagingBatch("go", "src/go", false)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	source := stagingDoc(".docs/wiki/RF-SYNTHETIC.md", "RF-SYNTHETIC")
	parserMention := model.DocMention{DocPath: source.Path, MentionType: "test_file", MentionValue: "TP-MISSING-001"}
	frontMatterMention := parserMention
	frontMatterMention.SourceBlock = "frontmatter"
	request := func(mentions []model.DocMention) GraphAssemblyRequest {
		return GraphAssemblyRequest{
			Batches:     []model.GraphObservationBatch{batch},
			Docs:        []model.DocRecord{source},
			DocMentions: mentions,
			CreatedAt:   time.Unix(1, 0),
		}
	}
	fromParserFirst, err := AssembleGraphObservationBatches(request([]model.DocMention{parserMention, frontMatterMention}))
	if err != nil {
		t.Fatal(err)
	}
	fromFrontMatterFirst, err := AssembleGraphObservationBatches(request([]model.DocMention{frontMatterMention, parserMention}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromParserFirst, fromFrontMatterFirst) {
		t.Fatalf("duplicate provenance changed with input order:\nparser first=%+v\nfrontmatter first=%+v", fromParserFirst.Unresolved, fromFrontMatterFirst.Unresolved)
	}
	if len(fromParserFirst.Unresolved) != 1 {
		t.Fatalf("duplicate source-protocol mentions produced %d unresolved claims", len(fromParserFirst.Unresolved))
	}
	claim := fromParserFirst.Unresolved[0]
	if claim.SourceDocument != source.Path || claim.SourceBlock != "frontmatter" || claim.TargetKind != "document" || claim.TargetValue != "TP-MISSING-001" {
		t.Fatalf("duplicate provenance lost canonical attribution: %+v", claim)
	}
	if claim.UnresolvedKey != model.GraphUnresolvedKey(claim) || claim.CrossRID != model.UnresolvedRID(claim.UnresolvedKey) {
		t.Fatalf("duplicate provenance changed unresolved identity: %+v", claim)
	}
}

func TestPublishGraphObservationBatchesReactivatesCanonicalGenerationAfterDocumentationDetour(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	docs := []model.DocRecord{stagingDoc(".docs/wiki/guide.md", "DOC-GUIDE")}
	batch := stagingBatch("go", "src/go", true)
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	fullRequest := func(createdAt time.Time) GraphAssemblyRequest {
		return GraphAssemblyRequest{
			Batches:   []model.GraphObservationBatch{batch},
			Docs:      docs,
			CreatedAt: createdAt,
		}
	}
	docsRequest := GraphAssemblyRequest{
		Docs:               docs,
		WorkspaceIdentity:  batch.WorkspaceIdentity,
		RepositoryIdentity: batch.RepositoryIdentity,
		CreatedAt:          time.Unix(101, 0).UTC(),
	}

	// Seed a canonical full graph, then publish the documentation-only graph
	// that a failed topology observation would have made active.
	full, err := PublishGraphObservationBatches(ctx, db, fullRequest(time.Unix(100, 0).UTC()), nil)
	if err != nil {
		t.Fatal(err)
	}
	docsOnly, err := PublishGraphObservationBatches(ctx, db, docsRequest, &full.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if full.GenerationID == docsOnly.GenerationID ||
		full.ContentDigest == docsOnly.ContentDigest ||
		full.SourceFingerprint == docsOnly.SourceFingerprint ||
		full.ConfigFingerprint == docsOnly.ConfigFingerprint ||
		full.BackendManifestDigest == docsOnly.BackendManifestDigest {
		t.Fatalf("documentation-only and Go generations collided: full=%+v docs=%+v", full, docsOnly)
	}
	for _, generation := range []model.GraphGeneration{full, docsOnly} {
		if _, err := store.ValidateGraphGeneration(ctx, db, generation.GenerationID); err != nil {
			t.Fatalf("generation %s failed validation: %v", generation.GenerationID, err)
		}
	}

	// Re-observing the unchanged Go source must reuse its stable canonical ID
	// while replacing the documentation detour, rather than treating the
	// retired equivalent row as corruption.
	replayed, err := PublishGraphObservationBatches(ctx, db, fullRequest(time.Unix(102, 0).UTC()), &docsOnly.GenerationID)
	if err != nil {
		t.Fatalf("replaying unchanged Go observation: %v", err)
	}
	if replayed.GenerationID != full.GenerationID ||
		replayed.ContentDigest != full.ContentDigest ||
		replayed.SourceFingerprint != full.SourceFingerprint ||
		replayed.ConfigFingerprint != full.ConfigFingerprint ||
		replayed.BackendManifestDigest != full.BackendManifestDigest {
		t.Fatalf("replayed generation identity changed: first=%+v replayed=%+v", full, replayed)
	}
	active, ok, err := store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != full.GenerationID {
		t.Fatalf("active generation=%x ok=%v err=%v, want %x", active, ok, err, full.GenerationID)
	}
	if _, err := store.ValidateGraphGeneration(ctx, db, active); err != nil {
		t.Fatalf("replayed active generation validation: %v", err)
	}
	var fullPrevious, docsPrevious []byte
	if err := db.QueryRowContext(ctx, "SELECT previous_generation_id FROM graph_generations WHERE generation_id = ?", full.GenerationID[:]).Scan(&fullPrevious); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT previous_generation_id FROM graph_generations WHERE generation_id = ?", docsOnly.GenerationID[:]).Scan(&docsPrevious); err != nil {
		t.Fatal(err)
	}
	if len(fullPrevious) != 0 || string(docsPrevious) != string(full.GenerationID[:]) {
		t.Fatalf("retired replay rewrote ancestry: full.previous=%x docs.previous=%x", fullPrevious, docsPrevious)
	}
	var workspacePrevious []byte
	if err := db.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key = ?", "previous_graph_generation_id").Scan(&workspacePrevious); err != nil {
		t.Fatal(err)
	}
	if string(workspacePrevious) != string(docsOnly.GenerationID[:]) {
		t.Fatalf("workspace rollback pointer=%x, want docs-only %x", workspacePrevious, docsOnly.GenerationID)
	}
	var ancestryCycles int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM graph_generations a JOIN graph_generations b ON a.previous_generation_id=b.generation_id AND b.previous_generation_id=a.generation_id`).Scan(&ancestryCycles); err != nil {
		t.Fatal(err)
	}
	if ancestryCycles != 0 {
		t.Fatalf("graph history contains %d direct ancestry cycles", ancestryCycles)
	}
	var activeRows, generations int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations WHERE status = ?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_generations").Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if activeRows != 1 || generations != 2 {
		t.Fatalf("graph history active=%d generations=%d, want 1 active and 2 immutable rows", activeRows, generations)
	}

	// A stale pointer remains a strict CAS conflict and cannot activate a
	// canonical equivalent through the idempotent path.
	wrongPrior := stagingDigest("wrong-prior")
	if _, err := PublishGraphObservationBatches(ctx, db, fullRequest(time.Unix(103, 0).UTC()), &wrongPrior); !errors.Is(err, model.ErrGraphPointerConflict) {
		t.Fatalf("stale pointer error=%v, want %v", err, model.ErrGraphPointerConflict)
	}
	active, ok, err = store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != full.GenerationID {
		t.Fatalf("active generation changed after stale CAS: %x ok=%v err=%v", active, ok, err)
	}

	// Persisted same-ID metadata tampering is still corruption, not a reason to
	// accept or silently restage the candidate.
	if _, err := db.ExecContext(ctx, "UPDATE graph_generations SET source_fingerprint = zeroblob(32) WHERE generation_id = ?", full.GenerationID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishGraphObservationBatches(ctx, db, fullRequest(time.Unix(104, 0).UTC()), &full.GenerationID); !errors.Is(err, model.ErrGraphGenerationCorrupt) {
		t.Fatalf("same-ID metadata mismatch error=%v, want %v", err, model.ErrGraphGenerationCorrupt)
	}
	active, ok, err = store.ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != full.GenerationID {
		t.Fatalf("active pointer changed after corruption: %x ok=%v err=%v", active, ok, err)
	}
}

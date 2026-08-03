package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoslynObservationWireContract(t *testing.T) {
	if os.Getenv("MILSP_RUN_ROSLYN_CROSSLANG") != "1" {
		t.Skip("set MILSP_RUN_ROSLYN_CROSSLANG=1 to run the opt-in Roslyn wire oracle")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	emitDir := t.TempDir()
	cmd := exec.Command("dotnet", "run", "--project", "worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj", "-c", "Release")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "MILSP_G2_EMIT_DIR="+emitDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Roslyn contract failed: %v\\n%s", err, output)
	}

	paths := []string{"complete.json", "compiler-error.json", "canceled.json"}
	batches := make(map[string]GraphObservationBatch, len(paths))
	for _, name := range paths {
		path := filepath.Join(emitDir, name)
		raw, readErr := os.ReadFile(path)
		if readErr != nil || len(raw) == 0 {
			t.Fatalf("missing or empty emitted artifact %s: %v", name, readErr)
		}
		if bytes.Contains(raw, []byte(emitDir)) {
			t.Fatalf("emitted artifact %s leaks its temporary output path", name)
		}
		var batch GraphObservationBatch
		if err := json.Unmarshal(raw, &batch); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		batches[name] = batch
	}

	complete := batches["complete.json"]
	if err := complete.ValidateCanonical(); err != nil {
		t.Fatalf("worker-produced complete JSON is not canonical before sealing: %v", err)
	}
	if observationNonzero(complete.Digest) {
		t.Fatal("complete batch was already sealed")
	}
	if err := SealGraphObservationBatch(&complete); err != nil {
		t.Fatalf("seal complete: %v", err)
	}
	before, err := json.Marshal(complete)
	if err != nil {
		t.Fatal(err)
	}
	if err := complete.Validate(); err != nil {
		t.Fatalf("validate complete: %v", err)
	}
	after, err := json.Marshal(complete)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Validate mutated complete batch")
	}
	if err := complete.ReadyForStaging(); err != nil {
		t.Fatalf("complete batch is not stageable: %v", err)
	}

	for _, name := range []string{"compiler-error.json", "canceled.json"} {
		batch := batches[name]
		if err := SealGraphObservationBatch(&batch); err != nil {
			t.Fatalf("seal %s: %v", name, err)
		}
		if err := batch.Validate(); err != nil {
			t.Fatalf("validate %s: %v", name, err)
		}
		if err := batch.ReadyForStaging(); !errors.Is(err, ErrGraphObservationNotStageable) {
			t.Fatalf("%s staging error = %v, want ErrGraphObservationNotStageable", name, err)
		}
		wantReason := "compiler_errors"
		if name == "canceled.json" {
			wantReason = "canceled"
		}
		if !slicesContainOmissionReason(batch.Omissions, wantReason) {
			t.Fatalf("%s lost typed reason %q", name, wantReason)
		}
	}

	withTamper := func(label string, mutate func(*GraphObservationBatch)) {
		t.Helper()
		raw := batches["complete.json"]
		batch := cloneObservation(&raw)
		mutate(&batch)
		if err := SealGraphObservationBatch(&batch); err == nil {
			t.Fatalf("tampered %s batch was accepted by SealGraphObservationBatch", label)
		}
	}
	withTamper("unsafe path", func(batch *GraphObservationBatch) {
		batch.Evidence[0].SourceURI = "../leak"
	})
	withTamper("dangling edge evidence", func(batch *GraphObservationBatch) {
		for i := range batch.Evidence {
			if batch.Evidence[i].EdgeRef != "" {
				batch.Evidence[i].EdgeRef = "edge:missing"
				return
			}
		}
		t.Fatal("complete batch has no edge evidence")
	})
	withTamper("missing edge evidence", func(batch *GraphObservationBatch) {
		for _, edge := range batch.Edges {
			filtered := batch.Evidence[:0]
			removed := false
			for _, evidence := range batch.Evidence {
				if evidence.EdgeRef == edge.Ref {
					removed = true
					continue
				}
				filtered = append(filtered, evidence)
			}
			if removed {
				batch.Evidence = filtered
				return
			}
		}
		t.Fatal("complete batch has no edge evidence")
	})
	withTamper("zero source digest", func(batch *GraphObservationBatch) {
		batch.Nodes[0].SourceDigest = GraphDigest{}
	})
}

func slicesContainOmissionReason(omissions []GraphObservationOmission, reason string) bool {
	for _, omission := range omissions {
		if omission.ReasonCode == reason {
			return true
		}
	}
	return false
}

func observationTestBatch() GraphObservationBatch {
	key := NodeKeyFields{RepositoryIdentity: "github.com/acme/Repo", BackendType: "roslyn", Language: "csharp", ProjectOrModule: "Src/App", OwnerPath: "Src/App/a.cs", SymbolKind: "type", SemanticIdentity: "Acme.App.Widget"}
	nodeDigest := digestBytes([]byte("node"))
	r := GraphObservationRange{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 7}
	return GraphObservationBatch{SchemaVersion: 1, WorkspaceIdentity: "https://github.com/acme/Repo", RepositoryIdentity: "github.com/acme/Repo", Backend: "roslyn", BackendVersion: "4.14", ExtractorVersion: "extractor-1", ProjectOrModule: "Src/App", SourceFingerprint: digestBytes([]byte("source")), ConfigFingerprint: digestBytes([]byte("config")), Completeness: GraphCompletenessComplete, Capabilities: []GraphObservationCapability{{Backend: "roslyn", Capability: "declarations", State: GraphObservationStatusStable}}, Coverage: []GraphObservationCoverage{{Backend: "roslyn", Capability: "declarations", Eligible: 1, Observed: 1}}, Nodes: []GraphObservationNode{{Ref: "N1", Key: key, DisplayName: "Widget", SourceDigest: nodeDigest, ClaimStatus: GraphRecordExact, Resolution: "roslyn"}}, Evidence: []GraphObservationEvidence{{Ref: "EV1", NodeRef: "N1", SourceURI: "Src/App/a.cs", Range: &r, Backend: "roslyn", ExtractorVersion: "extractor-1", SourceDigest: nodeDigest, ObservedDigest: digestBytes([]byte("observed")), ClaimKind: "declaration", Status: GraphRecordExact}}}
}

func TestGraphObservationNodeRejectsEmptyDisplayName(t *testing.T) {
	// Regression for tedi-agent-mcp: Roslyn error/incomplete types with Name="" and
	// DocCommentId "T:" must not seal (GPH_OBS_NODE_INVALID / display_name bounded).
	b := observationTestBatch()
	b.Nodes[0].DisplayName = ""
	err := SealGraphObservationBatch(&b)
	var ge *GraphObservationError
	if err == nil || !errors.As(err, &ge) || ge.Code != "GPH_OBS_NODE_INVALID" {
		t.Fatalf("got %v, want GPH_OBS_NODE_INVALID", err)
	}
	if ge.Message != "node violates canonical batch or matrix" {
		t.Fatalf("message = %q", ge.Message)
	}
}

func TestGraphObservationNodeProjectOrModuleMustMatchBatch(t *testing.T) {
	b := observationTestBatch()
	b.Nodes[0].Key.ProjectOrModule = "Src/Other.csproj"
	err := SealGraphObservationBatch(&b)
	var ge *GraphObservationError
	if err == nil || !errors.As(err, &ge) || ge.Code != "GPH_OBS_NODE_INVALID" || ge.Message != "node violates canonical batch or matrix" {
		t.Fatalf("got %v", err)
	}
}

func TestGraphObservationContractAndRuntimeExclusion(t *testing.T) {
	b := observationTestBatch()
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	if err := b.ReadyForStaging(); err != nil {
		t.Fatal(err)
	}
	digest := b.Digest
	b.ResourceStats = &GraphObservationResourceStats{ElapsedMilliseconds: 3, RSSBytes: 100}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if b.Digest != digest {
		t.Fatal("runtime changed digest")
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var decoded GraphObservationBatch
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Validate() != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		x := observationTestBatch()
		if err := SealGraphObservationBatch(&x); err != nil || x.Digest != digest {
			t.Fatalf("repeat %d: %v", i, err)
		}
	}
}

func TestGraphObservationValidationIsNonMutatingAndTamperResistant(t *testing.T) {
	b := observationTestBatch()
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	before := cloneObservation(&b)
	b.Nodes[0].DisplayName = "tampered"
	if err := b.Validate(); err == nil || reflect.DeepEqual(b, before) {
		t.Fatal("tamper was accepted")
	}
	if !reflect.DeepEqual(b.Nodes[0].DisplayName, "tampered") {
		t.Fatal("Validate mutated input")
	}
	unsealed := observationTestBatch()
	snapshot := cloneObservation(&unsealed)
	if err := unsealed.Validate(); err == nil {
		t.Fatal("unsealed accepted")
	}
	if !reflect.DeepEqual(cloneObservation(&unsealed), snapshot) {
		t.Fatal("Validate mutated unsealed input")
	}
}

func TestGraphObservationCoverageAndTypedGates(t *testing.T) {
	b := observationTestBatch()
	b.Coverage[0].Observed = 0
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("coverage mismatch accepted")
	}
	b = observationTestBatch()
	b.Capabilities = append(b.Capabilities, GraphObservationCapability{Backend: "roslyn", Capability: "calls", State: GraphObservationStatusGated})
	b.Coverage = append(b.Coverage, GraphObservationCoverage{Backend: "roslyn", Capability: "calls", Eligible: 1, Omitted: 1})
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("gated capability without omission accepted")
	}
	b.Omissions = []GraphObservationOmission{{Ref: "O1", OwnerPath: "Src/App/a.cs", SubjectKind: "file", Backend: "roslyn", Capability: "calls", ReasonCode: "backend_unavailable", RecoveryHintCode: "retry"}}
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	b.Omissions[0].Ref = "N1"
	if err := b.Validate(); err == nil {
		t.Fatal("duplicate global ref accepted")
	}
}

func TestGraphObservationPartialNeedsReason(t *testing.T) {
	b := observationTestBatch()
	b.Completeness = GraphCompletenessPartial
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("partial without reason accepted")
	}
	b = observationTestBatch()
	b.Completeness = GraphCompletenessPartial
	b.Capabilities = append(b.Capabilities, GraphObservationCapability{Backend: "roslyn", Capability: "calls", State: GraphObservationStatusUnavailable})
	b.Coverage = append(b.Coverage, GraphObservationCoverage{Backend: "roslyn", Capability: "calls", Eligible: 1, Omitted: 1})
	b.Omissions = append(b.Omissions, GraphObservationOmission{Ref: "O1", OwnerPath: "Src/App/a.cs", SubjectKind: "file", Backend: "roslyn", Capability: "calls", ReasonCode: "unavailable", RecoveryHintCode: "retry"})
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
}

func TestGraphObservationEvidenceAndBounds(t *testing.T) {
	b := observationTestBatch()
	b.Evidence[0].Status = GraphRecordExtracted
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("evidence status mismatch accepted")
	}
	b = observationTestBatch()
	b.Evidence[0].SourceURI = "other.cs"
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("evidence source mismatch accepted")
	}
	b = observationTestBatch()
	b.ResourceStats = &GraphObservationResourceStats{ElapsedMilliseconds: 24*60*60*1000 + 1}
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("resource bound accepted")
	}
	b = observationTestBatch()
	b.Nodes[0].Ref = string(make([]byte, 257))
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("ref bound accepted")
	}
}

func graphObservationNode(ref, backend, language, kind, identity, claim, resolution string) GraphObservationNode {
	return GraphObservationNode{
		Ref:          ref,
		Key:          NodeKeyFields{RepositoryIdentity: "github.com/acme/Repo", BackendType: backend, Language: language, ProjectOrModule: "Src/App", OwnerPath: "Src/App/a.cs", SymbolKind: kind, SemanticIdentity: identity},
		DisplayName:  identity,
		SourceDigest: digestBytes([]byte(ref)),
		ClaimStatus:  claim,
		Resolution:   resolution,
	}
}

func graphObservationEvidence(ref string, node *GraphObservationNode, edge *GraphObservationEdge, extractor string) GraphObservationEvidence {
	v := GraphObservationEvidence{Ref: ref, SourceURI: "Src/App/a.cs", Range: &GraphObservationRange{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2}, Backend: "go", ExtractorVersion: extractor, ObservedDigest: digestBytes([]byte(ref)), Status: GraphRecordExact}
	if node != nil {
		v.NodeRef, v.SourceDigest, v.ClaimKind, v.Status = node.Ref, node.SourceDigest, "declaration", node.ClaimStatus
	} else {
		v.EdgeRef, v.SourceDigest, v.ClaimKind, v.Status = edge.Ref, edge.SourceDigest, edge.Relation, edge.Status
	}
	return v
}

func graphObservationGoBatch() GraphObservationBatch {
	b := observationTestBatch()
	b.Backend, b.BackendVersion, b.ExtractorVersion = "go", "1.24", "extractor-go"
	n := graphObservationNode("N1", "go", "go", "type", "acme.Widget", GraphRecordExtracted, "go/ast")
	b.Nodes = []GraphObservationNode{n}
	b.Evidence = []GraphObservationEvidence{graphObservationEvidence("EV1", &n, nil, b.ExtractorVersion)}
	b.Capabilities = []GraphObservationCapability{{Backend: "go", Capability: "declarations", State: GraphObservationStatusStable}}
	b.Coverage = []GraphObservationCoverage{{Backend: "go", Capability: "declarations", Eligible: 1, Observed: 1}}
	return b
}

func addGraphObservationEdge(b *GraphObservationBatch, relation, status, resolution string) {
	e := GraphObservationEdge{Ref: "E1", FromRef: "N1", ToRef: "N1", Relation: relation, Scope: "symbol", Status: status, OwnerPath: "Src/App/a.cs", Backend: b.Backend, Resolution: resolution, SourceDigest: digestBytes([]byte("edge"))}
	b.Edges = append(b.Edges, e)
	b.Evidence = append(b.Evidence, graphObservationEvidence("EV2", nil, &e, b.ExtractorVersion))
	b.Capabilities = append(b.Capabilities, GraphObservationCapability{Backend: b.Backend, Capability: relation, State: GraphObservationStatusStable})
	b.Coverage = append(b.Coverage, GraphObservationCoverage{Backend: b.Backend, Capability: relation, Eligible: 1, Observed: 1})
}

func TestGraphObservationEdgeEvidenceRequired(t *testing.T) {
	b := observationTestBatch()
	e := GraphObservationEdge{Ref: "E1", FromRef: "N1", ToRef: "N1", Relation: "calls", Scope: "symbol", Status: GraphRecordExact, OwnerPath: "Src/App/a.cs", Backend: "roslyn", Resolution: "roslyn", SourceDigest: digestBytes([]byte("edge"))}
	b.Edges = []GraphObservationEdge{e}
	b.Capabilities = append(b.Capabilities, GraphObservationCapability{Backend: "roslyn", Capability: "calls", State: GraphObservationStatusStable})
	b.Coverage = append(b.Coverage, GraphObservationCoverage{Backend: "roslyn", Capability: "calls", Eligible: 1, Observed: 1})
	b.Evidence = append(b.Evidence, graphObservationEvidence("EV2", nil, &e, b.ExtractorVersion))
	b.Evidence[1].Backend = "roslyn"
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	b.Evidence = b.Evidence[:1]
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("edge without evidence accepted")
	}
}

func TestGraphObservationGoMatrix(t *testing.T) {
	cases := []struct {
		name, relation, status, resolution string
		want                               bool
	}{
		{"ast contains", "contains", GraphRecordExtracted, "go/ast", true},
		{"ast imports", "imports", GraphRecordExtracted, "go/ast", true},
		{"types calls", "calls", GraphRecordExact, "go/types", true},
		{"gopls references", "references", GraphRecordExact, "gopls", true},
		{"ast calls", "calls", GraphRecordExtracted, "go/ast", false},
		{"ast references", "references", GraphRecordExtracted, "go/ast", false},
		{"unsupported implements", "implements", GraphRecordExact, "go/types", false},
		{"unsupported extends", "extends", GraphRecordExact, "go/types", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := graphObservationGoBatch()
			addGraphObservationEdge(&b, tc.relation, tc.status, tc.resolution)
			err := SealGraphObservationBatch(&b)
			if (err == nil) != tc.want {
				t.Fatalf("SealGraphObservationBatch() = %v, want success %v", err, tc.want)
			}
		})
	}
}

func TestGraphObservationGatedAndLexicalBackends(t *testing.T) {
	for _, backend := range []string{"tsserver", "pyright"} {
		t.Run(backend, func(t *testing.T) {
			b := graphObservationGoBatch()
			b.Backend = backend
			b.Nodes[0].Key.BackendType, b.Nodes[0].Key.Language, b.Nodes[0].Resolution = backend, backend, backend
			b.Capabilities[0] = GraphObservationCapability{Backend: backend, Capability: "declarations", State: GraphObservationStatusStable}
			b.Evidence[0].Backend = backend
			if err := SealGraphObservationBatch(&b); err == nil {
				t.Fatal("stable semantic batch accepted")
			}
			b.Nodes, b.Evidence = nil, nil
			b.Capabilities[0].State = GraphObservationStatusGated
			b.Coverage[0] = GraphObservationCoverage{Backend: backend, Capability: "declarations", Eligible: 1, Omitted: 1}
			b.Omissions = []GraphObservationOmission{{Ref: "O1", OwnerPath: "Src/App/a.cs", SubjectKind: "file", Backend: backend, Capability: "declarations", ReasonCode: "gated", RecoveryHintCode: "retry"}}
			if err := SealGraphObservationBatch(&b); err != nil {
				t.Fatal(err)
			}
			if err := b.ReadyForStaging(); err == nil {
				t.Fatal("gated batch ready")
			}
		})
	}
	b := graphObservationGoBatch()
	b.Capabilities[0].State = GraphObservationStatusExperimental
	b.Nodes[0].Key.SymbolKind, b.Nodes[0].Resolution = "file", "lexical"
	b.Coverage[0] = GraphObservationCoverage{Backend: "go", Capability: "declarations", Eligible: 2, Observed: 1, Omitted: 1}
	b.Omissions = []GraphObservationOmission{{Ref: "O1", OwnerPath: "Src/App/a.cs", SubjectKind: "file", Backend: "go", Capability: "declarations", ReasonCode: "experimental", RecoveryHintCode: "retry"}}
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	if err := b.ReadyForStaging(); err == nil {
		t.Fatal("lexical batch ready")
	}
	b = graphObservationGoBatch()
	b.Capabilities[0].State, b.Nodes[0].Resolution = GraphObservationStatusExperimental, "lexical"
	b.Coverage[0] = GraphObservationCoverage{Backend: "go", Capability: "declarations", Eligible: 2, Observed: 1, Omitted: 1}
	b.Omissions = []GraphObservationOmission{{Ref: "O1", OwnerPath: "Src/App/a.cs", SubjectKind: "file", Backend: "go", Capability: "declarations", ReasonCode: "experimental", RecoveryHintCode: "retry"}}
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("lexical type node accepted")
	}
	b = graphObservationGoBatch()
	b.Capabilities[0].State = GraphObservationStatusExperimental
	b.Nodes[0].Key.SymbolKind, b.Nodes[0].Resolution = "file", "lexical"
	b.Coverage[0] = GraphObservationCoverage{Backend: "go", Capability: "declarations", Eligible: 2, Observed: 1, Omitted: 1}
	b.Omissions = []GraphObservationOmission{{Ref: "O1", OwnerPath: "Src/App/a.cs", SubjectKind: "file", Backend: "go", Capability: "declarations", ReasonCode: "experimental", RecoveryHintCode: "retry"}}
	addGraphObservationEdge(&b, "contains", GraphRecordExtracted, "lexical")
	if err := SealGraphObservationBatch(&b); err == nil {
		t.Fatal("lexical edge accepted")
	}
}

func TestGraphObservationVersionBounds(t *testing.T) {
	for _, field := range []string{"backend", "extractor"} {
		for _, value := range []string{"bad\nversion", string(make([]byte, 257))} {
			b := observationTestBatch()
			if field == "backend" {
				b.BackendVersion = value
			} else {
				b.ExtractorVersion = value
			}
			if err := SealGraphObservationBatch(&b); err == nil {
				t.Fatalf("%s %q accepted", field, value)
			}
		}
	}
}

func TestGraphObservationNilSlicesNormalizeToWireEmptyArrays(t *testing.T) {
	b := observationTestBatch()
	b.Edges, b.Unresolved, b.Omissions = nil, nil, nil
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	if b.Edges == nil || b.Unresolved == nil || b.Omissions == nil {
		t.Fatal("sealed batch retained nil wire slices")
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}

	incoming := b
	incoming.Edges, incoming.Unresolved, incoming.Omissions = nil, nil, nil
	before := cloneObservation(&incoming)
	if err := incoming.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, cloneObservation(&incoming)) {
		t.Fatal("Validate mutated nil-slice input")
	}
}

func TestGraphObservationOrderIndependence(t *testing.T) {
	a := observationTestBatch()
	if err := SealGraphObservationBatch(&a); err != nil {
		t.Fatal(err)
	}
	b := observationTestBatch()
	b.Capabilities = append([]GraphObservationCapability(nil), b.Capabilities...)
	b.Coverage = append([]GraphObservationCoverage(nil), b.Coverage...)
	if err := SealGraphObservationBatch(&b); err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Fatal("equal semantic batches have different digests")
	}
}

package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func observationTestBatch() GraphObservationBatch {
	key := NodeKeyFields{RepositoryIdentity: "github.com/acme/Repo", BackendType: "roslyn", Language: "csharp", ProjectOrModule: "Src/App", OwnerPath: "Src/App/a.cs", SymbolKind: "type", SemanticIdentity: "Acme.App.Widget"}
	nodeDigest := digestBytes([]byte("node"))
	r := GraphObservationRange{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 7}
	return GraphObservationBatch{SchemaVersion: 1, WorkspaceIdentity: "https://github.com/acme/Repo", RepositoryIdentity: "github.com/acme/Repo", Backend: "roslyn", BackendVersion: "4.14", ExtractorVersion: "extractor-1", ProjectOrModule: "Src/App", SourceFingerprint: digestBytes([]byte("source")), ConfigFingerprint: digestBytes([]byte("config")), Completeness: GraphCompletenessComplete, Capabilities: []GraphObservationCapability{{Backend: "roslyn", Capability: "declarations", State: GraphObservationStatusStable}}, Coverage: []GraphObservationCoverage{{Backend: "roslyn", Capability: "declarations", Eligible: 1, Observed: 1}}, Nodes: []GraphObservationNode{{Ref: "N1", Key: key, DisplayName: "Widget", SourceDigest: nodeDigest, ClaimStatus: GraphRecordExact, Resolution: "roslyn"}}, Evidence: []GraphObservationEvidence{{Ref: "EV1", NodeRef: "N1", SourceURI: "Src/App/a.cs", Range: &r, Backend: "roslyn", ExtractorVersion: "extractor-1", SourceDigest: nodeDigest, ObservedDigest: digestBytes([]byte("observed")), ClaimKind: "declaration", Status: GraphRecordExact}}}
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
	if !reflect.DeepEqual(unsealed, snapshot) {
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

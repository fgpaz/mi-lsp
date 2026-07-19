package model

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func observationTestBatch() GraphObservationBatch {
	key := NodeKeyFields{RepositoryIdentity: "github.com/acme/repo", BackendType: "roslyn", Language: "csharp", ProjectOrModule: "src/app", OwnerPath: "src/app/a.cs", SymbolKind: "type", SemanticIdentity: "Acme.App.Widget"}
	r := GraphObservationRange{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 7}
	return GraphObservationBatch{SchemaVersion: 1, WorkspaceIdentity: "github.com/acme/repo", RepositoryIdentity: "github.com/acme/repo", Backend: "roslyn", BackendVersion: "4.14", ExtractorVersion: "extractor-1", ProjectOrModule: "src/app", SourceFingerprint: digestBytes([]byte("source")), ConfigFingerprint: digestBytes([]byte("config")), Completeness: GraphCompletenessComplete, Capabilities: []GraphObservationCapability{{Backend: "roslyn", Capability: "declarations", State: GraphObservationStatusStable}}, Coverage: []GraphObservationCoverage{{Backend: "roslyn", Capability: "declarations", Eligible: 1, Observed: 1}}, Nodes: []GraphObservationNode{{Ref: "n1", Key: key, DisplayName: "Widget", SourceDigest: digestBytes([]byte("node")), ClaimStatus: GraphRecordExact, Resolution: "roslyn"}}, Evidence: []GraphObservationEvidence{{Ref: "ev1", NodeRef: "n1", SourceURI: "src/app/a.cs", Range: &r, Backend: "roslyn", ExtractorVersion: "extractor-1", SourceDigest: digestBytes([]byte("source")), ObservedDigest: digestBytes([]byte("observed")), ClaimKind: "declaration", Status: GraphRecordExact}}}
}
func TestGraphObservationContract(t *testing.T) {
	b := observationTestBatch()
	if e := SealGraphObservationBatch(&b); e != nil {
		t.Fatal(e)
	}
	if e := b.ReadyForStaging(); e != nil {
		t.Fatal(e)
	}
	before := b.Digest
	b.ResourceStats = &GraphObservationResourceStats{ElapsedMilliseconds: 3}
	if e := b.Validate(); e != nil {
		t.Fatal(e)
	}
	if b.Digest != before {
		t.Fatal("runtime changed digest")
	}
	raw, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	var got GraphObservationBatch
	if e = json.Unmarshal(raw, &got); e != nil || got.Validate() != nil {
		t.Fatal(e)
	}
	for i := 0; i < 30; i++ {
		x := observationTestBatch()
		if e := SealGraphObservationBatch(&x); e != nil || x.Digest != b.Digest {
			t.Fatalf("repeat %d: %v", i, e)
		}
	}
}
func TestGraphObservationValidateNonMutatingAndCanonical(t *testing.T) {
	b := observationTestBatch()
	b.Capabilities = append([]GraphObservationCapability{{Backend: "roslyn", Capability: "declarations", State: "stable"}}, b.Capabilities...)
	snapshot := cloneObservation(&b)
	if e := b.Validate(); e == nil {
		t.Fatal("unsealed accepted")
	}
	if !reflect.DeepEqual(b, snapshot) {
		t.Fatal("validate mutated")
	}
	b = observationTestBatch()
	if e := SealGraphObservationBatch(&b); e != nil {
		t.Fatal(e)
	}
	b.Nodes[0].DisplayName = "tampered"
	if e := b.Validate(); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestGraphObservationAdversarialGates(t *testing.T) {
	b := observationTestBatch()
	b.Capabilities[0].State = "experimental"
	if e := SealGraphObservationBatch(&b); e != nil {
		t.Fatal(e)
	}
	if e := b.ReadyForStaging(); !errors.Is(e, ErrGraphObservationNotStageable) {
		t.Fatal(e)
	}
	b = observationTestBatch()
	b.Coverage = nil
	if e := SealGraphObservationBatch(&b); e == nil {
		t.Fatal("missing coverage accepted")
	}
	b = observationTestBatch()
	b.Omissions = []GraphObservationOmission{{Ref: "o1", OwnerPath: "src/app/a.cs", SubjectKind: "file", Backend: "roslyn", Capability: "declarations", ReasonCode: "backend_unavailable", RecoveryHintCode: "retry"}}
	if e := SealGraphObservationBatch(&b); e != nil {
		t.Fatal(e)
	}
	b.Omissions[0].Ref = "n1"
	if e := b.Validate(); e == nil {
		t.Fatal("duplicate ref accepted")
	}
}

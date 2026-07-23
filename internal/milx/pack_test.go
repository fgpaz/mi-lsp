package milx

import (
	"encoding/json"
	"strings"
	"testing"
)

func packItem(kind, rid string) PackItem {
	return PackItem{Kind: kind, CrossRID: rid, Digest: strings.Repeat("a", 64), Path: "internal/milx/pack.go", Provenance: json.RawMessage(`{"generation_id":"g"}`)}
}
func packSelection() PackSelection {
	return PackSelection{GenerationID: "g", GraphSchemaVersion: 1, AuthorityProfileDigest: strings.Repeat("b", 64), Nodes: []PackItem{packItem("node", "RID-2"), packItem("node", "RID-1")}, OutputBudget: MaxPackBytes}
}
func TestBuildPackDeterministicAcrossPermutations(t *testing.T) {
	var digest string
	for i := 0; i < 30; i++ {
		s := packSelection()
		if i%2 == 1 {
			s.Nodes[0], s.Nodes[1] = s.Nodes[1], s.Nodes[0]
		}
		p, err := BuildPack(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := VerifyPack(p); err != nil {
			t.Fatal(err)
		}
		if digest != "" && digest != p.Digest {
			t.Fatalf("digest changed: %s != %s", digest, p.Digest)
		}
		digest = p.Digest
	}
}
func TestPackRejectsUnsafeInputsAndTamper(t *testing.T) {
	cases := []func(*PackSelection){
		func(s *PackSelection) { s.Nodes[0].CrossRID = "" },
		func(s *PackSelection) { s.Nodes[0].Path = "../raw/source.json" },
		func(s *PackSelection) { s.Nodes[0].Provenance = json.RawMessage(`{"secret":"x"}`) },
		func(s *PackSelection) { s.GraphSchemaVersion = 2 },
	}
	for _, mutate := range cases {
		s := packSelection()
		mutate(&s)
		if _, err := BuildPack(s); err == nil {
			t.Fatal("expected invalid selection")
		}
	}
	p, err := BuildPack(packSelection())
	if err != nil {
		t.Fatal(err)
	}
	p.Digest = "bad"
	if err := VerifyPack(p); err == nil {
		t.Fatal("expected digest failure")
	}
}
func TestVerifyPackRejectsAuthorityProvenanceBypass(t *testing.T) {
	p, err := BuildPack(packSelection())
	if err != nil {
		t.Fatal(err)
	}
	p.Provenance.ParametersDigest = strings.Repeat("c", 64)
	semantic, err := canonicalPack(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Digest = DigestHex(semantic)
	if err := VerifyPack(p); err == nil {
		t.Fatal("expected authority provenance rejection")
	}
}

func TestDerivedResultRejectsAuthorityAttempt(t *testing.T) {
	result := Result{Schema: "milx-result/v1", Result: json.RawMessage(`{"authority_claim":"accepted"}`), Provenance: Provenance{GenerationID: "g", ExtensionID: "x", ExtensionVersion: "1", ParametersDigest: "p"}}
	result.ResultDigest = DigestHex(result.Result)
	if err := ValidateDerivedResult(result, "g", "x", "1", "p"); err == nil {
		t.Fatal("expected authority rejection")
	}
}

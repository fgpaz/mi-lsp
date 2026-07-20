package model

import "testing"

func TestWikiCodeContextDigestIsStableAndExcludesBudgetTelemetry(t *testing.T) {
	context := WikiCodeContext{
		PrimaryDoc:     DocRecord{Path: ".docs/wiki/04_RF/RF-GPH-007.md", DocID: "RF-GPH-007"},
		AuthorityChain: []WikiCodeAuthorityEntry{{Path: ".docs/wiki/00_gobierno_documental.md", Role: "governance"}, {Path: ".docs/wiki/04_RF/RF-GPH-007.md", Role: "primary"}},
		CodeEvidence:   []WikiCodeEvidence{{Path: "internal/service/wiki_code_context.go", Kind: "file"}},
		TokenBudget:    100,
		TokenUsed:      40,
	}
	first := WikiCodeContextDigest(context)
	context.TokenBudget = 9000
	context.TokenUsed = 12
	context.Truncated = true
	if got := WikiCodeContextDigest(context); got != first {
		t.Fatalf("digest changed with budget telemetry: %q != %q", got, first)
	}
	if len(first) != 64 {
		t.Fatalf("digest length = %d, want 64", len(first))
	}
}

func TestCanonicalWikiAuthorityRejectsRawAuditAndSnapshots(t *testing.T) {
	for _, path := range []string{".docs/raw/task.md", ".docs/auditoria/evidence.md", ".docs/wiki/snapshot/readme.md"} {
		if CanonicalWikiAuthority(path) {
			t.Fatalf("%q was accepted as canonical", path)
		}
	}
	if !CanonicalWikiAuthority(".docs/wiki/04_RF/RF-GPH-007.md") {
		t.Fatal("canonical wiki document was rejected")
	}
}

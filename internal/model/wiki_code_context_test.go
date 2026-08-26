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

func TestWikiCodeContextDigestIncludesCanonicalBridgeFieldsButExcludesCost(t *testing.T) {
	context := WikiCodeContext{
		DirectCode: []WikiCodeEvidence{{Path: "src/z.mjs", Symbol: "runDemo", Relation: RelationImplements, Origin: "wiki_declared_path", BindingRef: "b"}, {Path: "src/a.mjs", Symbol: "runDemo", Relation: RelationImplements, Origin: "wiki_declared_path", BindingRef: "a"}},
		Tests:      []WikiCodeEvidence{{Path: "test/service.test.mjs", Relation: RelationTests, Origin: "wiki_declared_path"}},
		Cost:       WikiCodeResolveCost{FilesChecked: 1},
		TokenBudget: 10,
		TokenUsed:   10,
	}
	first := WikiCodeContextDigest(context)
	context.Cost.FilesChecked = 99
	context.TokenBudget = 9000
	context.TokenUsed = 3
	context.Truncated = true
	SortWikiCodeContext(&context)
	if got := WikiCodeContextDigest(context); got != first {
		t.Fatalf("bridge digest changed with telemetry/order: %q != %q", got, first)
	}
	if context.DirectCode[0].Path != "src/a.mjs" {
		t.Fatalf("bridge evidence was not canonicalized: %#v", context.DirectCode)
	}
}

func TestWikiCodeContextSortUsesSemanticRelationOrder(t *testing.T) {
	context := WikiCodeContext{
		DirectCode: []WikiCodeEvidence{
			{Path: "src/operate.go", Relation: RelationOperates},
			{Path: "src/test.go", Relation: RelationTests},
			{Path: "src/impl.go", Relation: RelationImplements},
			{Path: "src/config.go", Relation: RelationConfigures},
		},
	}
	SortWikiCodeContext(&context)
	want := []string{RelationImplements, RelationTests, RelationConfigures, RelationOperates}
	for i, relation := range want {
		if context.DirectCode[i].Relation != relation {
			t.Fatalf("relation[%d] = %q, want %q", i, context.DirectCode[i].Relation, relation)
		}
	}
	if WikiCodeRelationOrder(RelationImplements) >= WikiCodeRelationOrder(RelationTests) || WikiCodeRelationOrder(RelationTests) >= WikiCodeRelationOrder(RelationOperates) {
		t.Fatal("semantic relation ordering is not monotonic")
	}
}

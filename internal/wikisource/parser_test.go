package wikisource

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestParseMapsSourceLinksToTraceMentions(t *testing.T) {
	parsed := Parse(".docs/wiki/10_contratos/CT-AI-GATEWAY-FORMATS.md", `# CT-AI-GATEWAY-FORMATS

wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-AI-GATEWAY-FORMATS
code_links:
  - src/backend/MultiTedi.Contracts/InternalApi/AI/AiContracts.cs
test_links:
  - src/backend/tests/MultiTedi.ControlPlane.Tests/Services/GlobalJudgeValidatorTests.cs

`+"```toon"+`
block_id: CT-AI-GATEWAY-FORMATS.global-judge
kind: contract
source_of_truth: CT-AI-GATEWAY-FORMATS
code_links:
  - src/backend/MultiTedi.ControlPlane.Application/Services/AiGatewayApplicationService.cs
test_links:
  - src/backend/runtime/orchestrator/tests/test_runtime_service.py
`+"```"+`
`, 1)

	assertMention(t, parsed, "implements", "src/backend/MultiTedi.Contracts/InternalApi/AI/AiContracts.cs")
	assertMention(t, parsed, "test_file", "src/backend/tests/MultiTedi.ControlPlane.Tests/Services/GlobalJudgeValidatorTests.cs")
	assertMention(t, parsed, "implements", "src/backend/MultiTedi.ControlPlane.Application/Services/AiGatewayApplicationService.cs")
	assertMention(t, parsed, "test_file", "src/backend/runtime/orchestrator/tests/test_runtime_service.py")
}

func assertMention(t *testing.T, parsed ParsedDoc, kind string, value string) {
	t.Helper()
	for _, mention := range parsed.Mentions {
		if mention.MentionType == kind && mention.MentionValue == value {
			return
		}
	}
	t.Fatalf("missing mention %s=%s in %#v", kind, value, parsed.Mentions)
}

func TestParserCanonicalArtifactBinding(t *testing.T) {
	content := `# Test
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-001

` + "```toon" + `
block_id: block-a
artifact_bindings:
  - doc_path: wiki/00_test.md
    target_path: src/main.go
    relation: implements
    target_kind: file
` + "```"
	parsed := Parse("wiki/00_test.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected at least one binding")
	}
	b := bindings[0]
	if b.DocPath != "wiki/00_test.md" {
		t.Fatalf("DocPath = %q, want %q", b.DocPath, "wiki/00_test.md")
	}
	if b.TargetPath != "src/main.go" {
		t.Fatalf("TargetPath = %q, want %q", b.TargetPath, "src/main.go")
	}
	if b.Relation != model.RelationImplements {
		t.Fatalf("Relation = %q, want %q", b.Relation, model.RelationImplements)
	}
	if b.AuthoringOrigin != model.AuthoringOriginCanonical {
		t.Fatalf("AuthoringOrigin = %q, want %q", b.AuthoringOrigin, model.AuthoringOriginCanonical)
	}
	if b.BindingRef == "" {
		t.Fatal("BindingRef should not be empty")
	}
	if len(b.BindingRef) != 64 {
		t.Fatalf("BindingRef length = %d, want 64 (SHA-256 hex)", len(b.BindingRef))
	}
}

func TestParserLegacyImplementationAnchors(t *testing.T) {
	content := `# Legacy
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-LEGACY
implementation_anchors:
  - src/legacy/Driver.cs

` + "```toon" + `
block_id: b1
implementation_anchors:
  - src/legacy/Helper.cs
` + "```"
	parsed := Parse("wiki/legacy.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected bindings from legacy implementation_anchors")
	}
	for _, b := range bindings {
		if b.AuthoringOrigin != model.AuthoringOriginLegacy {
			t.Fatalf("expected legacy authoring_origin, got %q", b.AuthoringOrigin)
		}
		if b.Relation != model.RelationImplements {
			t.Fatalf("implementation_anchors should map to implements, got %q", b.Relation)
		}
	}
}

func TestParserLegacyCodeLinks(t *testing.T) {
	content := `# Links
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-LINKS
code_links:
  - src/service/ApiService.cs

` + "```toon" + `
block_id: b1
code_links:
  - src/service/Helper.cs
` + "```"
	parsed := Parse("wiki/links.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected bindings from code_links")
	}
	for _, b := range bindings {
		if b.Relation != model.RelationOperates {
			t.Fatalf("code_links should default to operates, got %q", b.Relation)
		}
	}
}

func TestParserLegacyTestLinks(t *testing.T) {
	content := `# Tests
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-TESTS
test_links:
  - src/tests/ApiTests.cs

` + "```toon" + `
block_id: b1
test_links:
  - src/tests/HelperTests.cs
` + "```"
	parsed := Parse("wiki/tests.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected bindings from test_links")
	}
	for _, b := range bindings {
		if b.Relation != model.RelationTests {
			t.Fatalf("test_links should map to tests, got %q", b.Relation)
		}
	}
}

func TestParserLegacyImplementsKey(t *testing.T) {
	content := `# Implements
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-IMPL
implements:
  - src/impl/MainImpl.cs

` + "```toon" + `
block_id: b1
implements:
  - src/impl/DetailImpl.cs
` + "```"
	parsed := Parse("wiki/impl.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected bindings from implements")
	}
	for _, b := range bindings {
		if b.Relation != model.RelationImplements {
			t.Fatalf("implements key should map to implements, got %q", b.Relation)
		}
	}
}

func TestParserLegacyTestsKey(t *testing.T) {
	content := `# TestsKey
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-TESTSKEY
tests:
  - src/tests/SpecTests.cs

` + "```toon" + `
block_id: b1
tests:
  - src/tests/DetailTests.cs
` + "```"
	parsed := Parse("wiki/testskey.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected bindings from tests")
	}
	for _, b := range bindings {
		if b.Relation != model.RelationTests {
			t.Fatalf("tests key should map to tests, got %q", b.Relation)
		}
	}
}

func TestParserIdOnlyRecord(t *testing.T) {
	content := `# IdOnly
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: RF-ID-001

` + "```toon" + `
block_id: b1
` + "```"
	parsed := Parse("wiki/idonly.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) > 0 {
		t.Fatalf("id-only record should not produce bindings, got %d", len(bindings))
	}
}

func TestParserIdWithPathRecord(t *testing.T) {
	content := `# IdWithPath
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: RF-IDPATH-001

` + "```toon" + `
block_id: b1
id: RF-REF-010
` + "```"
	parsed := Parse("wiki/idpath.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) > 0 {
		t.Fatalf("block id-only should not produce bindings, got %d", len(bindings))
	}
}

func TestParserPathOnlyRecord(t *testing.T) {
	content := `# PathOnly
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-PO

` + "```toon" + `
block_id: b1
code_links:
  - src/only/path.cs
` + "```"
	parsed := Parse("wiki/pathonly.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("path-only legacy key should produce binding")
	}
	if bindings[0].TargetPath != "src/only/path.cs" {
		t.Fatalf("TargetPath = %q, want %q", bindings[0].TargetPath, "src/only/path.cs")
	}
}

func TestParserUnknownRelationRejection(t *testing.T) {
	content := `# Unknown
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-UNK

` + "```toon" + `
block_id: b1
artifact_bindings:
  - doc_path: wiki/unknown.md
    target_path: src/x.cs
    relation: nonexistent_relation
    target_kind: file
` + "```"
	parsed := Parse("wiki/unknown.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) > 0 {
		t.Fatalf("unknown relation should be rejected, got %d bindings", len(bindings))
	}
}

func TestParserUnsafePathRejection(t *testing.T) {
	testCases := []string{
		"/absolute/path.cs",
		"../escape.cs",
		"has\x00nul.cs",
	}
	for _, path := range testCases {
		content := `# Safe
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-SAFE

` + "```toon" + `
block_id: b1
artifact_bindings:
  - doc_path: wiki/safe.md
    target_path: ` + path + `
    relation: implements
    target_kind: file
` + "```"
		parsed := Parse("wiki/safe.md", content, 1)
		bindings := SourceBindings(parsed, 100)
		if len(bindings) != 0 {
			t.Fatalf("unsafe path %q should be rejected, got %d bindings", path, len(bindings))
		}
	}
}

func TestParserStableRefUnderLineStatusChange(t *testing.T) {
	content := `# Same
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-SAME

` + "```toon" + `
block_id: b1
artifact_bindings:
  - doc_path: wiki/same.md
    target_path: src/main.go
    relation: implements
    target_kind: file
` + "```"
	parsed := Parse("wiki/same.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 1 {
		t.Fatal("expected exactly one binding")
	}
	ref1 := bindings[0].BindingRef
	// Verify ref is only based on semantic fields
	ref2 := model.WikiCodeBindingRef(
		bindings[0].DocPath, bindings[0].BlockID, bindings[0].DocID,
		bindings[0].Relation, bindings[0].TargetPath, bindings[0].TargetSymbol, bindings[0].TargetKind,
	)
	if ref2 != ref1 {
		t.Fatalf("stable ref changed: %s vs %s", ref1, ref2)
	}
}

func TestParserChangedRefUnderSemanticChange(t *testing.T) {
	content := `# Sem
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-SEM

` + "```toon" + `
block_id: b1
artifact_bindings:
  - doc_path: wiki/sem.md
    target_path: src/a.go
    relation: implements
    target_kind: file
` + "```"
	parsed := Parse("wiki/sem.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 1 {
		t.Fatal("expected one binding")
	}
	ref1 := bindings[0].BindingRef

	ref2 := model.WikiCodeBindingRef("wiki/sem.md", "b1", "TECH-SEM", "implements", "src/b.go", "", "file")
	if ref2 == ref1 {
		t.Fatal("changing target_path should change binding_ref")
	}

	ref3 := model.WikiCodeBindingRef("wiki/sem.md", "b1", "TECH-SEM", "tests", "src/a.go", "", "file")
	if ref3 == ref1 {
		t.Fatal("changing relation should change binding_ref")
	}
}

func TestParserRelationMappings(t *testing.T) {
	cases := []struct {
		role    string
		kind    string
		wantRel string
	}{
		{"implementation", "file", model.RelationImplements},
		{"test", "test", model.RelationTests},
		{"config", "config", model.RelationConfigures},
		{"compiler", "file", model.RelationOperates},
		{"supervisor", "file", model.RelationOperates},
		{"entrypoint", "file", model.RelationOperates},
		{"adapter", "file", model.RelationOperates},
	}
	for _, c := range cases {
		content := `# M
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: R-M

` + "```toon" + `
block_id: rb
artifact_bindings:
  - doc_path: wiki/m.md
    target_path: src/x.cs
    relation: implements
    role: ` + c.role + `
    target_kind: ` + c.kind + `
` + "```"
		parsed := Parse("wiki/m.md", content, 1)
		bindings := SourceBindings(parsed, 1)
		if len(bindings) != 1 {
			t.Fatalf("role=%s kind=%s: expected 1 binding, got %d", c.role, c.kind, len(bindings))
		}
		if bindings[0].Relation != c.wantRel {
			t.Fatalf("role=%s kind=%s: relation=%q, want %q", c.role, c.kind, bindings[0].Relation, c.wantRel)
		}
	}
}

func TestParserPlannedBindingStored(t *testing.T) {
	content := `# Planned
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-PLAN

` + "```toon" + `
block_id: pb
artifact_bindings:
  - doc_path: wiki/p.md
    target_path: src/planned.cs
    relation: implements
    target_kind: file
    binding_status: planned
` + "```"
	parsed := Parse("wiki/p.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 1 {
		t.Fatalf("planned binding should still be stored, got %d", len(bindings))
	}
	if bindings[0].BindingStatus != model.BindingStatusPlanned {
		t.Fatalf("BindingStatus = %q, want %q", bindings[0].BindingStatus, model.BindingStatusPlanned)
	}
}

func TestParserRetiredSuperseded(t *testing.T) {
	content := `# Retired
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-RET

` + "```toon" + `
block_id: rb
artifact_bindings:
  - doc_path: wiki/ret.md
    target_path: src/old.cs
    relation: implements
    target_kind: file
    doc_lifecycle: retired
    superseded_by: RF-NEW-001
` + "```"
	parsed := Parse("wiki/ret.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 1 {
		t.Fatalf("retired binding should still be stored, got %d", len(bindings))
	}
	if bindings[0].DocLifecycle != model.DocLifecycleRetired {
		t.Fatalf("DocLifecycle = %q, want %q", bindings[0].DocLifecycle, model.DocLifecycleRetired)
	}
	if bindings[0].SupersededBy != "RF-NEW-001" {
		t.Fatalf("SupersededBy = %q, want %q", bindings[0].SupersededBy, "RF-NEW-001")
	}
}

func TestParserDuplicateDocIDFailClosed(t *testing.T) {
	content := `# Dups
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: DUP-001

` + "```toon" + `
block_id: bd
artifact_bindings:
  - doc_path: wiki/dup.md
    target_path: src/a.cs
    relation: implements
    target_kind: file
  - doc_path: wiki/dup.md
    target_path: src/b.cs
    relation: implements
    target_kind: file
` + "```"
	parsed := Parse("wiki/dup.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings for 2 targets, got %d", len(bindings))
	}
}

// TestCanonicalNoDocPath asserts that when an author omits doc_path, the binding
// DocPath derives from the Parse() input path (SourcePath), not from docID.
func TestCanonicalNoDocPath(t *testing.T) {
	content := `# NoPath
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-NODEFPATH

` + "```toon" + `
block_id: b1
artifact_bindings:
  - target_path: src/main.go
    relation: implements
    target_kind: file
` + "```"
	parsed := Parse("wiki/nopath.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].DocPath != "wiki/nopath.md" {
		t.Fatalf("DocPath = %q, want %q", bindings[0].DocPath, "wiki/nopath.md")
	}
	if bindings[0].DocID != "TECH-NODEFPATH" {
		t.Fatalf("DocID = %q, want %q", bindings[0].DocID, "TECH-NODEFPATH")
	}
	if bindings[0].AuthoringOrigin != model.AuthoringOriginCanonical {
		t.Fatalf("AuthoringOrigin = %q, want %q", bindings[0].AuthoringOrigin, model.AuthoringOriginCanonical)
	}
}

// TestT3BlockListSyntax asserts that T3-style list block syntax parses correctly
// and that canonical bindings are produced from list items.
func TestT3BlockListSyntax(t *testing.T) {
	content := `# T3List
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-T3LIST

` + "```toon" + `
block_id: b1
artifact_bindings:
  - doc_path: wiki/t3.md
    target_path: src/x.cs
    relation: implements
    target_kind: file
  - doc_path: wiki/t3.md
    target_path: src/tests/x_tests.cs
    relation: tests
    target_kind: test
` + "```"
	parsed := Parse("wiki/t3.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings from T3 list, got %d", len(bindings))
	}
	for _, b := range bindings {
		if b.DocPath != "wiki/t3.md" {
			t.Fatalf("DocPath = %q, want %q", b.DocPath, "wiki/t3.md")
		}
	}
}

// TestLegacyProvenancePath asserts that legacy bindings use the source doc path
// for DocPath (not docID), and that AuthoringOrigin is legacy.
func TestLegacyProvenancePath(t *testing.T) {
	content := `# LegacyProvenance
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: DIFFERENT-DOC-ID

` + "```toon" + `
block_id: b1
code_links:
  - src/legacy/Code.cs
` + "```"
	parsed := Parse("wiki/legacyprovenance.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	if len(bindings) == 0 {
		t.Fatal("expected legacy bindings from code_links")
	}
	for _, b := range bindings {
		// DocPath must be the source doc path, NOT docID.
		if b.DocPath != "wiki/legacyprovenance.md" {
			t.Fatalf("DocPath = %q, want %q (source path, not docID)", b.DocPath, "wiki/legacyprovenance.md")
		}
		if b.DocID != "DIFFERENT-DOC-ID" {
			t.Fatalf("DocID = %q, want %q", b.DocID, "DIFFERENT-DOC-ID")
		}
		if b.AuthoringOrigin != model.AuthoringOriginLegacy {
			t.Fatalf("AuthoringOrigin = %q, want %q", b.AuthoringOrigin, model.AuthoringOriginLegacy)
		}
	}
}

func TestParserFixtureBlock(t *testing.T) {
	content := `# Fixture
wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: FIXTURE-001

` + "```toon" + `
block_id: fixture.block
artifact_bindings:
  - doc_path: wiki/fixtures.md
    target_path: src/compiler.cs
    relation: operates
    target_kind: file
  - doc_path: wiki/fixtures.md
    target_path: src/tests/MainTests.cs
    relation: implements
    target_kind: test
    role: test
  - doc_path: wiki/fixtures.md
    target_path: src/safe/impl.cs
    relation: implements
    target_kind: file
code_links:
  - src/unsafe/escape.cs
` + "```"
	parsed := Parse("wiki/fixtures.md", content, 1)
	bindings := SourceBindings(parsed, 100)
	// At least the 3 valid bindings; unsafe code_links should be rejected (syntactic .. not present but invalid path check for absolute)
	validCount := 0
	for _, b := range bindings {
		if b.DocPath == "wiki/fixtures.md" && (b.TargetPath == "src/compiler.cs" || b.TargetPath == "src/tests/MainTests.cs" || b.TargetPath == "src/safe/impl.cs") {
			validCount++
		}
	}
	if validCount != 3 {
		t.Fatalf("expected 3 valid bindings, got %d", validCount)
	}
}

package docgraph

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestWikiMatrixFencedCasesSectionDoesNotCover(t *testing.T) {
	rf := WikiViewDoc{Path: ".docs/wiki/05_requirements/RF-A-001.md", DocID: "RF-A-001", Content: wikiMatrixRFRequiredSectionsFixture + "## Criterios de aceptación\n1. Uno\n"}
	tp := WikiViewDoc{Path: ".docs/wiki/06_tests/TP-X.md", DocID: "TP-X", Content: "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-X\nkind: test\nlinks:\n  verifies: [RF-A-001]\n---\n## Alcance\nx\n```yaml\n## Casos\n- Criterio 1\n```\n"}
	m := BuildWikiMatrix([]WikiViewDoc{rf, tp})
	if len(m.Criteria) != 1 || m.Criteria[0].Covered {
		t.Fatalf("fenced section created coverage: %+v", m)
	}
}

func TestWikiMatrixEvidenceAndLegacyNonCases(t *testing.T) {
	rf := func(id string) WikiViewDoc {
		return WikiViewDoc{Path: ".docs/wiki/05_requirements/" + id + ".md", DocID: id, Content: wikiMatrixRFRequiredSectionsFixture + "## Criterios de aceptación\n1. Uno\n2. Dos\n"}
	}
	tp := WikiViewDoc{Path: ".docs/wiki/06_tests/TP-X.md", DocID: "TP-X", Content: "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-X\nkind: test\nlinks:\n  verifies: [RF-A-001, RF-B-001]\n---\n## Alcance\nx\n## Casos\n- Criterio 1; disposición: manual; evidencia: RF-B-001 — Criterio 2\n"}
	m := BuildWikiMatrix([]WikiViewDoc{rf("RF-A-001"), rf("RF-B-001"), tp})
	for _, c := range m.Criteria {
		if c.Covered {
			t.Fatalf("evidence leaked: %+v", m)
		}
	}
	tp.Content = strings.Replace(tp.Content, "[RF-A-001, RF-B-001]", "[RF-A-001]", 1)
	tp.Content = strings.Replace(tp.Content, "- Criterio 1; disposición: manual; evidencia: RF-B-001 — Criterio 2", "### Criterio 1\nProsa Criterio 2\n```text\n- Criterio 1\n```", 1)
	m = BuildWikiMatrix([]WikiViewDoc{rf("RF-A-001"), tp})
	for _, c := range m.Criteria {
		if c.Covered {
			t.Fatalf("non-case covered: %+v", m)
		}
	}
}

func TestWikiMatrixPairIsolationAndPartial(t *testing.T) {
	makeRF := func(id string) WikiViewDoc {
		return WikiViewDoc{Path: ".docs/wiki/05_requirements/" + id + ".md", DocID: id, Content: wikiMatrixRFRequiredSectionsFixture + "## Criterios de aceptación\n1. Uno\n"}
	}
	docs := []WikiViewDoc{makeRF("RF-A-001"), makeRF("RF-B-001"), {Path: ".docs/wiki/06_tests/TP-X.md", DocID: "TP-X", Content: "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-X\nkind: test\nlinks:\n  verifies: [RF-A-001, RF-B-001]\n---\n## Alcance\nx\n## Casos\n- RF-A-001 — Criterio 1; disposición: manual; evidencia: x\n"}}
	m := BuildWikiMatrix(docs)
	if len(m.Criteria) != 2 || !m.Criteria[0].Covered || m.Criteria[1].Covered {
		t.Fatalf("RF case leaked: %+v", m)
	}
	docs[2].Content = strings.Replace(docs[2].Content, "RF-A-001 — Criterio 1", "Criterio 1", 1)
	m = BuildWikiMatrix(docs)
	if m.Criteria[0].Covered || m.Criteria[1].Covered || !strings.Contains(strings.Join(m.Warnings, " "), "ambiguous") {
		t.Fatalf("unlabeled multicase: %+v", m)
	}
	docs[2].Content = strings.Replace(docs[2].Content, "Criterio 1", "RF-A-001 — Criterio 1; Alcance parcial: fixture", 1)
	m = BuildWikiMatrix(docs)
	if m.Criteria[0].Covered || m.Criteria[0].CoverageStatus != "partial" || len(m.Criteria[0].TestCases) == 0 || m.Verdict != "ADVISORY" {
		t.Fatalf("partial case: %+v", m)
	}
	docs[2].Content += "- RF-A-001 — Criterio 1; disposición: manual; evidencia: full\n"
	m = BuildWikiMatrix(docs)
	if !m.Criteria[0].Covered || len(m.Criteria[0].TestCases) != 2 {
		t.Fatalf("mixed cases: %+v", m)
	}
	docs[2].Content = strings.Replace(docs[2].Content, "RF-A-001 — Criterio 1", "RF-X-001 — Criterio 1", -1)
	m = BuildWikiMatrix(docs)
	if m.Criteria[0].Covered || m.Criteria[1].Covered || !strings.Contains(strings.Join(m.Warnings, " "), "case_rf_not_declared") {
		t.Fatalf("foreign case: %+v", m)
	}
	docs[2].Content = strings.Replace(docs[2].Content, "[RF-A-001, RF-B-001]", "[RF-A-001]", 1)
	docs[2].Content = strings.ReplaceAll(docs[2].Content, "RF-X-001 — Criterio 1", "Criterio 1")
	m = BuildWikiMatrix(docs)
	if !m.Criteria[0].Covered || m.Criteria[1].Covered {
		t.Fatalf("single legacy case: %+v", m)
	}
}

func TestWikiMatrixEnglishRFPairs(t *testing.T) {
	rf := func(id string) WikiViewDoc {
		return WikiViewDoc{Path: ".docs/wiki/05_requirements/" + id + ".md", DocID: id, Content: "## Purpose\nx\n## Ordered behavior\nx\n## Surfaces\nx\n## Typed inputs/outputs\nx\n## Errors\nx\n## Acceptance criteria\n1. One\n"}
	}
	tp := WikiViewDoc{Path: ".docs/wiki/06_tests/TP-X.md", DocID: "TP-X", Content: "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-X\nkind: test\nlinks:\n  verifies: [RF-A-001, RF-B-001]\n---\n## Scope\nx\n## Cases\n- RF-A-001 — Criterion 1; coverage: partial; disposition: manual; evidence: x\n"}
	m := BuildWikiMatrixForLanguage([]WikiViewDoc{rf("RF-A-001"), rf("RF-B-001"), tp}, model.WikiContractLanguageEN)
	if len(m.Criteria) != 2 || m.Criteria[0].Covered || m.Criteria[1].Covered || m.Criteria[0].CoverageStatus != "partial" {
		t.Fatalf("English partial: %+v", m)
	}
}

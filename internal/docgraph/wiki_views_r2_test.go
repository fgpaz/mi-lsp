package docgraph

import (
	"strings"
	"testing"
)

func TestBuildWikiMatrixParserWarningVersusGapR2(t *testing.T) {
	rf := WikiViewDoc{DocID: "RF-X-01", Path: ".docs/wiki/05_requirements/RF-X-01.md", Content: "## Criterios de aceptación (numerados)\n1. Uno\n2. Dos\n"}
	tp := WikiViewDoc{DocID: "TP-X-01", Path: ".docs/wiki/06_tests/TP-X-01.md", Content: "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-X-01\nkind: test\nlinks:\n  verifies: [RF-X-01]\n---\n## Casos\n- Criterio 1 — disposición: automated; evidencia: tests/x.go\n"}
	gap := BuildWikiMatrix([]WikiViewDoc{rf, tp})
	if gap.Verdict != "ADVISORY" || len(gap.UncoveredCriteria) != 1 || gap.Criteria[1].CoverageStatus != "gap" {
		t.Fatalf("gap=%+v", gap)
	}
	tp.Content += "- Criterio 2 — disposición: manual; evidencia: captura\n"
	full := BuildWikiMatrix([]WikiViewDoc{rf, tp})
	if full.Verdict != "PASS" || len(full.Warnings) != 0 || !full.Criteria[1].Covered {
		t.Fatalf("full=%+v", full)
	}
	tp.Content = strings.ReplaceAll(tp.Content, "Criterio 2", "Caso sin referencia")
	uncertain := BuildWikiMatrix([]WikiViewDoc{rf, tp})
	if uncertain.Verdict == "PASS" || len(uncertain.UncoveredCriteria) != 0 || uncertain.Criteria[1].CoverageStatus != "parser_warning" || uncertain.Criteria[1].Covered {
		t.Fatalf("uncertain=%+v", uncertain)
	}
	tp.Content = "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-X-01\nkind: test\nlinks:\n  verifies: [RF-X-01]\n---\n## Casos\nSin referencias\n"
	emptyTP := BuildWikiMatrix([]WikiViewDoc{rf, tp})
	if emptyTP.Verdict == "PASS" || len(emptyTP.UncoveredCriteria) != 0 {
		t.Fatalf("emptyTP=%+v", emptyTP)
	}
	rf.Content = "## Criterios de aceptación (numerados)\nSin números\n"
	emptyRF := BuildWikiMatrix([]WikiViewDoc{rf, tp})
	if emptyRF.Verdict == "PASS" || len(emptyRF.Criteria) != 0 || len(emptyRF.Warnings) == 0 {
		t.Fatalf("emptyRF=%+v", emptyRF)
	}
}

package docgraph

import (
	"strings"
	"testing"
)

func TestBuildWikiMatrixHarnessMetadataNeverCovers(t *testing.T) {
	rf := WikiViewDoc{DocID: "RF-AUD-01", Path: ".docs/wiki/05_requirements/RF-AUD-01.md", Content: wikiMatrixRFRequiredSectionsFixture + "## Criterios de aceptación\n1. Uno\n2. Dos\n"}
	actual := "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-AUD-01\nkind: test\nlinks:\n  verifies: [RF-AUD-01]\n---\n## Alcance\nFixture de cobertura.\n## Casos\n- Criterio 1 — disposición: automated; evidencia: actual.go\n"
	valid := "```yaml\nharness_protocol: SDD-HARNESS-v1\nverify:\n  - \"Criterio 2 — disposición: automated; evidencia: metadata_only.go\"\n```\n"
	for name, metadata := range map[string]string{
		"valid":     valid,
		"spaced":    strings.Replace(valid, "```yaml", "``` yaml", 1),
		"tilde":     strings.ReplaceAll(valid, "```", "~~~"),
		"malformed": strings.Replace(valid, "\n```", "\nbroken: [\n```", 1),
		"casefold":  strings.Replace(valid, "SDD-HARNESS-v1", "sdd-harness-v1", 1),
		"unclosed":  strings.TrimSuffix(valid, "```\n"),
	} {
		t.Run(name, func(t *testing.T) {
			tp := WikiViewDoc{DocID: "TP-AUD-01", Path: ".docs/wiki/06_tests/TP-AUD-01.md", Content: actual + metadata}
			matrix := BuildWikiMatrix([]WikiViewDoc{rf, tp})
			if matrix.Verdict == "PASS" || len(matrix.Criteria) != 2 || !matrix.Criteria[0].Covered || matrix.Criteria[1].Covered {
				t.Fatalf("metadata falsely satisfied coverage: %+v", matrix)
			}
			if name != "valid" && name != "spaced" && name != "tilde" && (len(matrix.Warnings) == 0 || matrix.Criteria[1].CoverageStatus != "parser_warning") {
				t.Fatalf("invalid metadata lost uncertainty: %+v", matrix)
			}
		})
	}
}

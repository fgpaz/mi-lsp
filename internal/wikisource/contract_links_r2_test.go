package wikisource

import (
	"strings"
	"testing"
)

func TestContractAcceptanceLiteralNumberedHeadingR2(t *testing.T) {
	for _, heading := range []string{"Criterios de aceptación", "Criterios de aceptación (numerados)", "CRITERIOS DE ACEPTACION (NUMERADOS)", "Criterios de aceptacio\u0301n (numerados)"} {
		t.Run(heading, func(t *testing.T) {
			body := "## " + heading + "\r\n1. Acción válida.\r\n2. Otra.\r\n## Otro\r\n3. No pertenece.\r\n"
			got := ParseAcceptanceCriteria(body)
			if len(got) != 2 || got[1] != "Acción válida." {
				t.Fatalf("criteria: %v", got)
			}
			for _, warning := range ContractBodySectionWarnings("RF-X", "requirement", body) {
				if strings.Contains(warning, "aceptación") || strings.Contains(warning, "unnumbered") {
					t.Fatal(warning)
				}
			}
		})
	}
	if got := ParseAcceptanceCriteria("## Criterios de aceptación (numerados)\nSin números"); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestContractTestCasesR2(t *testing.T) {
	body := "## Casos\n- Criterio 1 — disposición: automated; evidencia: test.go\n2. Criterio 2 — disposición: manual; evidencia: captura\n### Criterio 3\n- Sin referencia\n## Otro\n- Criterio 4\n"
	cases, warnings := ParseTestPlanCasesWithWarnings(body)
	if len(cases) != 3 || cases[1].Criterion != 2 || cases[0].Evidence != "test.go" || len(warnings) != 1 {
		t.Fatalf("cases=%+v warnings=%v", cases, warnings)
	}
}

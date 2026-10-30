package model

import "testing"

func TestWikiContractCasesExcludeHarnessMetadata(t *testing.T) {
	actual := "## Casos\n- Criterio 1 — disposición: automated; evidencia: real_test.go\n"
	for _, key := range []string{"harness_protocol", "'harness_protocol'"} {
		metadata := "```yaml\n" + key + ": SDD-HARNESS-v1\nverify:\n  - \"Criterio 2 — disposición: automated; evidencia: metadata_only.go\"\n  - A metadata list is not a test case.\n## Casos\n```\n"
		cases, findings := ParseWikiContractTestCases(actual+metadata, WikiContractLanguageES)
		if len(cases) != 1 || cases[0].Number != 1 || len(findings) != 0 {
			t.Fatalf("metadata changed actual cases: cases=%+v findings=%+v", cases, findings)
		}
		cases, findings = ParseWikiContractTestCases("## Casos\n"+metadata, WikiContractLanguageES)
		if len(cases) != 0 || len(findings) != 1 || findings[0].Code != "no_parseable_cases" {
			t.Fatalf("metadata falsely supplied coverage: cases=%+v findings=%+v", cases, findings)
		}
		cases, findings = ParseWikiContractTestCases(metadata, WikiContractLanguageES)
		if len(cases) != 0 || len(findings) != 1 || findings[0].Code != "section_missing" {
			t.Fatalf("metadata supplied a section: cases=%+v findings=%+v", cases, findings)
		}
	}
}

func TestWikiContractCasesStillWarnOnOrdinaryContent(t *testing.T) {
	body := "## Casos\n- Criterio 1 — disposición: automated; evidencia: real_test.go\n- Caso sin referencia\n```yaml\nother_protocol: example\ncase: Criterio 2 — disposición: manual; evidencia: fixture\n```\n- Criterio 3 — disposición: automated; evidencia: after.go\n"
	cases, findings := ParseWikiContractTestCases(body, WikiContractLanguageES)
	for _, finding := range findings {
		if finding.Code == "invalid_harness_metadata" {
			t.Fatalf("ordinary YAML incorrectly marked invalid Harness: %+v", findings)
		}
	}
	if len(cases) != 2 || cases[0].Number != 1 || cases[1].Number != 3 || len(findings) != 1 || findings[0].Code != "case_without_criterion" {
		t.Fatalf("ordinary fence contaminated cases or lost outside warning: cases=%+v findings=%+v", cases, findings)
	}
}

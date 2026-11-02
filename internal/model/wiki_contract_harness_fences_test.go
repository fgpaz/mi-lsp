package model

import "testing"

func TestWikiContractCasesHarnessFenceDialects(t *testing.T) {
	for _, fence := range []struct{ open, close string }{{"```yaml", "```"}, {"``` yaml", "```"}, {"~~~yaml", "~~~"}, {"~~~~ YML", "~~~~"}, {"````yaml", "````"}, {"  ~~~ yaml", "  ~~~"}} {
		body := "## Casos\n- Criterio 1 — disposición: automated; evidencia: actual.go\n" + fence.open + "\nharness_protocol: SDD-HARNESS-v1\nverify:\n  - \"Criterio 2 — disposición: automated; evidencia: metadata.go\"\n" + fence.close + "\n- Criterio 3 — disposición: automated; evidencia: after.go\n"
		cases, findings := ParseWikiContractTestCases(body, WikiContractLanguageES)
		if len(cases) != 2 || cases[0].Number != 1 || cases[1].Number != 3 || len(findings) != 0 {
			t.Fatalf("fence %q contaminated coverage: cases=%+v findings=%+v", fence.open, cases, findings)
		}
	}
}

func TestWikiContractCasesHarnessMentionIsNotMetadata(t *testing.T) {
	body := "## Casos\n- Criterio 1 — disposición: automated; evidencia: actual.go\n```yaml\n# harness_protocol: is only mentioned here\nnote: 'harness_protocol: is an ordinary value'\ndescription: |\n  harness_protocol: inside a literal value\ncase: 'Criterio 2 — disposición: automated; evidencia: ordinary.go'\n```\n- Criterio 3 — disposición: automated; evidencia: after.go\n"
	cases, findings := ParseWikiContractTestCases(body, WikiContractLanguageES)
	for _, finding := range findings {
		if finding.Code == "invalid_harness_metadata" {
			t.Fatalf("ordinary YAML incorrectly marked invalid Harness: %+v", findings)
		}
	}
	if len(cases) != 2 || cases[0].Number != 1 || cases[1].Number != 3 || len(findings) != 0 {
		t.Fatalf("ordinary YAML contaminated cases or hid outside content: cases=%+v findings=%+v", cases, findings)
	}
}

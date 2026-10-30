package model

import "testing"

func TestWikiContractCasesFencedSectionSelection(t *testing.T) {
	for _, tc := range []struct {
		lang               WikiContractLanguage
		heading, criterion string
	}{
		{WikiContractLanguageES, "Casos", "Criterio"}, {WikiContractLanguageEN, "Cases", "Criterion"},
	} {
		t.Run(string(tc.lang), func(t *testing.T) {
			fake := "```yaml\n## " + tc.heading + "\n- " + tc.criterion + " 1\n```\n"
			cases, findings := ParseWikiContractTestCases(fake, tc.lang)
			if len(cases) != 0 || len(findings) == 0 || findings[0].Code != "section_missing" {
				t.Fatalf("fake section: cases=%+v findings=%+v", cases, findings)
			}
			invalid := "```yaml\nharness_protocol: [broken\n```\n" + fake
			_, invalidFindings := ParseWikiContractTestCases(invalid, tc.lang)
			if len(invalidFindings) < 2 || invalidFindings[0].Code != "invalid_harness_metadata" || invalidFindings[1].Code != "section_missing" {
				t.Fatalf("lost metadata uncertainty: %+v", invalidFindings)
			}
			cases, _ = ParseWikiContractTestCases(fake+"## "+tc.heading+"\n- "+tc.criterion+" 2\n", tc.lang)
			if len(cases) != 1 || cases[0].Number != 2 {
				t.Fatalf("fake before real: %+v", cases)
			}
			cases, _ = ParseWikiContractTestCases("## "+tc.heading+"\n```yaml\n## Other\n- "+tc.criterion+" 1\n```\n- "+tc.criterion+" 3\n", tc.lang)
			if len(cases) != 1 || cases[0].Number != 3 {
				t.Fatalf("fake section end: %+v", cases)
			}
		})
	}
}

func TestWikiContractCaseEvidenceAndFenceIsolation(t *testing.T) {
	for _, tc := range []struct {
		lang                                      WikiContractLanguage
		heading, criterion, disposition, evidence string
	}{
		{WikiContractLanguageES, "Casos", "Criterio", "disposición", "evidencia"},
		{WikiContractLanguageEN, "Cases", "Criterion", "disposition", "evidence"},
	} {
		body := "## " + tc.heading + "\n### " + tc.criterion + " 9\nLa prosa " + tc.criterion + " 8 no es caso.\n```text\n- " + tc.criterion + " 7\n```\n- " + tc.criterion + " 1; " + tc.disposition + ": manual; " + tc.evidence + ": `RF-B-001 — " + tc.criterion + " 2`\n- RF-A-001 — " + tc.criterion + " 3; " + tc.evidence + ": RF-B-001 — " + tc.criterion + " 4\n- RF-A-001 — " + tc.criterion + " 5; " + tc.criterion + " 6; " + tc.evidence + ": fixture\n"
		cases, findings := ParseWikiContractTestCases(body, tc.lang)
		if len(cases) != 3 || cases[0].RFID != "" || cases[0].Number != 1 || cases[1].RFID != "RF-A-001" || cases[1].Number != 3 || cases[2].Number != 5 {
			t.Fatalf("%s contamination: %+v warnings=%+v", tc.lang, cases, findings)
		}
		found := false
		for _, f := range findings {
			if f.Code == "unpaired_case_criterion" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s missing unpaired warning: %+v", tc.lang, findings)
		}
	}
}

func TestWikiContractCaseRFPairsAndPartial(t *testing.T) {
	for _, tc := range []struct {
		lang WikiContractLanguage
		body string
	}{
		{WikiContractLanguageES, "## Casos\n- RF-A-001 — Criterio 1; RF-B-001 — Criterio 2; Alcance parcial: parser\n"},
		{WikiContractLanguageEN, "## Cases\n- RF-A-001 — Criterion 1; RF-B-001 — Criterion 2; Coverage: partial\n"},
	} {
		cases, _ := ParseWikiContractTestCases(tc.body, tc.lang)
		if len(cases) != 2 || cases[0].RFID != "RF-A-001" || cases[1].RFID != "RF-B-001" || cases[1].Number != 2 || !cases[0].Partial || !cases[1].Partial {
			t.Fatalf("pairs: %+v", cases)
		}
	}
	body := "## Casos\n### RF-A-001 — Criterio 1\n- RF-B-001 — Criterio 2; evidencia: x\n```yaml\nharness_protocol: SDD-HARNESS-v1\nverify:\n  - RF-A-001 — Criterio 3\n```\n"
	cases, _ := ParseWikiContractTestCases(body, WikiContractLanguageES)
	if len(cases) != 1 || cases[0].RFID != "RF-B-001" || cases[0].Number != 2 {
		t.Fatalf("heading or metadata became case: %+v", cases)
	}
	cases, findings := ParseWikiContractTestCases("## Casos\n- RF-OTHER Criterio 1; evidencia: x\n", WikiContractLanguageES)
	if len(cases) != 0 || len(findings) == 0 || findings[0].Code != "unpaired_case_rf" {
		t.Fatalf("unpaired RF inferred as legacy: cases=%+v findings=%+v", cases, findings)
	}
}

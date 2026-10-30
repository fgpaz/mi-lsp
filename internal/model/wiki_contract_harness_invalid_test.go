package model

import (
	"fmt"
	"strings"
	"testing"
)

func TestWikiContractCasesInvalidHarnessCannotCover(t *testing.T) {
	for _, lang := range []struct {
		language                                  WikiContractLanguage
		heading, criterion, disposition, evidence string
	}{
		{WikiContractLanguageES, "Casos", "Criterio", "disposición", "evidencia"},
		{WikiContractLanguageEN, "Cases", "Criterion", "disposition", "evidence"},
	} {
		line := func(n int) string {
			return fmt.Sprintf("%s %d — %s: automated; %s: fixture.go", lang.criterion, n, lang.disposition, lang.evidence)
		}
		actual := "## " + lang.heading + "\n- " + line(1) + "\n"
		valid := "```yaml\nharness_protocol: SDD-HARNESS-v1\nverify:\n  - \"" + line(2) + "\"\n```\n"
		variants := map[string]string{
			"malformed":     strings.Replace(valid, "\n```", "\nbroken: [\n```", 1),
			"casefold":      strings.Replace(valid, "SDD-HARNESS-v1", "sdd-harness-v1", 1),
			"unsupported":   strings.Replace(valid, "SDD-HARNESS-v1", "SDD-HARNESS-v2", 1),
			"unclosed":      strings.TrimSuffix(valid, "```\n"),
			"missing_colon": strings.Replace(valid, "harness_protocol:", "harness_protocol", 1),
		}
		for name, metadata := range variants {
			t.Run(string(lang.language)+"/"+name, func(t *testing.T) {
				cases, findings := ParseWikiContractTestCases(actual+metadata, lang.language)
				if len(cases) != 1 || cases[0].Number != 1 || len(findings) != 1 || findings[0].Code != "invalid_harness_metadata" {
					t.Fatalf("invalid metadata supplied cases or lost uncertainty: cases=%+v findings=%+v", cases, findings)
				}
			})
		}
		cases, findings := ParseWikiContractTestCases(actual+valid+"- "+line(3)+"\n", lang.language)
		if len(cases) != 2 || cases[0].Number != 1 || cases[1].Number != 3 || len(findings) != 0 {
			t.Fatalf("real post-fence case lost: cases=%+v findings=%+v", cases, findings)
		}
	}
}

package docgraph

import "testing"

func TestNormalizeWikiKindAliases(t *testing.T) {
	cases := map[string]string{
		"RF": "requirement", "tp": "test", "FL": "flow", "RS": "outcome",
		"test-plan": "test", "requisitos": "requirement",
		"module_map": "ux_module", "uxm": "ux_module", "ux_module": "ux_module",
		"uj": "journey", "journey": "journey",
	}
	for input, want := range cases {
		got, ok := NormalizeWikiKind(input)
		if !ok || got != want {
			t.Fatalf("NormalizeWikiKind(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
}

func TestWikiContractKindForPathLegacyFolders(t *testing.T) {
	if kind := WikiContractKindForPath(".docs/wiki/04_RF/RF-X.md", ""); kind != WikiKindRequirement {
		t.Fatalf("expected requirement, got %q", kind)
	}
	if kind := WikiContractKindForPath(".docs/wiki/06_pruebas/TP-X.md", ""); kind != WikiKindTest {
		t.Fatalf("expected test, got %q", kind)
	}
	if kind := WikiContractKindForPath(".docs/wiki/05_requirements/RF-X.md", ""); kind != WikiKindRequirement {
		t.Fatalf("expected requirement for v1 path, got %q", kind)
	}
}

func TestLegacyPathRenameWarning(t *testing.T) {
	warn := LegacyPathRenameWarning(".docs/wiki/00_gobierno_documental.md")
	if warn == "" || !contains(warn, "00_governance.md") {
		t.Fatalf("expected governance rename warning, got %q", warn)
	}
	warn = LegacyPathRenameWarning(".docs/wiki/06_matriz_pruebas_RF.md")
	if warn == "" || !contains(warn, "nav wiki matrix") {
		t.Fatalf("expected matrix deprecation warning, got %q", warn)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

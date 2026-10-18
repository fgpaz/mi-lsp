package docgraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikiLayersDoctorUsesGovernanceExceptionsOverPolicy(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "product")
	policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
	policyBody, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	policyText := string(policyBody) + "\nexceptions:\n  - layer: flow\n    reason: legacy\n    authorized_by: test\n    date: 2026-10-18\n"
	if err := os.WriteFile(policyPath, []byte(policyText), 0o644); err != nil {
		t.Fatal(err)
	}
	writeGovernanceYAML(t, root, canonGovernanceV1RelPath, `wiki_contract: WIKI-CONTRACT-v1
version: 1
layer_exceptions:
  - layer: requirement
    reason: deferred
    authorized_by: Gabriel
    date: 2026-10-18
`)
	writeWikiFile(t, root, ".docs/wiki/01_alcance_funcional.md", "# Alcance\n\nContenido.\n")

	report, warnings := wikiLayersDoctor(root)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.Exceptions) != 1 || report.Exceptions[0].Layer != "requirement" {
		t.Fatalf("expected governance exception on requirement, got %v", report.Exceptions)
	}
	if !containsWarningPrefix(warnings, wikiExceptionsLegacyLocation) {
		t.Fatalf("expected legacy location warning, got %v", warnings)
	}
}

func TestWikiLayersDoctorLegacyPolicyExceptionsAlias(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "product")
	policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
	policyBody, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	policyText := string(policyBody) + "\nexceptions:\n  - layer: flow\n    reason: legacy only\n    authorized_by: test\n    date: 2026-10-18\n"
	if err := os.WriteFile(policyPath, []byte(policyText), 0o644); err != nil {
		t.Fatal(err)
	}
	writeWikiFile(t, root, ".docs/wiki/01_alcance_funcional.md", "# Alcance\n\nContenido.\n")

	report, warnings := wikiLayersDoctor(root)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.Exceptions) != 1 || report.Exceptions[0].Layer != "flow" {
		t.Fatalf("expected policy exception alias, got %v", report.Exceptions)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, wikiExceptionsLegacyLocation) {
			t.Fatalf("should not warn legacy location when governance has no exceptions: %v", warnings)
		}
	}
}

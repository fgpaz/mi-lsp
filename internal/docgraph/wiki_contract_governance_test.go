package docgraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGovernanceYAML(t *testing.T, root, rel, body string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Governance\n\n```yaml\n" + body + "\n```\n"
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverGovernanceRelPathPrefersV1(t *testing.T) {
	root := t.TempDir()
	writeGovernanceYAML(t, root, canonGovernanceV1RelPath, "wiki_contract: WIKI-CONTRACT-v1\nversion: 1\n")
	rel, warnings := discoverGovernanceRelPath(root)
	if rel != canonGovernanceV1RelPath {
		t.Fatalf("expected %s, got %s", canonGovernanceV1RelPath, rel)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestDiscoverGovernanceRelPathLegacyWarning(t *testing.T) {
	root := t.TempDir()
	writeGovernanceYAML(t, root, legacyGovernanceRelPath, "wiki_contract: WIKI-CONTRACT-v1\nversion: 1\n")
	rel, warnings := discoverGovernanceRelPath(root)
	if rel != legacyGovernanceRelPath {
		t.Fatalf("expected legacy path, got %s", rel)
	}
	if !containsWarning(warnings, legacyGovernanceFilenameWarning) {
		t.Fatalf("expected legacy filename warning, got %v", warnings)
	}
}

func TestParseWikiContractLayerExceptionsYAMLShape(t *testing.T) {
	block := `wiki_contract: WIKI-CONTRACT-v1
layer_exceptions:
  - layer: outcome
    reason: internal
    authorized_by: Gabriel
    date: 2026-10-18
`
	parsed, warnings := ParseWikiContractGovernanceYAML(block)
	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}
	if parsed == nil || len(parsed.LayerExceptions) != 1 {
		t.Fatalf("parsed=%v exceptions=%v", parsed, parsed.LayerExceptions)
	}
}

func TestParseWikiContractGovernanceTraitsAndExceptions(t *testing.T) {
	block := `wiki_contract: WIKI-CONTRACT-v1
version: 1
traits:
  stores_data: "SQLite"
  has_ui:
    reason: "Panel"
    ui_roots: [ui/]
  bogus_trait: "x"
layer_exceptions:
  - layer: outcome
    reason: "internal lib"
    authorized_by: Gabriel
    date: 2026-10-18
`
	parsed, warnings := ParseWikiContractGovernanceYAML(block)
	if parsed == nil {
		t.Fatal("expected parsed governance")
	}
	if !parsed.TraitEnabled("stores_data") || !parsed.TraitEnabled("has_ui") {
		t.Fatal("expected traits enabled")
	}
	if len(parsed.AggregatedUIRoots()) != 1 || parsed.AggregatedUIRoots()[0] != "ui/" {
		t.Fatalf("unexpected ui_roots: %v", parsed.AggregatedUIRoots())
	}
	if len(parsed.LayerExceptions) != 1 || parsed.LayerExceptions[0].Layer != "outcome" {
		t.Fatalf("unexpected exceptions: %v", parsed.LayerExceptions)
	}
	foundUnknown := false
	for _, warning := range warnings {
		if strings.Contains(warning, "bogus_trait") {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatalf("expected unknown trait warning, got %v", warnings)
	}
}

func TestAttachWikiContractGovernanceOnStatus(t *testing.T) {
	root := t.TempDir()
	writeGovernanceYAML(t, root, legacyGovernanceRelPath, `wiki_contract: WIKI-CONTRACT-v1
version: 1
traits:
  exposes_api: "CLI"
`)
	status := InspectGovernance(root, false)
	if status.WikiContract != WikiContractProtocolV1 {
		t.Fatalf("expected wiki contract on status, got %q", status.WikiContract)
	}
	if len(status.Traits) == 0 {
		t.Fatal("expected traits on governance status")
	}
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if warning == needle {
			return true
		}
	}
	return false
}

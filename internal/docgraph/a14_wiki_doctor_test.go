package docgraph

import "testing"

func TestA14DoctorEffectiveGovernanceRootAndLayers(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "product")
	writeWikiFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n[governance]\nsource_doc = 'wiki/00_governance.md'\n")
	writeWikiFile(t, root, "wiki/00_governance.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: es\n---\n")
	for _, path := range []string{"wiki/01_scope.md", "wiki/02_outcomes/RS-001.md", "wiki/03_architecture.md", "wiki/04_flows/FL-001.md", "wiki/05_requirements/RF-001.md", "wiki/06_tests/TP-001.md", "wiki/07_technical/TECH-001.md"} {
		writeWikiFile(t, root, path, "# Fixture\n\nContenido.\n")
	}
	status := InspectGovernance(root, false)
	report := status.WikiLayers
	if report == nil || report.WikiRoot != "wiki" || !report.LayersRequired || len(report.Missing) != 0 || containsWarningPrefix(status.Warnings, "wiki_layer_gap:") {
		t.Fatalf("doctor must report effective root and actual layers: report=%+v warnings=%v", report, status.Warnings)
	}
}

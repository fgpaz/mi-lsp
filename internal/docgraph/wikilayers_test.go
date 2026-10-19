package docgraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRepoPolicy(t *testing.T, root string, kind string) {
	t.Helper()
	dir := filepath.Join(root, ".docs", "ae")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "schema: ae-repo-policy/v1\nversion: 1\nrepo:\n  name: fixture\n"
	if kind != "" {
		body += "  kind: " + kind + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "repo-policy.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeWikiFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWikiLayersDoctorProductGaps(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "product")
	writeWikiFile(t, root, ".docs/wiki/01_alcance_funcional.md", "# Alcance\n\nContenido real.\n")
	writeWikiFile(t, root, ".docs/wiki/02_resultados/RS-001.md", "# RS\n\nResultado.\n")
	writeWikiFile(t, root, ".docs/wiki/03_arquitectura.md", "# Arquitectura\n\nDetalle.\n")

	report, warnings := wikiLayersDoctor(root)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.Missing) != 4 {
		t.Fatalf("expected 4 missing layers, got %v", report.Missing)
	}
	if !containsWarningPrefix(warnings, "wiki_layer_gap:") {
		t.Fatalf("expected wiki_layer_gap warnings, got %v", warnings)
	}
}

func TestWikiLayersDoctorBrowserProductFixtureNoGaps(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "product")
	for _, path := range []string{
		".docs/wiki/01_scope.md",
		".docs/wiki/02_outcomes/RS-001.md",
		".docs/wiki/03_architecture.md",
		".docs/wiki/04_flows/FL-001.md",
		".docs/wiki/05_requirements/RF-001.md",
		".docs/wiki/06_tests/TP-001.md",
		".docs/wiki/07_tech/TECH-001.md",
	} {
		writeWikiFile(t, root, path, "# Fixture\n\nContenido real.\n")
	}
	report, warnings := wikiLayersDoctor(root)
	if report == nil || len(report.Missing) != 0 {
		t.Fatalf("browser-like product fixture has missing layers: %+v", report)
	}
	if containsWarningPrefix(warnings, "wiki_layer_gap:") {
		t.Fatalf("unexpected layer warning for complete browser fixture: %v", warnings)
	}
}

func TestWikiLayersDoctorDocsRepoNoGaps(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "docs")
	report, warnings := wikiLayersDoctor(root)
	if report == nil {
		t.Fatal("expected report")
	}
	if report.LayersRequired {
		t.Fatal("docs repo should not require product layers")
	}
	if len(report.Missing) != 0 {
		t.Fatalf("unexpected missing: %v", report.Missing)
	}
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "wiki_layer_gap:") {
			t.Fatalf("unexpected gap warning: %s", warning)
		}
	}
}

func TestWikiLayersDoctorKernelNoGaps(t *testing.T) {
	root := t.TempDir()
	writeRepoPolicy(t, root, "kernel")
	report, warnings := wikiLayersDoctor(root)
	if len(report.Missing) != 0 {
		t.Fatalf("kernel should not report missing product layers: %v", report.Missing)
	}
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "wiki_layer_gap:") {
			t.Fatalf("unexpected gap warning: %s", warning)
		}
	}
}

func containsWarningPrefix(warnings []string, prefix string) bool {
	for _, warning := range warnings {
		if strings.HasPrefix(warning, prefix) {
			return true
		}
	}
	return false
}

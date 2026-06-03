package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
)

func writeSpecBackendGovernanceFixture(t *testing.T, root string) {
	t.Helper()
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", strings.Join([]string{
		"# 00. Gobierno documental",
		"",
		"## Governance Source",
		"",
		"```yaml",
		"version: 1",
		"profile: spec_backend",
		"overlays:",
		"  - spec_core",
		"  - technical",
		"numbering_recommended: true",
		"hierarchy:",
		"  - id: governance",
		"    label: Gobierno documental",
		"    layer: \"00\"",
		"    family: functional",
		"    pack_stage: governance",
		"    paths:",
		"      - .docs/wiki/00_gobierno_documental.md",
		"  - id: scope",
		"    label: Alcance",
		"    layer: \"01\"",
		"    family: functional",
		"    pack_stage: scope",
		"    paths:",
		"      - .docs/wiki/01_*.md",
		"  - id: architecture",
		"    label: Arquitectura",
		"    layer: \"02\"",
		"    family: functional",
		"    pack_stage: architecture",
		"    paths:",
		"      - .docs/wiki/02_*.md",
		"  - id: flow",
		"    label: Flujos",
		"    layer: \"03\"",
		"    family: functional",
		"    pack_stage: flow",
		"    paths:",
		"      - .docs/wiki/03_FL.md",
		"      - .docs/wiki/03_FL/*.md",
		"  - id: requirements",
		"    label: Requerimientos",
		"    layer: \"04\"",
		"    family: functional",
		"    pack_stage: requirements",
		"    paths:",
		"      - .docs/wiki/04_RF.md",
		"      - .docs/wiki/04_RF/*.md",
		"  - id: data",
		"    label: Modelo de datos",
		"    layer: \"05\"",
		"    family: functional",
		"    pack_stage: data",
		"    paths:",
		"      - .docs/wiki/05_*.md",
		"  - id: tests",
		"    label: Pruebas",
		"    layer: \"06\"",
		"    family: functional",
		"    pack_stage: tests",
		"    paths:",
		"      - .docs/wiki/06_*.md",
		"      - .docs/wiki/06_pruebas/*.md",
		"  - id: technical_baseline",
		"    label: Baseline tecnica",
		"    layer: \"07\"",
		"    family: technical",
		"    pack_stage: technical_baseline",
		"    paths:",
		"      - .docs/wiki/07_*.md",
		"      - .docs/wiki/07_tech/*.md",
		"  - id: physical_data",
		"    label: Modelo fisico",
		"    layer: \"08\"",
		"    family: technical",
		"    pack_stage: physical_data",
		"    paths:",
		"      - .docs/wiki/08_*.md",
		"      - .docs/wiki/08_db/*.md",
		"  - id: contracts",
		"    label: Contratos tecnicos",
		"    layer: \"09\"",
		"    family: technical",
		"    pack_stage: contracts",
		"    paths:",
		"      - .docs/wiki/09_*.md",
		"      - .docs/wiki/09_contratos/*.md",
		"context_chain:",
		"  - governance",
		"  - scope",
		"  - architecture",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"closure_chain:",
		"  - governance",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - contracts",
		"  - tests",
		"audit_chain:",
		"  - governance",
		"  - flow",
		"  - requirements",
		"  - technical_baseline",
		"  - physical_data",
		"  - contracts",
		"  - tests",
		"blocking_rules:",
		"  - missing_human_governance_doc",
		"  - missing_governance_yaml",
		"  - invalid_governance_schema",
		"  - projection_out_of_sync",
		"projection:",
		"  output: .docs/wiki/_mi-lsp/read-model.toml",
		"  format: toml",
		"  auto_sync: true",
		"  versioned: true",
		"```",
	}, "\n"))

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("expected governance fixture to be valid, got blocked status: %#v", status)
	}
}

func writeSpecBackendGovernanceFixtureWithAE(t *testing.T, root string, aeRoot string) {
	t.Helper()
	writeSpecBackendGovernanceFixture(t, root)
	addAEDeclarationToGovernanceFixture(t, root, aeRoot)

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("expected governance fixture with AE to be valid, got blocked status: %#v", status)
	}
}

func addAEDeclarationToGovernanceFixture(t *testing.T, root string, aeRoot string) {
	t.Helper()
	path := filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read governance fixture: %v", err)
	}
	aeHierarchy := strings.Join([]string{
		"  - id: agent_engineering",
		"    label: Agent Engineering",
		"    layer: AE",
		"    family: technical",
		"    pack_stage: technical_detail",
		"    paths:",
		"      - " + aeRoot + "/*.md",
	}, "\n")
	updated := strings.Replace(string(content), "context_chain:", aeHierarchy+"\ncontext_chain:", 1)
	updated = strings.Replace(updated, "  - contracts\nclosure_chain:", "  - contracts\n  - agent_engineering\nclosure_chain:", 1)
	updated = strings.Replace(updated, "  - contracts\n  - tests\naudit_chain:", "  - contracts\n  - agent_engineering\n  - tests\naudit_chain:", 1)
	updated = strings.Replace(updated, "  - contracts\n  - tests\nblocking_rules:", "  - contracts\n  - agent_engineering\n  - tests\nblocking_rules:", 1)
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", updated)
}

func writeAECanonModules(t *testing.T, root string, aeRoot string) {
	t.Helper()
	for _, name := range []string{
		"README.md",
		"AE-PHASES.md",
		"AE-HARNESS-MANIFEST.md",
		"AE-HARNESS-ORCHESTRATION.md",
		"AE-WORK-MODES.md",
		"AE-SESSION-CONTRACT.md",
		"AE-PROJECTION-POLICY.md",
		"AE-EVIDENCE-POLICY.md",
		"AE-RELEASE-DISTRIBUTION.md",
	} {
		writeWorkspaceFile(t, root, aeRoot+"/"+name, "# "+strings.TrimSuffix(name, ".md")+"\n")
	}
}

package service

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestA14GovernanceFrontmatterLanguage(t *testing.T) {
	root := t.TempDir()
	writeContractTestWikiFile(t, root, ".docs/wiki/00_governance.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nlanguage: es\n---\n")
	got := ResolveWikiContractLanguageAtRoot(root)
	if !got.Declared || !got.Valid || got.Language != model.WikiContractLanguageES {
		t.Fatalf("frontmatter language must be declared and valid: %+v", got)
	}
}

func TestA14RetirementsEntries(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/00_retirements.md"
	archived := ".docs/wiki/_retired/requirement/RF-OLD-01.md"
	successor := ".docs/wiki/07_technical.md"
	writeContractTestWikiFile(t, root, archived, "# Archivo\n")
	writeContractTestWikiFile(t, root, successor, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TECH-X-01\nkind: technical\nstatus: active\n---\n## Resumen\nx\n## Mapa de componentes\nx\n")
	row := "```yaml\nentries:\n  - id: RF-OLD-01\n    old_path: .docs/wiki/05_requirements/RF-OLD-01.md\n    archived_path: " + archived + "\n    superseded_by: TECH-X-01\n    terminal: false\n    reason: Reemplazado\n```\n"
	for _, tc := range []struct {
		name, body string
		pass       bool
	}{
		{"valid", row, true},
		{"missing_successor", strings.Replace(row, "TECH-X-01", "TECH-MISSING", 1), false},
		{"incomplete", "```yaml\nentries:\n  - id: RF-OLD-01\n```\n", false},
		{"invalid_yaml", "```yaml\nentries: [\n```\n", false},
		{"empty", "", false},
		{"explicit_empty_entries", "```yaml\nentries: []\n```\n", true},
		{"ambiguous", "No hay novedades.\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeContractTestWikiFile(t, root, path, tc.body)
			got := compileWikiContractValidation(root, []model.DocRecord{{Path: path}, {Path: successor}}, nil)
			if (got.ContractVerdict == "PASS") != tc.pass || got.ContractsReviewed == 0 {
				t.Fatalf("unexpected verdict: %+v", got)
			}
		})
	}
}

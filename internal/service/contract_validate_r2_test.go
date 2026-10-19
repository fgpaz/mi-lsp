package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestCompileWikiContractRetirementsR2(t *testing.T) {
	root := t.TempDir()
	retirementPath := ".docs/wiki/00_retirements.md"
	archived := ".docs/wiki/_retired/requirement/RF-OLD-01.md"
	successorPath := ".docs/wiki/07_technical.md"
	writeContractTestWikiFile(t, root, archived, "# Archivo histórico\n")
	writeContractTestWikiFile(t, root, successorPath, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TECH-X-01\nkind: technical\nstatus: active\n---\n## Resumen\nx\n## Mapa de componentes\nx\n")
	base := "```yaml\nretirements:\n  - id: RF-OLD-01\n    old_path: .docs/wiki/04_RF/RF-OLD-01.md\n    archived_path: " + archived + "\n    superseded_by: %s\n    terminal: %s\n    reason: Reemplazado\n%s```\n"
	for _, tc := range []struct{ name, successor, terminal, extra, want string }{
		{name: "terminal", successor: "null", terminal: "true"},
		{name: "terminal_without_successor_key", successor: "null", terminal: "true"},
		{name: "successor", successor: "TECH-X-01", terminal: "false"},
		{name: "terminal_with_successor", successor: "TECH-X-01", terminal: "true"},
		{name: "unknown_successor", successor: "TECH-MISSING", terminal: "true", want: "superseded_by target missing"},
		{name: "nonterminal_no_successor", successor: "null", terminal: "false", want: "requires superseded_by"},
		{name: "string_boolean", successor: "null", terminal: "\"true\"", want: "terminal must be boolean"},
		{name: "legacy_fields", successor: "null", terminal: "true", extra: "    block_id: old\n    source_of_truth: old\n    verify: old\n    evidence: old\n", want: "forbidden key block_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(base, tc.successor, tc.terminal, tc.extra)
			if tc.name == "terminal_without_successor_key" {
				body = strings.Replace(body, "    superseded_by: null\n", "", 1)
			}
			writeContractTestWikiFile(t, root, retirementPath, body)
			result := compileWikiContractValidation(root, []model.DocRecord{{Path: retirementPath}, {Path: successorPath}}, nil)
			warnings := strings.Join(result.ContractWarnings, "\n")
			if tc.want == "" {
				if result.ContractVerdict != "PASS" || warnings != "" {
					t.Fatalf("%+v", result)
				}
			} else if !strings.Contains(warnings, tc.want) || result.ContractVerdict != "ADVISORY" {
				t.Fatalf("want %s got %+v", tc.want, result)
			}
			if strings.Contains(warnings, "missing WIKI-CONTRACT") || strings.Contains(warnings, "missing verify") {
				t.Fatal(warnings)
			}
			if tc.name == "legacy_fields" {
				for _, key := range []string{"block_id", "source_of_truth", "verify", "evidence"} {
					if !strings.Contains(warnings, "forbidden key "+key) {
						t.Fatal(warnings)
					}
				}
			}
		})
	}
	writeContractTestWikiFile(t, root, ".docs/wiki/04_RF/RF-OLD-01.md", "old")
	writeContractTestWikiFile(t, root, retirementPath, fmt.Sprintf(base, "null", "true", ""))
	result := compileWikiContractValidation(root, []model.DocRecord{{Path: retirementPath}}, nil)
	if !strings.Contains(strings.Join(result.ContractWarnings, "\n"), "old_path still present") {
		t.Fatal(result)
	}
	for _, archive := range []string{".docs/wiki/not-retired/RF-OLD-01.md", ".docs/wiki/_retired/requirement/MISSING.md", "../../outside.md"} {
		body := strings.ReplaceAll(fmt.Sprintf(base, "null", "true", ""), archived, archive)
		writeContractTestWikiFile(t, root, retirementPath, body)
		result := compileWikiContractValidation(root, []model.DocRecord{{Path: retirementPath}}, nil)
		if !strings.Contains(strings.Join(result.ContractWarnings, "\n"), "archived_path") {
			t.Fatal(result)
		}
	}
}

func TestCompileWikiContractOverviewAndForbiddenR2(t *testing.T) {
	for _, tc := range []struct{ path, kind, id, sections string }{
		{"07_technical.md", "technical", "TECH-X-01", "## Resumen\nx\n## Mapa de componentes\nx\n"},
		{"03_architecture.md", "architecture", "ARCH-X-01", "## Contexto\nx\n## Contenedores (C4)\nx\n## Decisiones clave\nx\n"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			root := t.TempDir()
			path := ".docs/wiki/" + tc.path
			content := "---\nwiki_contract: WIKI-CONTRACT-v1\nid: " + tc.id + "\nkind: " + tc.kind + "\nstatus: active\n---\n" + tc.sections
			writeContractTestWikiFile(t, root, path, content)
			result := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
			if result.ContractVerdict != "PASS" {
				t.Fatal(result)
			}
			bad := strings.Replace(content, "id: "+tc.id, "id: RF-WRONG-01", 1)
			writeContractTestWikiFile(t, root, path, bad)
			result = compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
			if !strings.Contains(strings.Join(result.ContractWarnings, "\n"), "prefixed id") {
				t.Fatal(result)
			}
			bad = strings.Replace(content, "status: active", "status: active\nbusiness_anchor: NEG-X\nimports: []\nexports: []\nplanned: true", 1) + "```toon\nid: NORMATIVE\n```\n"
			writeContractTestWikiFile(t, root, path, bad)
			result = compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
			for _, want := range []string{"business_anchor", "imports", "exports", "planned", "toon"} {
				if !strings.Contains(strings.Join(result.ContractWarnings, "\n"), want) {
					t.Fatalf("missing %s: %+v", want, result)
				}
			}
		})
	}
	for _, kind := range []string{"governance", "scope"} {
		header, keys, _ := parseWikiContractHeader("---\nwiki_contract: WIKI-CONTRACT-v1\nkind: " + kind + "\nstatus: active\n---\n")
		for _, warning := range validateWikiContractHeader(kind, header, keys, nil) {
			if strings.Contains(warning, "missing id") {
				t.Fatal(warning)
			}
		}
	}
}

func TestCompileWikiContractDoesNotTreatIDsOrMarkersAsPathsR2(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/07_technical.md"
	writeContractTestWikiFile(t, root, "src/real.go", "package real\n")
	content := "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TECH-X-01\nkind: technical\nstatus: active\nlinks:\n  code:\n    - path: src/real.go\n      role: FL-X-01\n      symbol: TP-X-01\n      note: Criterio 2\n---\n## Resumen\nFL-X-01, TP-X-01, `Criterio 2` and `FL-X-01` are not paths.\n## Mapa de componentes\nx\n"
	writeContractTestWikiFile(t, root, path, content)
	result := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
	if result.ContractVerdict != "PASS" || result.LinksReviewed != 1 {
		t.Fatal(result)
	}
	for _, marker := range []string{"FL-X-01", "TP-X-01", "Criterio 2"} {
		writeContractTestWikiFile(t, root, path, strings.Replace(content, "path: src/real.go", "path: "+marker, 1))
		result = compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
		warnings := strings.Join(result.ContractWarnings, "\n")
		if strings.Contains(warnings, "link_code_path_missing") || !strings.Contains(warnings, "link_code_invalid_path") {
			t.Fatal(result)
		}
	}
}

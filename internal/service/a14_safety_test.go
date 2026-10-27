package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
)

const a14SafetyGovernance = "---\nwiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: es\n---\n"

func TestA14SafetyGovernanceFailClosed(t *testing.T) {
	for _, tc := range []struct{ name, path, content string }{
		{"unknown_setup", ".sandinas/project.yaml", "schema: sandinas-project/v99\n"},
		{"invalid_source", ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n[governance]\nsource_doc = '../outside.md'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeContractTestWikiFile(t, root, ".docs/wiki/00_governance.md", a14SafetyGovernance)
			writeContractTestWikiFile(t, root, tc.path, tc.content)
			if got := ResolveWikiContractLanguageAtRoot(root); got.Valid {
				t.Errorf("invalid authority accepted for language: %+v", got)
			}
			if got, warnings := docgraph.LoadWikiContractGovernance(root); got != nil {
				t.Errorf("invalid authority accepted for governance: %+v warnings=%v", got, warnings)
			}
		})
	}
}

func TestA14SafetyGovernanceSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "governance.md")
	if err := os.WriteFile(outside, []byte(a14SafetyGovernance), 0o644); err != nil {
		t.Fatal(err)
	}
	wiki := filepath.Join(root, ".docs", "wiki")
	if err := os.MkdirAll(wiki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(wiki, "00_governance.md")); err != nil {
		t.Skipf("symlink unavailable without privileges: %v", err)
	}
	if got := ResolveWikiContractLanguageAtRoot(root); got.Valid {
		t.Errorf("symlink authority accepted for language: %+v", got)
	}
	if got, warnings := docgraph.LoadWikiContractGovernance(root); got != nil {
		t.Errorf("symlink authority accepted for governance: %+v warnings=%v", got, warnings)
	}
}

func TestA14SafetyGovernanceAbsentAndLegacy(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		root := t.TempDir()
		if got := ResolveWikiContractLanguageAtRoot(root); got.Declared || !got.Valid || got.Language != model.WikiContractLanguageES || len(got.Warnings) == 0 {
			t.Errorf("absent default preserves undeclared Spanish advisory, not authority: %+v", got)
		}
		if got, _ := docgraph.LoadWikiContractGovernance(root); got != nil {
			t.Errorf("absent governance loaded: %+v", got)
		}
	})
	t.Run("legacy", func(t *testing.T) {
		root := t.TempDir()
		writeContractTestWikiFile(t, root, ".docs/wiki/00_gobierno_documental.md", a14SafetyGovernance)
		got := ResolveWikiContractLanguageAtRoot(root)
		if !got.Valid || !got.Declared || got.Language != model.WikiContractLanguageES {
			t.Errorf("legacy language not preserved: %+v", got)
		}
		if gov, warnings := docgraph.LoadWikiContractGovernance(root); gov == nil {
			t.Errorf("legacy governance not loaded: warnings=%v", warnings)
		}
	})
}

func TestA14SafetyRetirementMultilineReason(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/00_retirements.md"
	archived := ".docs/wiki/_retired/requirement/RF-OLD-01.md"
	writeContractTestWikiFile(t, root, archived, "# Archivo\n")
	row := "```yaml\nentries:\n  - id: RF-OLD-01\n    old_path: .docs/wiki/05_requirements/RF-OLD-01.md\n    archived_path: " + archived + "\n    terminal: true\n    reason: |\n      No hay retiros registrados.\n```\n"
	for _, tc := range []struct{ name, prefix string; pass bool }{
		{"reason_only", "", true},
		{"real_contradiction", "No hay retiros registrados.\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeContractTestWikiFile(t, root, path, tc.prefix+row)
			got := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
			if (got.ContractVerdict == "PASS") != tc.pass || got.ContractsReviewed != 1 {
				t.Errorf("unexpected retirement result: %+v", got)
			}
			if !tc.pass && !strings.Contains(strings.Join(got.ContractWarnings, " "), "contradictory empty retirement inventory") {
				t.Errorf("real contradiction not reported: %+v", got)
			}
		})
	}
}

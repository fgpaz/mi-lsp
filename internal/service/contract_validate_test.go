package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func writeContractTestWikiFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseWikiContractHeaderFindings(t *testing.T) {
	content := `---
wiki_contract: WIKI-CONTRACT-v1
id: RF-TEST-01
kind: requirement
status: active
title: "Test"
doc_id: RF-LEGACY
imports: [FL-01]
links:
  implements: [FL-TEST-01]
---
`
	header, keys, warnings := parseWikiContractHeader(content)
	if header == nil {
		t.Fatal("expected header")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected parse warnings: %v", warnings)
	}
	docWarnings := validateWikiContractHeader("RF-TEST-01", header, keys, nil)
	foundForbidden := false
	for _, warning := range docWarnings {
		if strings.Contains(warning, "doc_id") || strings.Contains(warning, "imports") {
			foundForbidden = true
		}
	}
	if !foundForbidden {
		t.Fatalf("expected forbidden key warnings, got %v", docWarnings)
	}
}

func TestCompileWikiContractValidationRequiredLinks(t *testing.T) {
	root := t.TempDir()
	gov := &docgraph.WikiContractGovernance{
		WikiContract: docgraph.WikiContractProtocolV1,
		Traits: map[string]model.WikiContractTraitView{
			"has_ui": {Name: "has_ui", Reason: "ui"},
		},
	}
	records := []model.DocRecord{{
		Path: ".docs/wiki/03_FL/FL-TEST-01.md",
	}}
	writeContractTestWikiFile(t, root, records[0].Path, `---
wiki_contract: WIKI-CONTRACT-v1
id: FL-TEST-01
kind: flow
status: draft
title: "Flow"
---
`)
	result := compileWikiContractValidation(root, records, gov)
	if len(result.ContractWarnings) == 0 {
		t.Fatal("expected contract warnings for missing fulfills")
	}
	if !strings.Contains(result.ContractWarnings[0], "fulfills") {
		t.Fatalf("expected fulfills warning, got %v", result.ContractWarnings)
	}
	if result.ContractVerdict != "ADVISORY" {
		t.Fatalf("expected ADVISORY verdict, got %s", result.ContractVerdict)
	}
}

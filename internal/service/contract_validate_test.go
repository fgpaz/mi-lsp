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

func TestCompileWikiContractValidationLinkTargetsAndSections(t *testing.T) {
	root := t.TempDir()
	rsPath := ".docs/wiki/02_outcomes/RS-FIX-01.md"
	flPath := ".docs/wiki/04_flows/FL-FIX-01.md"
	rfPath := ".docs/wiki/05_requirements/RF-FIX-01.md"
	writeContractTestWikiFile(t, root, rsPath, `---
wiki_contract: WIKI-CONTRACT-v1
id: RS-FIX-01
kind: outcome
status: active
title: "Outcome"
---
## Resultado observable
x
## Beneficiario
x
## Criterios de éxito
x
## Fuera de alcance del RS
x
`)
	writeContractTestWikiFile(t, root, flPath, `---
wiki_contract: WIKI-CONTRACT-v1
id: FL-FIX-01
kind: flow
status: active
title: "Flow"
links:
  fulfills: [RS-FIX-01]
  implements: [RS-FIX-01]
---
## Objetivo
x
## Alcance
x
## Actores
x
## Pre/postcondiciones
x
## Secuencia
x
## Restricciones ADR
x
`)
	writeContractTestWikiFile(t, root, rfPath, `---
wiki_contract: WIKI-CONTRACT-v1
id: RF-FIX-01
kind: requirement
status: active
title: "Requirement"
links:
  implements: [FL-MISSING-99]
  code:
    - path: missing/file.go
      role: behavior
---
## Propósito
x
## Comportamiento ordenado
x
## Superficies
x
## Entradas/salidas tipadas
x
## Errores
x
## Criterios de aceptación
sin numerar
`)
	records := []model.DocRecord{
		{Path: rsPath, DocID: "RS-FIX-01"},
		{Path: flPath, DocID: "FL-FIX-01"},
		{Path: rfPath, DocID: "RF-FIX-01"},
	}
	result := compileWikiContractValidation(root, records, nil)
	if result.TargetsMissing < 1 {
		t.Fatalf("expected targets_missing, got %d warnings=%v", result.TargetsMissing, result.ContractWarnings)
	}
	if result.LinksReviewed < 3 {
		t.Fatalf("expected links reviewed, got %d", result.LinksReviewed)
	}
	foundMissing := false
	foundMismatch := false
	foundCode := false
	foundAcceptance := false
	for _, w := range result.ContractWarnings {
		if strings.Contains(w, "link_target_missing:RF-FIX-01->implements->FL-MISSING-99") {
			foundMissing = true
		}
		if strings.Contains(w, "link_target_kind_mismatch:FL-FIX-01->implements->RS-FIX-01") {
			foundMismatch = true
		}
		if strings.Contains(w, "link_code_path_missing:RF-FIX-01:missing/file.go") {
			foundCode = true
		}
		if strings.Contains(w, "acceptance_criteria_unnumbered") {
			foundAcceptance = true
		}
	}
	if !foundMissing || !foundMismatch || !foundCode || !foundAcceptance {
		t.Fatalf("expected depth warnings, got %v", result.ContractWarnings)
	}
}

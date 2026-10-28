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

func TestCompileWikiContractValidationIndexesUnselectedCanonicalTargets(t *testing.T) {
	root := t.TempDir()
	sourcePath := ".docs/wiki/04_flows/FL-SELECTED-01.md"
	targetPath := ".docs/wiki/02_outcomes/RS-UNSELECTED-01.md"
	writeContractTestWikiFile(t, root, sourcePath, `---
wiki_contract: WIKI-CONTRACT-v1
id: FL-SELECTED-01
kind: flow
status: active
title: "Selected flow"
links:
  fulfills: [RS-UNSELECTED-01]
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
	writeContractTestWikiFile(t, root, targetPath, `---
wiki_contract: WIKI-CONTRACT-v1
id: RS-UNSELECTED-01
kind: outcome
status: active
title: "Unselected outcome"
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
	allRecords := []model.DocRecord{
		{Path: sourcePath, DocID: "FL-SELECTED-01"},
		{Path: targetPath, DocID: "RS-UNSELECTED-01"},
	}
	selectedRecords := allRecords[:1]
	result := compileWikiContractValidationWithIndexRecords(root, selectedRecords, allRecords, nil)
	if result.ContractsReviewed != 1 {
		t.Fatalf("expected only selected source reviewed, got %d", result.ContractsReviewed)
	}
	if result.TargetsMissing != 0 {
		t.Fatalf("expected selected source to resolve unselected canonical target, got %d warnings=%v", result.TargetsMissing, result.ContractWarnings)
	}
	for _, warning := range result.ContractWarnings {
		if strings.Contains(warning, "link_target_missing") {
			t.Fatalf("unexpected missing target warning: %v", result.ContractWarnings)
		}
	}
}

func lineOfContractNeedle(t *testing.T, content, needle string) int {
	t.Helper()
	index := strings.Index(content, needle)
	if index < 0 {
		t.Fatalf("needle %q not found in fixture", needle)
	}
	return strings.Count(content[:index], "\n") + 1
}

func findAnchorRef(refs []model.WikiAnchorRef, ref string) (model.WikiAnchorRef, bool) {
	for _, item := range refs {
		if item.Ref == ref {
			return item, true
		}
	}
	return model.WikiAnchorRef{}, false
}

func hasContractWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

func TestCompileWikiContractValidationResolvesAnchorRefs(t *testing.T) {
	root := t.TempDir()
	flPath := ".docs/wiki/04_flows/FL-ANC-01.md"
	rfPath := ".docs/wiki/05_requirements/RF-ANC-01.md"
	uxmPath := ".docs/wiki/10_ux/UXM-ANC-01.md"
	plainPath := ".docs/wiki/04_flows/FL-PLAIN-01.md"
	flContent := `---
wiki_contract: WIKI-CONTRACT-v1
id: FL-ANC-01
kind: flow
status: active
title: "Flow"
---
## Objetivo
### P9 · fuera de secuencia
x
## Actores
x
## Pre/postcondiciones
x
## Secuencia
### P1 · El admin entra
x
` + "```mermaid\n### P7 · dentro de fence\n```\n" + `### P2 · El admin carga
x
### P2 · duplicado
x
## Restricciones ADR
x
`
	rfContent := `---
wiki_contract: WIKI-CONTRACT-v1
id: RF-ANC-01
kind: requirement
status: active
title: "Requirement"
links:
  implements: [FL-ANC-01]
---
## Propósito
Ver FL-ANC-01#P1 y [[FL-ANC-01#P2]], el error RF-ANC-01#E2.
Falla en FL-ANC-01#P9 y en UXM-ANC-01#UXQ-09; usa UXM-ANC-01#UXQ-01.
Sin documento: FL-NOPE-99#P1.
` + "```text\nFL-ANC-01#P5 dentro de fence\n```\n" + `## Comportamiento ordenado
x
## Superficies
x
## Entradas/salidas tipadas
x
## Errores
### E1 · usuario_vacio
«Ingresá un nombre de usuario.»
### E1 · repetido
x
### E2 · usuario_largo
x
## Criterios de aceptación (numerados)
1. El sistema valida el nombre.
`
	uxmContent := `---
wiki_contract: WIKI-CONTRACT-v1
id: UXM-ANC-01
kind: module_map
status: active
title: "Module"
links:
  maps: [RF-ANC-01]
---
## Pantalla de alta
### UXQ-01 · ¿Se permite alias?
Opciones: sí, no.
### UXD-01 · Alias opcional
x
`
	plainContent := `---
wiki_contract: WIKI-CONTRACT-v1
id: FL-PLAIN-01
kind: flow
status: active
title: "Plain"
---
## Secuencia
Sin anclas declaradas.
`
	writeContractTestWikiFile(t, root, flPath, flContent)
	writeContractTestWikiFile(t, root, rfPath, rfContent)
	writeContractTestWikiFile(t, root, uxmPath, uxmContent)
	writeContractTestWikiFile(t, root, plainPath, plainContent)
	allRecords := []model.DocRecord{
		{Path: flPath, DocID: "FL-ANC-01"},
		{Path: rfPath, DocID: "RF-ANC-01"},
		{Path: uxmPath, DocID: "UXM-ANC-01"},
		{Path: plainPath, DocID: "FL-PLAIN-01"},
	}

	selected := compileWikiContractValidationWithIndexRecords(root, allRecords[1:2], allRecords, nil)

	wantResolved := map[string]model.WikiAnchorRef{
		"FL-ANC-01#P1":      {Ref: "FL-ANC-01#P1", SourcePath: rfPath, SourceLine: lineOfContractNeedle(t, rfContent, "Ver FL-ANC-01#P1"), TargetPath: flPath, TargetLine: lineOfContractNeedle(t, flContent, "### P1 · El admin entra"), Status: model.WikiAnchorStatusResolved},
		"FL-ANC-01#P2":      {Ref: "FL-ANC-01#P2", SourcePath: rfPath, SourceLine: lineOfContractNeedle(t, rfContent, "[[FL-ANC-01#P2]]"), TargetPath: flPath, TargetLine: lineOfContractNeedle(t, flContent, "### P2 · El admin carga"), Status: model.WikiAnchorStatusResolved},
		"UXM-ANC-01#UXQ-01": {Ref: "UXM-ANC-01#UXQ-01", SourcePath: rfPath, SourceLine: lineOfContractNeedle(t, rfContent, "usa UXM-ANC-01#UXQ-01"), TargetPath: uxmPath, TargetLine: lineOfContractNeedle(t, uxmContent, "### UXQ-01 · ¿Se permite alias?"), Status: model.WikiAnchorStatusResolved},
		"RF-ANC-01#E2":      {Ref: "RF-ANC-01#E2", SourcePath: rfPath, SourceLine: lineOfContractNeedle(t, rfContent, "el error RF-ANC-01#E2"), TargetPath: rfPath, TargetLine: lineOfContractNeedle(t, rfContent, "### E2 · usuario_largo"), Status: model.WikiAnchorStatusResolved},
	}
	for ref, want := range wantResolved {
		got, ok := findAnchorRef(selected.AnchorRefs, ref)
		if !ok {
			t.Fatalf("expected anchor ref %s in %+v", ref, selected.AnchorRefs)
		}
		if got != want {
			t.Fatalf("anchor ref %s: got %+v want %+v", ref, got, want)
		}
	}
	for _, ref := range []string{"FL-ANC-01#P9", "UXM-ANC-01#UXQ-09"} {
		got, ok := findAnchorRef(selected.AnchorRefs, ref)
		if !ok || got.Status != model.WikiAnchorStatusMissing || got.TargetLine != 0 {
			t.Fatalf("expected %s as anchor_missing with no target line, got %+v ok=%v", ref, got, ok)
		}
	}
	for _, ref := range selected.AnchorRefs {
		if strings.Contains(ref.Ref, "P5") || strings.Contains(ref.Ref, "NOPE") {
			t.Fatalf("fenced or unindexed citation must not be resolved: %+v", ref)
		}
	}

	// Only the selected RF reports drift and its own duplicate E1; the FL duplicate belongs to the FL record.
	for _, want := range []string{
		"anchor_missing:RF-ANC-01->FL-ANC-01#P9",
		"anchor_missing:RF-ANC-01->UXM-ANC-01#UXQ-09",
		"anchor_duplicate:RF-ANC-01:E1",
	} {
		if !hasContractWarning(selected.ContractWarnings, want) {
			t.Fatalf("expected warning %q in %v", want, selected.ContractWarnings)
		}
	}
	for _, warning := range selected.ContractWarnings {
		if strings.Contains(warning, "NOPE") || strings.Contains(warning, "FL-ANC-01:P2") {
			t.Fatalf("unexpected anchor warning for unindexed target or unselected doc: %q", warning)
		}
	}

	full := compileWikiContractValidationWithIndexRecords(root, allRecords, allRecords, nil)
	// The module_map UXM is a valid ux_module: the full run must not report its kind as unknown.
	for _, warning := range full.ContractWarnings {
		if strings.Contains(warning, "unknown kind") {
			t.Fatalf("module_map must validate as ux_module, got %q", warning)
		}
	}
	// Anchor diagnostics are the only anchor_* warnings; the plain doc without anchors adds none.
	var anchorWarnings []string
	for _, warning := range full.ContractWarnings {
		if strings.HasPrefix(warning, "anchor_") {
			anchorWarnings = append(anchorWarnings, warning)
		}
	}
	wantAnchorWarnings := []string{
		"anchor_duplicate:FL-ANC-01:P2",
		"anchor_duplicate:RF-ANC-01:E1",
		"anchor_missing:RF-ANC-01->FL-ANC-01#P9",
		"anchor_missing:RF-ANC-01->UXM-ANC-01#UXQ-09",
	}
	if len(anchorWarnings) != len(wantAnchorWarnings) {
		t.Fatalf("anchor warnings=%v want=%v", anchorWarnings, wantAnchorWarnings)
	}
	for _, want := range wantAnchorWarnings {
		if !hasContractWarning(anchorWarnings, want) {
			t.Fatalf("expected anchor warning %q in %v", want, anchorWarnings)
		}
	}
}

func TestDeclaredWikiAnchorsScopesBySectionAndKind(t *testing.T) {
	body := `## Secuencia
### P1 · paso
## Errores
### E1 · clave
### UXQ-01 · pregunta
`
	cases := []struct {
		name     string
		kind     string
		language model.WikiContractLanguage
		want     []string
	}{
		{name: "flow declares P under sequence only", kind: "flow", language: model.WikiContractLanguageES, want: []string{"P1"}},
		{name: "requirement declares E under errors only", kind: "requirement", language: model.WikiContractLanguageES, want: []string{"E1"}},
		{name: "ux_module declares UXD/UXQ anywhere", kind: "ux_module", language: model.WikiContractLanguageES, want: []string{"UXQ-01"}},
		{name: "english governance does not accept spanish sequence heading", kind: "flow", language: model.WikiContractLanguageEN, want: nil},
		{name: "invalid language declares nothing", kind: "flow", language: model.WikiContractLanguage(""), want: nil},
		{name: "unknown kind declares nothing", kind: "scope", language: model.WikiContractLanguageES, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared, findings := model.DeclaredWikiAnchors("DOC-01", body, tc.kind, tc.language)
			if len(findings) != 0 {
				t.Fatalf("unexpected findings: %+v", findings)
			}
			if len(declared) != len(tc.want) {
				t.Fatalf("declared=%v want=%v", declared, tc.want)
			}
			for _, anchor := range tc.want {
				if _, ok := declared[anchor]; !ok {
					t.Fatalf("anchor %s missing from %v", anchor, declared)
				}
			}
		})
	}
}

func TestParseWikiAnchorCitationsBoundaries(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{name: "plain citation", body: "Ver FL-A#P1 aquí.", want: []string{"FL-A#P1"}},
		{name: "glued prefix is not a citation", body: "xFL-A#P1 y file.FL-A#P1", want: nil},
		{name: "glued suffix is not a citation", body: "FL-A#P1a y FL-A#P1-x", want: nil},
		{name: "terminal dot and two-digit anchor", body: "Paso FL-A#P12.", want: []string{"FL-A#P12"}},
		{name: "wikilink and comma separated pair", body: "[[FL-A#E1]] y (FL-A#P1,FL-B#P2)", want: []string{"FL-A#E1", "FL-A#P1", "FL-B#P2"}},
		{name: "hyphenated UXM target", body: "UXM-SEG#UXQ-01", want: []string{"UXM-SEG#UXQ-01"}},
		{name: "fenced code is ignored", body: "```\nFL-A#P1\n```\n", want: nil},
		{name: "front matter is ignored", body: "---\nlinks: [FL-A#P1]\n---\nTexto", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, citation := range model.ParseWikiAnchorCitations(tc.body) {
				got = append(got, citation.DocID+"#"+citation.Anchor)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("citations=%v want=%v", got, tc.want)
			}
		})
	}
}

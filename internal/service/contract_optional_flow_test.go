package service

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

func TestFlowWithoutADRConstraintsHasNoSectionFinding(t *testing.T) {
	body := `## Objetivo
x
## Alcance
x
## Actores
x
## Pre/postcondiciones
x
## Secuencia
### P1 · Paso
x
`
	withADRConstraints := body + "\n## Restricciones ADR\nUna restricción.\n"
	for _, content := range []struct {
		name string
		body string
	}{{name: "omitted", body: body}, {name: "present", body: withADRConstraints}} {
		t.Run(content.name, func(t *testing.T) {
			findings := model.ValidateWikiContractSections("FL-FIX-01", "flow", content.body, model.WikiContractLanguageES)
			if len(findings) != 0 {
				t.Fatalf("unexpected localized section findings: %+v", findings)
			}
			warnings := wikisource.ContractBodySectionWarnings("FL-FIX-01", "flow", content.body)
			if len(warnings) != 0 {
				t.Fatalf("unexpected legacy section warnings: %v", warnings)
			}
		})
	}
}

func TestValidateContractFlowWithoutADROrSequenceDiagramHasNoFindings(t *testing.T) {
	root := t.TempDir()
	path := "wiki/04_flows/FL-DIAG-01.md"
	writeContractTestWikiFile(t, root, path, `---
wiki_contract: WIKI-CONTRACT-v1
id: FL-DIAG-01
kind: flow
status: active
title: "Flow"
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
### P1 · Paso
x
`)

	envelope := model.Envelope{}
	attachWikiContractV11Details(root, []model.DocRecord{{Path: path, DocID: "FL-DIAG-01"}}, &envelope)
	if len(envelope.Checks) != 1 {
		t.Fatalf("expected one flow check, got %+v", envelope.Checks)
	}
	for _, finding := range envelope.Checks[0].Findings {
		if finding.Code == "diagram_missing" || finding.Code == "section_missing" {
			t.Fatalf("unexpected contract finding: %+v", finding)
		}
	}
}

package wikisource

import "testing"

const fixtureRF = `---
wiki_contract: WIKI-CONTRACT-v1
id: RF-FIX-01
kind: requirement
status: active
title: "Fixture RF"
links:
  implements: [FL-FIX-01]
---
## Criterios de aceptación
1. Primero hace A.
2. Segundo hace B.
`

const fixtureTP = `---
wiki_contract: WIKI-CONTRACT-v1
id: TP-FIX-01
kind: test
status: draft
title: "Fixture TP"
links:
  verifies: [RF-FIX-01]
---
## Casos
- Criterio 1 — disposición: automated; evidencia: tests/fixture_test.go
- Criterio 2 — disposición: manual; evidencia: screenshot
`

func TestParseContractMetadataLinks(t *testing.T) {
	meta := ParseContractMetadata(fixtureRF)
	if meta.ID != "RF-FIX-01" || len(meta.Links.Implements) != 1 || meta.Links.Implements[0] != "FL-FIX-01" {
		t.Fatalf("unexpected RF metadata: %+v", meta)
	}
	tp := ParseContractMetadata(fixtureTP)
	if len(tp.Links.Verifies) != 1 || tp.Links.Verifies[0] != "RF-FIX-01" {
		t.Fatalf("unexpected verifies: %+v", tp.Links)
	}
}

func TestDeriveInverseEdges(t *testing.T) {
	byID := map[string]ContractLinks{
		"TP-FIX-01": {Verifies: []string{"RF-FIX-01"}},
		"RF-FIX-01": {Implements: []string{"FL-FIX-01"}},
	}
	edges := DeriveInverseEdges(byID)
	if len(edges) < 2 {
		t.Fatalf("expected derived edges, got %d", len(edges))
	}
	found := false
	for _, edge := range edges {
		if edge.InverseType == "verified_by" && edge.FromDocID == "RF-FIX-01" && edge.ToDocID == "TP-FIX-01" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing verified_by edge: %+v", edges)
	}
}

func TestContractBodySectionWarnings(t *testing.T) {
	warnings := ContractBodySectionWarnings("RF-X", "requirement", `---
id: RF-X
---
## Propósito
ok
`)
	if len(warnings) < 4 {
		t.Fatalf("expected section_missing warnings, got %v", warnings)
	}
}

func TestBuildContractWikiIDIndexDataModel(t *testing.T) {
	index := BuildContractWikiIDIndex([]ContractDocBody{{
		Path:    ".docs/wiki/05_requirements/data-model.md",
		Content: "---\nwiki_contract: WIKI-CONTRACT-v1\nid: DM-ROOT\nkind: data_model\n---\n",
	}})
	if index.LookupKind("data-model") != "data_model" {
		t.Fatalf("data-model alias: %+v", index)
	}
}

func TestParseAcceptanceCriteriaAndCases(t *testing.T) {
	criteria := ParseAcceptanceCriteria(fixtureRF)
	if len(criteria) != 2 || criteria[1] == "" {
		t.Fatalf("criteria: %+v", criteria)
	}
	cases := ParseTestPlanCases(fixtureTP)
	if len(cases) != 2 || cases[0].Criterion != 1 || cases[1].Criterion != 2 {
		t.Fatalf("cases: %+v", cases)
	}
}

package docgraph

import "testing"

func TestBuildWikiIndexFixture(t *testing.T) {
	docs := []WikiViewDoc{
		{Path: ".docs/wiki/04_RF/RF-FIX-01.md", DocID: "RF-FIX-01", Title: "RF", Content: `---
wiki_contract: WIKI-CONTRACT-v1
id: RF-FIX-01
kind: requirement
status: active
title: "Fixture RF"
---
`},
		{Path: ".docs/wiki/06_pruebas/TP-FIX-01.md", DocID: "TP-FIX-01", Title: "TP", Content: `---
wiki_contract: WIKI-CONTRACT-v1
id: TP-FIX-01
kind: test
status: draft
title: "Fixture TP"
---
`},
	}
	idx := BuildWikiIndex(docs, "RF")
	if idx.Count != 1 || idx.Entries[0].ID != "RF-FIX-01" {
		t.Fatalf("index: %+v", idx)
	}
	if idx.Entries[0].LegacyWarning == "" {
		t.Fatalf("expected legacy path warning on 04_RF")
	}
}

func TestBuildWikiMatrixFixture(t *testing.T) {
	rf := `---
wiki_contract: WIKI-CONTRACT-v1
id: RF-FIX-01
kind: requirement
status: active
---
## Criterios de aceptación
1. Uno.
2. Dos.
`
	tp := `---
wiki_contract: WIKI-CONTRACT-v1
id: TP-FIX-01
kind: test
links:
  verifies: [RF-FIX-01]
---
## Casos
- Criterio 1 — disposición: automated; evidencia: a.go
`
	docs := []WikiViewDoc{
		{Path: ".docs/wiki/05_requirements/RF-FIX-01.md", DocID: "RF-FIX-01", Content: rf},
		{Path: ".docs/wiki/06_tests/TP-FIX-01.md", DocID: "TP-FIX-01", Content: tp},
	}
	matrix := BuildWikiMatrix(docs)
	if matrix.RFsReviewed != 1 || matrix.TPsReviewed != 1 {
		t.Fatalf("reviewed counts: %+v", matrix)
	}
	if len(matrix.UncoveredCriteria) != 1 || matrix.UncoveredCriteria[0].Criterion != 2 {
		t.Fatalf("uncovered: %+v", matrix.UncoveredCriteria)
	}
}

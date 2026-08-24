package service

import "testing"

func TestClassifyWikiMapHub(t *testing.T) {
	cases := map[string]string{
		"wiki/00-identidad-karen.md":             "persona",
		"wiki/10-chiamo.md":                      "proyectos",
		"wiki/30-dashboard.md":                   "sistema",
		"wiki/20-proyectos-activos.md":           "sistema",
		"bibliotecas/memorias/ficha.md":          "materia",
		"wiki/31-workers/WORKER_CAFE.md":         "",
		"wiki/32-contratos/contrato.md":          "",
		"wiki/24-aprendizaje/learning-log.md":    "",
		".docs/wiki/00_gobierno_documental.md":   "",
	}
	for path, want := range cases {
		if got := classifyWikiMapHub(path); got != want {
			t.Fatalf("classifyWikiMapHub(%q)=%q want %q", path, got, want)
		}
	}
}

func TestGroupWikiMapDocsOrder(t *testing.T) {
	hubs := groupWikiMapDocs([]wikiMapDoc{
		{Path: "wiki/10-chiamo.md", Title: "Chiamo"},
		{Path: "wiki/00-identidad-karen.md", Title: "Identidad"},
		{Path: "bibliotecas/memorias/x.md", Title: "Ficha"},
	})
	if len(hubs) != 3 {
		t.Fatalf("hubs=%v", hubs)
	}
	if hubs[0].ID != "persona" || hubs[1].ID != "proyectos" || hubs[2].ID != "materia" {
		t.Fatalf("order=%v", hubs)
	}
}

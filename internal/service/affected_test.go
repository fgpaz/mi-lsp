package service

import "testing"

func TestAffectedDocSuggestionsPreserveGraphWikiLayers(t *testing.T) {
	want := map[string]bool{
		".docs/wiki/05_requirements/RF-GPH-007.md":                    false,
		".docs/wiki/06_tests/TP-GPH.md":                                false,
		".docs/wiki/06_tests/TP-WIKI.md":                               false,
		".docs/wiki/07_technical/TECH-GRAPH-NATIVE.md":                 false,
		".docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md":       false,
	}
	for _, item := range affectedDocSuggestions("internal/docgraph/route.go", "home") {
		if _, ok := want[item.Path]; ok {
			want[item.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("graph impact suggestions must preserve required wiki layer %q", path)
		}
	}
}

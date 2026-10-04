package query

import (
	"strings"
	"testing"
)

func TestParsePipelineAndGlobalOptions(t *testing.T) {
	p, err := Parse(`sym "App.Execute" kind=method | edges callers depth=2 | read ±3 fields=id,rev,text budget=1200 max_bytes=8192 ws=repo fresh`)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 3 || p.Stages[0].Verb != "sym" || p.Stages[0].Args[0] != "App.Execute" || p.Stages[1].Options["depth"] != "2" || p.Budget != 1200 || p.MaxBytes != 8192 || p.Workspace != "repo" || !p.Fresh {
		t.Fatalf("pipeline: %#v", p)
	}
}
func TestParseRejectsMalformedOrOversizedPipeline(t *testing.T) {
	for _, input := range []string{`sym "unterminated`, `sym a|sym b|sym c|sym d|sym e|sym f|sym g|sym h|sym i`, `sym a budget=0`, `sym a max_bytes=1`} {
		if _, err := Parse(input); err == nil {
			t.Fatalf("Parse(%q) succeeded", input)
		}
	}
}
func TestRecipesHaveVersionedSourcesAndExpand(t *testing.T) {
	want := []string{"explain", "explain-change", "find-def", "impact", "nav-affected", "nav-change-pack", "nav-find", "nav-flow-slice", "nav-intent", "nav-multi-read", "nav-overview", "nav-pack", "nav-refs", "nav-related", "nav-route", "nav-search", "nav-wiki", "trace", "who-calls"}
	got := RecipeList()
	if len(got) != len(want) {
		t.Fatalf("recipes=%d want=%d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name || got[i].Version != ContractVersion {
			t.Fatalf("recipe[%d]=%+v", i, got[i])
		}
		source, err := LoadRecipeSource(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(source, "contract_version: q-v1") {
			t.Fatalf("recipe source %s is not versioned", name)
		}
	}
	for _, name := range want {
		args := []string{"Thing"}
		if name == "impact" || name == "explain-change" || name == "nav-change-pack" || name == "nav-affected" {
			args = nil
		}
		stages, err := ExpandRecipe("@"+name, args)
		if err != nil {
			t.Fatalf("ExpandRecipe(%s): %v", name, err)
		}
		if len(stages) == 0 {
			t.Fatalf("recipe %s has no stages", name)
		}
	}
}

package cli

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestGraphCommandsExposeCanonicalArityAndFlags(t *testing.T) {
	commands := newGraphQueryCommands(nil)
	want := map[string]int{"neighbors": 1, "callers": 1, "callees": 1, "explain": 1, "path": 2, "graph": 0}
	for _, command := range commands {
		name := command.Name()
		arity, ok := want[name]
		if !ok {
			t.Fatalf("unexpected graph command %q", name)
		}
		if name == "graph" {
			continue
		}
		for _, flag := range []string{"generation", "depth", "limit", "token-budget", "direction", "edge", "cursor"} {
			if command.Flags().Lookup(flag) == nil {
				t.Errorf("%s missing --%s", name, flag)
			}
		}
		if arity == 2 && command.Use != "path <from> <to>" {
			t.Errorf("path Use = %q", command.Use)
		}
		if arity == 1 && command.Use == "" {
			t.Errorf("%s has empty Use", name)
		}
	}
	graph := commands[len(commands)-1]
	for _, name := range []string{"stats", "validate"} {
		child, _, err := graph.Find([]string{name})
		if err != nil || child == nil {
			t.Fatalf("nested nav graph %s missing: %v", name, err)
		}
		if child.Flags().Lookup("token-budget") == nil {
			t.Errorf("nested graph %s missing --token-budget", name)
		}
	}
	if model.GraphQueryDefaultToken != 4000 || model.GraphQueryMaxToken != 20000 {
		t.Fatalf("unexpected graph token contract: %d/%d", model.GraphQueryDefaultToken, model.GraphQueryMaxToken)
	}
}

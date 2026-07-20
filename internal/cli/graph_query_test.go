package cli

import (
	"reflect"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/spf13/cobra"
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

func TestGraphCommandsForwardExactContractThroughTestHook(t *testing.T) {
	var gotOp string
	var gotPayload map[string]any
	var gotPrefer bool
	state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, payload map[string]any, preferDaemon bool) error {
		gotOp, gotPayload, gotPrefer = operation, payload, preferDaemon
		return nil
	}}
	commands := newGraphQueryCommands(state)
	cases := []struct {
		name    string
		command *cobra.Command
		args    []string
		op      string
		want    map[string]any
	}{
		{"neighbors", commands[0], []string{"pkg.Root"}, "nav.neighbors", map[string]any{"selector": "pkg.Root"}},
		{"callers", commands[1], []string{"pkg.Root"}, "nav.callers", map[string]any{"selector": "pkg.Root"}},
		{"callees", commands[2], []string{"pkg.Root"}, "nav.callees", map[string]any{"selector": "pkg.Root"}},
		{"explain", commands[3], []string{"edge-1"}, "nav.explain", map[string]any{"selector": "edge-1"}},
		{"path", commands[4], []string{"from", "to"}, "nav.path", map[string]any{"from": "from", "to": "to"}},
		{"stats", commands[5].Commands()[0], nil, "nav.graph.stats", map[string]any{}},
		{"validate", commands[5].Commands()[1], nil, "nav.graph.validate", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for name, value := range map[string]string{"generation": "gen-1", "depth": "3", "limit": "7", "token-budget": "1234", "direction": "out", "edge": "calls", "cursor": "cursor-1"} {
				if err := tc.command.Flags().Set(name, value); err != nil {
					t.Fatal(err)
				}
			}
			if err := tc.command.RunE(tc.command, tc.args); err != nil {
				t.Fatal(err)
			}
			if gotOp != tc.op || !gotPrefer {
				t.Fatalf("operation=%q prefer=%v", gotOp, gotPrefer)
			}
			for key, value := range map[string]any{"generation": "gen-1", "depth": 3, "limit": 7, "token_budget": 1234, "direction": "out", "cursor": "cursor-1", "edge": []string{"calls"}} {
				if !reflect.DeepEqual(gotPayload[key], value) {
					t.Errorf("payload[%s]=%#v want %#v", key, gotPayload[key], value)
				}
			}
			for key, value := range tc.want {
				if !reflect.DeepEqual(gotPayload[key], value) {
					t.Errorf("payload[%s]=%#v want %#v", key, gotPayload[key], value)
				}
			}
		})
	}
}

func TestGraphCommandsRejectMissingAndExtraArguments(t *testing.T) {
	commands := newGraphQueryCommands(nil)
	cases := []struct {
		name  string
		index int
		args  []string
	}{
		{"neighbors missing", 0, nil},
		{"callers extra", 1, []string{"a", "b"}},
		{"callees missing", 2, nil},
		{"explain extra", 3, []string{"edge", "extra"}},
		{"path missing", 4, []string{"from"}},
		{"path extra", 4, []string{"from", "to", "extra"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var command *cobra.Command = commands[tc.index]
			if err := command.RunE(command, tc.args); err == nil {
				t.Fatal("expected argument validation error")
			}
		})
	}
}

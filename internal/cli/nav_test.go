package cli

import "testing"

func TestNavCommandExposesRepoScopeFlags(t *testing.T) {
	command := newNavCommand(&rootState{})

	intent, _, err := command.Find([]string{"intent"})
	if err != nil {
		t.Fatalf("find intent command: %v", err)
	}
	if intent.Name() != "intent" {
		t.Fatalf("intent command name = %q, want intent", intent.Name())
	}

	for _, name := range []string{"find", "search", "intent", "ask", "route", "pack"} {
		subcommand, _, err := command.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s command: %v", name, err)
		}
		if subcommand.Flags().Lookup("repo") == nil {
			t.Fatalf("%s command should expose --repo", name)
		}
	}

	for _, name := range []string{"symbols", "find", "overview", "intent"} {
		subcommand, _, err := command.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s command: %v", name, err)
		}
		if subcommand.Flags().Lookup("offset") == nil {
			t.Fatalf("%s command should expose --offset", name)
		}
	}
}

func TestNavCommandExposesWikiGroup(t *testing.T) {
	command := newNavCommand(&rootState{})

	wiki, _, err := command.Find([]string{"wiki"})
	if err != nil {
		t.Fatalf("find wiki command: %v", err)
	}
	if wiki.Name() != "wiki" {
		t.Fatalf("wiki command name = %q, want wiki", wiki.Name())
	}

	search, _, err := command.Find([]string{"wiki", "search"})
	if err != nil {
		t.Fatalf("find wiki search command: %v", err)
	}
	for _, flag := range []string{"layer", "top", "offset", "include-content"} {
		if search.Flags().Lookup(flag) == nil {
			t.Fatalf("wiki search should expose --%s", flag)
		}
	}

	pack, _, err := command.Find([]string{"wiki", "pack"})
	if err != nil {
		t.Fatalf("find wiki pack command: %v", err)
	}
	for _, flag := range []string{"rf", "fl", "doc"} {
		if pack.Flags().Lookup(flag) == nil {
			t.Fatalf("wiki pack should expose --%s", flag)
		}
	}

	trace, _, err := command.Find([]string{"wiki", "trace"})
	if err != nil {
		t.Fatalf("find wiki trace command: %v", err)
	}
	if trace.Flags().Lookup("all") == nil {
		t.Fatalf("wiki trace should expose --all")
	}
}

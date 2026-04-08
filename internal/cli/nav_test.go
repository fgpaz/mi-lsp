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

	for _, name := range []string{"find", "search", "intent"} {
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

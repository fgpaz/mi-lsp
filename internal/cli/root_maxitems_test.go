package cli

import "testing"

func TestEffectiveMaxItems_ImplicitAgentSearchDefaultsTo20(t *testing.T) {
	t.Setenv("MI_LSP_CLIENT_NAME", "codex")
	tty := true
	state := &rootState{maxItems: 50, stdoutIsTerminal: &tty}
	cmd := testAXICommand()
	if got := state.effectiveMaxItems(cmd, "nav.search", false, false); got != 20 {
		t.Fatalf("effectiveMaxItems(nav.search) = %d, want 20", got)
	}
	if got := state.effectiveMaxItems(cmd, "nav.find", false, false); got != 5 {
		t.Fatalf("effectiveMaxItems(nav.find) = %d, want 5", got)
	}
	if got := state.effectiveMaxItems(cmd, "nav.search", false, true); got != 50 {
		t.Fatalf("effectiveMaxItems(nav.search, full) = %d, want 50", got)
	}
}

func TestEffectiveMaxItems_ExplicitMaxItemsWinsOverImplicitSearchCap(t *testing.T) {
	t.Setenv("MI_LSP_CLIENT_NAME", "codex")
	tty := true
	state := &rootState{maxItems: 35, stdoutIsTerminal: &tty}
	cmd := testAXICommand()
	if err := cmd.Flags().Set("max-items", "35"); err != nil {
		t.Fatal(err)
	}
	if got := state.effectiveMaxItems(cmd, "nav.search", false, false); got != 35 {
		t.Fatalf("effectiveMaxItems(nav.search) = %d, want 35", got)
	}
}

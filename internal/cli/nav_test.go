package cli

import (
	"strings"
	"testing"
)

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

	validateHarness, _, err := command.Find([]string{"wiki", "validate-harness"})
	if err != nil {
		t.Fatalf("find wiki validate-harness command: %v", err)
	}
	if validateHarness.Name() != "validate-harness" {
		t.Fatalf("wiki validate command name = %q, want validate-harness", validateHarness.Name())
	}
	for _, flag := range []string{"ids", "paths"} {
		if validateHarness.Flags().Lookup(flag) == nil {
			t.Fatalf("wiki validate-harness should expose --%s", flag)
		}
	}

	validateSource, _, err := command.Find([]string{"wiki", "validate-source"})
	if err != nil {
		t.Fatalf("find wiki validate-source command: %v", err)
	}
	if validateSource.Name() != "validate-source" {
		t.Fatalf("wiki validate-source command name = %q, want validate-source", validateSource.Name())
	}
}

func TestParseContextTargetAcceptsFileLineShorthand(t *testing.T) {
	file, line, err := parseContextTarget([]string{"internal/service/context.go:42"})
	if err != nil {
		t.Fatalf("parseContextTarget: %v", err)
	}
	if file != "internal/service/context.go" || line != 42 {
		t.Fatalf("got file=%q line=%d", file, line)
	}
}

func TestParseContextTargetAcceptsWindowsFileLineShorthand(t *testing.T) {
	file, line, err := parseContextTarget([]string{`C:\repos\mios\mi-lsp\internal\service\context.go:42`})
	if err != nil {
		t.Fatalf("parseContextTarget: %v", err)
	}
	if !strings.HasSuffix(file, `context.go`) || line != 42 {
		t.Fatalf("got file=%q line=%d", file, line)
	}
}

func TestParseContextTargetReturnsCorrectedCommandOnBadLine(t *testing.T) {
	_, _, err := parseContextTarget([]string{"internal/service/context.go:not-a-line"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "corrected form") {
		t.Fatalf("error = %q, want corrected command guidance", err.Error())
	}
}

// TestNavWikiSearch_ExposesFederatedFlags verifies that wiki search command exposes --all-workspaces
func TestNavWikiSearch_ExposesFederatedFlags(t *testing.T) {
	command := newNavCommand(&rootState{})

	search, _, err := command.Find([]string{"wiki", "search"})
	if err != nil {
		t.Fatalf("find wiki search command: %v", err)
	}
	if search.Flags().Lookup("all-workspaces") == nil {
		t.Fatalf("wiki search should expose --all-workspaces")
	}
}

// TestNavWikiRoute_ExposesFederatedFlags verifies that wiki route command exposes --all-workspaces
func TestNavWikiRoute_ExposesFederatedFlags(t *testing.T) {
	command := newNavCommand(&rootState{})

	route, _, err := command.Find([]string{"wiki", "route"})
	if err != nil {
		t.Fatalf("find wiki route command: %v", err)
	}
	if route.Flags().Lookup("all-workspaces") == nil {
		t.Fatalf("wiki route should expose --all-workspaces")
	}
}

// TestNavWikiTrace_ExposesFederatedFlags verifies that wiki trace command exposes --all-workspaces
func TestNavWikiTrace_ExposesFederatedFlags(t *testing.T) {
	command := newNavCommand(&rootState{})

	trace, _, err := command.Find([]string{"wiki", "trace"})
	if err != nil {
		t.Fatalf("find wiki trace command: %v", err)
	}
	if trace.Flags().Lookup("all-workspaces") == nil {
		t.Fatalf("wiki trace should expose --all-workspaces")
	}
}

// TestNavWikiPack_ExposesFederatedFlags verifies that wiki pack command exposes --all-workspaces
func TestNavWikiPack_ExposesFederatedFlags(t *testing.T) {
	command := newNavCommand(&rootState{})

	pack, _, err := command.Find([]string{"wiki", "pack"})
	if err != nil {
		t.Fatalf("find wiki pack command: %v", err)
	}
	if pack.Flags().Lookup("all-workspaces") == nil {
		t.Fatalf("wiki pack should expose --all-workspaces")
	}
}

// TestNavWikiInventory_ExposesFederatedFlags verifies that wiki inventory command exposes --all-workspaces
func TestNavWikiInventory_ExposesFederatedFlags(t *testing.T) {
	command := newNavCommand(&rootState{})

	inventory, _, err := command.Find([]string{"wiki", "inventory"})
	if err != nil {
		t.Fatalf("find wiki inventory command: %v", err)
	}
	if inventory.Flags().Lookup("all-workspaces") == nil {
		t.Fatalf("wiki inventory should expose --all-workspaces")
	}
}

// TestNavWikiInventory_ExposesWithLayerCountsFlag verifies that wiki inventory command exposes --with-layer-counts
func TestNavWikiInventory_ExposesWithLayerCountsFlag(t *testing.T) {
	command := newNavCommand(&rootState{})

	inventory, _, err := command.Find([]string{"wiki", "inventory"})
	if err != nil {
		t.Fatalf("find wiki inventory command: %v", err)
	}
	if inventory.Flags().Lookup("with-layer-counts") == nil {
		t.Fatalf("wiki inventory should expose --with-layer-counts")
	}
}

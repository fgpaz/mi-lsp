package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWorkspacePruneDefaultsToSafeStalePreview(t *testing.T) {
	var gotOperation string
	var gotPayload map[string]any
	state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, payload map[string]any, _ bool) error {
		gotOperation = operation
		gotPayload = payload
		return nil
	}}
	command := newWorkspaceCommand(state)
	prune, _, err := command.Find([]string{"prune"})
	if err != nil {
		t.Fatalf("find workspace prune command: %v", err)
	}
	if prune.Flags().Lookup("stale") == nil {
		t.Fatal("workspace prune must retain the --stale compatibility flag")
	}
	if err := prune.RunE(prune, nil); err != nil {
		t.Fatalf("workspace prune without flags: %v", err)
	}
	if gotOperation != "workspace.prune" || gotPayload["stale"] != true || gotPayload["apply"] != false {
		t.Fatalf("operation=%q payload=%#v; want stale-only dry-run", gotOperation, gotPayload)
	}
}

func TestWorkspaceLinkCommandExistsAndRequiresRole(t *testing.T) {
	var gotOperation string
	var gotPreferDaemon bool
	var gotPayload map[string]any
	state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, payload map[string]any, preferDaemon bool) error {
		gotOperation = operation
		gotPreferDaemon = preferDaemon
		gotPayload = payload
		return nil
	}}
	command := newWorkspaceCommand(state)

	link, _, err := command.Find([]string{"link"})
	if err != nil {
		t.Fatalf("find workspace link command: %v", err)
	}
	if link.Use != "link <alias>" {
		t.Fatalf("workspace link Use=%q, want %q", link.Use, "link <alias>")
	}
	if link.Flags().Lookup("role") == nil {
		t.Fatal("workspace link should expose --role")
	}

	if err := link.RunE(link, []string{"wiki-canon"}); err == nil {
		t.Fatal("workspace link without --role should fail")
	} else if !strings.Contains(err.Error(), "--role") {
		t.Fatalf("error %q should mention --role", err)
	}

	if err := link.Flags().Set("role", "producto"); err != nil {
		t.Fatalf("set --role: %v", err)
	}
	if err := link.RunE(link, []string{"wiki-canon"}); err != nil {
		t.Fatalf("run workspace link: %v", err)
	}
	if gotOperation != "workspace.link" {
		t.Fatalf("operation = %q, want workspace.link", gotOperation)
	}
	if gotPreferDaemon {
		t.Fatal("workspace.link should not prefer daemon")
	}
	if gotPayload["alias"] != "wiki-canon" || gotPayload["role"] != "producto" {
		t.Fatalf("payload = %#v, want alias=wiki-canon role=producto", gotPayload)
	}
}

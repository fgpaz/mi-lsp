package service

import "testing"

func TestCanonicalHarnessClient(t *testing.T) {
	harness := []string{
		"claude-code", "codex", "grok", "pi", "cursor",
		"root", "builtin_child", "mi-lsp-mcp",
		"pi-root", "pi-chief", "pi-scout", "grok-measure-leaf", "mi-pi-control-plane",
	}
	for _, name := range harness {
		if !isHarnessClientName(name) {
			t.Fatalf("%q should be a harness client", name)
		}
	}
	for _, name := range []string{"", "manual-cli", "cli", "someone"} {
		if isHarnessClientName(name) {
			t.Fatalf("%q should stay a human client", name)
		}
	}
	if family, ok := CanonicalHarnessClient("grok-measure-leaf"); !ok || family != "grok" {
		t.Fatalf("grok-measure-leaf = %q %v", family, ok)
	}
	if family, ok := CanonicalHarnessClient("pi-chief"); !ok || family != "pi" {
		t.Fatalf("pi-chief = %q %v", family, ok)
	}
	if family, ok := CanonicalHarnessClient("root"); !ok || family != "agent" {
		t.Fatalf("root = %q %v", family, ok)
	}
}

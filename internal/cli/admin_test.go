package cli

import "testing"

func TestResolveExportWindowRejectsExplicitSinceWithRecent(t *testing.T) {
	cmd := newExportCommand(&rootState{})
	if err := cmd.Flags().Set("since", "30d"); err != nil {
		t.Fatalf("Set(since): %v", err)
	}
	if _, err := resolveExportWindow(cmd, "30d", true); err == nil {
		t.Fatal("expected conflict error when --recent and explicit --since are both set")
	}
}

func TestResolveExportWindowUsesRecentPreset(t *testing.T) {
	cmd := newExportCommand(&rootState{})
	window, err := resolveExportWindow(cmd, "7d", true)
	if err != nil {
		t.Fatalf("resolveExportWindow: %v", err)
	}
	if window.Name != "recent" {
		t.Fatalf("Name = %q, want recent", window.Name)
	}
}

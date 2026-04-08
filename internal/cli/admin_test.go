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

func TestExportQueryLimit(t *testing.T) {
	tests := []struct {
		name         string
		limitFlag    int
		limitChanged bool
		summary      bool
		want         int
	}{
		{name: "raw export keeps default limit", limitFlag: 500, limitChanged: false, summary: false, want: 500},
		{name: "raw export keeps explicit limit", limitFlag: 1000, limitChanged: true, summary: false, want: 1000},
		{name: "summary ignores default limit", limitFlag: 500, limitChanged: false, summary: true, want: 0},
		{name: "summary keeps explicit limit", limitFlag: 1000, limitChanged: true, summary: true, want: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exportQueryLimit(tt.limitFlag, tt.limitChanged, tt.summary)
			if got != tt.want {
				t.Fatalf("exportQueryLimit(%d, %t, %t) = %d, want %d", tt.limitFlag, tt.limitChanged, tt.summary, got, tt.want)
			}
		})
	}
}

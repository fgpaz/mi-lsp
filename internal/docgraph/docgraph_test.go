package docgraph

import "testing"

func TestIsSnapshotPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"old segment", "docs/wiki/old/foo.md", true},
		{"archive segment", "docs/archive/bar.md", true},
		{"deprecated segment", "docs/deprecated/baz.md", true},
		{"historico segment", "docs/historico/qux.md", true},
		{"legacy segment", "legacy/docs/readme.md", true},
		{"case insensitive Old", "docs/Old/foo.md", true},
		{"case insensitive ARCHIVE", "docs/ARCHIVE/bar.md", true},
		{"case insensitive Deprecated", "docs/Deprecated/baz.md", true},
		{"normal doc", "docs/wiki/01_alcance.md", false},
		{"code file", "src/main.go", false},
		{"empty string", "", false},
		{"old at start", "old/foo.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSnapshotPath(tt.path)
			if got != tt.want {
				t.Errorf("isSnapshotPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

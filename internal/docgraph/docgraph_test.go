package docgraph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/workspace"
)

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

func TestIndexWorkspaceDocsHonorsGitignoreReincludeForWiki(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".gitignore"), strings.Join([]string{
		"/.docs/*",
		"!/.docs/wiki/",
		"!/.docs/wiki/**",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"functional\"",
		"  intent_keywords = [\"flow\", \"rf\", \"fl\"]",
		"  paths = [\".docs/wiki/03_FL/*.md\", \".docs/wiki/04_RF/*.md\"]",
		"",
		"[generic_docs]",
		"  paths = [\"README.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "03_FL", "FL-QRY-01.md"), "# FL-QRY-01\n\ncontinuation memory pointer\n")
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "04_RF", "RF-QRY-010.md"), "# RF-QRY-010\n\ncontinuation memory pointer\n")
	mustWriteDocgraphFile(t, filepath.Join(root, "README.md"), "# repo\n")

	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher returned error: %v", err)
	}

	docs, _, _, warnings, err := IndexWorkspaceDocs(context.Background(), root, matcher)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocs returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("IndexWorkspaceDocs warnings = %v, want none", warnings)
	}

	paths := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		paths[doc.Path] = struct{}{}
	}
	for _, expected := range []string{
		".docs/wiki/03_FL/FL-QRY-01.md",
		".docs/wiki/04_RF/RF-QRY-010.md",
		"README.md",
	} {
		if _, ok := paths[expected]; !ok {
			t.Fatalf("expected %s to be indexed; got paths=%v", expected, paths)
		}
	}
}

func mustWriteDocgraphFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

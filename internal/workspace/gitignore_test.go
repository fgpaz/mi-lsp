package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIgnoreMatcherSupportsMilspIgnore(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".gitignore"), "coverage/\n")
	mustWriteFile(t, filepath.Join(root, ".milspignore"), ".docs/template/\ntemp/\n")

	matcher, err := LoadIgnoreMatcher(root, []string{"artifacts/"})
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher returned error: %v", err)
	}

	assertIgnored(t, matcher, root, filepath.Join(root, "node_modules", "pkg", "index.js"))
	assertIgnored(t, matcher, root, filepath.Join(root, "coverage", "report.json"))
	assertIgnored(t, matcher, root, filepath.Join(root, ".docs", "template", "sample.md"))
	assertIgnored(t, matcher, root, filepath.Join(root, "temp", "generated.cs"))
	assertIgnored(t, matcher, root, filepath.Join(root, "artifacts", "bundle.zip"))
	assertNotIgnored(t, matcher, root, filepath.Join(root, "src", "Program.cs"))
}

func TestIgnoreMatcherMatchesNestedSegments(t *testing.T) {
	matcher, err := LoadIgnoreMatcher(t.TempDir(), []string{"temp/", ".docs/template/"})
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher returned error: %v", err)
	}

	assertIgnored(t, matcher, "C:/repo", "C:/repo/src/temp/file.cs")
	assertIgnored(t, matcher, "C:/repo", "C:/repo/packages/app/.docs/template/seed.md")
	assertNotIgnored(t, matcher, "C:/repo", "C:/repo/packages/app/templates/seed.md")
}

func assertIgnored(t *testing.T, matcher *IgnoreMatcher, root, path string) {
	t.Helper()
	if !matcher.ShouldIgnore(root, path) {
		t.Fatalf("expected %s to be ignored", path)
	}
}

func assertNotIgnored(t *testing.T, matcher *IgnoreMatcher, root, path string) {
	t.Helper()
	if matcher.ShouldIgnore(root, path) {
		t.Fatalf("expected %s to be included", path)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkillsRootSkipsBinCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join("tool", "SKILL.md"), "---\nname: tool\ndescription: ok\n---\nbody\n")
	// Mixed-case bin must not be indexed (binaries live under skill bin/ trees).
	write(filepath.Join("tool", "Bin", "nested", "SKILL.md"), "---\nname: evil-bin-case\ndescription: skip\n---\n")
	write(filepath.Join("tool", "bin", "nested2", "SKILL.md"), "---\nname: evil-bin-lower\ndescription: skip\n---\n")
	write(filepath.Join("secrets", "hidden", "SKILL.md"), "---\nname: evil-secret\ndescription: skip\n---\n")

	scans, warnings, err := ScanSkillsRoot(root)
	if err != nil {
		t.Fatalf("ScanSkillsRoot: %v", err)
	}

	ids := map[string]string{}
	for _, s := range scans {
		ids[s.ID] = s.SourcePath
	}
	if _, ok := ids["tool"]; !ok {
		t.Fatalf("expected tool skill, got %v", ids)
	}
	for _, bad := range []string{"evil-bin-case", "evil-bin-lower", "evil-secret"} {
		if path, ok := ids[bad]; ok {
			t.Fatalf("indexed forbidden skill %s at %s (warnings=%v)", bad, path, warnings)
		}
	}
}

func TestScanSkillsRootSkipsNodeModulesCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pkg", "Node_Modules", "x", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: evil-nm\ndescription: skip\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "SKILL.md"), []byte("---\nname: pkg\ndescription: ok\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scans, _, err := ScanSkillsRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range scans {
		if s.ID == "evil-nm" {
			t.Fatalf("indexed node_modules skill at %s", s.SourcePath)
		}
	}
}

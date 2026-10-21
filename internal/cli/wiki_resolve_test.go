package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWikiSupportsJSONAndLegacyWikiRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".docs/wiki"), 0755); err != nil {
		t.Fatal(err)
	}
	gov := filepath.Join(root, ".docs/wiki/00_governance.md")
	if err := os.WriteFile(gov, []byte("{"+`"schema":"sandinas-project/v2","wiki":{"mode":"repo","canon_root":".docs/wiki","ae_root":".docs/ae","governance_file":".docs/wiki/00_governance.md","external":{"repository":"org/wiki"}}`+"}"), 0600); err != nil {
		t.Fatal(err)
	}
	got := resolveWiki(root)
	if got.Pending || got.Schema != "sandinas-wiki-resolution/v1" || got.Mode != "repo" || got.CanonRoot != filepath.Join(root, ".docs/wiki") || got.AERoot != filepath.Join(root, ".docs/ae") || got.GovernanceFile != gov {
		t.Fatalf("JSON resolution: %+v", got)
	}
	if err := os.WriteFile(gov, []byte("version: 2\nwiki_root: .docs/wiki\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got = resolveWiki(root)
	if got.Pending || got.CanonRoot != filepath.Join(root, ".docs/wiki") {
		t.Fatalf("legacy resolution: %+v", got)
	}
}

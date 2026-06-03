package docgraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectGovernanceBlocksSourceDocOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatalf("write outside doc: %v", err)
	}
	writeReadModel(t, root, "../../"+filepath.Base(outside))

	status := InspectGovernance(root, false)
	if !status.Blocked {
		t.Fatalf("Blocked = false, want true")
	}
	if status.Sync != "invalid" {
		t.Fatalf("Sync = %q, want invalid", status.Sync)
	}
	if !strings.HasPrefix(status.HumanDoc, "INVALID:") {
		t.Fatalf("HumanDoc = %q, want invalid marker", status.HumanDoc)
	}
	if !strings.Contains(strings.Join(status.Issues, " "), "source_doc") {
		t.Fatalf("Issues = %v, want source_doc guidance", status.Issues)
	}
}

func TestSafeGovernanceSourceDocAllowsRelativeWikiPath(t *testing.T) {
	root := t.TempDir()
	got, ok := safeGovernanceSourceDoc(root, ".docs/wiki/00_gobierno_documental.md")
	if !ok {
		t.Fatal("safeGovernanceSourceDoc ok = false, want true")
	}
	if got != ".docs/wiki/00_gobierno_documental.md" {
		t.Fatalf("safe path = %q, want canonical wiki path", got)
	}
}

func writeReadModel(t *testing.T, root string, sourceDoc string) {
	t.Helper()
	path := ProfilePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir read model: %v", err)
	}
	body := "version = 1\n\n[governance]\nsource_doc = " + strconvQuote(sourceDoc) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write read model: %v", err)
	}
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

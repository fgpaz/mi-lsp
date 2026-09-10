package docgraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestGovernedFilenameAliasDoesNotInventDocumentIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, owner                         string
		duplicate, wantAlias, wantAmbiguous bool
	}{
		{name: "versioned", owner: "AE-SYNTHETIC-V2", wantAlias: true},
		{name: "other-owner", owner: "OTHER-001"},
		{name: "numeric-owner", owner: "12"},
		{name: "empty-owner"},
		{name: "ambiguous", owner: "AE-SYNTHETIC-V2", duplicate: true, wantAmbiguous: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			navigationWrite(t, root, "canon/AE-SYNTHETIC.md", "---\ndoc_id: "+tc.owner+"\n---\n# Gobierno\n")
			if tc.duplicate {
				navigationWrite(t, root, "other/AE-SYNTHETIC.md", "---\ndoc_id: AE-SYNTHETIC-V3\n---\n# Otro gobierno\n")
			}
			profile := model.DocsReadProfile{Governance: model.DocsGovernanceProfile{Hierarchy: []model.GovernanceHierarchyItem{{Layer: "AE", Paths: []string{"canon/**", "other/**"}}}}}
			path, id, ambiguous := containingDocForExplicitID(root, profile, "AE-SYNTHETIC")
			if id != "" || ambiguous != tc.wantAmbiguous || (path != "") != tc.wantAlias {
				t.Fatalf("filename alias became identity or lost ambiguity: path=%q id=%q ambiguous=%v", path, id, ambiguous)
			}
			if tc.wantAlias && path != "canon/AE-SYNTHETIC.md" {
				t.Fatalf("alias path = %q", path)
			}
		})
	}
}

func navigationCanonFixture(t *testing.T, external bool) (string, string) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	declared := "wiki"
	if external {
		declared = "../canon"
	}
	canon := filepath.Join(root, filepath.FromSlash(declared))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: "synthetic", Kind: model.WorkspaceKindSingle},
		Canons:  []model.WorkspaceCanon{{ID: "synthetic-wiki", Root: declared, Role: "gobierno_local", Mode: "read-only"}},
	}); err != nil {
		t.Fatal(err)
	}
	navigationWrite(t, canon, "governance.md", "---\ndoc_id: GOV-SYNTHETIC\n---\n# Gobierno\n")
	navigationWrite(t, canon, "_mi-lsp/read-model.toml", `version = 1
[[family]]
name = "functional"
paths = ["50-flows/"]
[governance]
source_doc = "governance.md"
[[governance.hierarchy]]
id = "flow"
family = "functional"
layer = "FL"
pack_stage = "flow"
paths = ["50-flows/"]
`)
	return root, declared
}

func navigationWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeclaredCanonNavigationUsesProfileAndFrontmatter(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "workspace", true: "external"}[external], func(t *testing.T) {
			root, declared := navigationCanonFixture(t, external)
			owner := declared + "/50-flows/start.md"
			navigationWrite(t, root, owner, "---\ndoc_id: FL-SYNTHETIC-001\n---\n# Inicio sintético\n")
			profile, _, _ := LoadProfile(root)
			lane, why := Tier1CanonicalRoute("FL-SYNTHETIC-001", profile, root)
			if lane.AnchorDoc.Path != owner || lane.AnchorDoc.DocID != "FL-SYNTHETIC-001" || !lane.Authoritative {
				t.Fatalf("owner not resolved: %#v; why=%v", lane, why)
			}
			gov, err := ResolvedGovernanceDocument(root)
			if err != nil || gov != declared+"/governance.md" {
				t.Fatalf("governance=%q err=%v", gov, err)
			}
			resolved := RouteReadProfile(root, profile)
			if resolved.Families[0].Paths[0] != declared+"/50-flows/" || profile.Families[0].Paths[0] != "50-flows/" {
				t.Fatalf("profile rebasing mutated input or used wrong root: %#v", resolved.Families)
			}
		})
	}
}

func TestDeclaredCanonNavigationDoesNotPromoteMentions(t *testing.T) {
	root, declared := navigationCanonFixture(t, false)
	for name, content := range map[string]string{
		"mention.md": "# Referencia\nFL-SYNTHETIC-001\n",
		"empty.md":   "---\ndoc_id: \n---\nFL-SYNTHETIC-001\n",
		"other.md":   "---\ndoc_id: FL-OTHER-001\n---\nFL-SYNTHETIC-001\n",
	} {
		navigationWrite(t, root, declared+"/50-flows/"+name, content)
	}
	profile, _, _ := LoadProfile(root)
	lane, _ := Tier1CanonicalRoute("FL-SYNTHETIC-001", profile, root)
	if lane.AnchorDoc.DocID != "" || lane.AnchorDoc.Path != declared+"/governance.md" {
		t.Fatalf("body mention promoted to owner: %#v", lane)
	}
}

func TestDeclaredCanonNavigationReportsDuplicateOwners(t *testing.T) {
	root, declared := navigationCanonFixture(t, false)
	for _, name := range []string{"one", "two"} {
		navigationWrite(t, root, declared+"/50-flows/"+name+".md", "---\ndoc_id: FL-SYNTHETIC-001\n---\n# Inicio\n")
	}
	profile, _, _ := LoadProfile(root)
	lane, why := Tier1CanonicalRoute("FL-SYNTHETIC-001", profile, root)
	if lane.Authoritative || lane.AnchorDoc.Path != "" || !strings.Contains(strings.Join(why, " "), "ambiguous_doc_id") {
		t.Fatalf("duplicate owner silently selected: %#v why=%v", lane, why)
	}
}

func TestDeclaredCanonNavigationMissingAndUnsafeGovernance(t *testing.T) {
	root, declared := navigationCanonFixture(t, false)
	if err := os.Remove(filepath.Join(root, declared, "governance.md")); err != nil {
		t.Fatal(err)
	}
	if path, err := ResolvedGovernanceDocument(root); path != "" || err == nil {
		t.Fatalf("missing government invented: %q %v", path, err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# No autorizado\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, declared, "governance.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if path, err := ResolvedGovernanceDocument(root); path != "" || err == nil {
		t.Fatalf("symlink accepted: %q %v", path, err)
	}
}

func TestCanonicalNavigationPreservesLegacyEmbeddedFlow(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/03_FL/FL-LEGACY-001.md"
	navigationWrite(t, root, path, "# FL-LEGACY-001\n\nFlujo legado.\n")
	lane, _ := Tier1CanonicalRoute("FL-LEGACY-001", DefaultProfile(), root)
	if lane.AnchorDoc.Path != path || lane.AnchorDoc.DocID != "FL-LEGACY-001" {
		t.Fatalf("legacy route changed: %#v", lane)
	}
}

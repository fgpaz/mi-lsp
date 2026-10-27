package docgraph

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSchemaFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetupWikiSchemaAbsentUsesGovernanceSourceDoc(t *testing.T) {
	root := t.TempDir()
	writeSchemaFixture(t, root, ".sandinas/project.yaml", "wiki:\n  canon_roots:\n    - role: producto\n      path: wiki\n")
	writeSchemaFixture(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n[governance]\nsource_doc = 'wiki/custom-governance.md'\n")
	writeSchemaFixture(t, root, ".mi-lsp/project.toml", "[[canon]]\nid = 'product'\nrole = 'producto'\nroot = 'wiki'\n")
	writeSchemaFixture(t, root, "wiki/custom-governance.md", "# Governance\n")
	if got := ResolveSetupWikiSource(root); got.Configured {
		t.Errorf("missing schema must not configure Setup: %+v", got)
	}
	got, err := ResolvedGovernanceDocument(root)
	if err != nil || got != "wiki/custom-governance.md" {
		t.Fatalf("source_doc = %q, err = %v", got, err)
	}
}

func TestSetupWikiSchemaPresentFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, yaml string }{
		{"unknown", "schema: sandinas-project/v99\n"},
		{"empty", "schema: ''\n"},
		{"null", "schema: null\n"},
		{"number", "schema: 42\n"},
		{"mapping", "schema: {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSchemaFixture(t, root, ".sandinas/project.yaml", tc.yaml)
			got := ResolveSetupWikiSource(root)
			if !got.Configured || got.State != "pending" || got.ReasonCode != "setup_schema_unsupported" {
				t.Fatalf("unexpected source: %+v", got)
			}
		})
	}
}

func TestSetupWikiSchemaSupportedAndUnreadable(t *testing.T) {
	for _, schema := range []string{"sandinas-project/v1", "sandinas-project/v2"} {
		t.Run(schema, func(t *testing.T) {
			root := t.TempDir()
			writeSchemaFixture(t, root, ".sandinas/project.yaml", "schema: "+schema+"\nwiki:\n  setup_source: repo_folder\n  setup_path: wiki\n")
			writeSchemaFixture(t, root, "wiki/00_governance.md", "# Governance\n")
			got := ResolveSetupWikiSource(root)
			if !got.Configured || got.State != "resolved" || got.Source != "repo_folder" || got.Root != filepath.Join(root, "wiki") {
				t.Fatalf("unexpected source: %+v", got)
			}
		})
	}
	t.Run("unreadable_yaml", func(t *testing.T) {
		root := t.TempDir()
		writeSchemaFixture(t, root, ".sandinas/project.yaml", "wiki: [\n")
		got := ResolveSetupWikiSource(root)
		if !got.Configured || got.State != "pending" || got.ReasonCode != "setup_contract_unreadable" {
			t.Fatalf("unexpected source: %+v", got)
		}
	})
}

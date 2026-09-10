package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func declaredPackFixture(t *testing.T, duplicate bool) (string, string) {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "declared-pack-" + filepath.Base(root)
	writeWorkspaceFile(t, root, "wiki/governance.md", "---\ndoc_id: GOV-SYNTHETIC\n---\n# Gobierno\n")
	writeWorkspaceFile(t, root, "wiki/overview.md", "# Alcance sintético\n")
	writeWorkspaceFile(t, root, "README.md", "# README\n\nReferencia a FL-SYNTHETIC-001, no propietario.\n")
	writeWorkspaceFile(t, root, "wiki/50-flows/start.md", "---\ndoc_id: FL-SYNTHETIC-001\n---\n# Inicio\n\nPaso sintético verificable.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", `version = 1
[[family]]
name = "knowledge"
intent_keywords = ["knowledge"]
paths = ["wiki/"]
[generic_docs]
paths = ["README.md", "wiki/"]
[governance]
source_doc = "wiki/governance.md"
source_format = "markdown"
profile = "knowledge-wiki"
effective_base = "knowledge-wiki"
context_chain = ["wiki/governance.md", "wiki/overview.md"]
audit_chain = [".docs/auditoria/"]
`)
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
		Canons:  []model.WorkspaceCanon{{ID: "synthetic-wiki", Root: "wiki", Role: "gobierno_local", Mode: "read-only"}},
	}); err != nil {
		t.Fatal(err)
	}
	docs := []model.DocRecord{
		{Path: "README.md", Family: "generic", Layer: "generic", Title: "Referencia", SearchText: "FL-SYNTHETIC-001 referencia inicio", IndexedAt: 1},
		{Path: "wiki/50-flows/start.md", DocID: "FL-SYNTHETIC-001", Family: "generic", Layer: "generic", Title: "Inicio", SearchText: "FL-SYNTHETIC-001 inicio", IndexedAt: 1},
	}
	if duplicate {
		writeWorkspaceFile(t, root, "wiki/50-flows/duplicate.md", "---\ndoc_id: FL-SYNTHETIC-001\n---\n# Duplicado\n")
		docs = append(docs, model.DocRecord{Path: "wiki/50-flows/duplicate.md", DocID: "FL-SYNTHETIC-001", Family: "generic", Layer: "generic", Title: "Duplicado", SearchText: "FL-SYNTHETIC-001", IndexedAt: 1})
	}
	// Governance and the complete fixture precede DB creation; no stale fixture
	// or real backend/index operation is needed for this documentary oracle.
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceDocs(context.Background(), db, docs, nil, nil); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	return root, alias
}

func TestNavPackDeclaredKnowledgeCanonUsesExactOwner(t *testing.T) {
	root, alias := declaredPackFixture(t, false)
	for _, full := range []bool{false, true} {
		env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
			Operation: "nav.wiki.pack", Context: model.QueryOptions{Workspace: alias, Full: full, AXI: !full},
			Payload: map[string]any{"task": "FL-SYNTHETIC-001"},
		})
		if err != nil || !env.Ok {
			t.Fatalf("pack: err=%v envelope=%#v", err, env)
		}
		items, ok := env.Items.([]model.PackResult)
		if !ok || len(items) != 1 || items[0].PrimaryDoc != "wiki/50-flows/start.md" || len(items[0].Docs) == 0 || items[0].Docs[0].DocID != "FL-SYNTHETIC-001" {
			t.Fatalf("exact indexed owner lost: %#v", env)
		}
		if strings.Contains(strings.Join(env.Warnings, " "), "index is empty") {
			t.Fatalf("current declared generic canon treated as empty: %v", env.Warnings)
		}
		if full && !strings.Contains(items[0].Docs[0].SliceText, "Paso sintético") {
			t.Fatalf("missing substantive full slice: %#v", items[0].Docs[0])
		}
	}
}

func TestNavPackDeclaredKnowledgeCanonRejectsDuplicateOwners(t *testing.T) {
	root, alias := declaredPackFixture(t, true)
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.pack", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"task": "FL-SYNTHETIC-001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := env.Items.([]model.PackResult)
	if !ok || len(items) != 1 || items[0].PrimaryDoc != "" || len(items[0].Docs) != 0 || !strings.Contains(strings.Join(env.Warnings, " "), "ambiguous document identifier") {
		t.Fatalf("ambiguous owner not explicit: %#v", env)
	}
}

func TestWikiRootDeclaredKnowledgeCanonUsesActualGovernance(t *testing.T) {
	root, alias := declaredPackFixture(t, false)
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki-root", Context: model.QueryOptions{Workspace: alias}})
	if err != nil {
		t.Fatal(err)
	}
	items := env.Items.([]model.WikiRootResolution)
	if len(items) != 1 || items[0].WikiRoot != "wiki" || items[0].GovernanceDoc != "wiki/governance.md" || items[0].ResolvedFrom != "canon.synthetic-wiki" {
		t.Fatalf("incorrect governance resolution: %#v", env)
	}
}

func TestWikiRootDeclaredKnowledgeCanonMissingSourceIsExplicit(t *testing.T) {
	root, alias := declaredPackFixture(t, false)
	if err := os.Remove(filepath.Join(root, "wiki", "governance.md")); err != nil {
		t.Fatal(err)
	}
	// Even an existing conventional filename must not override source_doc.
	writeWorkspaceFile(t, root, "wiki/00_gobierno_documental.md", "# No es la fuente declarada\n")
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki-root", Context: model.QueryOptions{Workspace: alias}})
	if err != nil {
		t.Fatal(err)
	}
	items := env.Items.([]model.WikiRootResolution)
	if len(items) != 1 || items[0].WikiRoot != "wiki" || items[0].GovernanceDoc != "" || len(env.Warnings) == 0 {
		t.Fatalf("missing source not explicit: %#v", env)
	}
}

func TestPackInferredLegacyIDDoesNotOverrideRouteAnchor(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/04_RF.md"
	writeWorkspaceFile(t, root, path, "# Índice\n\nRF-LEGACY-001 es una referencia.\n")
	doc := model.DocRecord{Path: path, DocID: "RF-LEGACY-001", Family: "functional"}
	anchor, _ := resolvePackAnchor(nil, "RF-LEGACY-001", []model.DocRecord{doc}, nil, model.DocsReadProfile{}, root)
	if anchor.DocID != "" || anchor.DocPath != "" {
		t.Fatalf("inferred ID overrode governed route selection: %#v", anchor)
	}
}

func TestDeclaredCanonUnindexedAnchorSurvivesRankedSupport(t *testing.T) {
	for _, external := range []bool{false, true} {
		name := "local"
		if external {
			name = "external"
		}
		t.Run(name, func(t *testing.T) {
			ensureWritableTestHome(t)
			base := t.TempDir()
			root := filepath.Join(base, "workspace")
			canonPath := "wiki"
			if external {
				canonPath = "../canon"
			}
			canonRoot := filepath.Join(root, filepath.FromSlash(canonPath))
			writeWorkspaceFile(t, root, "README.md", "# Workspace sintético\n")
			writeWorkspaceFile(t, canonRoot, "governance.md", "# Gobierno sintético\n")
			writeWorkspaceFile(t, canonRoot, "flows/start.md", "---\ndoc_id: RF-SYNTHETIC-UNINDEXED\n---\n# Propietario sin indexar\n")
			writeWorkspaceFile(t, canonRoot, "support.md", "# Apoyo\n\nReferencia a RF-SYNTHETIC-UNINDEXED, no propietario.\n")
			writeWorkspaceFile(t, canonRoot, "_mi-lsp/read-model.toml", `version = 1
[[family]]
name = "functional"
intent_keywords = ["synthetic"]
paths = ["flows/**"]
[governance]
source_doc = "governance.md"
source_format = "markdown"
profile = "knowledge-wiki"
effective_base = "knowledge-wiki"
context_chain = ["governance.md"]
audit_chain = [".docs/auditoria/"]
`)
			alias := "unindexed-canon-" + filepath.Base(base)
			if err := workspace.SaveProjectFile(root, model.ProjectFile{
				Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
				Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
				Canons:  []model.WorkspaceCanon{{ID: "synthetic-wiki", Root: canonPath, Role: "gobierno_local", Mode: "read-only"}},
			}); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			docs := []model.DocRecord{{Path: canonPath + "/support.md", Family: "functional", Layer: "RF", Title: "Apoyo", SearchText: "RF-SYNTHETIC-UNINDEXED referencia synthetic", IndexedAt: 1}}
			if err := store.ReplaceDocs(context.Background(), db, docs, nil, nil); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Kind: model.WorkspaceKindSingle}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
			app := New(root, nil)
			rootEnv, err := app.Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki-root", Context: model.QueryOptions{Workspace: alias}})
			if err != nil || !rootEnv.Ok {
				t.Fatalf("root: err=%v envelope=%#v", err, rootEnv)
			}
			roots := rootEnv.Items.([]model.WikiRootResolution)
			if len(roots) != 1 || roots[0].GovernanceDoc != canonPath+"/governance.md" || roots[0].ResolvedFrom != "canon.synthetic-wiki" {
				t.Fatalf("canon-relative government lost: %#v", roots)
			}
			for _, full := range []bool{false, true} {
				env, err := app.Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki.pack", Context: model.QueryOptions{Workspace: alias, Full: full, AXI: !full}, Payload: map[string]any{"task": "RF-SYNTHETIC-UNINDEXED"}})
				if err != nil || !env.Ok {
					t.Fatalf("pack: err=%v envelope=%#v", err, env)
				}
				packs, ok := env.Items.([]model.PackResult)
				if !ok {
					t.Fatalf("pack did not pass governance gate: %#v", env)
				}
				if len(packs) != 1 || packs[0].PrimaryDoc != canonPath+"/flows/start.md" || len(packs[0].Docs) == 0 || packs[0].Docs[0].DocID != "RF-SYNTHETIC-UNINDEXED" || !strings.Contains(strings.Join(packs[0].Why, " "), "tier1=anchor_not_indexed") {
					t.Fatalf("support replaced unindexed declared anchor: %#v", env)
				}
			}
		})
	}
}

func TestPackExplicitUnknownIdentifierDoesNotFallBackToRankedDocs(t *testing.T) {
	doc := model.DocRecord{Path: "README.md", Family: "generic"}
	if primary, ok := selectPackPrimary(packAnchor{DocID: "FL-MISSING-001"}, []model.DocRecord{doc}, nil, []scoredDoc{{record: doc}}); ok {
		t.Fatalf("unresolved explicit ID became %#v", primary)
	}
}

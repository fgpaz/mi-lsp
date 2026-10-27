package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func a14CustomWiki(t *testing.T) (string, string) {
	t.Helper()
	root, name := setupTestWorkspace(t)
	t.Chdir(root)
	writeContractTestWikiFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n[governance]\nsource_doc = 'wiki/00_governance.md'\n")
	writeContractTestWikiFile(t, root, "wiki/00_governance.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: es\n---\n# Gobernanza\n\n```yaml\ntraits: {}\n```\n")
	writeContractTestWikiFile(t, root, "wiki/02_outcomes/RS-FIX-01.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RS-FIX-01\nkind: outcome\nstatus: active\ntitle: Resultado\n---\n## Resultado observable\nx\n## Beneficiario\nx\n## Criterios de éxito\nx\n## Fuera de alcance del RS\nx\n")
	writeContractTestWikiFile(t, root, "wiki/04_flows/FL-FIX-01.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: FL-FIX-01\nkind: flow\nstatus: active\ntitle: Flujo\nlinks:\n  fulfills: [RS-FIX-01]\n---\n## Objetivo\nx\n## Alcance\nx\n## Actores\nx\n## Pre/postcondiciones\nx\n## Secuencia\nx\n## Restricciones ADR\nx\n")
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	docs := []model.DocRecord{{Path: "wiki/00_governance.md"}, {Path: "wiki/02_outcomes/RS-FIX-01.md", DocID: "RS-FIX-01"}, {Path: "wiki/04_flows/FL-FIX-01.md", DocID: "FL-FIX-01"}}
	if err := store.ReplaceDocs(context.Background(), db, docs, nil, nil); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return root, name
}

func TestA14AppValidateContractEffectiveRootAndScope(t *testing.T) {
	root, name := a14CustomWiki(t)
	app := New(root, nil)
	for _, tc := range []struct {
		name, paths string
		contracts   int
	}{
		{"default", "", 2},
		{"repo_relative", "wiki/04_flows/FL-FIX-01.md", 1},
		{"wiki_relative", "04_flows/FL-FIX-01.md", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{}
			if tc.paths != "" {
				payload["paths"] = tc.paths
			}
			env, err := app.Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki.validate-contract", Context: model.QueryOptions{Workspace: name, NoIndexRefresh: true}, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			results, ok := env.Items.([]model.WikiContractValidationResult)
			if !ok || len(results) != 1 {
				t.Fatalf("items=%#v", env.Items)
			}
			got := results[0]
			if got.ContractsReviewed < tc.contracts || got.LinksReviewed < 1 || got.TargetsMissing != 0 {
				t.Fatalf("scope must validate source and resolve target against full corpus: %+v", got)
			}
		})
	}
	lang := ResolveWikiContractLanguageAtRoot(root)
	if !lang.Declared || !lang.Valid || lang.Language != model.WikiContractLanguageES {
		t.Fatalf("effective governance frontmatter must win over later fence: %+v", lang)
	}
	t.Run("outside_scope_not_pass", func(t *testing.T) {
		env, err := app.Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki.validate-contract", Context: model.QueryOptions{Workspace: name, NoIndexRefresh: true}, Payload: map[string]any{"paths": "outside/absent.md"}})
		if err != nil {
			t.Fatal(err)
		}
		results, ok := env.Items.([]model.WikiContractValidationResult)
		if !ok || len(results) != 1 || results[0].ContractVerdict != "ADVISORY" || results[0].ContractsReviewed != 0 {
			t.Fatalf("empty scope must be advisory: %#v", env)
		}
	})
}

func TestA14AppUnknownSetupSchemaFailsClosed(t *testing.T) {
	root, name := a14CustomWiki(t)
	writeContractTestWikiFile(t, root, ".sandinas/project.yaml", "schema: sandinas-project/v99\n")
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.wiki.validate-contract", Context: model.QueryOptions{Workspace: name, NoIndexRefresh: true}})
	if err != nil {
		t.Fatal(err)
	}
	results, ok := env.Items.([]model.WikiContractValidationResult)
	if !ok || len(results) != 1 || results[0].ContractVerdict == "PASS" || !strings.Contains(strings.Join(results[0].ContractWarnings, " "), "setup_schema_unsupported") {
		t.Fatalf("unknown schema must remain closed: %#v", env)
	}
}

func TestA14RetirementsExplicitNoneAndNegativeControls(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/00_retirements.md"
	for _, tc := range []struct {
		name, body   string
		absent, pass bool
	}{
		{name: "none_prose", body: "# Registro de retiros\n\nNo hay retiros registrados.\n", pass: true},
		{name: "none_yaml", body: "```yaml\nretirements: []\n```\n", pass: true},
		{name: "absent", absent: true},
		{name: "empty"},
		{name: "ambiguous", body: "# Registro\nSin novedades por ahora.\n"},
		{name: "bad_yaml", body: "```yaml\nentries: [\n```\n"},
		{name: "not_list", body: "```yaml\nentries: {}\n```\n"},
		{name: "not_mapping", body: "```yaml\nentries: [oops]\n```\n"},
		{name: "contradiction", body: "No hay retiros registrados.\n\n```yaml\nentries:\n  - id: RF-OLD-01\n```\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.absent {
				writeContractTestWikiFile(t, root, path, tc.body)
			} else {
				_ = os.Remove(root + "/" + path)
			}
			got := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
			if (got.ContractVerdict == "PASS") != tc.pass || (tc.pass && got.ContractsReviewed != 1) {
				t.Fatalf("unexpected retirement result: %+v", got)
			}
		})
	}
}

func TestA14LegacyRetirementsYAMLAndEntrySemantics(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/00_retirements.md"
	archived := ".docs/wiki/_retired/requirement/RF-OLD-01.md"
	writeContractTestWikiFile(t, root, archived, "# Archivo\n")
	successor := ".docs/wiki/07_technical.md"
	writeContractTestWikiFile(t, root, successor, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TECH-X-01\nkind: technical\nstatus: active\n---\n## Resumen\nx\n## Mapa de componentes\nx\n")
	row := "  - id: RF-OLD-01\n    old_path: .docs/wiki/05_requirements/RF-OLD-01.md\n    archived_path: " + archived + "\n    superseded_by: TECH-X-01\n    terminal: false\n    reason: Reemplazado\n"
	for _, tc := range []struct {
		name, body string
		pass       bool
	}{
		{"legacy_retirements", "```yaml\nretirements:\n" + row + "```\n", true},
		{"entries_terminal", "```yaml\nentries:\n" + strings.Replace(strings.Replace(row, "superseded_by: TECH-X-01", "superseded_by: null", 1), "terminal: false", "terminal: true", 1) + "```\n", true},
		{"entries_missing_successor", "```yaml\nentries:\n" + strings.Replace(row, "TECH-X-01", "TECH-MISSING", 1) + "```\n", false},
		{"entries_nonterminal_none", "```yaml\nentries:\n" + strings.Replace(row, "TECH-X-01", "null", 1) + "```\n", false},
		{"entries_wrong_boolean", "```yaml\nentries:\n" + strings.Replace(row, "terminal: false", "terminal: 'false'", 1) + "```\n", false},
		{"entries_escape", "```yaml\nentries:\n" + strings.Replace(row, archived, "../../outside.md", 1) + "```\n", false},
		{"entries_missing_archive", "```yaml\nentries:\n" + strings.Replace(row, archived, ".docs/wiki/_retired/requirement/MISSING.md", 1) + "```\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeContractTestWikiFile(t, root, path, tc.body)
			got := compileWikiContractValidation(root, []model.DocRecord{{Path: path}, {Path: successor}}, nil)
			if (got.ContractVerdict == "PASS") != tc.pass || got.ContractsReviewed == 0 {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

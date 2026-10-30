package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

func TestMachineRetirementsIgnoreHarnessFences(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/00_retirements.md"
	archived := ".docs/wiki/_retired/requirement/RF-OLD.md"
	writeContractTestWikiFile(t, root, archived, "# old\n")
	inventory := "- id: RF-OLD\n  old_path: .docs/wiki/05_requirements/RF-OLD.md\n  archived_path: " + archived + "\n  terminal: true\n  reason: moved\n"
	index := wikisource.BuildContractWikiIDIndex(nil)
	baseline := validateWikiContractRetirements(root, path, inventory, index)
	if len(baseline) != 0 {
		t.Fatalf("baseline: %v", baseline)
	}
	missing := strings.Replace(inventory, archived, ".docs/wiki/_retired/requirement/MISSING.md", 1)
	baselineMissing := strings.Join(validateWikiContractRetirements(root, path, missing, index), "\n")
	if !strings.Contains(baselineMissing, "archived_path missing") {
		t.Fatal(baselineMissing)
	}
	for _, tc := range []struct {
		name, metadata string
		invalid        bool
	}{
		{"valid", "```yaml\nharness_protocol: SDD-HARNESS-v1\nagent_must_read: [foo]\n```\n", false},
		{"invalid", "```yaml\nharness_protocol: [broken\nagent_must_read: [foo]\n```\n", true},
		{"unclosed", "```yaml\nharness_protocol: SDD-HARNESS-v1\nagent_must_read: [foo]\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, row := range []string{inventory, missing} {
				warnings := validateWikiContractRetirements(root, path, row+"\n"+tc.metadata, index)
				joined := strings.Join(warnings, "\n")
				if strings.Contains(joined, "invalid_harness_metadata") != tc.invalid {
					t.Fatalf("metadata warning: %v", warnings)
				}
				var filtered []string
				for _, w := range warnings {
					if !strings.Contains(w, "invalid_harness_metadata") {
						filtered = append(filtered, w)
					}
				}
				want := ""
				if row == missing {
					want = baselineMissing
				}
				if strings.Join(filtered, "\n") != want {
					t.Fatalf("inventory warnings changed: got %v want %q", filtered, want)
				}
			}
		})
	}
	missingArchive := strings.Replace(inventory, archived, ".docs/wiki/_retired/requirement/MISSING.md", 1)
	missingWarnings := validateWikiContractRetirements(root, path, missingArchive+"\n```yaml\nharness_protocol: SDD-HARNESS-v1\n```\n", index)
	if !strings.Contains(strings.Join(missingWarnings, " "), "archived_path missing") {
		t.Fatalf("retirement row disappeared: %v", missingWarnings)
	}
	dual := validateWikiContractRetirements(root, path, "- id: [broken\n\n```yaml\nharness_protocol: [broken\n```\n", index)
	if !strings.Contains(strings.Join(dual, " "), "invalid YAML") || !strings.Contains(strings.Join(dual, " "), "invalid_harness_metadata") {
		t.Fatalf("dual error lost: %v", dual)
	}
	bad := validateWikiContractRetirements(root, path, "- id: [broken\n\n```yaml\nharness_protocol: SDD-HARNESS-v1\n```\n", index)
	if !strings.Contains(strings.Join(bad, " "), "invalid YAML") {
		t.Errorf("invalid inventory concealed: %v", bad)
	}
}

func TestMachineRetiredReferenceObservation(t *testing.T) {
	root := t.TempDir()
	gov := ".docs/wiki/00_governance.md"
	retirements := ".docs/wiki/00_retirements.md"
	archive := ".docs/wiki/_retired/requirement/RF-OLD.md"
	tp := ".docs/wiki/06_tests/TP-ACTIVE.md"
	writeContractTestWikiFile(t, root, gov, "```yaml\nwiki_contract: WIKI-CONTRACT-v1\nlanguage: es\n```\n")
	writeContractTestWikiFile(t, root, archive, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: retired\n---\n")
	writeContractTestWikiFile(t, root, tp, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n")
	row := "- id: RF-OLD\n  old_path: .docs/wiki/05_requirements/RF-OLD.md\n  archived_path: " + archive + "\n  terminal: true\n  reason: moved\n"
	successor := ".docs/wiki/05_requirements/RF-NEW.md"
	writeContractTestWikiFile(t, root, successor, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-NEW\nkind: requirement\nstatus: active\nlinks:\n  implements: [FL-X]\n---\n")
	for _, inventory := range []string{row, strings.Replace(row, "terminal: true", "terminal: false\n  superseded_by: RF-NEW", 1)} {
		for _, metadata := range []string{"", "\n```yaml\nharness_protocol: SDD-HARNESS-v1\n```\n", "\n```yaml\nharness_protocol: [broken\n```\n"} {
			writeContractTestWikiFile(t, root, retirements, inventory+metadata)
			index := []model.DocRecord{{Path: gov}, {Path: retirements}, {Path: archive, DocID: "RF-OLD"}, {Path: successor, DocID: "RF-NEW"}, {Path: tp, DocID: "TP-ACTIVE"}}
			result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp, DocID: "TP-ACTIVE"}}, index, nil)
			if result.TargetsMissing != 0 || !strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_retired:TP-ACTIVE->verifies->RF-OLD") {
				t.Errorf("successor=%v metadata=%q targets_missing=%d warnings=%v", inventory != row, metadata, result.TargetsMissing, result.ContractWarnings)
			}
		}
	}
	writeContractTestWikiFile(t, root, tp, strings.ReplaceAll("---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-NEW]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n", "unused", "unused"))
	index := []model.DocRecord{{Path: archive, DocID: "RF-OLD"}, {Path: successor, DocID: "RF-NEW"}, {Path: tp, DocID: "TP-ACTIVE"}}
	result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
	if strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_retired:") {
		t.Fatalf("active successor rejected: %v", result.ContractWarnings)
	}
}

func TestMachineLifecycleIndexedUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		missing    bool
	}{
		{"no_header", "legacy text\n", false},
		{"invalid_yaml", "---\nwiki_contract: [broken\n---\n", false},
		{"invalid_protocol", "---\nwiki_contract: NOT-WIKI\nid: RF-OLD\nkind: requirement\nstatus: active\n---\n", false},
		{"missing_status", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\n---\n", false},
		{"unreadable", "", true},
		{"id_mismatch", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OTHER\nkind: requirement\nstatus: active\n---\n", false},
	} {
		for _, duplicate := range []bool{false, true} {
			t.Run(tc.name+fmt.Sprint(duplicate), func(t *testing.T) {
				root := t.TempDir()
				tp := ".docs/wiki/06_tests/TP-ACTIVE.md"
				unknown := ".docs/wiki/05_requirements/unknown.md"
				current := ".docs/wiki/05_requirements/current.md"
				writeContractTestWikiFile(t, root, tp, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n")
				if !tc.missing {
					writeContractTestWikiFile(t, root, unknown, tc.body)
				}
				index := []model.DocRecord{{Path: tp, DocID: "TP-ACTIVE"}, {Path: unknown, DocID: "RF-OLD"}}
				if duplicate {
					writeContractTestWikiFile(t, root, current, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: active\n---\n")
					index = append(index, model.DocRecord{Path: current, DocID: "RF-OLD"})
				}
				result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
				if result.TargetsMissing != 0 || !strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_ambiguous:TP-ACTIVE->verifies->RF-OLD") {
					t.Fatalf("unknown omitted: %+v", result)
				}
			})
		}
	}
}

func TestMachineSelectedExternalWikiRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	external := filepath.Join(base, "authorized-wiki")
	sibling := filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	writeContractTestWikiFile(t, root, ".sandinas/project.yaml", "schema: sandinas-project/v1\nwiki:\n  setup_source: azure_wiki\n")
	writeContractTestWikiFile(t, root, ".sandinas/project.local.yaml", "paths:\n  wiki_checkout: ../authorized-wiki\n")
	writeContractTestWikiFile(t, external, "00_governance.md", "```yaml\nwiki_contract: WIKI-CONTRACT-v1\nlanguage: es\n```\n")
	tp := "../authorized-wiki/06_tests/TP-ACTIVE.md"
	old := "../authorized-wiki/_retired/requirement/RF-OLD.md"
	writeContractTestWikiFile(t, external, "06_tests/TP-ACTIVE.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n")
	writeContractTestWikiFile(t, external, "_retired/requirement/RF-OLD.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: retired\n---\n")
	writeContractTestWikiFile(t, sibling, "RF-ESCAPE.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-ESCAPE\nkind: requirement\nstatus: active\n---\n")
	roots, err := docgraph.CanonicalWikiRoots(root)
	if err != nil || len(roots) != 1 {
		t.Fatalf("roots=%v err=%v", roots, err)
	}
	all := []model.DocRecord{{Path: tp, DocID: "TP-ACTIVE"}, {Path: old, DocID: "RF-OLD"}, {Path: "../sibling/RF-ESCAPE.md", DocID: "RF-ESCAPE"}}
	filtered := filterGovernedWikiRecordsInRoots(root, all, roots)
	if len(filtered) != 2 {
		t.Fatalf("filtered=%+v roots=%v", filtered, roots)
	}
	result := compileWikiContractValidationInRoots(root, filtered[:1], filtered, nil, roots)
	if !strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_retired:TP-ACTIVE->verifies->RF-OLD") {
		t.Fatalf("selected external wiki rejected: %+v", result)
	}
	active := "../authorized-wiki/05_requirements/RF-OLD.md"
	writeContractTestWikiFile(t, external, "05_requirements/RF-OLD.md", "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: active\n---\n")
	filtered = append(filtered, model.DocRecord{Path: active, DocID: "RF-OLD"})
	result = compileWikiContractValidationInRoots(root, filtered[:1], filtered, nil, roots)
	if strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_retired:") || result.TargetsMissing != 0 {
		t.Fatalf("active target rejected: %+v", result)
	}
	link := filepath.Join(external, "05_requirements/escape.md")
	if err := os.Symlink(filepath.Join(sibling, "RF-ESCAPE.md"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	escape := model.DocRecord{Path: "../authorized-wiki/05_requirements/escape.md", DocID: "RF-ESCAPE"}
	if got := filterGovernedWikiRecordsInRoots(root, []model.DocRecord{escape}, roots); len(got) != 0 {
		t.Fatalf("escape entered governed filter: %+v", got)
	}
	if _, reason := wikiContractContainedPath(root, escape.Path, roots); reason != "outside_root" {
		t.Fatalf("escape reason=%q", reason)
	}
	incoming := filepath.Join(sibling, "incoming.md")
	if err := os.Symlink(filepath.Join(external, "05_requirements/RF-OLD.md"), incoming); err != nil {
		t.Fatal(err)
	}
	if _, reason := wikiContractContainedPath(root, "../sibling/incoming.md", roots); reason != "outside_root" {
		t.Fatalf("undeclared sibling alias reason=%q", reason)
	}
}

func TestMachineExternalUnidentifiedBodyAndContradictoryIDs(t *testing.T) {
	root := t.TempDir()
	tp := ".docs/wiki/06_tests/TP-ACTIVE.md"
	target := ".docs/wiki/05_requirements/target.md"
	external := ".docs/wiki/05_requirements/external.md"
	writeContractTestWikiFile(t, root, tp, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n")
	body := "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: active\n---\n"
	writeContractTestWikiFile(t, root, target, body)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, external)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, duplicate := range []bool{false, true} {
		index := []model.DocRecord{{Path: tp, DocID: "TP-ACTIVE"}, {Path: external}}
		if duplicate {
			index = append(index, model.DocRecord{Path: target, DocID: "RF-OLD"})
		}
		result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
		joined := strings.Join(result.ContractWarnings, " ")
		if !strings.Contains(joined, "index_body_untrusted:") {
			t.Errorf("untrusted path silent: %v", result.ContractWarnings)
		}
		if !duplicate && result.TargetsMissing == 0 {
			t.Errorf("external body introduced presence: %+v", result)
		}
	}
	writeContractTestWikiFile(t, root, target, strings.Replace(body, "RF-OLD", "RF-BODY", 1))
	index := []model.DocRecord{{Path: tp, DocID: "TP-ACTIVE"}, {Path: target, DocID: "RF-OLD"}}
	for _, id := range []string{"RF-OLD", "RF-BODY"} {
		writeContractTestWikiFile(t, root, tp, strings.Replace("---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n", "RF-OLD", id, 1))
		result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
		if id == "RF-OLD" && result.TargetsMissing != 0 {
			t.Errorf("indexed identity incorrectly missing: %+v", result)
		}
		if !strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_ambiguous:TP-ACTIVE->verifies->"+id) {
			t.Errorf("contradictory %s silently trusted: %+v", id, result)
		}
	}
}

func TestMachineLifecycleSymlinkIdentity(t *testing.T) {
	root := t.TempDir()
	tp := ".docs/wiki/06_tests/TP-ACTIVE.md"
	target := ".docs/wiki/05_requirements/RF-OLD.md"
	alias := ".docs/wiki/05_requirements/alias.md"
	outside := filepath.Join(t.TempDir(), "external.md")
	writeContractTestWikiFile(t, root, tp, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n")
	body := "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: active\n---\n"
	writeContractTestWikiFile(t, root, target, body)
	if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, alias)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	index := []model.DocRecord{{Path: tp, DocID: "TP-ACTIVE"}, {Path: target, DocID: "RF-OLD"}, {Path: alias, DocID: "RF-OLD"}}
	r := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
	if strings.Contains(strings.Join(r.ContractWarnings, " "), "link_target_ambiguous:") {
		t.Fatalf("same physical file misidentified: %v", r.ContractWarnings)
	}
	if err := os.WriteFile(outside, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(root, ".docs/wiki/05_requirements/external.md")
	if err := os.Symlink(outside, external); err != nil {
		t.Fatal(err)
	}
	index = append(index, model.DocRecord{Path: ".docs/wiki/05_requirements/external.md", DocID: "RF-OLD"})
	r = compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
	if !strings.Contains(strings.Join(r.ContractWarnings, " "), "link_target_ambiguous:") {
		t.Fatalf("outside target trusted: %v", r.ContractWarnings)
	}
}

func TestMachineActiveRFRetiredFlow(t *testing.T) {
	root := t.TempDir()
	rf := ".docs/wiki/05_requirements/RF-ACTIVE.md"
	flow := ".docs/wiki/_retired/flow/FL-OLD.md"
	writeContractTestWikiFile(t, root, rf, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-ACTIVE\nkind: requirement\nstatus: active\nlinks:\n  implements: [FL-OLD]\n---\n")
	writeContractTestWikiFile(t, root, flow, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: FL-OLD\nkind: flow\nstatus: retired\n---\n")
	index := []model.DocRecord{{Path: rf, DocID: "RF-ACTIVE"}, {Path: flow, DocID: "FL-OLD"}}
	result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: rf}}, index, nil)
	if result.TargetsMissing != 0 || !strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_retired:RF-ACTIVE->implements->FL-OLD") {
		t.Fatalf("retired flow not reported: %+v", result)
	}
}

func TestMachineLifecycleCurrentIdentityAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	tp := ".docs/wiki/06_tests/TP-ACTIVE.md"
	archive := ".docs/wiki/_retired/requirement/RF-OLD.md"
	current := ".docs/wiki/05_requirements/RF-OLD.md"
	other := ".docs/wiki/05_requirements/other.md"
	writeContractTestWikiFile(t, root, tp, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: TP-ACTIVE\nkind: test\nstatus: active\nlinks:\n  verifies: [RF-OLD]\n---\n## Alcance\nx\n## Casos\n- Criterio 1 — disposición: manual; evidencia: x\n")
	writeContractTestWikiFile(t, root, archive, "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: retired\n---\n")
	for _, status := range []string{"active", "deprecated"} {
		body := "---\nwiki_contract: WIKI-CONTRACT-v1\nid: RF-OLD\nkind: requirement\nstatus: " + status + "\n---\n"
		writeContractTestWikiFile(t, root, current, body)
		index := []model.DocRecord{{Path: archive, DocID: "RF-OLD"}, {Path: current, DocID: "RF-OLD"}, {Path: tp, DocID: "TP-ACTIVE"}}
		result := compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
		if strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_retired:") || strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_ambiguous:") {
			t.Fatalf("%s current incorrectly rejected: %v", status, result.ContractWarnings)
		}
		writeContractTestWikiFile(t, root, other, body)
		index = append(index, model.DocRecord{Path: other, DocID: "RF-OLD"})
		result = compileWikiContractValidationWithIndexRecords(root, []model.DocRecord{{Path: tp}}, index, nil)
		if !strings.Contains(strings.Join(result.ContractWarnings, " "), "link_target_ambiguous:TP-ACTIVE->verifies->RF-OLD") {
			t.Fatalf("ambiguous target accepted: %v", result.ContractWarnings)
		}
	}
}

func TestMachineGovernanceDecoysAreNotExempt(t *testing.T) {
	root := t.TempDir()
	governance := ".docs/wiki/00_governance.md"
	machine := "```yaml\nwiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: es\n```\n"
	writeContractTestWikiFile(t, root, governance, machine)
	for _, path := range []string{".docs/wiki/05_requirements/not00_governance.md", ".docs/wiki/05_requirements/00_governance.md", ".docs/wiki/_retired/governance/00_governance.md"} {
		writeContractTestWikiFile(t, root, path, machine)
		result := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
		warnings := strings.Join(result.ContractWarnings, " ")
		for _, key := range []string{"missing id", "missing kind", "missing status"} {
			if !strings.Contains(warnings, key) {
				t.Errorf("%s exempted %s: %s", path, key, warnings)
			}
		}
		writeContractTestWikiFile(t, root, path, "no header\n")
		result = compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
		if !strings.Contains(strings.Join(result.ContractWarnings, " "), "missing WIKI-CONTRACT-v1 header") {
			t.Errorf("%s missing header exempted: %v", path, result.ContractWarnings)
		}
	}
}

func TestMachineGovernanceSelectedSourceAndUnresolvedSetup(t *testing.T) {
	root := t.TempDir()
	selected := "wiki/custom-governance.md"
	machine := "```yaml\nwiki_contract: WIKI-CONTRACT-v1\nlanguage: es\n```\n"
	writeContractTestWikiFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n[governance]\nsource_doc = 'wiki/custom-governance.md'\n")
	writeContractTestWikiFile(t, root, ".mi-lsp/project.toml", "[[canon]]\nid = 'product'\nrole = 'producto'\nroot = 'wiki'\n")
	writeContractTestWikiFile(t, root, selected, machine)
	if got := ResolveWikiContractLanguageAtRoot(root); got.Source != selected {
		t.Fatalf("source=%q warnings=%v", got.Source, got.Warnings)
	}
	result := compileWikiContractValidation(root, []model.DocRecord{{Path: selected}}, nil)
	if strings.Contains(strings.Join(result.ContractWarnings, " "), "missing kind") {
		t.Fatalf("selected source rejected: %v", result.ContractWarnings)
	}
	writeContractTestWikiFile(t, root, ".sandinas/project.yaml", "schema: sandinas-project/v99\n")
	result = compileWikiContractValidation(root, []model.DocRecord{{Path: selected}}, nil)
	if !strings.Contains(strings.Join(result.ContractWarnings, " "), "untrusted markdown") {
		t.Fatalf("unresolved setup did not fail closed: %v", result.ContractWarnings)
	}
}

func TestMachineGovernanceDoesNotRequireArtifactHeader(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/00_governance.md"
	writeContractTestWikiFile(t, root, path, "```yaml\nwiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: es\ntraits:\n  stores_data: SQLite\nlayer_exceptions: []\n```\n")
	lang := ResolveWikiContractLanguageAtRoot(root)
	if !lang.Valid || string(lang.Language) != "es" {
		t.Fatalf("machine governance language lost: %+v", lang)
	}
	result := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
	for _, w := range result.ContractWarnings {
		if strings.Contains(w, "missing id") || strings.Contains(w, "missing kind") || strings.Contains(w, "missing status") {
			t.Errorf("machine governance treated as artifact: %v", result.ContractWarnings)
		}
	}
	writeContractTestWikiFile(t, root, path, "```yaml\nwiki_contract: WIKI-CONTRACT-v1\nlanguage: [broken\n```\n")
	bad := compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
	if !strings.Contains(strings.Join(bad.ContractWarnings, " "), "invalid header YAML") {
		t.Fatalf("invalid governance YAML concealed: %v", bad.ContractWarnings)
	}
	header, keys, _ := parseWikiContractHeader("---\nwiki_contract: WIKI-CONTRACT-v1\nkind: requirement\n---\n")
	warnings := strings.Join(validateWikiContractHeader("rf", header, keys, nil), " ")
	if !strings.Contains(warnings, "missing id") || !strings.Contains(warnings, "missing status") {
		t.Fatalf("RF guard lost: %s", warnings)
	}
}

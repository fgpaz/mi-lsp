package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestValidateHarnessValidLLMFirstContract(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-HARNESS.md", validHarnessContract("llm-first", "CT-HARNESS", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(".docs/wiki/09_contratos/CT-HARNESS.md", "CT-HARNESS")})

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "PASS" || result.HarnessReadiness != "ready" {
		t.Fatalf("unexpected verdict: %#v", result)
	}
	if result.HarnessContractsReviewed != 1 {
		t.Fatalf("contracts reviewed = %d, want 1", result.HarnessContractsReviewed)
	}
	if len(result.HarnessEvidenceFound) != 1 || result.HarnessEvidenceFound[0] != "artifacts/harness/evidence.md" {
		t.Fatalf("evidence found = %#v", result.HarnessEvidenceFound)
	}
}

func TestValidateHarnessMissingContractBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-HARNESS.md", "# CT-HARNESS\n\nNo contract yet.\n")
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(".docs/wiki/09_contratos/CT-HARNESS.md", "CT-HARNESS")})

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.HarnessVerdict)
	}
	if len(result.HarnessDocsMissingContract) != 1 || result.HarnessDocsMissingContract[0] != "CT-HARNESS" {
		t.Fatalf("missing contract docs = %#v", result.HarnessDocsMissingContract)
	}
}

func TestValidateHarnessBrokenObsidianImportBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-HARNESS.md", validHarnessContract("llm-first", "CT-HARNESS", "artifacts/harness/evidence.md", "This points to [[MISSING-DOC]]."))
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(".docs/wiki/09_contratos/CT-HARNESS.md", "CT-HARNESS")})

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.HarnessVerdict)
	}
	if !strings.Contains(strings.Join(result.HarnessBlockers, " | "), "broken import/link MISSING-DOC") {
		t.Fatalf("expected broken Obsidian blocker, got %#v", result.HarnessBlockers)
	}
}

func TestValidateHarnessEditAllowDenyConflictBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	content := strings.Replace(validHarnessContract("llm-first", "CT-HARNESS", "artifacts/harness/evidence.md", ""), "  - .docs/wiki/00_gobierno_documental.md", "  - .docs/wiki/09_contratos/CT-HARNESS.md", 1)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-HARNESS.md", content)
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(".docs/wiki/09_contratos/CT-HARNESS.md", "CT-HARNESS")})

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.HarnessVerdict)
	}
	if !strings.Contains(strings.Join(result.HarnessBlockers, " | "), "edit allow/deny conflict") {
		t.Fatalf("expected edit conflict blocker, got %#v", result.HarnessBlockers)
	}
}

func TestValidateHarnessHumanAndDualContractsMaySkipStrictRuntimeGates(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-HUMAN.md", relaxedHarnessContract("human", "CT-HUMAN"))
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-DUAL.md", relaxedHarnessContract("dual", "CT-DUAL"))
	replaceHarnessDocs(t, root, []model.DocRecord{
		harnessDocRecord(".docs/wiki/09_contratos/CT-HUMAN.md", "CT-HUMAN"),
		harnessDocRecord(".docs/wiki/09_contratos/CT-DUAL.md", "CT-DUAL"),
	})

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "WARN" || len(result.HarnessBlockers) != 0 {
		t.Fatalf("verdict = %q, blockers=%#v, warnings=%#v", result.HarnessVerdict, result.HarnessBlockers, result.HarnessWarnings)
	}
	if len(result.HarnessWarnings) == 0 {
		t.Fatalf("expected non-blocking warnings for relaxed human/dual contracts")
	}
}

func TestValidateHarnessUnknownAudienceBlocksAndToonExposesFields(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-HARNESS.md", relaxedHarnessContract("", "CT-HARNESS"))
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(".docs/wiki/09_contratos/CT-HARNESS.md", "CT-HARNESS")})

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.validate-harness",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("nav.wiki.validate-harness: %v", err)
	}
	rendered, err := output.Render(env, "toon", false)
	if err != nil {
		t.Fatalf("render toon: %v", err)
	}
	text := string(rendered)
	for _, field := range []string{"harness_protocol", "harness_readiness", "harness_verdict", "harness_docs_unknown_audience"} {
		if !strings.Contains(text, field) {
			t.Fatalf("TOON output missing %s:\n%s", field, text)
		}
	}
}

func TestValidateHarnessScopedIDsFilterBeforeLoadingDocs(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-PILOT.md", validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-GLOBAL.md", "# CT-GLOBAL\n\nNo contract yet.\n")
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{
		harnessDocRecord(".docs/wiki/09_contratos/CT-PILOT.md", "CT-PILOT"),
		harnessDocRecord(".docs/wiki/09_contratos/CT-GLOBAL.md", "CT-GLOBAL"),
	})

	result := executeHarnessValidationPayload(t, root, alias, map[string]any{"ids": "ct-pilot"})
	if result.HarnessVerdict != "PASS" || result.HarnessContractsReviewed != 1 {
		t.Fatalf("scoped verdict = %#v, want one passing contract", result)
	}
}

func TestValidateHarnessScopedIDsPreferCanonicalPathOverAggregateRecord(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-PILOT.md", validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos_tecnicos.md", "# Index\n\nMentions CT-PILOT without owning its harness contract.\n")
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	aggregate := harnessDocRecord(".docs/wiki/09_contratos_tecnicos.md", "CT-PILOT")
	aggregate.Title = "CT-PILOT"
	replaceHarnessDocs(t, root, []model.DocRecord{
		aggregate,
		harnessDocRecord(".docs/wiki/09_contratos/CT-PILOT.md", "CT-PILOT"),
	})

	result := executeHarnessValidationPayload(t, root, alias, map[string]any{"ids": "CT-PILOT"})
	if result.HarnessVerdict != "PASS" || result.HarnessContractsReviewed != 1 {
		t.Fatalf("scoped verdict = %#v, want only canonical passing contract", result)
	}
}

func TestValidateHarnessScopedPathsFilterByBasename(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-PILOT.md", validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-GLOBAL.md", "# CT-GLOBAL\n\nNo contract yet.\n")
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{
		harnessDocRecord(".docs/wiki/09_contratos/CT-PILOT.md", "CT-PILOT"),
		harnessDocRecord(".docs/wiki/09_contratos/CT-GLOBAL.md", "CT-GLOBAL"),
	})

	result := executeHarnessValidationPayload(t, root, alias, map[string]any{"paths": "CT-PILOT.md"})
	if result.HarnessVerdict != "PASS" || result.HarnessContractsReviewed != 1 {
		t.Fatalf("scoped verdict = %#v, want one passing contract", result)
	}
}

func TestValidateHarnessScopedNoMatchesBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	writeHarnessDoc(t, root, ".docs/wiki/09_contratos/CT-PILOT.md", validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(".docs/wiki/09_contratos/CT-PILOT.md", "CT-PILOT")})

	result := executeHarnessValidationPayload(t, root, alias, map[string]any{"ids": "CT-MISSING"})
	if result.HarnessVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.HarnessVerdict)
	}
	if !strings.Contains(strings.Join(result.HarnessBlockers, " | "), "matched no indexed wiki docs") {
		t.Fatalf("expected scoped no-match blocker, got %#v", result.HarnessBlockers)
	}
}

func createHarnessWorkspace(t *testing.T) (string, string) {
	t.Helper()
	alias := "harness-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	return alias, root
}

func replaceHarnessDocs(t *testing.T, root string, docs []model.DocRecord) {
	t.Helper()
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := store.ReplaceDocs(context.Background(), db, docs, nil, nil); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
}

func executeHarnessValidation(t *testing.T, root string, alias string) model.HarnessValidationResult {
	t.Helper()
	return executeHarnessValidationPayload(t, root, alias, map[string]any{})
}

func executeHarnessValidationPayload(t *testing.T, root string, alias string, payload map[string]any) model.HarnessValidationResult {
	t.Helper()
	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.validate-harness",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("nav.wiki.validate-harness: %v", err)
	}
	results, ok := env.Items.([]model.HarnessValidationResult)
	if !ok || len(results) != 1 {
		t.Fatalf("unexpected items: %#v", env.Items)
	}
	return results[0]
}

func writeHarnessDoc(t *testing.T, root string, path string, content string) {
	t.Helper()
	writeWorkspaceFile(t, root, path, "# "+strings.TrimSuffix(filepath.Base(path), ".md")+"\n\n"+content)
}

func harnessDocRecord(path string, docID string) model.DocRecord {
	return model.DocRecord{
		Path:       path,
		Title:      docID,
		DocID:      docID,
		Layer:      "09",
		Family:     "technical",
		SearchText: docID + " SDD-HARNESS-v1",
		IndexedAt:  1,
	}
}

func validHarnessContract(audience string, id string, evidence string, body string) string {
	return strings.Join([]string{
		"```yaml",
		"harness_protocol: SDD-HARNESS-v1",
		"id: " + id,
		"kind: contract",
		"audience: " + audience,
		"imports:",
		"  - '[[" + id + "]]'",
		"exports:",
		"  - " + id,
		"agent_must_read:",
		"  - .docs/wiki/09_contratos/" + id + ".md",
		"agent_may_edit:",
		"  - .docs/wiki/09_contratos/" + id + ".md",
		"agent_must_not_edit:",
		"  - .docs/wiki/00_gobierno_documental.md",
		"verify:",
		"  - go test ./internal/service",
		"stop_if:",
		"  - governance_blocked=true",
		"evidence:",
		"  - " + evidence,
		"```",
		"",
		body,
	}, "\n")
}

func relaxedHarnessContract(audience string, id string) string {
	return strings.Join([]string{
		"```yaml",
		"harness_protocol: SDD-HARNESS-v1",
		"id: " + id,
		"kind: contract",
		"audience: " + audience,
		"imports:",
		"  - '[[" + id + "]]'",
		"exports:",
		"  - " + id,
		"agent_must_read:",
		"  - .docs/wiki/09_contratos/" + id + ".md",
		"agent_may_edit:",
		"  - none",
		"agent_must_not_edit:",
		"  - .docs/wiki/00_gobierno_documental.md",
		"verify: []",
		"stop_if: []",
		"evidence: []",
		"```",
	}, "\n")
}

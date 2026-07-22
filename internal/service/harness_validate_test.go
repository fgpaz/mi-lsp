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

func TestHarnessRefExistsSupportsValidatedKernelV2Canon(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	if !kernelV2CanonReferenceExists(root, "<kernel_home>/canon/AE-KERNEL-V2.md") {
		t.Fatal("expected validated external kernel_v2 module reference to resolve")
	}
	if !kernelV2CanonReferenceExists(root, "ae_canon") {
		t.Fatal("expected ae_canon export to resolve for validated kernel_v2 governance")
	}
	if kernelV2CanonReferenceExists(root, "<kernel_home>/canon/UNKNOWN.md") {
		t.Fatal("unknown external kernel_v2 module must not resolve")
	}
}

func TestHarnessRefExistsSupportsCodeEvidencePath(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "internal/nav/fanout_wiki.go", "package nav\n")

	if !harnessRefExists(root, map[string]struct{}{}, ".docs/wiki/07_tech/TECH-WIKI-FANOUT.md", "internal/nav/fanout_wiki.go") {
		t.Fatalf("expected existing .go evidence path to resolve without appending .md")
	}
}

func TestHarnessRefExistsStripsCodeEvidenceLineRange(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "internal/service/ask.go", "package service\n")

	if !harnessRefExists(root, map[string]struct{}{}, ".docs/wiki/07_tech/TECH-WIKI-FANOUT.md", "internal/service/ask.go:465-564") {
		t.Fatalf("expected existing .go evidence path with line range to resolve")
	}
}

func TestHarnessRefExistsKeepsExtensionlessDocRefs(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, ".docs/wiki/09_contratos/CT-HARNESS.md", "# CT-HARNESS\n")

	if !harnessRefExists(root, map[string]struct{}{}, ".docs/wiki/09_contratos/CT-INDEX.md", ".docs/wiki/09_contratos/CT-HARNESS") {
		t.Fatalf("expected extensionless wiki path to keep resolving through .md fallback")
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

func TestValidateHarnessScopedImportsUseCanonicalCorpus(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	pilotPath := ".docs/wiki/09_contratos/CT-PILOT.md"
	globalPath := ".docs/wiki/09_contratos/CT-GLOBAL.md"
	pilot := strings.Replace(validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""), "'[[CT-PILOT]]'", "'[[CT-GLOBAL]]'", 1)
	writeHarnessDoc(t, root, pilotPath, pilot)
	writeHarnessDoc(t, root, globalPath, validHarnessContract("llm-first", "CT-GLOBAL", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{
		harnessDocRecord(pilotPath, "CT-PILOT"),
		harnessDocRecord(globalPath, "CT-GLOBAL"),
	})

	result := executeHarnessValidationPayload(t, root, alias, map[string]any{"ids": "CT-PILOT"})
	if result.HarnessVerdict != "PASS" || result.HarnessContractsReviewed != 1 || result.HarnessLinksReviewed != 2 {
		t.Fatalf("scoped cross-document import result = %#v", result)
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

func TestValidateHarnessUnscopedPrefersCanonicalPathOverAggregateRecord(t *testing.T) {
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

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "PASS" || result.HarnessContractsReviewed != 1 {
		t.Fatalf("unscoped verdict = %#v, want only canonical passing contract", result)
	}
}

func TestValidateHarnessExcludesRawAndAuditRecords(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	wikiPath := ".docs/wiki/09_contratos/CT-PILOT.md"
	rawPath := ".docs/raw/CT-RAW.md"
	auditPath := ".docs/auditoria/session/CT-AUDIT.md"
	writeHarnessDoc(t, root, wikiPath, validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""))
	writeHarnessDoc(t, root, rawPath, "# invalid raw contract\n")
	writeHarnessDoc(t, root, auditPath, "# invalid audit contract\n")
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	replaceHarnessDocs(t, root, []model.DocRecord{harnessDocRecord(wikiPath, "CT-PILOT"), harnessDocRecord(rawPath, "CT-RAW"), harnessDocRecord(auditPath, "CT-AUDIT")})

	result := executeHarnessValidation(t, root, alias)
	if result.HarnessVerdict != "PASS" || result.HarnessContractsReviewed != 1 || len(result.HarnessDocsMissingContract) != 0 {
		t.Fatalf("raw/audit records leaked into validation: %#v", result)
	}
}

func TestValidateHarnessCanonicalInvalidRecordWinsDuplicate(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	canonicalPath := ".docs/wiki/09_contratos/CT-PILOT.md"
	aggregatePath := ".docs/wiki/09_contratos_tecnicos.md"
	writeWorkspaceFile(t, root, canonicalPath, "# CT-PILOT\n\nNo contract in canonical path.\n")
	writeHarnessDoc(t, root, aggregatePath, validHarnessContract("llm-first", "CT-PILOT", "artifacts/harness/evidence.md", ""))
	writeWorkspaceFile(t, root, "artifacts/harness/evidence.md", "verified")
	aggregate := harnessDocRecord(aggregatePath, "CT-PILOT")
	aggregate.Title = "CT-PILOT"
	replaceHarnessDocs(t, root, []model.DocRecord{aggregate, harnessDocRecord(canonicalPath, "CT-PILOT")})

	result := executeHarnessValidationPayload(t, root, alias, map[string]any{"ids": "CT-PILOT"})
	if result.HarnessVerdict != "BLOCKED" || result.HarnessContractsReviewed != 0 || len(result.HarnessDocsMissingContract) != 1 {
		t.Fatalf("canonical duplicate was not authoritative: %#v", result)
	}
}

func TestValidateHarnessScopedRecordsDeduplicateDeterministically(t *testing.T) {
	canonical := harnessDocRecord(".docs/wiki/09_contratos/CT-PILOT.md", "CT-PILOT")
	aggregate := harnessDocRecord(".docs/wiki/09_contratos_tecnicos.md", "CT-PILOT")
	got := filterGovernedWikiDocRecords([]model.DocRecord{aggregate, canonical, canonical}, map[string]any{})
	if len(got) != 1 || got[0].Path != canonical.Path {
		t.Fatalf("records = %#v, want one canonical record", got)
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

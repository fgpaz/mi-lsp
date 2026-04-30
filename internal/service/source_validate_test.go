package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestValidateSourceValidArtifact(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	writeWorkspaceFile(t, root, path, validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", ""))
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, []model.DocSourceRecord{sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016")})

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "PASS" || result.WikiSourceReadiness != "ready" || result.NavigationReadiness != "ready" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.WikiSourceArtifactsReviewed != 1 || result.WikiSourceBlocksReviewed != 1 || result.WikiSourceRecordsReviewed != 1 {
		t.Fatalf("review counts = %#v", result)
	}
}

func TestValidateSourceMissingBlockIDBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	writeWorkspaceFile(t, root, path, strings.Replace(validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", ""), "block_id: CT-SOURCE.contract\n", "", 1))
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, nil, nil)

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.WikiSourceVerdict)
	}
	if !strings.Contains(strings.Join(result.WikiSourceBlockers, " | "), "missing block_id") {
		t.Fatalf("expected missing block_id blocker, got %#v", result.WikiSourceBlockers)
	}
}

func TestValidateSourceMissingDocIDBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	writeWorkspaceFile(t, root, path, strings.Replace(validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", ""), "doc_id: CT-SOURCE\n", "", 1))
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "")}, nil, nil)

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.WikiSourceVerdict)
	}
	if !strings.Contains(strings.Join(result.WikiSourceBlockers, " | "), "missing doc_id") {
		t.Fatalf("expected missing doc_id blocker, got %#v", result.WikiSourceBlockers)
	}
}

func TestValidateSourceMissingKindAndSourceOfTruthBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	content := validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", "")
	content = strings.Replace(content, "kind: contract\n", "", 1)
	content = strings.Replace(content, "source_of_truth: CT-SOURCE\n", "", 1)
	writeWorkspaceFile(t, root, path, content)
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, []model.DocSourceRecord{sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016")})

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.WikiSourceVerdict)
	}
	blockers := strings.Join(result.WikiSourceBlockers, " | ")
	if !strings.Contains(blockers, "block missing kind") || !strings.Contains(blockers, "block missing source_of_truth") {
		t.Fatalf("expected kind/source_of_truth blockers, got %#v", result.WikiSourceBlockers)
	}
}

func TestValidateSourceRecordWithoutIDBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	content := strings.Replace(validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", ""), "id: RF-QRY-016", "records:\n  - type: RF\n    title: missing id", 1)
	writeWorkspaceFile(t, root, path, content)
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, nil)

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.WikiSourceVerdict)
	}
	if !strings.Contains(strings.Join(result.WikiSourceBlockers, " | "), "referencable record missing id") {
		t.Fatalf("expected missing record id blocker, got %#v", result.WikiSourceBlockers)
	}
}

func TestValidateSourceBrokenImportAndExportBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	content := validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", "")
	content = strings.Replace(content, "  - '[[00_gobierno_documental]]'", "  - '[[MISSING-DOC]]'", 1)
	content = strings.Replace(content, "  - CT-SOURCE", "  - MISSING-EXPORT", 1)
	writeWorkspaceFile(t, root, path, content)
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, []model.DocSourceRecord{sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016")})

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.WikiSourceVerdict)
	}
	blockers := strings.Join(result.WikiSourceBlockers, " | ")
	if !strings.Contains(blockers, "broken import") || !strings.Contains(blockers, "export not indexed") {
		t.Fatalf("expected import/export blockers, got %#v", result.WikiSourceBlockers)
	}
}

func TestValidateSourceNormativeTableWithoutExceptionBlocks(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	table := "| Campo | Valor |\n| --- | --- |\n| decision | source |\n"
	writeWorkspaceFile(t, root, path, validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", table))
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, []model.DocSourceRecord{sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016")})

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "BLOCKED" {
		t.Fatalf("verdict = %q, want BLOCKED", result.WikiSourceVerdict)
	}
	if !strings.Contains(strings.Join(result.WikiSourceBlockers, " | "), "normative Markdown table") {
		t.Fatalf("expected table blocker, got %#v", result.WikiSourceBlockers)
	}
}

func TestValidateSourceTableExceptionWithToonSourcePasses(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	table := "wiki_source_table_exception: true\n\n| Campo | Valor |\n| --- | --- |\n| decision | source |\n"
	writeWorkspaceFile(t, root, path, validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", table))
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, []model.DocSourceRecord{sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016")})

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "PASS" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Documents) != 1 || !strings.Contains(strings.Join(result.Documents[0].Exceptions, " | "), "wiki_source_table_exception") {
		t.Fatalf("expected table exception detail, got %#v", result.Documents)
	}
}

func TestValidateSourceUnmigratedDocsIgnored(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-LEGACY.md"
	writeWorkspaceFile(t, root, path, "# CT-LEGACY\n\nNo source protocol yet.\n")
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-LEGACY")}, nil, nil)

	result := executeSourceValidation(t, root, alias)
	if result.WikiSourceVerdict != "PASS" || result.WikiSourceReadiness != "not_declared" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestValidateSourceToonOutputExposesFields(t *testing.T) {
	alias, root := createHarnessWorkspace(t)
	path := ".docs/wiki/09_contratos/CT-SOURCE.md"
	writeWorkspaceFile(t, root, path, validSourceDoc("CT-SOURCE", "CT-SOURCE.contract", "RF-QRY-016", "llm-first", ""))
	replaceSourceDocs(t, root, []model.DocRecord{sourceDocRecord(path, "CT-SOURCE")}, []model.DocSourceBlock{sourceBlockRecord(path, "CT-SOURCE", "CT-SOURCE.contract")}, []model.DocSourceRecord{sourceRecord(path, "CT-SOURCE.contract", "RF-QRY-016")})

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.validate-source",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("nav.wiki.validate-source: %v", err)
	}
	rendered, err := output.Render(env, "toon", false)
	if err != nil {
		t.Fatalf("render toon: %v", err)
	}
	text := string(rendered)
	for _, field := range []string{"wiki_source_protocol", "wiki_source_readiness", "wiki_source_verdict", "navigation_readiness", "documents", "source_protocol", "blocks", "records"} {
		if !strings.Contains(text, field) {
			t.Fatalf("TOON output missing %s:\n%s", field, text)
		}
	}
}

func executeSourceValidation(t *testing.T, root string, alias string) model.WikiSourceValidationResult {
	t.Helper()
	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki.validate-source",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("nav.wiki.validate-source: %v", err)
	}
	results, ok := env.Items.([]model.WikiSourceValidationResult)
	if !ok || len(results) != 1 {
		t.Fatalf("unexpected items: %#v", env.Items)
	}
	return results[0]
}

func replaceSourceDocs(t *testing.T, root string, docs []model.DocRecord, blocks []model.DocSourceBlock, records []model.DocSourceRecord) {
	t.Helper()
	docs = append(docs, model.DocRecord{
		Path:       ".docs/wiki/00_gobierno_documental.md",
		Title:      "00 gobierno documental",
		DocID:      "00_gobierno_documental",
		Layer:      "00",
		Family:     "functional",
		SearchText: "00_gobierno_documental",
		IndexedAt:  1,
	})
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := store.ReplaceDocsWithSources(context.Background(), db, docs, nil, nil, blocks, records); err != nil {
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}
}

func sourceDocRecord(path string, docID string) model.DocRecord {
	return model.DocRecord{
		Path:       path,
		Title:      docID,
		DocID:      docID,
		Layer:      "09",
		Family:     "technical",
		SearchText: docID + " SDD-WIKI-SOURCE-v1",
		IndexedAt:  1,
	}
}

func sourceBlockRecord(path string, docID string, blockID string) model.DocSourceBlock {
	return model.DocSourceBlock{
		DocPath:      path,
		BlockID:      blockID,
		DocID:        docID,
		Kind:         "contract",
		SourceFormat: "SDD-WIKI-SOURCE-v1",
		Ordinal:      1,
		StartLine:    8,
		EndLine:      14,
		IndexedAt:    1,
	}
}

func sourceRecord(path string, blockID string, recordID string) model.DocSourceRecord {
	return model.DocSourceRecord{
		DocPath:    path,
		BlockID:    blockID,
		RecordID:   recordID,
		RecordType: strings.Split(recordID, "-")[0],
		Ordinal:    1,
		StartLine:  10,
		EndLine:    13,
		IndexedAt:  1,
	}
}

func validSourceDoc(docID string, blockID string, recordID string, audience string, tail string) string {
	return strings.Join([]string{
		"# " + strings.TrimSuffix(filepath.Base(docID+".md"), ".md"),
		"",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"harness_protocol: SDD-HARNESS-v1",
		"doc_id: " + docID,
		"audience: " + audience,
		"imports:",
		"  - '[[00_gobierno_documental]]'",
		"exports:",
		"  - " + docID,
		"",
		"```toon",
		"block_id: " + blockID,
		"kind: contract",
		"source_of_truth: " + docID,
		"verify:",
		"  - go test ./internal/service",
		"evidence:",
		"  - .docs/wiki/09_contratos/CT-SOURCE.md",
		"id: " + recordID,
		"```",
		"",
		tail,
	}, "\n")
}

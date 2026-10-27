package service

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestHarnessFreshnessEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, reference, diskPath string
		indexable                 bool
	}{
		{"toml", ".docs/wiki/_mi-lsp/read-model.toml", ".docs/wiki/_mi-lsp/read-model.toml", false},
		{"json", ".docs/wiki/_mi-lsp/read-model.json", ".docs/wiki/_mi-lsp/read-model.json", false},
		{"yaml", ".docs/wiki/_mi-lsp/read-model.yaml", ".docs/wiki/_mi-lsp/read-model.yaml", false},
		{"md", ".docs/wiki/evidence.md", ".docs/wiki/evidence.md", true},
		{"mdown", ".docs/wiki/evidence.mdown", ".docs/wiki/evidence.mdown", true},
		{"markdown", ".docs/wiki/evidence.markdown", ".docs/wiki/evidence.markdown", true},
		{"uppercase", ".docs/wiki/evidence.MD", ".docs/wiki/evidence.MD", true},
		{"extensionless", ".docs/wiki/evidence", ".docs/wiki/evidence.md", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			docPath := ".docs/wiki/fixture.md"
			content := fmt.Sprintf(`---
harness_protocol: SDD-HARNESS-v1
id: HARNESS-FIXTURE
kind: contract
audience: llm-first
imports: ["%s"]
exports: [HARNESS-FIXTURE]
agent_must_read: ["%s"]
agent_may_edit: [src/]
agent_must_not_edit: [secrets/]
verify: [go test]
stop_if: [verification fails]
evidence: ["%s"]
---
# Harness fixture
`, docPath, docPath, tc.reference)
			for path, body := range map[string]string{docPath: content, tc.diskPath: "evidence"} {
				abs := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(abs, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			doc := harnessDoc{
				record:   model.DocRecord{Path: docPath, ContentHash: fmt.Sprintf("%x", sha1.Sum([]byte(content)))},
				content:  content,
				contract: extractHarnessContract(content),
			}
			if doc.contract == nil {
				t.Fatal("fixture contract not parsed")
			}
			docs := []harnessDoc{doc}
			result := compileHarnessValidation(root, docs, docs, []string{".docs/wiki"})
			wantFreshness := "current"
			if tc.indexable {
				wantFreshness = "stale"
			}
			if result.IndexFreshness != wantFreshness {
				t.Fatalf("freshness=%s, want %s; warnings=%v", result.IndexFreshness, wantFreshness, result.HarnessWarnings)
			}
			warning := "index stale; evidence exists on disk but is not indexed: " + tc.reference
			hasWarning := strings.Contains(strings.Join(result.HarnessWarnings, "\n"), warning)
			if hasWarning != tc.indexable {
				t.Fatalf("stale warning=%t, want %t: %v", hasWarning, tc.indexable, result.HarnessWarnings)
			}
			if len(result.HarnessEvidenceFound) != 1 || result.HarnessEvidenceFound[0] != tc.reference {
				t.Fatalf("existing evidence not found: %v", result.HarnessEvidenceFound)
			}
			if len(result.HarnessBlockers) != 0 {
				t.Fatalf("unexpected blockers: %v", result.HarnessBlockers)
			}
			if tc.indexable {
				corpus := append([]harnessDoc{doc}, harnessDoc{record: model.DocRecord{Path: tc.diskPath}})
				indexed := compileHarnessValidation(root, docs, corpus, []string{".docs/wiki"})
				if indexed.IndexFreshness != "current" {
					t.Fatalf("indexed markdown stays stale: %v", indexed.HarnessWarnings)
				}
			}
			exists, diskOnly := harnessRefStatus(root, buildHarnessDocIndex(docs), docPath, ".docs/wiki/_mi-lsp/missing.toml")
			if exists || diskOnly {
				t.Fatalf("missing evidence: exists=%t diskOnly=%t", exists, diskOnly)
			}
		})
	}
}

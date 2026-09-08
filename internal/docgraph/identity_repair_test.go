package docgraph

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestAuthoritativeDocumentIDIgnoresBodyReferencesAndKeepsEdges(t *testing.T) {
	content := []byte("# Legacy index\n\nLinks to [[CT-EXAMPLE]] and TECH-EXAMPLE in a planning example; DB-EXAMPLE is not this document.\n")
	doc, edges, mentions, _, _, _, _, err := ParseSingleDoc(t.TempDir(), "docs/index.md", content, model.DocsReadProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocID != "" {
		t.Fatalf("body references manufactured owner identity %q", doc.DocID)
	}
	wantEdges := map[string]int{
		"doc_id:CT-EXAMPLE:": 1,
		"doc_id:TECH-EXAMPLE:": 1,
		"doc_id:DB-EXAMPLE:": 1,
		"doc_wikilink::CT-EXAMPLE.md": 1,
	}
	if len(edges) != len(wantEdges) {
		t.Fatalf("reference edges were not preserved: %#v", edges)
	}
	for _, edge := range edges {
		key := edge.Kind + ":" + edge.ToDocID + ":" + edge.ToPath
		if wantEdges[key] != 1 {
			t.Fatalf("unexpected or duplicate reference edge: %#v", edge)
		}
		wantEdges[key]--
	}
	if len(mentions) == 0 {
		t.Fatal("expected body reference mention to remain available")
	}
	readme, _, _, _, _, _, _, err := ParseSingleDoc(t.TempDir(), "README.md", []byte("# Project README\n\nPlanning link to RF-PLAN-001 and [[TECH-PLAN-001]].\n"), model.DocsReadProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if readme.DocID != "" {
		t.Fatalf("README link mention manufactured owner identity %q", readme.DocID)
	}
}

func TestAuthoritativeDocumentIDRestrictsDeclarationsToLeadingMetadata(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{"body after prose", "docs/body.md", "# Notes\n\nProse before the example.\ndoc_id: RF-BODY-001\n", ""},
		{"nested and comment", "docs/nested.md", "# Notes\n# doc_id: RF-COMMENT-001\nmeta:\n  id: RF-NESTED-001\nid: RF-BODY-001\n", ""},
		{"CRLF and BOM frontmatter", "docs/crlf.md", "\ufeff---\r\nid: RF-LEGACY-001\r\ndoc_id: RF-OWNER-001\r\nmeta:\r\n  id: RF-NESTED-001\r\n---\r\n# Notes\r\n", "RF-OWNER-001"},
		{"legacy fenced harness", "07_baseline_tecnica.md", "# 07. Baseline tecnica\n\n```yaml\nharness_protocol: SDD-HARNESS-v1\nid: \"07_baseline_tecnica\"\nkind: support-doc\n```\n", "07_baseline_tecnica"},
		{"id before doc_id", "docs/order.md", "# Owner\nid: RF-LEGACY-001\ndoc_id: RF-OWNER-001\n", "RF-OWNER-001"},
		{"fenced example heading", "docs/example.md", "# Notes\n\nExample follows.\n```md\n# RF-EXAMPLE-001 - Not an owner\n```\n", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			doc, _, _, _, _, _, _, err := ParseSingleDoc(t.TempDir(), test.path, []byte(test.content), model.DocsReadProfile{})
			if err != nil {
				t.Fatal(err)
			}
			if doc.DocID != test.want {
				t.Fatalf("DocID = %q, want %q", doc.DocID, test.want)
			}
		})
	}
}

func TestAuthoritativeDocumentIDAcceptsExplicitTitleAndFilenameOwners(t *testing.T) {
	cases := []struct {
		path    string
		content string
		want    string
	}{
		{"docs/explicit.md", "---\ndoc_id: RF-OWNER-001\n---\n# Planning notes\n", "RF-OWNER-001"},
		{"docs/title.md", "# CT-TITLE-001\n\nReferences TECH-OTHER-001.\n", "CT-TITLE-001"},
		{"docs/title-description.md", "# RF-TITLE-002 - Description\n\nReferences TECH-OTHER-002.\n", "RF-TITLE-002"},
		{"docs/TECH-FILENAME-001.md", "# Technical notes\n\nReferences DB-OTHER-001.\n", "TECH-FILENAME-001"},
	}
	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			doc, _, _, _, _, _, _, err := ParseSingleDoc(t.TempDir(), test.path, []byte(test.content), model.DocsReadProfile{})
			if err != nil {
				t.Fatal(err)
			}
			if doc.DocID != test.want {
				t.Fatalf("DocID = %q, want %q", doc.DocID, test.want)
			}
		})
	}
}

func TestAuthoritativeDocumentIDPreservesTrueDuplicatesForValidation(t *testing.T) {
	content := "---\ndoc_id: CT-DUPLICATE-001\n---\n# Duplicate owner\n"
	first, _, _, _, _, _, _, err := ParseSingleDoc(t.TempDir(), "docs/one.md", []byte(content), model.DocsReadProfile{})
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, _, _, _, _, err := ParseSingleDoc(t.TempDir(), "docs/two.md", []byte(content), model.DocsReadProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if first.DocID != "CT-DUPLICATE-001" || second.DocID != first.DocID {
		t.Fatalf("explicit duplicate owners were not preserved: %q, %q", first.DocID, second.DocID)
	}
}

func TestFullAndIncrementalIdentityRepairParity(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[generic_docs]",
		"  paths = [\"docs/*.md\"]",
	}, "\n"))
	path := filepath.Join(root, "docs", "legacy.md")
	mustWriteDocgraphFile(t, path, "# Legacy\n\nSee TECH-LINKED-001 in an example.\n")
	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, _, err := IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstDocs) != 1 || firstDocs[0].DocID != "" {
		t.Fatalf("full parse identity = %#v", firstDocs)
	}
	prior := BuildPriorDocSnapshot(firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, nil)
	prior.Docs["docs/legacy.md"] = model.DocRecord{Path: "docs/legacy.md", ContentHash: firstDocs[0].ContentHash, DocID: "TECH-LINKED-001"}
	parsed := 0
	secondDocs, _, _, _, _, _, err := IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, func(_ context.Context, progress Progress) error {
		if progress.Stage == "docs.read" {
			parsed = progress.Parsed
		}
		return nil
	}, prior)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondDocs) != 1 || secondDocs[0].DocID != "" || parsed != 1 {
		t.Fatalf("incremental identity diverged: docs=%#v parsed=%d", secondDocs, parsed)
	}
}

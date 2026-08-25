package docgraph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestIsSnapshotPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"old segment", "docs/wiki/old/foo.md", true},
		{"archive segment", "docs/archive/bar.md", true},
		{"deprecated segment", "docs/deprecated/baz.md", true},
		{"historico segment", "docs/historico/qux.md", true},
		{"legacy segment", "legacy/docs/readme.md", true},
		{"case insensitive Old", "docs/Old/foo.md", true},
		{"case insensitive ARCHIVE", "docs/ARCHIVE/bar.md", true},
		{"case insensitive Deprecated", "docs/Deprecated/baz.md", true},
		{"normal doc", "docs/wiki/01_alcance.md", false},
		{"code file", "src/main.go", false},
		{"empty string", "", false},
		{"old at start", "old/foo.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSnapshotPath(tt.path)
			if got != tt.want {
				t.Errorf("isSnapshotPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIndexWorkspaceDocsHonorsGitignoreReincludeForWiki(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".gitignore"), strings.Join([]string{
		"/.docs/*",
		"!/.docs/wiki/",
		"!/.docs/wiki/**",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"functional\"",
		"  intent_keywords = [\"flow\", \"rf\", \"fl\"]",
		"  paths = [\".docs/wiki/03_FL/*.md\", \".docs/wiki/04_RF/*.md\"]",
		"",
		"[generic_docs]",
		"  paths = [\"README.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "03_FL", "FL-QRY-01.md"), "# FL-QRY-01\n\ncontinuation memory pointer\n")
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "04_RF", "RF-QRY-010.md"), "# RF-QRY-010\n\ncontinuation memory pointer\n")
	mustWriteDocgraphFile(t, filepath.Join(root, "README.md"), "# repo\n")

	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher returned error: %v", err)
	}

	docs, _, _, warnings, err := IndexWorkspaceDocs(context.Background(), root, matcher)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocs returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("IndexWorkspaceDocs warnings = %v, want none", warnings)
	}

	paths := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		paths[doc.Path] = struct{}{}
	}
	for _, expected := range []string{
		".docs/wiki/03_FL/FL-QRY-01.md",
		".docs/wiki/04_RF/RF-QRY-010.md",
		"README.md",
	} {
		if _, ok := paths[expected]; !ok {
			t.Fatalf("expected %s to be indexed; got paths=%v", expected, paths)
		}
	}
}

func TestIndexWorkspaceDocsExtractsAEDocID(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"technical\"",
		"  intent_keywords = [\"ae\", \"release\", \"binary\"]",
		"  paths = [\".docs/wiki/ae/*.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "ae", "AE-RELEASE-DISTRIBUTION.md"), strings.Join([]string{
		"# AE-RELEASE-DISTRIBUTION",
		"",
		"source_protocol: SDD-WIKI-SOURCE-v1",
		"harness_protocol: SDD-HARNESS-v1",
		"doc_id: AE-RELEASE-DISTRIBUTION",
		"audience: llm-first",
		"imports:",
		"  - '[[00_gobierno_documental]]'",
		"exports:",
		"  - AE-RELEASE-DISTRIBUTION",
		"",
		"```toon",
		"doc_id: AE-RELEASE-DISTRIBUTION",
		"block_id: AE-RELEASE-DISTRIBUTION.gate",
		"kind: policy",
		"source_of_truth: this",
		"verify:",
		"  - mi-lsp nav governance --workspace mi-lsp --format toon",
		"evidence:",
		"  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md",
		"```",
	}, "\n"))

	docs, _, mentions, blocks, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsWithSourcesWithProgress: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %#v", docs)
	}
	if docs[0].DocID != "AE-RELEASE-DISTRIBUTION" || docs[0].Layer != "AE" || docs[0].Family != "technical" {
		t.Fatalf("AE doc = %#v", docs[0])
	}
	if len(blocks) != 1 || blocks[0].DocID != "AE-RELEASE-DISTRIBUTION" || blocks[0].BlockID != "AE-RELEASE-DISTRIBUTION.gate" {
		t.Fatalf("source blocks = %#v", blocks)
	}
	found := false
	for _, mention := range mentions {
		if mention.MentionType == "doc_id" && mention.MentionValue == "AE-RELEASE-DISTRIBUTION" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing AE doc_id mention in %#v", mentions)
	}
}

func TestIndexWorkspaceDocsExtractsWikiSourceDocID(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"functional\"",
		"  intent_keywords = [\"governance\", \"gobierno\"]",
		"  paths = [\".docs/wiki/00_gobierno_documental.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md"), strings.Join([]string{
		"# 00. Gobierno documental",
		"",
		"source_protocol: SDD-WIKI-SOURCE-v1",
		"harness_protocol: SDD-HARNESS-v1",
		"doc_id: ING-GOV-00-PROJ",
		"audience: dual",
		"imports:",
		"  - Ingenieria/00_gobierno_documental.md",
		"exports:",
		"  - governance",
		"",
		"```toon",
		"block_id: ING-GOV-00-PROJ.harness-contract",
		"id: ING-GOV-00-PROJ",
		"doc_id: ING-GOV-00-PROJ",
		"kind: POLICY",
		"source_of_truth: Ingenieria/00_gobierno_documental.md",
		"verify:",
		"  - test -f Ingenieria/00_gobierno_documental.md",
		"evidence:",
		"  - Ingenieria/00_gobierno_documental.md",
		"```",
	}, "\n"))

	docs, _, mentions, blocks, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsWithSourcesWithProgress: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %#v", docs)
	}
	// The doc_id must come from wikisource.Parse, not from the regex.
	if docs[0].DocID != "ING-GOV-00-PROJ" {
		t.Fatalf("doc ID = %q, want ING-GOV-00-PROJ", docs[0].DocID)
	}
	if len(blocks) != 1 || blocks[0].DocID != "ING-GOV-00-PROJ" || blocks[0].BlockID != "ING-GOV-00-PROJ.harness-contract" {
		t.Fatalf("source blocks = %#v", blocks)
	}
	found := false
	for _, mention := range mentions {
		if mention.MentionType == "doc_id" && mention.MentionValue == "ING-GOV-00-PROJ" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing ING-GOV-00-PROJ doc_id mention in %#v", mentions)
	}
}

func TestIndexWorkspaceDocsExtractsWikiSourceBlocksAndRecords(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"technical\"",
		"  intent_keywords = [\"contract\"]",
		"  paths = [\".docs/wiki/09_contratos/*.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "09_contratos", "CT-SOURCE.md"), strings.Join([]string{
		"# CT-SOURCE",
		"",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"doc_id: CT-SOURCE",
		"audience: llm-first",
		"imports:",
		"  - '[[00_gobierno_documental]]'",
		"exports:",
		"  - CT-SOURCE",
		"",
		"```toon",
		"block_id: CT-SOURCE.contract",
		"kind: contract",
		"id: RF-QRY-016",
		"title: Validate source",
		"```",
	}, "\n"))

	docs, _, mentions, blocks, records, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsWithSourcesWithProgress: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %#v", docs)
	}
	if len(blocks) != 1 || blocks[0].BlockID != "CT-SOURCE.contract" || blocks[0].DocID != "CT-SOURCE" {
		t.Fatalf("blocks = %#v", blocks)
	}
	if len(records) != 1 || records[0].RecordID != "RF-QRY-016" || records[0].BlockID != "CT-SOURCE.contract" {
		t.Fatalf("records = %#v", records)
	}
	seen := map[string]bool{}
	for _, mention := range mentions {
		seen[mention.MentionType+"="+mention.MentionValue] = true
	}
	for _, expected := range []string{"source_protocol=SDD-WIKI-SOURCE-v1", "block_id=CT-SOURCE.contract", "record_id=RF-QRY-016", "source_export=CT-SOURCE"} {
		if !seen[expected] {
			t.Fatalf("missing source mention %s in %#v", expected, mentions)
		}
	}
}

func TestIndexWorkspaceDocsExtractsFrontMatterTraceLinksForTechnicalDocs(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"technical\"",
		"  intent_keywords = [\"contract\"]",
		"  paths = [\".docs/wiki/09_contratos/*.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "09_contratos", "CT-TRACE.md"), strings.Join([]string{
		"---",
		"doc_id: CT-TRACE",
		"title: Traceable technical contract",
		"layer: CT",
		"implements:",
		"  - internal/service/trace.go",
		"tests:",
		"  - internal/service/trace_filesystem_test.go",
		"---",
		"",
		"# CT-TRACE",
	}, "\n"))

	_, _, mentions, _, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsWithSourcesWithProgress: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	seen := map[string]bool{}
	for _, mention := range mentions {
		seen[mention.MentionType+"="+mention.MentionValue] = true
	}
	for _, expected := range []string{"implements=internal/service/trace.go", "test_file=internal/service/trace_filesystem_test.go"} {
		if !seen[expected] {
			t.Fatalf("missing frontmatter mention %s in %#v", expected, mentions)
		}
	}
}

func TestIndexWorkspaceDocsExtractsCanonicalWikiSourceShape(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"technical\"",
		"  intent_keywords = [\"contract\"]",
		"  paths = [\".docs/wiki/09_contratos/*.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "09_contratos", "CT-SOURCE.md"), strings.Join([]string{
		"# CT-SOURCE",
		"",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"harness_protocol: SDD-HARNESS-v1",
		"doc_id: CT-SOURCE",
		"audience: llm-first",
		"links:",
		"  imports:",
		"    - '[[00_gobierno_documental]]'",
		"  exports:",
		"    - CT-SOURCE",
		"",
		"```toon",
		"block_id: CT-SOURCE.contract",
		"kind: contract",
		"source_of_truth: CT-SOURCE",
		"verify:",
		"  - go test ./internal/docgraph",
		"evidence:",
		"  - .docs/wiki/09_contratos/CT-SOURCE.md",
		"records:",
		"  - type: RF",
		"    id: RF-QRY-016",
		"```",
	}, "\n"))

	_, _, mentions, blocks, records, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsWithSourcesWithProgress: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(blocks) != 1 || blocks[0].BlockID != "CT-SOURCE.contract" || blocks[0].Kind != "contract" {
		t.Fatalf("blocks = %#v", blocks)
	}
	if len(records) != 1 || records[0].RecordID != "RF-QRY-016" || records[0].RecordType != "RF" {
		t.Fatalf("records = %#v", records)
	}
	seen := map[string]bool{}
	for _, mention := range mentions {
		seen[mention.MentionType+"="+mention.MentionValue] = true
	}
	for _, expected := range []string{"source_import=[[00_gobierno_documental]]", "source_export=CT-SOURCE", "record_id=RF-QRY-016"} {
		if !seen[expected] {
			t.Fatalf("missing source mention %s in %#v", expected, mentions)
		}
	}
}

func mustWriteDocgraphFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestExpandPatternSkipsIgnoredDirectory(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "keep.md"), "# keep\n")
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "ignored", "skip.md"), "# skip\n")
	matcher, err := workspace.LoadIgnoreMatcher(root, []string{".docs/wiki/ignored/"})
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher: %v", err)
	}

	visited := make([]string, 0, 2)
	if err := expandPattern(context.Background(), root, ".docs/wiki/", matcher, func(absPath string) {
		rel, relErr := filepath.Rel(root, absPath)
		if relErr != nil {
			t.Fatalf("Rel(%s): %v", absPath, relErr)
		}
		visited = append(visited, filepath.ToSlash(rel))
	}); err != nil {
		t.Fatalf("expandPattern: %v", err)
	}

	for _, path := range visited {
		if strings.Contains(path, "/ignored/") || strings.HasSuffix(path, "skip.md") {
			t.Fatalf("ignored path visited: %v", visited)
		}
	}
	foundKeep := false
	for _, path := range visited {
		if path == ".docs/wiki/keep.md" {
			foundKeep = true
			break
		}
	}
	if !foundKeep {
		t.Fatalf("expected keep.md visited, got %v", visited)
	}
}

func TestIndexWorkspaceDocsProgressReportsElapsed(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"functional\"",
		"  intent_keywords = [\"flow\"]",
		"  paths = [\".docs/wiki/*.md\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "A.md"), "# A\n")
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "B.md"), "# B\n")

	events := make([]Progress, 0, 8)
	_, _, _, _, _, _, err := IndexWorkspaceDocsWithSourcesWithProgress(context.Background(), root, nil, func(ctx context.Context, progress Progress) error {
		events = append(events, progress)
		return nil
	})
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsWithSourcesWithProgress: %v", err)
	}

	foundCollectElapsed := false
	foundReadElapsed := false
	for _, event := range events {
		if event.Stage == "docs.collect" && event.Force && strings.HasPrefix(event.Path, "elapsed_ms=") {
			foundCollectElapsed = true
		}
		if event.Stage == "docs.read" && event.Force && strings.HasPrefix(event.Path, "elapsed_ms=") {
			foundReadElapsed = true
		}
	}
	if !foundCollectElapsed || !foundReadElapsed {
		t.Fatalf("missing elapsed progress markers in %#v", events)
	}
}

func TestIndexWorkspaceDocsHonorsMilspignoreSiblingWikiCopy(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".milspignore"), "WikiIA/\n")
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"functional\"",
		"  intent_keywords = [\"governance\"]",
		"  paths = [\".docs/wiki/*.md\", \".docs/wiki/**/*.md\", \"WikiIA/*.md\", \"WikiIA/**/*.md\"]",
		"",
		"[generic_docs]",
		"  paths = [\"WikiIA/\", \".docs/\"]",
	}, "\n"))
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md"), "# 00 gobierno canon\n")
	mustWriteDocgraphFile(t, filepath.Join(root, "WikiIA", "00_gobierno_documental.md"), "# 00 gobierno copy\n")
	mustWriteDocgraphFile(t, filepath.Join(root, "WikiIA", "ae", "README.md"), "# ae copy\n")

	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher: %v", err)
	}
	docs, _, _, warnings, err := IndexWorkspaceDocs(context.Background(), root, matcher)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocs: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	for _, doc := range docs {
		if strings.HasPrefix(doc.Path, "WikiIA/") || strings.Contains(doc.Path, "/WikiIA/") {
			t.Fatalf("indexed ignored sibling wiki path %q among %#v", doc.Path, docs)
		}
	}
	foundCanon := false
	for _, doc := range docs {
		if doc.Path == ".docs/wiki/00_gobierno_documental.md" {
			foundCanon = true
			break
		}
	}
	if !foundCanon {
		t.Fatalf("expected canon doc indexed, got %#v", docs)
	}
}

func TestExpandPatternSkipsIgnoredGlobAndExplicitPaths(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "keep.md"), "# keep\n")
	mustWriteDocgraphFile(t, filepath.Join(root, "WikiIA", "skip.md"), "# skip\n")
	matcher, err := workspace.LoadIgnoreMatcher(root, []string{"WikiIA/", "WikiIA/**"})
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher: %v", err)
	}

	visited := make([]string, 0, 4)
	visit := func(absPath string) {
		rel, relErr := filepath.Rel(root, absPath)
		if relErr != nil {
			t.Fatalf("Rel: %v", relErr)
		}
		visited = append(visited, filepath.ToSlash(rel))
	}
	if err := expandPattern(context.Background(), root, "WikiIA/*.md", matcher, visit); err != nil {
		t.Fatalf("glob expandPattern: %v", err)
	}
	if err := expandPattern(context.Background(), root, "WikiIA/skip.md", matcher, visit); err != nil {
		t.Fatalf("explicit expandPattern: %v", err)
	}
	if err := expandPattern(context.Background(), root, ".docs/wiki/keep.md", matcher, visit); err != nil {
		t.Fatalf("keep expandPattern: %v", err)
	}
	for _, path := range visited {
		if strings.HasPrefix(path, "WikiIA/") {
			t.Fatalf("ignored path visited: %v", visited)
		}
	}
	foundKeep := false
	for _, path := range visited {
		if path == ".docs/wiki/keep.md" {
			foundKeep = true
		}
	}
	if !foundKeep {
		t.Fatalf("expected keep.md, got %v", visited)
	}
}

func TestExtractReferencesParsesWikilinks(t *testing.T) {
	mentions, edges := extractReferences("/tmp", "wiki/10-chiamo.md", "Ver [[00-identidad-karen]] y ![[12-cafe|café]].")
	docs := []model.DocRecord{{Path: "wiki/10-chiamo.md"}, {Path: "wiki/00-identidad-karen.md"}, {Path: "wiki/12-cafe.md"}}
	edges = resolveDocEdges(docs, edges, []string{"wiki/"})
	kinds := map[string]string{}
	for _, edge := range edges {
		kinds[edge.Kind] = edge.ToPath
	}
	if kinds["doc_wikilink"] != "wiki/00-identidad-karen.md" {
		t.Fatalf("wikilink=%q kinds=%v", kinds["doc_wikilink"], kinds)
	}
	if kinds["doc_embed"] != "wiki/12-cafe.md" {
		t.Fatalf("embed=%q kinds=%v", kinds["doc_embed"], kinds)
	}
	foundAlias := false
	for _, mention := range mentions {
		foundAlias = foundAlias || mention.MentionType == model.DocMentionTypeAlias && mention.MentionValue == "café"
	}
	if !foundAlias {
		t.Fatalf("alias mention missing: %v", mentions)
	}
}

func TestAppendStructuralDocEdgesLinksGobiernoAndReadme(t *testing.T) {
	docs := []model.DocRecord{
		{Path: "wiki/00-gobierno.md"},
		{Path: "wiki/10-chiamo.md"},
		{Path: "bibliotecas/memorias/README.md"},
		{Path: "bibliotecas/memorias/ficha.md"},
	}
	edges := appendStructuralDocEdges(docs, nil)
	foundGobierno, foundReadme := false, false
	for _, edge := range edges {
		if edge.Kind != "doc_hierarchy" {
			continue
		}
		if edge.FromPath == "wiki/10-chiamo.md" && edge.ToPath == "wiki/00-gobierno.md" {
			foundGobierno = true
		}
		if edge.FromPath == "bibliotecas/memorias/ficha.md" && edge.ToPath == "bibliotecas/memorias/README.md" {
			foundReadme = true
		}
	}
	if !foundGobierno || !foundReadme {
		t.Fatalf("structural edges=%v gobierno=%v readme=%v", edges, foundGobierno, foundReadme)
	}
}

func TestExtractReferencesSupportsObsidianFormsAndSkipsFences(t *testing.T) {
	content := "---\nrelated: [[frontmatter#Heading]]\n---\n" +
		"[[target]] [[nested/target.md#Section|Alias]] ![[embed]] [Guide](../docs/Guide%20One.md?mode=read#Part) [[#Local]]\n" +
		"```md\n[[hidden]] [hidden](hidden.md)\n```\n" +
		"[external](https://example.com/x) [[https://example.com/x]]"
	mentions, edges := extractReferences(t.TempDir(), "wiki/source.md", content)

	kinds := map[string]int{}
	labels := map[string]bool{}
	for _, edge := range edges {
		kinds[edge.Kind]++
		labels[edge.Label] = true
		if strings.Contains(edge.Label, "hidden") || strings.Contains(edge.Label, "example.com") {
			t.Fatalf("unexpected fenced/external edge: %#v", edge)
		}
	}
	if kinds["doc_wikilink"] != 3 || kinds["doc_embed"] != 1 || kinds["doc_markdown_link"] != 1 {
		t.Fatalf("edge kinds=%v edges=%#v", kinds, edges)
	}
	for _, label := range []string{"frontmatter#Heading", "nested/target.md#Section|Alias", "../docs/Guide%20One.md?mode=read#Part"} {
		if !labels[label] {
			t.Fatalf("raw label %q missing from %v", label, labels)
		}
	}
	anchors, aliases := map[string]bool{}, map[string]bool{}
	for _, mention := range mentions {
		switch mention.MentionType {
		case model.DocMentionTypeAnchor:
			anchors[mention.MentionValue] = true
		case model.DocMentionTypeAlias:
			aliases[mention.MentionValue] = true
		}
	}
	for _, anchor := range []string{"Heading", "Section", "Part", "Local"} {
		if !anchors[anchor] {
			t.Fatalf("anchor %q missing from %v", anchor, anchors)
		}
	}
	if !aliases["Alias"] {
		t.Fatalf("alias missing from %v", aliases)
	}
}

func TestResolveDocEdgesUsesDeterministicPrecedence(t *testing.T) {
	docs := []model.DocRecord{
		{Path: "wiki/source.md"},
		{Path: "wiki/target.md"},
		{Path: "wiki/nested/target.md"},
		{Path: "docs/Guide One.md"},
		{Path: "wiki/Case.md"},
	}
	_, parsed := extractReferences(t.TempDir(), "wiki/source.md", "[[target]] [[nested/target]] [[Guide One]] [[CASE]]")
	resolved := resolveDocEdges(docs, parsed, []string{"wiki/", "bibliotecas/"})
	paths := map[string]bool{}
	for _, edge := range resolved {
		if edge.UnresolvedReason != "" {
			t.Fatalf("unexpected unresolved edge: %#v", edge)
		}
		paths[edge.ToPath] = true
	}
	for _, path := range []string{"wiki/target.md", "wiki/nested/target.md", "docs/Guide One.md", "wiki/Case.md"} {
		if !paths[path] {
			t.Fatalf("resolved path %q missing from %v", path, paths)
		}
	}
}

func TestResolveDocEdgesLeavesAmbiguousBasenameAndDocIDUnresolved(t *testing.T) {
	docs := []model.DocRecord{
		{Path: "notes/source.md"},
		{Path: "wiki/target.md", DocID: "RF-DUP-001"},
		{Path: "docs/target.md", DocID: "RF-DUP-001"},
	}
	edges := []model.DocEdge{
		{FromPath: "notes/source.md", ToPath: "target.md", Kind: "doc_wikilink", Label: "target"},
		{FromPath: "notes/source.md", ToDocID: "RF-DUP-001", Kind: "doc_id", Label: "RF-DUP-001"},
	}
	resolved := resolveDocEdges(docs, edges, nil)
	if len(resolved) != 2 {
		t.Fatalf("resolved=%#v", resolved)
	}
	for _, edge := range resolved {
		if edge.UnresolvedReason != "ambiguous_doc_target" || len(edge.Candidates) != 2 {
			t.Fatalf("ambiguous edge not preserved: %#v", edge)
		}
		if edge.Candidates[0] != "docs/target.md" || edge.Candidates[1] != "wiki/target.md" {
			t.Fatalf("candidates not deterministic: %v", edge.Candidates)
		}
	}
}

func TestKnowledgeWikiRootsAreIndexedAndLinksResolve(t *testing.T) {
	root := t.TempDir()
	write := func(relative, content string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("wiki/source.md", "# Source\n\n[[target#Details|Open target]]")
	write("wiki/target.md", "# Target\n")
	write("bibliotecas/topic.md", "# Topic\n")

	docs, edges, mentions, warnings, err := IndexWorkspaceDocs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v", warnings)
	}
	paths := map[string]bool{}
	for _, doc := range docs {
		paths[doc.Path] = true
	}
	for _, path := range []string{"wiki/source.md", "wiki/target.md", "bibliotecas/topic.md"} {
		if !paths[path] {
			t.Fatalf("indexed path %q missing from %v", path, paths)
		}
	}
	foundResolved := false
	for _, edge := range edges {
		if edge.FromPath == "wiki/source.md" && edge.ToPath == "wiki/target.md" && edge.Kind == "doc_wikilink" {
			foundResolved = true
		}
	}
	if !foundResolved {
		t.Fatalf("resolved wikilink missing from %#v", edges)
	}
	foundAnchor, foundAlias := false, false
	for _, mention := range mentions {
		foundAnchor = foundAnchor || mention.MentionType == model.DocMentionTypeAnchor && mention.MentionValue == "Details"
		foundAlias = foundAlias || mention.MentionType == model.DocMentionTypeAlias && mention.MentionValue == "Open target"
	}
	if !foundAnchor || !foundAlias {
		t.Fatalf("anchor=%v alias=%v mentions=%#v", foundAnchor, foundAlias, mentions)
	}
}

func TestLoadProfileCanDisableAutomaticKnowledgeRoots(t *testing.T) {
	root := t.TempDir()
	path := ProfilePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version = 1\n[wiki_map]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile, source, warnings := LoadProfile(root)
	if source != "project" || len(warnings) != 0 {
		t.Fatalf("source=%q warnings=%v", source, warnings)
	}
	for _, path := range profile.GenericDocs.Paths {
		if strings.TrimSuffix(path, "/") == "wiki" || strings.TrimSuffix(path, "/") == "bibliotecas" {
			t.Fatalf("disabled wiki root was appended: %v", profile.GenericDocs.Paths)
		}
	}
}

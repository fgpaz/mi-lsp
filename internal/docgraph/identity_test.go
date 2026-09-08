package docgraph

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

func TestParseSingleDocUsesDeclaredOwnerNotImportedID(t *testing.T) {
	content := []byte(`# Trazabilidad histórica

` + "```yaml" + `
harness_protocol: SDD-HARNESS-v1
id: "2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria"
imports:
  - '[[TECH-DAEMON-GOBERNANZA]]'
` + "```" + `

El cuerpo menciona RF-BODY-001 y no declara una segunda identidad.
`)

	doc, _, mentions, _, _, _, _, err := ParseSingleDoc("", "notes/history.md", content, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocID != "2026-05-19-stash-telemetry-hardening-trazabilidad-auditoria" {
		t.Fatalf("DocID = %q, want declared owner", doc.DocID)
	}
	foundReference := false
	for _, mention := range mentions {
		if mention.MentionValue == "TECH-DAEMON-GOBERNANZA" && mention.MentionType == model.DocMentionTypeDocID {
			foundReference = true
		}
	}
	if !foundReference {
		t.Fatal("imported ID reference was not preserved as a mention")
	}
}

func TestParseSingleDocLeavesUnownedLinkedMarkdownWithoutDeclaration(t *testing.T) {
	content := []byte("# Not an owner\n\nSee RF-LINK-001 and [[TECH-LINK-002]].\n\n```yaml\nid: RF-CODE-003\n```\n")
	doc, _, _, _, _, _, _, err := ParseSingleDoc("", "notes/linked.md", content, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocID != "" {
		t.Fatalf("DocID = %q, want empty for body/link-only document", doc.DocID)
	}
}

func TestParseSingleDocIdentityPrecedenceAndCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{
			name:    "frontmatter explicit doc id",
			path:    "notes/unicode.md",
			content: "---\ndoc_id: \"Док-π-001\"\nid: RF-FALLBACK-001\n---\n# See RF-BODY-001\n",
			want:    "Док-π-001",
		},
		{
			name:    "legacy id-leading title",
			path:    "notes/legacy.md",
			content: "# RF-LEGACY-001 — Documento heredado\n\nSee RF-BODY-002.\n",
			want:    "RF-LEGACY-001",
		},
		{
			name:    "legacy filename",
			path:    "notes/RF-FILENAME-001.md",
			content: "# Documento heredado\n\nSee RF-BODY-003.\n",
			want:    "RF-FILENAME-001",
		},
		{
			name:    "prose is not title ownership",
			path:    "notes/prose.md",
			content: "# See RF-BODY-004\n",
			want:    "",
		},
		{
			name:    "inline comment is not part of owner",
			path:    "notes/comment.md",
			content: "---\ndoc_id: RF-COMMENT-001 # owner\nmetadata:\n  id: RF-NESTED-002\n---\n# Notes\n",
			want:    "RF-COMMENT-001",
		},
		{
			name:    "empty explicit owner does not fall back",
			path:    "notes/empty.md",
			content: "---\ndoc_id: # intentionally empty\n---\n# See RF-BODY-005\n",
			want:    "",
		},
		{
			name:    "identical duplicate declaration is stable",
			path:    "notes/duplicate.md",
			content: "---\ndoc_id: RF-DUPLICATE-001\ndoc_id: RF-DUPLICATE-001\n---\n# Notes\n",
			want:    "RF-DUPLICATE-001",
		},
		{
			name:    "unclosed frontmatter is bounded and unowned",
			path:    "notes/unclosed-frontmatter.md",
			content: "---\ndoc_id: RF-UNFINISHED-001\n# See RF-BODY-006\n",
			want:    "",
		},
		{
			name:    "unclosed harness fence is bounded and unowned",
			path:    "notes/unclosed-fence.md",
			content: "# Notes\n\n```yaml\nharness_protocol: SDD-HARNESS-v1\nid: RF-UNFINISHED-002\n",
			want:    "",
		},
		{
			name:    "source example after prose is not an envelope",
			path:    "notes/source-example.md",
			content: "# Notes\n\nThis prose precedes an example.\n\n```yaml\nsource_protocol: SDD-WIKI-SOURCE-v1\ndoc_id: RF-EXAMPLE-003\n```\n",
			want:    "",
		},
		{
			name:    "source example under examples heading is not an envelope",
			path:    "notes/source-heading-example.md",
			content: "# Notes\n\n## Examples\n\n```yaml\nharness_protocol: SDD-HARNESS-v1\nid: RF-EXAMPLE-004\n```\n",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _, _, _, _, _, _, err := ParseSingleDoc("", tt.path, []byte(tt.content), DefaultProfile())
			if err != nil {
				t.Fatal(err)
			}
			if doc.DocID != tt.want {
				t.Fatalf("DocID = %q, want %q", doc.DocID, tt.want)
			}
		})
	}
}

func TestParseSingleDocRejectsConflictingDeclarations(t *testing.T) {
	content := []byte("---\ndoc_id: RF-DECLARED-001\ndoc_id: RF-DECLARED-002\n---\n# RF-BODY-003\n")
	doc, _, _, _, _, _, _, err := ParseSingleDoc("", "notes/conflict.md", content, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocID != "" {
		t.Fatalf("DocID = %q, want empty for conflicting declarations", doc.DocID)
	}
}

func TestParseSingleDocConflictingSourceOwnersDoNotLeakIntoBlocks(t *testing.T) {
	content := []byte(`# Conflict

wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-SOURCE-001
doc_id: CT-SOURCE-002

` + "```toon" + `
block_id: CT-SOURCE-002.block
kind: contract
` + "```" + `
`)
	doc, _, _, blocks, _, _, _, err := ParseSingleDoc("", "wiki/conflict.md", content, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocID != "" {
		t.Fatalf("DocID = %q, want empty for conflicting source owners", doc.DocID)
	}
	if len(blocks) != 1 || blocks[0].DocID != "" {
		t.Fatalf("source blocks leaked conflicting owner: %#v", blocks)
	}
}

func TestParseSingleDocKeepsSourceOwnerSeparateFromBlockAndRecord(t *testing.T) {
	content := []byte(`# CT-OWNER-001

wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-OWNER-001

` + "```toon" + `
block_id: RF-SOURCE-BLOCK-001
kind: contract
records:
  - id: TP-SOURCE-RECORD-001
` + "```" + `
`)
	doc, _, _, blocks, records, _, _, err := ParseSingleDoc("", "wiki/owner.md", content, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocID != "CT-OWNER-001" {
		t.Fatalf("DocID = %q, want source owner", doc.DocID)
	}
	if len(blocks) != 1 || blocks[0].DocID != "CT-OWNER-001" || blocks[0].BlockID != "RF-SOURCE-BLOCK-001" {
		t.Fatalf("source blocks = %#v", blocks)
	}
	if len(records) != 1 || records[0].RecordID != "TP-SOURCE-RECORD-001" || records[0].BlockID != "RF-SOURCE-BLOCK-001" {
		t.Fatalf("source records = %#v", records)
	}
}

func TestReplaceDocsWithSourcesRefreshesWrongStoredIdentityAtomically(t *testing.T) {
	root := t.TempDir()
	content := []byte(`# CT-REFRESH-001

wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-REFRESH-001

` + "```toon" + `
block_id: CT-REFRESH-001.block
kind: contract
records:
  - id: RF-REFRESH-001
` + "```" + `
`)
	path := ".docs/wiki/09_contratos/CT-REFRESH-001.md"
	stablePath := ".docs/wiki/09_contratos/CT-STABLE-001.md"
	mustWriteDocgraphFile(t, filepath.Join(root, filepath.FromSlash(path)), string(content))
	mustWriteDocgraphFile(t, filepath.Join(root, filepath.FromSlash(stablePath)), "# CT-STABLE-001\n\nstable document\n")
	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, firstBindings, _, err := IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(context.Background(), root, matcher, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	prior := BuildPriorDocSnapshot(firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, firstBindings)
	if prior == nil {
		t.Fatal("expected prior snapshot")
	}
	wrongDoc := prior.Docs[path]
	wrongDoc.DocID = "TECH-WRONG-OWNER"
	prior.Docs[path] = wrongDoc
	if sourceBlocks := prior.Blocks[path]; len(sourceBlocks) > 0 {
		sourceBlocks[0].DocID = "TECH-WRONG-OWNER"
		prior.Blocks[path] = sourceBlocks
	}
	parsed, skipped := 0, 0
	progress := func(_ context.Context, value Progress) error {
		if value.Stage == "docs.read" {
			parsed = value.Parsed
			skipped = value.Skipped
		}
		return nil
	}
	secondDocs, secondEdges, secondMentions, secondBlocks, secondRecords, secondBindings, _, err := IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(context.Background(), root, matcher, progress, prior)
	if err != nil {
		t.Fatal(err)
	}
	var refreshed model.DocRecord
	for _, candidate := range secondDocs {
		if candidate.Path == path {
			refreshed = candidate
		}
	}
	if refreshed.DocID != "CT-REFRESH-001" || parsed < 1 || skipped < 1 ||
		len(secondEdges) != len(firstEdges) || len(secondMentions) != len(firstMentions) ||
		len(secondBlocks) != len(firstBlocks) || len(secondRecords) != len(firstRecords) ||
		len(secondBindings) != len(firstBindings) {
		t.Fatalf("refreshed=%#v parsed=%d skipped=%d collections=%d/%d/%d/%d/%d want=%d/%d/%d/%d/%d", refreshed, parsed, skipped,
			len(secondEdges), len(secondMentions), len(secondBlocks), len(secondRecords), len(secondBindings),
			len(firstEdges), len(firstMentions), len(firstBlocks), len(firstRecords), len(firstBindings))
	}
	doc, edges, mentions, blocks, records, bindings, _, err := ParseSingleDoc(root, path, content, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	oldDoc := doc
	oldDoc.DocID = "TECH-WRONG-OWNER"
	oldBlock := blocks[0]
	oldBlock.DocID = oldDoc.DocID
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := store.ReplaceDocsWithSources(ctx, db, []model.DocRecord{oldDoc}, edges, mentions, []model.DocSourceBlock{oldBlock}, records, bindings); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceDocsWithSources(ctx, db, []model.DocRecord{doc}, edges, mentions, blocks, records, bindings); err != nil {
		t.Fatal(err)
	}
	storedDocs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedDocs) != 1 || storedDocs[0].DocID != "CT-REFRESH-001" {
		t.Fatalf("stored docs = %#v", storedDocs)
	}
	wrongOwner, err := store.FindDocRecordsBySourceID(ctx, db, "TECH-WRONG-OWNER")
	if err != nil {
		t.Fatal(err)
	}
	if len(wrongOwner) != 0 {
		t.Fatalf("stale owner remained canonical: %#v", wrongOwner)
	}
	correctOwner, err := store.FindDocRecordsBySourceID(ctx, db, "CT-REFRESH-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(correctOwner) != 1 || correctOwner[0].DocID != "CT-REFRESH-001" {
		t.Fatalf("correct owner lookup = %#v", correctOwner)
	}
	storedBlocks, err := store.ListDocSourceBlocks(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedBlocks) != 1 || storedBlocks[0].DocID != "CT-REFRESH-001" {
		t.Fatalf("stored blocks = %#v", storedBlocks)
	}
	storedRecords, err := store.ListDocSourceRecords(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedRecords) != 1 || storedRecords[0].RecordID != "RF-REFRESH-001" {
		t.Fatalf("stored records = %#v", storedRecords)
	}
	storedMentions, err := store.ListDocMentions(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, mention := range storedMentions {
		if mention.MentionValue == "TECH-WRONG-OWNER" {
			t.Fatalf("stale owner mention survived refresh: %#v", mention)
		}
	}
	ftsDocs, _, err := store.FTSSearchDocs(ctx, db, "CT-REFRESH-001", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ftsDocs) != 1 || ftsDocs[0].DocID != "CT-REFRESH-001" {
		t.Fatalf("FTS docs = %#v", ftsDocs)
	}
	if _, err := os.Stat(store.WorkspaceDBPath(root)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.SearchText, "ct refresh 001") {
		t.Fatalf("unexpected search text = %q", doc.SearchText)
	}
}

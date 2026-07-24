package docgraph

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestIndexWorkspaceDocsSkipReparseByContentHash(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"), strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		"  name = \"functional\"",
		"  intent_keywords = [\"flow\"]",
		"  paths = [\".docs/wiki/03_FL/*.md\"]",
		"",
		"[generic_docs]",
		"  paths = [\"README.md\"]",
	}, "\n"))
	stablePath := filepath.Join(root, ".docs", "wiki", "03_FL", "FL-STABLE.md")
	changedPath := filepath.Join(root, ".docs", "wiki", "03_FL", "FL-CHANGED.md")
	mustWriteDocgraphFile(t, stablePath, "# FL-STABLE\n\nstable body with `internal/docgraph/docgraph.go` link\n")
	mustWriteDocgraphFile(t, changedPath, "# FL-CHANGED\n\nversion one\n")
	mustWriteDocgraphFile(t, filepath.Join(root, "README.md"), "# repo\n")

	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher: %v", err)
	}
	ctx := context.Background()

	firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, _, err := IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, nil, nil)
	if err != nil {
		t.Fatalf("first index: %v", err)
	}
	if len(firstDocs) < 2 {
		t.Fatalf("expected at least 2 docs, got %d", len(firstDocs))
	}
	prior := BuildPriorDocSnapshot(firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords)
	if prior == nil {
		t.Fatal("expected prior snapshot")
	}

	var skipped, parsed int
	progress := func(_ context.Context, value Progress) error {
		if value.Stage == "docs.read" && (value.Parsed > 0 || value.Skipped > 0) {
			parsed = value.Parsed
			skipped = value.Skipped
		}
		return nil
	}
	secondDocs, _, _, _, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, progress, prior)
	if err != nil {
		t.Fatalf("second index: %v", err)
	}
	if skipped < 2 {
		t.Fatalf("expected skipped>=2 on unchanged corpus, got parsed=%d skipped=%d warnings=%v", parsed, skipped, warnings)
	}
	if parsed != 0 {
		t.Fatalf("expected parsed=0 when all hashes match, got parsed=%d skipped=%d", parsed, skipped)
	}
	byPath := map[string]model.DocRecord{}
	for _, doc := range secondDocs {
		byPath[doc.Path] = doc
	}
	stableRel := ".docs/wiki/03_FL/FL-STABLE.md"
	if byPath[stableRel].ContentHash != prior.Docs[stableRel].ContentHash {
		t.Fatalf("stable hash changed on reuse")
	}
	if byPath[stableRel].IndexedAt != prior.Docs[stableRel].IndexedAt {
		t.Fatalf("stable IndexedAt should be preserved on skip-reparse")
	}

	mustWriteDocgraphFile(t, changedPath, "# FL-CHANGED\n\nversion two changed\n")
	skipped, parsed = 0, 0
	thirdDocs, _, _, _, _, _, err := IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, progress, prior)
	if err != nil {
		t.Fatalf("third index: %v", err)
	}
	if parsed != 1 {
		t.Fatalf("expected exactly 1 reparsed doc after single change, got parsed=%d skipped=%d", parsed, skipped)
	}
	if skipped < 1 {
		t.Fatalf("expected some skipped docs after single change, got skipped=%d", skipped)
	}
	changedRel := ".docs/wiki/03_FL/FL-CHANGED.md"
	thirdByPath := map[string]model.DocRecord{}
	for _, doc := range thirdDocs {
		thirdByPath[doc.Path] = doc
	}
	if thirdByPath[changedRel].ContentHash == prior.Docs[changedRel].ContentHash {
		t.Fatalf("changed doc hash should update")
	}
	if thirdByPath[stableRel].ContentHash != prior.Docs[stableRel].ContentHash {
		t.Fatalf("stable doc should remain skipped/reused")
	}
}

func TestBuildPriorDocSnapshotNilWhenEmpty(t *testing.T) {
	if got := BuildPriorDocSnapshot(nil, nil, nil, nil, nil); got != nil {
		t.Fatalf("expected nil prior for empty docs, got %#v", got)
	}
}

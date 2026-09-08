package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/language"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// TestModernExtensionWalk verifies that all four modern JS/TS module extensions
// are collected by the supported-extension filter (mirrors what WalkWorkspace does).
func TestModernExtensionWalk(t *testing.T) {
	extensions := []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}
	for _, ext := range extensions {
		// Simulate the check done in WalkWorkspace.
		if !language.IsSupportedCodePath("file" + ext) {
			t.Errorf("WalkWorkspace should include %s files", ext)
		}
	}
}

// TestModernExtensionWalkCaseInsensitive verifies case-insensitive matching.
func TestModernExtensionWalkCaseInsensitive(t *testing.T) {
	testCases := []string{".JS", ".MJS", ".MTS", ".CTS", ".JSX", ".TSX"}
	for _, ext := range testCases {
		if !language.IsSupportedCodePath("file" + ext) {
			t.Errorf("WalkWorkspace should include %s files (case-insensitive)", ext)
		}
	}
}

// TestWalkerSupportedExtensionsAreSubsetOfRegistry verifies that every extension
// in the walker's internal list is still present in the shared registry.
func TestWalkerSupportedExtensionsAreSubsetOfRegistry(t *testing.T) {
	// This test catches the situation where the old supportedExtensions map
	// was the single source of truth and the registry was added later but
	// walker code still references the old map.  After migration the walker
	// no longer has supportedExtensions; this test verifies the registry
	// covers every extension that any consumer of WalkWorkspace expects.
	registryExtensions := extensionsFromRegistry()

	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".cs", ".go", ".py", ".pyi"} {
		found := false
		for _, re := range registryExtensions {
			if re == ext {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("registry missing %q that walkers rely on", ext)
		}
	}
}

func TestLoadPriorDocSnapshotKeepsGoodOwnerAndContentHashWithBadOldMentions(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/04_RF/RF-SNAPSHOT-OWNER.md"
	content := "---\ndoc_id: RF-SNAPSHOT-OWNER\n---\n# RF-SNAPSHOT-OWNER\n\nCurrent owner content.\n"
	if err := writeFileRoot(root, path, content); err != nil {
		t.Fatal(err)
	}
	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	docs, edges, mentions, blocks, records, bindings, _, err := docgraph.IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(context.Background(), root, matcher, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldMentions := append(append([]model.DocMention(nil), mentions...), model.DocMention{
		DocPath: path, MentionType: model.DocMentionTypeDocID, MentionValue: "RF-OLD-MENTION",
	})
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.ReplaceWorkspaceDocsWithReferenceSnapshot(context.Background(), db, "snapshot-good", docs, edges, oldMentions, blocks, records, bindings, model.ReentryMemorySnapshot{}); err != nil {
		t.Fatal(err)
	}

	prior, err := loadPriorDocSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if prior == nil {
		t.Fatal("expected current published snapshot")
	}
	stored, ok := prior.Docs[path]
	if !ok || stored.DocID != "RF-SNAPSHOT-OWNER" || stored.ContentHash == "" {
		t.Fatalf("snapshot owner/hash = %#v, want current owner and content hash", stored)
	}
	foundOldMention := false
	for _, mention := range prior.Mentions[path] {
		if mention.MentionValue == "RF-OLD-MENTION" {
			foundOldMention = true
			break
		}
	}
	if !foundOldMention {
		t.Fatalf("published prior mentions lost synthetic old mention: %#v", prior.Mentions[path])
	}
}

func TestLoadPriorDocSnapshotRejectsMissingOrOldIdentityMarker(t *testing.T) {
	for _, marker := range []struct {
		name  string
		value string
	}{
		{name: "missing", value: ""},
		{name: "old", value: "legacy-extraction-version"},
	} {
		t.Run(marker.name, func(t *testing.T) {
			root := t.TempDir()
			db, err := store.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{{Path: "wiki/legacy.md", Title: "Legacy", DocID: "RF-LEGACY", ContentHash: "content"}}, nil, nil, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			if marker.value != "" {
				if err := store.UpsertWorkspaceMeta(context.Background(), db, store.WorkspaceMetaDocIdentitySnapshotVersion, marker.value); err != nil {
					t.Fatal(err)
				}
			}
			prior, err := loadPriorDocSnapshot(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			if prior != nil {
				t.Fatalf("untrusted %s identity marker returned prior snapshot: %#v", marker.name, prior)
			}
		})
	}
}

func TestLoadPriorDocSnapshotReusesCurrentUnchangedSnapshot(t *testing.T) {
	root := t.TempDir()
	path := ".docs/wiki/04_RF/RF-SNAPSHOT-REUSE.md"
	content := "---\ndoc_id: RF-SNAPSHOT-REUSE\n---\n# RF-SNAPSHOT-REUSE\n\nUnchanged snapshot content.\n"
	if err := writeFileRoot(root, path, content); err != nil {
		t.Fatal(err)
	}
	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, firstBindings, _, err := docgraph.IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(context.Background(), root, matcher, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceWorkspaceDocsWithReferenceSnapshot(context.Background(), db, "snapshot-reuse", firstDocs, firstEdges, firstMentions, firstBlocks, firstRecords, firstBindings, model.ReentryMemorySnapshot{}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	prior, err := loadPriorDocSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if prior == nil {
		t.Fatal("expected current snapshot for unchanged reuse")
	}
	parsed, skipped := 0, 0
	progress := func(_ context.Context, value docgraph.Progress) error {
		if value.Stage == "docs.read" {
			parsed, skipped = value.Parsed, value.Skipped
		}
		return nil
	}
	secondDocs, _, _, _, _, _, _, err := docgraph.IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(context.Background(), root, matcher, progress, prior)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondDocs) != 1 || parsed != 0 || skipped != 1 || secondDocs[0].DocID != "RF-SNAPSHOT-REUSE" {
		t.Fatalf("unchanged snapshot reuse docs=%#v parsed=%d skipped=%d", secondDocs, parsed, skipped)
	}
}

func TestFailedDocSnapshotPublicationLeavesIdentityTrustInvalid(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.UpsertWorkspaceMeta(context.Background(), db, store.WorkspaceMetaDocIdentitySnapshotVersion, "legacy-extraction-version"); err != nil {
		t.Fatal(err)
	}
	job, err := store.CreateIndexJob(context.Background(), db, "snapshot-failure", root, store.IndexModeDocs, false)
	if err != nil {
		t.Fatal(err)
	}
	fence := store.IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := store.MarkIndexJobRunning(context.Background(), db, job.JobID, os.Getpid(), "indexing", fence); err != nil {
		t.Fatal(err)
	}
	restore := store.SetIndexPublicationBeforeCommitHookForTest(func() error { return errors.New("abort snapshot publication") })
	defer restore()
	err = store.ReplaceWorkspaceDocsForJobWithReferenceSnapshot(context.Background(), db, job.JobID, job.GenerationID, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{}, fence, nil)
	if err == nil {
		t.Fatal("failed publication unexpectedly succeeded")
	}
	current, err := store.DocIdentitySnapshotCurrent(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("failed publication advanced identity trust")
	}
}

// extensionsFromRegistry returns all extensions that language.IsSupportedCodePath recognises.
func extensionsFromRegistry() []string {
	all := []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".cs", ".go", ".py", ".pyi",
		".txt", ".json", ".md", ".scss", ".css"}
	var result []string
	for _, ext := range all {
		if language.IsSupportedCodePath("file" + ext) {
			result = append(result, ext)
		}
	}
	return result
}

// TestLanguageClassificationConsistency verifies that the language returned by
// languageForPath matches what the shared registry would return.
func TestLanguageClassificationConsistency(t *testing.T) {
	files := []struct {
		path   string
		expect string
	}{
		{"app.js", "javascript"},
		{"app.mjs", "javascript"},
		{"app.cjs", "javascript"},
		{"app.ts", "typescript"},
		{"app.mts", "typescript"},
		{"app.cts", "typescript"},
		{"app.cs", "csharp"},
		{"main.go", "go"},
		{"run.py", "python"},
		{"stub.pyi", "python"},
	}
	for _, f := range files {
		got := languageForPath(f.path)
		if got != f.expect {
			t.Errorf("languageForPath(%q) = %q; want %q", f.path, got, f.expect)
		}
	}
}

// TestLanguageFromExtConsistency verifies that languageFromExt matches ForPath.
func TestLanguageFromExtConsistency(t *testing.T) {
	testCases := []struct {
		ext    string
		expect string
	}{
		{".cs", "csharp"},
		{".go", "go"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".mts", "typescript"},
		{".cts", "typescript"},
		{".js", "javascript"},
		{".jsx", "javascript"},
		{".mjs", "javascript"},
		{".cjs", "javascript"},
		{".py", "python"},
		{".pyi", "python"},
		{"", ""},
		{".txt", ""},
	}
	for _, tc := range testCases {
		got := languageFromExt(tc.ext)
		if got != tc.expect {
			t.Errorf("languageFromExt(%q) = %q; want %q", tc.ext, got, tc.expect)
		}
	}
}

// TestIncrementalSkipsUnknownExtensions verifies that the incremental index
// skips files with unknown extensions (languageFromExt returns "").
func TestIncrementalSkipsUnknownExtensions(t *testing.T) {
	// This simulates the check in incrementalIndexWithGraphProgress:
	//   if languageFromExt(...) == "" { skippedFiles++ }
	skipPaths := []string{"doc.txt", "config.json", "style.scss"}
	for _, p := range skipPaths {
		if languageFromExt(filepath.Ext(p)) != "" {
			t.Errorf("languageFromExt(%q) returned %q; expected empty to trigger skip", p, languageFromExt(filepath.Ext(p)))
		}
	}
}

// TestExtractMeaningfulPathSegmentsModernExts verifies that extension stripping
// works for all modern extensions.
func TestExtractMeaningfulPathSegmentsModernExts(t *testing.T) {
	files := []struct {
		path string
		want string // expected segment after stripping
	}{
		{"src/foo.mjs", "foo"},
		{"src/foo.cjs", "foo"},
		{"src/foo.mts", "foo"},
		{"src/foo.cts", "foo"},
		{"src/foo.js", "foo"},
		{"src/foo.ts", "foo"},
		{"src/foo.js.config", "foo.js.config"}, // .config is not a known source ext
	}
	for _, f := range files {
		segments := extractMeaningfulPathSegments(filepath.Join("/tmp", f.path))
		found := false
		for _, seg := range segments {
			if seg == f.want {
				found = true
				break
			}
		}
		if !found && len(segments) > 0 {
			t.Errorf("extractMeaningfulPathSegments(%q) missing segment %q; got %v", f.path, f.want, segments)
		}
	}
	// Verify that .config (unknown ext) is NOT stripped.
	got := extractMeaningfulPathSegments("/tmp/src/foo.js.config")
	found := false
	for _, s := range got {
		if s == "foo.js.config" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("extractMeaningfulPathSegments(%q) lost unknown suffix; got %v", "src/foo.js.config", got)
	}
}

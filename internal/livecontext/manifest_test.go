package livecontext

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestValidateCanonicalRootRejectsSymlinkAncestor(t *testing.T) {
	base := t.TempDir()
	realParent := filepath.Join(base, "real-parent")
	realRoot := filepath.Join(realParent, "wiki")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateCanonicalRoot(realRoot); err != nil {
		t.Fatalf("direct real root rejected: %v", err)
	}

	targetParent := t.TempDir()
	targetRoot := filepath.Join(targetParent, "wiki")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasParent := filepath.Join(base, "alias-parent")
	if err := os.MkdirAll(aliasParent, 0o755); err != nil {
		t.Fatal(err)
	}
	ancestorLink := filepath.Join(aliasParent, "linked-parent")
	if err := os.Symlink(targetParent, ancestorLink); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := validateCanonicalRoot(filepath.Join(ancestorLink, "wiki")); err == nil {
		t.Fatal("canonical root accepted a directory below a symlink ancestor")
	}
}

func TestDiscoverCanonicalManifestExcludesRawAuditAndSymlinkDocs(t *testing.T) {
	root := t.TempDir()
	profile := model.DocsReadProfile{Version: 1, GenericDocs: model.DocsGenericFallback{Paths: []string{".docs/"}}}
	writeOverlayFile(t, root, ".docs/wiki/current.md", []byte("# current\n"))
	writeOverlayFile(t, root, ".docs/raw/raw.md", []byte("# raw\n"))
	writeOverlayFile(t, root, ".docs/auditoria/audit.md", []byte("# audit\n"))
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".docs", "wiki", "link.md")
	if err := os.Symlink(outside, link); err != nil {
		// Symlink support is optional on restricted Windows runners; all other
		// manifest assertions remain meaningful there.
		link = ""
	}
	entries, err := DiscoverCanonicalManifest(context.Background(), root, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	if len(paths) != 1 || paths[0] != ".docs/wiki/current.md" {
		t.Fatalf("canonical paths=%v link=%q", paths, link)
	}
}

func TestReconcileCanonicalDocsClassifiesUnchangedChangedAndDeleted(t *testing.T) {
	root := t.TempDir()
	currentPath := writeOverlayFile(t, root, "current.md", []byte("# current\n"))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(currentPath, old, old); err != nil {
		t.Fatal(err)
	}
	currentContent, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	state := overlayStateFor(t, root, "current.md", currentContent, model.DocsReadProfile{Version: 1})
	states := testArtifactStates{states: []model.DocArtifactState{state, model.DocArtifactState{Path: "gone.md", Lifecycle: model.DocLifecycleActive}}}
	changes, err := ReconcileCanonicalDocs(context.Background(), []string{root}, states)
	if err != nil {
		t.Fatal(err)
	}
	classifications := map[string]string{}
	for _, change := range changes {
		classifications[change.Path] = change.Classification
	}
	if classifications["current.md"] != DocChangeUnchanged || classifications["gone.md"] != DocChangeDeleted {
		t.Fatalf("initial classifications=%v", classifications)
	}
	if err := os.WriteFile(currentPath, []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, err = ReconcileCanonicalDocs(context.Background(), []string{root}, states)
	if err != nil {
		t.Fatal(err)
	}
	classifications = map[string]string{}
	for _, change := range changes {
		classifications[change.Path] = change.Classification
	}
	if classifications["current.md"] != DocChangeChanged {
		t.Fatalf("changed classifications=%v", classifications)
	}
}

func TestReconcileCanonicalDocsClassifiesExcludedAliveWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	ignoredPath := writeOverlayFile(t, root, "ignored.md", []byte("# ignored\n"))
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(ignoredPath)
	if err != nil {
		t.Fatal(err)
	}
	state := overlayStateFor(t, root, "ignored.md", content, model.DocsReadProfile{Version: 1})
	changes, err := ReconcileCanonicalDocs(context.Background(), []string{root}, testArtifactStates{states: []model.DocArtifactState{state}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Classification != DocChangeExcludedAlive || changes[0].Action != "omit" {
		t.Fatalf("excluded-alive changes=%+v", changes)
	}
}

func TestReconcileCanonicalDocsDoesNotExposeAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	content := []byte("# safe\n")
	writeOverlayFile(t, root, "safe.md", content)
	changes, err := ReconcileCanonicalDocs(context.Background(), []string{root}, testArtifactStates{})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if filepath.IsAbs(change.Path) || strings.Contains(change.Path, root) {
			t.Fatalf("absolute path leaked in change=%+v", change)
		}
	}
}

func TestNormalizeScopeSortsSelectorsForDigest(t *testing.T) {
	first, err := normalizeScope(model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/b.md", "docs/a.md", "docs/a.md"}, DocIDs: []string{"RF-B", "RF-A"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeScope(model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/a.md", "docs/b.md"}, DocIDs: []string{"RF-A", "RF-B"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != second.Kind || strings.Join(first.DocPaths, ",") != strings.Join(second.DocPaths, ",") || strings.Join(first.DocIDs, ",") != strings.Join(second.DocIDs, ",") {
		t.Fatalf("scope normalization differs first=%+v second=%+v", first, second)
	}
}

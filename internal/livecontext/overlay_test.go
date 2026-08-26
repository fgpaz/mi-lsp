package livecontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type testArtifactStates struct {
	states []model.DocArtifactState
	err    error
}

func (r testArtifactStates) ListDocArtifactStates(context.Context) ([]model.DocArtifactState, error) {
	return append([]model.DocArtifactState(nil), r.states...), r.err
}

func overlayTestProfile() model.DocsReadProfile {
	return model.DocsReadProfile{
		Version: 1,
		Families: []model.DocsReadFamily{{Name: "functional", Paths: []string{"docs/"}}},
		GenericDocs: model.DocsGenericFallback{},
	}
}

func overlayDoc(targetPath, targetSymbol string) []byte {
	return []byte(strings.Join([]string{
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"id: RF-OVERLAY-001",
		"",
		"```toon",
		"wiki_source_protocol: SDD-WIKI-SOURCE-v1",
		"id: RF-OVERLAY-001",
		"block_id: RF-OVERLAY-001.core",
		"artifact_bindings:",
		"  - target_kind: symbol",
		"    relation: implements",
		"    target_path: " + targetPath,
		"    target_symbol: " + targetSymbol,
		"```",
		"",
	}, "\n"))
}

func writeOverlayFile(t *testing.T, root, relative string, content []byte) string {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return absolute
}

func overlayStateFor(t *testing.T, root, relative string, content []byte, profile model.DocsReadProfile) model.DocArtifactState {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Stat(absolute)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return model.DocArtifactState{
		Path:                relative,
		Size:                info.Size(),
		MtimeNsec:           info.ModTime().UnixNano(),
		ContentSHA256:       hex.EncodeToString(sum[:]),
		ParserVersion:       model.ParserVersion,
		AuthorityConfigHash: store.AuthorityConfigDigestForProfile(profile, nil),
		Lifecycle:           model.DocLifecycleActive,
	}
}

func overlayRequest(root string, profile model.DocsReadProfile, scope model.WikiCodeScope, states []model.DocArtifactState) OverlayRequest {
	return OverlayRequest{
		WorkspaceRoot: root,
		BaseGeneration: "docs=g0",
		Profile:        &profile,
		Scope:          scope,
		StateReader:    testArtifactStates{states: states},
	}
}

func TestOverlayUnchangedDocumentZeroBodyRead(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	content := overlayDoc("src/current.go", "Current")
	path := writeOverlayFile(t, root, "docs/current.md", content)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	state := overlayStateFor(t, root, "docs/current.md", content, profile)
	calls := 0
	priorRead := safeReadBytes
	safeReadBytes = func(string) ([]byte, error) {
		calls++
		return nil, errors.New("unchanged document must not be read")
	}
	defer func() { safeReadBytes = priorRead }()
	overlay, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/current.md"}}, []model.DocArtifactState{state}))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || overlay.Cost.FilesHashed != 0 || overlay.Cost.UnchangedReused != 1 {
		t.Fatalf("unchanged costs=%+v body_reads=%d", overlay.Cost, calls)
	}
	if len(overlay.Additions) != 0 || len(overlay.Tombstones) != 0 {
		t.Fatalf("unchanged overlay changed persisted rows: %+v", overlay)
	}
}

func TestOverlayEditAddDeleteAndRemovedDeclarationTombstone(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	oldContent := overlayDoc("src/old.go", "Old")
	newContent := overlayDoc("src/new.go", "New")
	writeOverlayFile(t, root, "src/old.go", []byte("package old\n"))
	writeOverlayFile(t, root, "src/new.go", []byte("package new\n"))
	docPath := writeOverlayFile(t, root, "docs/change.md", oldContent)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(docPath, old, old); err != nil {
		t.Fatal(err)
	}
	state := overlayStateFor(t, root, "docs/change.md", oldContent, profile)
	if err := os.WriteFile(docPath, newContent, 0o644); err != nil {
		t.Fatal(err)
	}
	edited, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/change.md"}}, []model.DocArtifactState{state}))
	if err != nil {
		t.Fatal(err)
	}
	if len(edited.Additions) != 1 || edited.Additions[0].TargetPath != "src/new.go" {
		t.Fatalf("edited additions=%+v", edited.Additions)
	}
	if len(edited.Tombstones) != 1 || edited.Tombstones[0].DocPath != "docs/change.md" {
		t.Fatalf("edited tombstones=%+v", edited.Tombstones)
	}
	oldBinding := model.DocArtifactBinding{
		DocPath: "docs/change.md", BlockID: "RF-OVERLAY-001.core", Relation: model.RelationImplements,
		TargetPath: "src/old.go", TargetSymbol: "Old", TargetKind: model.TargetKindSymbol,
		BindingRef: model.WikiCodeBindingRef("docs/change.md", "RF-OVERLAY-001.core", "RF-OVERLAY-001", model.RelationImplements, "src/old.go", "Old", model.TargetKindSymbol),
	}
	if got := EffectiveBindings([]model.DocArtifactBinding{oldBinding}, edited); len(got) != 0 {
		t.Fatalf("removed declaration remained active: %+v", got)
	}
	if err := os.Remove(docPath); err != nil {
		t.Fatal(err)
	}
	deleted, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/change.md"}}, []model.DocArtifactState{state}))
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Tombstones) != 1 || deleted.ChangedInputs[0].Classification != DocChangeDeleted {
		t.Fatalf("deleted overlay=%+v", deleted)
	}
}

func TestOverlayReverseDiscoversNewBinding(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	writeOverlayFile(t, root, "src/new.go", []byte("package new\n"))
	writeOverlayFile(t, root, "docs/new.md", overlayDoc("src/new.go", "New"))
	overlay, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: "src/new.go", TargetSymbol: "New"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(overlay.Additions) != 1 || overlay.Additions[0].TargetPath != "src/new.go" {
		t.Fatalf("reverse additions=%+v", overlay.Additions)
	}
	if overlay.Cost.FilesParsed == 0 || overlay.Cost.FilesHashed == 0 {
		t.Fatalf("reverse cost=%+v", overlay.Cost)
	}
}

func TestOverlayManifestExcludesRawAndAuditDocuments(t *testing.T) {
	root := t.TempDir()
	profile := model.DocsReadProfile{Version: 1, GenericDocs: model.DocsGenericFallback{Paths: []string{".docs/"}}}
	writeOverlayFile(t, root, "src/current.go", []byte("package current\n"))
	writeOverlayFile(t, root, ".docs/raw/raw.md", overlayDoc("src/current.go", "Raw"))
	writeOverlayFile(t, root, ".docs/auditoria/audit.md", overlayDoc("src/current.go", "Audit"))
	overlay, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: "src/current.go"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(overlay.Additions) != 0 {
		t.Fatalf("raw/audit additions leaked: %+v", overlay.Additions)
	}
}

func TestOverlaySameSizeSameTickRewriteIsHashed(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	oldContent := overlayDoc("src/one.go", "One")
	newContent := overlayDoc("src/two.go", "Two")
	writeOverlayFile(t, root, "src/one.go", []byte("package one\n"))
	writeOverlayFile(t, root, "src/two.go", []byte("package two\n"))
	docPath := writeOverlayFile(t, root, "docs/rewrite.md", oldContent)
	// Keep the stamp inside the racily-clean window while the test builds the
	// replacement, without relying on filesystem timestamp precision.
	tick := time.Now().Add(time.Second)
	if err := os.Chtimes(docPath, tick, tick); err != nil {
		t.Fatal(err)
	}
	state := overlayStateFor(t, root, "docs/rewrite.md", oldContent, profile)
	if err := os.WriteFile(docPath, newContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(docPath, tick, tick); err != nil {
		t.Fatal(err)
	}
	overlay, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/rewrite.md"}}, []model.DocArtifactState{state}))
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Cost.FilesHashed != 1 || len(overlay.Additions) != 1 || overlay.Additions[0].TargetPath != "src/two.go" {
		t.Fatalf("same-tick rewrite overlay=%+v cost=%+v", overlay, overlay.Cost)
	}
}

func TestSafeReadFileConcurrentChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changing.md")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	priorRead := safeReadBytes
	reads := 0
	safeReadBytes = func(filePath string) ([]byte, error) {
		reads++
		content := []byte(strings.Repeat("x", reads+1))
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			return nil, err
		}
		return content, nil
	}
	defer func() { safeReadBytes = priorRead }()
	_, status, err := SafeReadFile(path)
	var concurrent *model.ErrConcurrentChange
	if status != ReadStatusConcurrent || !errors.As(err, &concurrent) || reads != 2 {
		t.Fatalf("status=%q reads=%d err=%v", status, reads, err)
	}
}

func TestSafeResolveTargetRejectsUnsafePathAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	writeOverlayFile(t, root, "src/current.go", []byte("package current\n"))
	if _, omission, err := SafeResolveTarget(root, "../outside.go"); omission != model.OmissionUnsafeTarget || err == nil {
		t.Fatalf("traversal result omission=%q err=%v", omission, err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "src", "outside.go")
	if err := os.Symlink(outside, link); err == nil {
		if _, omission, err := SafeResolveTarget(root, "src/outside.go"); omission != model.OmissionUnsafeTarget || err == nil {
			t.Fatalf("symlink escape result omission=%q err=%v", omission, err)
		}
	}
	resolved, omission, err := SafeResolveTarget(root, "src/current.go")
	if err != nil || omission != "" || !strings.HasSuffix(filepath.ToSlash(resolved), "src/current.go") {
		t.Fatalf("safe target resolved=%q omission=%q err=%v", resolved, omission, err)
	}
}

func TestOverlayUnsafeSymlinkTargetIsOmitted(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "src", "outside.go")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink creation unavailable")
	}
	writeOverlayFile(t, root, "docs/unsafe.md", overlayDoc("src/outside.go", "Outside"))
	overlay, err := BuildOverlay(context.Background(), overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/unsafe.md"}}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(overlay.Additions) != 0 || len(overlay.Omissions) == 0 || overlay.Omissions[0].Code != model.OmissionUnsafeTarget {
		t.Fatalf("unsafe target overlay=%+v", overlay)
	}
}

func TestOverlayDeterministicOrderingAndDigest(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	writeOverlayFile(t, root, "src/a.go", []byte("package a\n"))
	writeOverlayFile(t, root, "src/b.go", []byte("package b\n"))
	writeOverlayFile(t, root, "docs/b.md", overlayDoc("src/b.go", "B"))
	writeOverlayFile(t, root, "docs/a.md", overlayDoc("src/a.go", "A"))
	reversed := overlayRequest(root, profile, model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/b.md", "docs/a.md"}}, nil)
	first, err := BuildOverlay(context.Background(), reversed)
	if err != nil {
		t.Fatal(err)
	}
	reversed.Scope.DocPaths = []string{"docs/a.md", "docs/b.md"}
	second, err := BuildOverlay(context.Background(), reversed)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == "" || first.Digest != second.Digest || !reflect.DeepEqual(first.Additions, second.Additions) {
		t.Fatalf("non-deterministic overlays first=%+v second=%+v", first, second)
	}
}

func TestOverlayDigestExcludesTimestampsAndCostTelemetry(t *testing.T) {
	base := model.WikiCodeOverlay{
		BaseGeneration: "docs=g0",
		Mode: model.OverlayModeRAMOnly,
		Scope: model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/current.md"}},
		Additions: []model.DocArtifactBinding{{DocPath: "docs/current.md", TargetPath: "src/current.go", BindingRef: "ref", IndexedAt: 10}},
		ChangedInputs: []model.WikiCodeChangedInput{{Path: "docs/current.md", Classification: DocChangeChanged, Status: string(ReadStatusStable)}},
	}
	first := OverlayDigest(base)
	base.Additions[0].IndexedAt = 9999
	base.ChangedInputs[0].Status = string(ReadStatusRacy)
	base.Cost = model.WikiCodeOverlayCost{MetadataChecked: 20, FilesHashed: 3, BytesRead: 900}
	if got := OverlayDigest(base); got != first {
		t.Fatalf("digest changed with timestamp/status/cost telemetry: %q != %q", got, first)
	}
}

func TestOverlayPlannedAndRetiredBindingsAreFiltered(t *testing.T) {
	base := []model.DocArtifactBinding{
		{DocPath: "docs/planned.md", TargetPath: "src/planned.go", BindingRef: "planned", BindingStatus: model.BindingStatusPlanned, DocLifecycle: model.DocLifecycleActive},
		{DocPath: "docs/retired.md", TargetPath: "src/retired.go", BindingRef: "retired", BindingStatus: model.BindingStatusExact, DocLifecycle: model.DocLifecycleRetired},
		{DocPath: "docs/current.md", TargetPath: "src/current.go", BindingRef: "current", BindingStatus: model.BindingStatusExact, DocLifecycle: model.DocLifecycleActive},
	}
	got := EffectiveBindings(base, model.WikiCodeOverlay{})
	if len(got) != 1 || got[0].BindingRef != "current" {
		t.Fatalf("default filtered bindings=%+v", got)
	}
	got = EffectiveBindingsWithOptions(base, model.WikiCodeOverlay{}, true, true)
	if len(got) != 3 {
		t.Fatalf("explicit historical/planned bindings=%+v", got)
	}
}

func TestOverlayQueryDoesNotWriteSQLite(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	writeOverlayFile(t, root, "docs/current.md", overlayDoc("src/current.go", "Current"))
	writeOverlayFile(t, root, "src/current.go", []byte("package current\n"))
	writer, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenReadOnlyExisting(root, store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before, err := os.ReadFile(store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	var beforeCount int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM doc_artifact_bindings").Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	_, err = BuildOverlay(context.Background(), OverlayRequest{
		WorkspaceRoot:  root,
		DB:             db,
		BaseGeneration: "docs=g0",
		Profile:        &profile,
		Scope:          model.WikiCodeScope{Kind: model.ScopeExactWiki, DocPaths: []string{"docs/current.md"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("overlay changed the SQLite database")
	}
	var afterCount int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM doc_artifact_bindings").Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if beforeCount != afterCount {
		t.Fatalf("overlay changed binding row count: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestOverlayUsesCanonicalIgnoreMatcher(t *testing.T) {
	root := t.TempDir()
	profile := overlayTestProfile()
	writeOverlayFile(t, root, ".gitignore", "docs/ignored.md\n")
	writeOverlayFile(t, root, "src/current.go", []byte("package current\n"))
	writeOverlayFile(t, root, "docs/ignored.md", overlayDoc("src/current.go", "Ignored"))
	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := BuildOverlay(context.Background(), OverlayRequest{
		WorkspaceRoot: root,
		BaseGeneration: "docs=g0",
		Profile: &profile,
		Matcher: matcher,
		Scope: model.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: "src/current.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(overlay.Additions) != 0 {
		t.Fatalf("ignored canonical document leaked: %+v", overlay.Additions)
	}
}

package service

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestWikiCodeResolverForwardSeparatesDirectTestsAndGraphSupporting(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/service.mjs", "export function runDemo() { return helper(); }\n")
	writeBridgeTestFile(t, root, "test/service.test.mjs", "import { runDemo } from '../src/service.mjs';\nexport function testRunDemo() { return runDemo(); }\n")
	db := bridgeTestDB(t, root)
	docPath := ".docs/wiki/04_RF/RF-DEMO-001.md"
	doc := model.DocRecord{Path: docPath, DocID: "RF-DEMO-001", Layer: "04", Family: "functional"}
	bindings := []model.DocArtifactBinding{
		bridgeBinding(docPath, "RF-DEMO-001.bindings", doc.DocID, model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1),
		bridgeBinding(docPath, "RF-DEMO-001.bindings", doc.DocID, model.RelationTests, "test/service.test.mjs", "", model.TargetKindTest, 2),
	}
	if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{doc}, nil, nil, nil, nil, bindings); err != nil {
		t.Fatal(err)
	}
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "bridge", Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}}}
	files := []model.FileRecord{
		{FilePath: "src/service.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1([]byte("export function runDemo() { return helper(); }\n"))},
		{FilePath: "test/service.test.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1([]byte("import { runDemo } from '../src/service.mjs';\nexport function testRunDemo() { return runDemo(); }\n"))},
	}
	symbols := []model.SymbolRecord{{FilePath: "src/service.mjs", RepoID: "main", Name: "runDemo", QualifiedName: "runDemo", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: files[0].ContentHash}}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{docPath}, TokenBudget: 20_000}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 1 || got.DirectCode[0].Path != "src/service.mjs" || got.DirectCode[0].Symbol != "runDemo" || got.DirectCode[0].Status != model.WikiCodeStatusResolvedSymbol {
		t.Fatalf("direct code = %#v", got.DirectCode)
	}
	if got.DirectCode[0].Origin != "wiki_declared_path" || got.DirectCode[0].ObservedOrigin != "catalog_observed" || got.DirectCode[0].Relation != model.RelationImplements || got.DirectCode[0].BindingRef == "" || got.DirectCode[0].SourceDoc != docPath || got.DirectCode[0].SourceBlock == "" || got.DirectCode[0].SourceLine != 1 {
		t.Fatalf("direct provenance = %#v", got.DirectCode[0])
	}
	if len(got.Tests) != 1 || got.Tests[0].Path != "test/service.test.mjs" || len(got.DirectCode) == len(got.Tests) && got.DirectCode[0].Path == got.Tests[0].Path {
		t.Fatalf("test separation direct=%#v tests=%#v", got.DirectCode, got.Tests)
	}
	if len(got.SupportingCode) != 0 {
		t.Fatalf("graph-unavailable query promoted support: %#v", got.SupportingCode)
	}
	for _, item := range got.DirectCode {
		if item.Path == "src/helper.mjs" {
			t.Fatal("helper reached direct implementation evidence")
		}
	}
	if !hasBridgeOmission(got, model.WikiCodeStatusGraphUnavailable) {
		t.Fatalf("missing graph-unavailable omission: %#v", got.Omissions)
	}
	if got.Classification != model.WikiCodeClassificationDirectSDDBinding {
		t.Fatalf("classification = %q", got.Classification)
	}
}

func TestWikiCodeResolverForwardStatusesAndCandidateOrdering(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/present.mjs", "export function Present() {}\n")
	writeBridgeTestFile(t, root, "src/ambiguous.mjs", "export function Duplicate() {}\nexport function Duplicate() {}\n")
	db := bridgeTestDB(t, root)
	docPath := ".docs/wiki/04_RF/RF-STATUS.md"
	doc := model.DocRecord{Path: docPath, DocID: "RF-STATUS", Layer: "04", Family: "functional"}
	bindings := []model.DocArtifactBinding{
		bridgeBinding(docPath, "status", doc.DocID, model.RelationImplements, "src/present.mjs", "Missing", model.TargetKindSymbol, 1),
		bridgeBinding(docPath, "status", doc.DocID, model.RelationImplements, "src/ambiguous.mjs", "Duplicate", model.TargetKindSymbol, 2),
		bridgeBinding(docPath, "status", doc.DocID, model.RelationImplements, "src/present.mjs", "Present", model.TargetKindSymbol, 3),
		bridgeBinding(docPath, "status", doc.DocID, model.RelationImplements, "src/present.mjs", "", model.TargetKindFile, 4),
		bridgeBinding(docPath, "status", doc.DocID, model.RelationImplements, "../outside.mjs", "", model.TargetKindFile, 5),
	}
	if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{doc}, nil, nil, nil, nil, bindings); err != nil {
		t.Fatal(err)
	}
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "bridge", Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}}}
	present := []byte("export function Present() {}\n")
	ambiguous := []byte("export function Duplicate() {}\nexport function Duplicate() {}\n")
	files := []model.FileRecord{
		{FilePath: "src/present.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1(present)},
		{FilePath: "src/ambiguous.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1(ambiguous)},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "src/present.mjs", RepoID: "main", Name: "Present", QualifiedName: "Present", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: files[0].ContentHash},
		{FilePath: "src/ambiguous.mjs", RepoID: "main", Name: "Duplicate", QualifiedName: "DuplicateA", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: files[1].ContentHash},
		{FilePath: "src/ambiguous.mjs", RepoID: "main", Name: "Duplicate", QualifiedName: "DuplicateB", Kind: "function", Language: "javascript", StartLine: 2, EndLine: 2, FileHash: files[1].ContentHash},
	}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{docPath}, TokenBudget: 20_000}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 3 {
		t.Fatalf("direct code = %#v", got.DirectCode)
	}
	if !hasBridgeOmission(got, model.WikiCodeStatusMissingPath) || !hasBridgeOmission(got, model.WikiCodeStatusMissingSymbol) || !hasBridgeOmission(got, model.WikiCodeStatusAmbiguousSymbol) || !hasBridgeOmission(got, model.WikiCodeStatusUnsafeTarget) {
		t.Fatalf("statuses = %#v", got.Omissions)
	}
	if len(got.Candidates) == 0 {
		t.Fatalf("missing lexical candidates for unresolved symbol: %#v", got)
	}
}

func TestWikiCodeResolverGraphStalePreservesDirectAndSuppressesSupporting(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/service.mjs", "export function runDemo() {}\n")
	db := bridgeTestDB(t, root)
	docPath := ".docs/wiki/04_RF/RF-STALE.md"
	doc := model.DocRecord{Path: docPath, DocID: "RF-STALE", Layer: "04", Family: "functional"}
	binding := bridgeBinding(docPath, "stale", doc.DocID, model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol)
	if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{doc}, nil, nil, nil, nil, []model.DocArtifactBinding{binding}); err != nil {
		t.Fatal(err)
	}
	content := []byte("export function runDemo() {}\n")
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "bridge", Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}}}
	file := model.FileRecord{FilePath: "src/service.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1(content)}
	symbol := model.SymbolRecord{FilePath: file.FilePath, RepoID: "main", Name: "runDemo", QualifiedName: "runDemo", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: file.ContentHash}
	if err := store.ReplaceCatalog(context.Background(), db, project, []model.FileRecord{file}, []model.SymbolRecord{symbol}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{docPath}, GraphFreshness: model.GraphFreshness{State: model.GraphFreshnessStale}}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 1 || got.DirectCode[0].Status != model.WikiCodeStatusResolvedSymbol || len(got.SupportingCode) != 0 {
		t.Fatalf("stale result direct=%#v support=%#v", got.DirectCode, got.SupportingCode)
	}
	if !hasBridgeOmission(got, model.WikiCodeStatusGraphStale) {
		t.Fatalf("stale graph omission missing: %#v", got.Omissions)
	}
}

func TestWikiCodeResolverOverlayTombstonesAndAdditions(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/old.mjs", "export function Old() {}\n")
	writeBridgeTestFile(t, root, "src/new.mjs", "export function New() {}\n")
	db := bridgeTestDB(t, root)
	docPath := ".docs/wiki/04_RF/RF-OVERLAY.md"
	doc := model.DocRecord{Path: docPath, DocID: "RF-OVERLAY", Layer: "04", Family: "functional"}
	oldBinding := bridgeBinding(docPath, "overlay", doc.DocID, model.RelationImplements, "src/old.mjs", "Old", model.TargetKindSymbol, 1)
	if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{doc}, nil, nil, nil, nil, []model.DocArtifactBinding{oldBinding}); err != nil {
		t.Fatal(err)
	}
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "bridge", Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}}}
	oldContent := []byte("export function Old() {}\n")
	newContent := []byte("export function New() {}\n")
	files := []model.FileRecord{{FilePath: "src/old.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1(oldContent)}, {FilePath: "src/new.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1(newContent)}}
	symbols := []model.SymbolRecord{{FilePath: "src/old.mjs", RepoID: "main", Name: "Old", QualifiedName: "Old", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: files[0].ContentHash}, {FilePath: "src/new.mjs", RepoID: "main", Name: "New", QualifiedName: "New", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: files[1].ContentHash}}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatal(err)
	}
	addition := bridgeBinding(docPath, "overlay", doc.DocID, model.RelationImplements, "src/new.mjs", "New", model.TargetKindSymbol, 2)
	overlay := model.WikiCodeOverlay{Status: "overlay", Additions: []model.DocArtifactBinding{addition}, Tombstones: []model.BindingTombstone{bridgeTombstone(t, oldBinding.BindingRef)}}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{docPath}}, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 1 || got.DirectCode[0].Path != "src/new.mjs" || got.DirectCode[0].Symbol != "New" {
		t.Fatalf("overlay direct code = %#v", got.DirectCode)
	}
	for _, item := range got.DirectCode {
		if item.Path == "src/old.mjs" {
			t.Fatal("tombstoned old binding remained active")
		}
	}
}

func TestWikiCodeResolverReverseReturnsActiveWikiAndRFFLParents(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/service.mjs", "export function runDemo() {}\n")
	db := bridgeTestDB(t, root)
	rfPath := ".docs/wiki/04_RF/RF-DEMO-001.md"
	flPath := ".docs/wiki/03_FL/FL-DEMO-001.md"
	rf := model.DocRecord{Path: rfPath, DocID: "RF-DEMO-001", Layer: "04", Family: "functional"}
	fl := model.DocRecord{Path: flPath, DocID: "FL-DEMO-001", Layer: "03", Family: "functional"}
	binding := bridgeBinding(rfPath, "binding", rf.DocID, model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1)
	edge := model.DocEdge{FromPath: rfPath, ToPath: flPath, ToDocID: fl.DocID, Kind: "doc_wikilink"}
	if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{rf, fl}, []model.DocEdge{edge}, nil, nil, nil, []model.DocArtifactBinding{binding}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionCodeToWiki, TargetPath: "src\\service.mjs", TargetSymbol: "runDemo"}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.WikiContext) != 1 || got.WikiContext[0].DocID != rf.DocID || got.WikiContext[0].BindingRef != binding.BindingRef {
		t.Fatalf("reverse context = %#v", got.WikiContext)
	}
	if len(got.WikiContext[0].Parents) != 1 || got.WikiContext[0].Parents[0].DocID != fl.DocID {
		t.Fatalf("reverse parents = %#v", got.WikiContext[0].Parents)
	}
}

func TestWikiCodeResolverRetiredPlannedRawAuditAndDuplicateIDFailClosed(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/service.mjs", "export function runDemo() {}\n")
	db := bridgeTestDB(t, root)
	activePath := ".docs/wiki/04_RF/RF-ACTIVE.md"
	retiredPath := ".docs/wiki/_retired/RF-OLD.md"
	plannedPath := ".docs/wiki/04_RF/RF-PLANNED.md"
	docs := []model.DocRecord{{Path: activePath, DocID: "RF-ACTIVE", Layer: "04", Family: "functional"}, {Path: retiredPath, DocID: "RF-OLD", Layer: "04", Family: "functional"}, {Path: plannedPath, DocID: "RF-PLANNED", Layer: "04", Family: "functional"}, {Path: ".docs/raw/decoy.md", DocID: "RF-RAW"}, {Path: ".docs/auditoria/decoy.md", DocID: "RF-AUDIT"}}
	active := bridgeBinding(activePath, "b", "RF-ACTIVE", model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1)
	retired := bridgeBinding(retiredPath, "b", "RF-OLD", model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 2)
	retired.DocLifecycle = model.DocLifecycleRetired
	retired.SupersededBy = "RF-ACTIVE"
	planned := bridgeBinding(plannedPath, "b", "RF-PLANNED", model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1)
	planned.BindingStatus = model.BindingStatusPlanned
	raw := bridgeBinding(".docs/raw/decoy.md", "b", "RF-RAW", model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1)
	audit := bridgeBinding(".docs/auditoria/decoy.md", "b", "RF-AUDIT", model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1)
	if err := store.ReplaceDocsWithSources(context.Background(), db, docs, nil, nil, nil, nil, []model.DocArtifactBinding{active, retired, planned, raw, audit}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{activePath}}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 1 || got.DirectCode[0].DocID != "RF-ACTIVE" {
		t.Fatalf("raw/audit/retired rows reached direct output: %#v", got.DirectCode)
	}
	redirect, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionCodeToWiki, TargetPath: "src/service.mjs", TargetSymbol: "runDemo", ExplicitHistorical: true}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	foundRedirect := false
	for _, item := range redirect.WikiContext {
		if item.Status == "redirect" && item.DocID == "RF-OLD" && item.SupersededBy == "RF-ACTIVE" {
			foundRedirect = true
		}
	}
	if !foundRedirect {
		t.Fatalf("historical redirect missing: %#v", redirect.WikiContext)
	}

	duplicateDocs := append([]model.DocRecord{}, docs...)
	duplicateDocs = append(duplicateDocs, model.DocRecord{Path: ".docs/wiki/04_RF/RF-ACTIVE-2.md", DocID: "RF-ACTIVE", Layer: "04", Family: "functional"})
	if err := store.ReplaceDocsWithSources(context.Background(), db, duplicateDocs, nil, nil, nil, nil, []model.DocArtifactBinding{active}); err != nil {
		t.Fatal(err)
	}
	_, err = ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{activePath}}, model.WikiCodeOverlay{})
	var contextErr *model.WikiCodeContextError
	if !errors.As(err, &contextErr) || contextErr.Code != "GPH_WIKI_DUPLICATE_DOC_ID" {
		t.Fatalf("duplicate ID error = %v", err)
	}
}

func TestWikiCodeResolverCatalogUnavailableAndNoBindingStatuses(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/unmapped.mjs", "export function unmapped() {}\n")
	binding := bridgeBinding(".docs/wiki/04_RF/RF-CATALOG.md", "catalog", "RF-CATALOG", model.RelationImplements, "src/unmapped.mjs", "unmapped", model.TargetKindSymbol)
	got, err := ResolveWikiCodeContext(context.Background(), root, nil, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{binding.DocPath}}, model.WikiCodeOverlay{Additions: []model.DocArtifactBinding{binding}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 1 || got.DirectCode[0].Status != model.WikiCodeStatusResolvedFile || !hasBridgeOmission(got, model.WikiCodeStatusCatalogUnavailable) {
		t.Fatalf("catalog-unavailable result = %#v", got)
	}

	unmapped, err := ResolveWikiCodeContext(context.Background(), root, nil, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionCodeToWiki, TargetPath: "src/unmapped.mjs", TargetSymbol: "unmapped"}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(unmapped.WikiContext) != 0 || unmapped.Classification != model.WikiCodeClassificationUnmappedChangedCode {
		t.Fatalf("unmapped result = %#v", unmapped)
	}
	if len(unmapped.DirectCode) != 0 || len(unmapped.Tests) != 0 {
		t.Fatalf("unmapped query manufactured code evidence: %#v", unmapped)
	}
	if len(unmapped.NextQueries) == 0 {
		t.Fatal("unmapped result did not expose bounded next queries")
	}
}

func TestWikiCodeResolverConcurrentTargetChangeIsOmitted(t *testing.T) {
	root := t.TempDir()
	oldContent := []byte("export function runDemo() {}\n")
	newContent := []byte("export function runDemo() { return 2; }\n")
	writeBridgeTestFile(t, root, "src/service.mjs", string(newContent))
	db := bridgeTestDB(t, root)
	docPath := ".docs/wiki/04_RF/RF-CONCURRENT.md"
	doc := model.DocRecord{Path: docPath, DocID: "RF-CONCURRENT", Layer: "04", Family: "functional"}
	binding := bridgeBinding(docPath, "concurrent", doc.DocID, model.RelationImplements, "src/service.mjs", "runDemo", model.TargetKindSymbol, 1)
	if err := store.ReplaceDocsWithSources(context.Background(), db, []model.DocRecord{doc}, nil, nil, nil, nil, []model.DocArtifactBinding{binding}); err != nil {
		t.Fatal(err)
	}
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "bridge", Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}}}
	file := model.FileRecord{FilePath: "src/service.mjs", RepoID: "main", Language: "javascript", ContentHash: bridgeSHA1(oldContent)}
	symbol := model.SymbolRecord{FilePath: file.FilePath, RepoID: "main", Name: "runDemo", QualifiedName: "runDemo", Kind: "function", Language: "javascript", StartLine: 1, EndLine: 1, FileHash: file.ContentHash}
	if err := store.ReplaceCatalog(context.Background(), db, project, []model.FileRecord{file}, []model.SymbolRecord{symbol}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWikiCodeContext(context.Background(), root, db, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{docPath}}, model.WikiCodeOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DirectCode) != 0 || !hasBridgeOmission(got, model.WikiCodeStatusConcurrentChange) {
		t.Fatalf("concurrent target was presented as current: %#v", got)
	}
}

func TestWikiCodeResolverRejectsSelfEdgesAndReportsClosedClassification(t *testing.T) {
	source := model.GraphNodeRecord{NodeID: 7, Identity: model.NodeKeyFields{OwnerPath: "src/service.mjs", SemanticIdentity: "runDemo"}}
	target := source
	if !bridgeGraphSelfEdge(source, target, model.WikiCodeEvidence{Path: "src/service.mjs", Symbol: "runDemo"}) {
		t.Fatal("source self-edge was accepted")
	}
	other := model.GraphNodeRecord{NodeID: 8, Identity: model.NodeKeyFields{OwnerPath: "src/helper.mjs", SemanticIdentity: "helper"}}
	if bridgeGraphSelfEdge(source, other, model.WikiCodeEvidence{Path: "src/service.mjs", Symbol: "runDemo"}) {
		t.Fatal("distinct supporting node was rejected as self-edge")
	}
	for _, classification := range []string{model.WikiCodeClassificationDirectSDDBinding, model.WikiCodeClassificationSupportingCode, model.WikiCodeClassificationSharedTechnical, model.WikiCodeClassificationGeneratedOrVendor, model.WikiCodeClassificationMechanicalNoImpact, model.WikiCodeClassificationUnmappedChangedCode} {
		if classification == "" {
			t.Fatal("closed classification contains an empty value")
		}
	}
}

func TestWikiCodeResolverDigestStableForThirtyOrderEquivalentInputs(t *testing.T) {
	root := t.TempDir()
	writeBridgeTestFile(t, root, "src/a.mjs", "export function A() {}\n")
	writeBridgeTestFile(t, root, "src/b.mjs", "export function B() {}\n")
	bindings := []model.DocArtifactBinding{
		bridgeBinding(".docs/wiki/04_RF/RF-ORDER.md", "order", "RF-ORDER", model.RelationOperates, "src/b.mjs", "", model.TargetKindFile),
		bridgeBinding(".docs/wiki/04_RF/RF-ORDER.md", "order", "RF-ORDER", model.RelationImplements, "src/a.mjs", "", model.TargetKindFile),
	}
	var want string
	for i := 0; i < 30; i++ {
		if i%2 == 1 {
			bindings[0], bindings[1] = bindings[1], bindings[0]
		}
		got, err := ResolveWikiCodeContext(context.Background(), root, nil, WikiCodeResolveRequest{Direction: model.WikiCodeDirectionWikiToCode, DocSelectors: []string{".docs/wiki/04_RF/RF-ORDER.md"}}, model.WikiCodeOverlay{Additions: bindings})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			want = got.DeterminismDigest
		} else if got.DeterminismDigest != want {
			t.Fatalf("run %d digest=%q want %q", i, got.DeterminismDigest, want)
		}
		if len(got.DirectCode) != 2 || got.DirectCode[0].Relation != model.RelationImplements || got.DirectCode[1].Relation != model.RelationOperates {
			t.Fatalf("run %d semantic order = %#v", i, got.DirectCode)
		}
	}
}

func bridgeTestDB(t *testing.T, root string) *sql.DB {
	t.Helper()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func bridgeBinding(docPath, blockID, docID, relation, targetPath, targetSymbol, targetKind string, ordinal ...int) model.DocArtifactBinding {
	order := 1
	if len(ordinal) > 0 && ordinal[0] > 0 {
		order = ordinal[0]
	}
	return model.DocArtifactBinding{DocPath: docPath, BlockID: blockID, DocID: docID, Relation: relation, TargetPath: targetPath, TargetSymbol: targetSymbol, TargetKind: targetKind, AuthoringOrigin: model.AuthoringOriginCanonical, BindingStatus: model.BindingStatusExact, DocLifecycle: model.DocLifecycleActive, Ordinal: order, StartLine: order, EndLine: order + 1, BindingRef: model.WikiCodeBindingRef(docPath, blockID, docID, relation, targetPath, targetSymbol, targetKind)}
}

func bridgeTombstone(t *testing.T, bindingRef string) model.BindingTombstone {
	t.Helper()
	var tombstone model.BindingTombstone
	if err := json.Unmarshal([]byte(`{"binding_ref":"`+bindingRef+`"}`), &tombstone); err != nil {
		t.Fatal(err)
	}
	return tombstone
}

func bridgeSHA1(content []byte) string {
	digest := sha1.Sum(content)
	return hex.EncodeToString(digest[:])
}

func writeBridgeTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasBridgeOmission(context model.WikiCodeContext, code string) bool {
	for _, omission := range context.Omissions {
		if omission.Code == code || omission.Status == code {
			return true
		}
	}
	return false
}


package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestBuildWikiCodeContextDocsFirstWhenGraphAbsent(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	docs := []model.DocRecord{
		{Path: ".docs/wiki/00_gobierno_documental.md", DocID: "00_gOBIERNO", Layer: "00", ContentHash: "gov"},
		{Path: ".docs/wiki/04_RF/RF-GPH-007.md", DocID: "RF-GPH-007", Layer: "04", ContentHash: "primary"},
	}
	if err := store.ReplaceDocs(context.Background(), db, docs, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := BuildWikiCodeContext(context.Background(), db, docs[1], 2000)
	if err != nil {
		t.Fatal(err)
	}
	if got.PrimaryDoc.Path != docs[1].Path || len(got.AuthorityChain) != 2 {
		t.Fatalf("authority chain = %#v", got.AuthorityChain)
	}
	if len(got.Omissions) != 1 || got.Omissions[0].Code != "GPH_WIKI_GRAPH_UNAVAILABLE" {
		t.Fatalf("omissions = %#v", got.Omissions)
	}
	if !got.Provenance.QueryOnly {
		t.Fatal("context must be query-only")
	}
}

func TestBuildWikiCodeContextUsesNormalIndexGraphEvidence(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "wiki-code-context", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/wiki-code-context", Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeWikiCodeContextFixture(t, root, "go.mod", "module example.com/wiki-code-context\n\ngo 1.23\n")
	writeWikiCodeContextFixture(t, root, "internal/demo/demo.go", "package demo\n\nfunc Value() int { return 1 }\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/00_gobierno_documental.md", "# Gobierno\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/04_RF/RF-GPH-007.md", "# RF-GPH-007\n\nImplementado en `internal/demo/demo.go`.\n")
	result, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "index-v1")
	if err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	if result.GraphGenerationID == "" {
		t.Fatalf("IndexWorkspace did not publish a graph: %#v", result)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	docs, err := store.ListDocRecords(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	var primary model.DocRecord
	for _, doc := range docs {
		if doc.Path == ".docs/wiki/04_RF/RF-GPH-007.md" {
			primary = doc
		}
	}
	if primary.Path == "" {
		t.Fatalf("primary document not indexed: %#v", docs)
	}
	var digest string
	for i := 0; i < 30; i++ {
		got, err := BuildWikiCodeContext(context.Background(), db, primary, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		if got.PrimaryDoc.Path != primary.Path {
			t.Fatalf("primary changed: got %q want %q", got.PrimaryDoc.Path, primary.Path)
		}
		if len(got.AuthorityChain) < 2 || got.AuthorityChain[0].Role != "governance" || got.AuthorityChain[0].Path != ".docs/wiki/00_gobierno_documental.md" || got.AuthorityChain[1].Role != "primary" || got.AuthorityChain[1].Path != primary.Path {
			t.Fatalf("authority inversion: %#v", got.AuthorityChain)
		}
		if got.DocGenerationID == "" || got.CodeGenerationID != result.GraphGenerationID || !got.Provenance.QueryOnly {
			t.Fatalf("generation/provenance = %#v", got)
		}
		if len(got.CodeEvidence) != 1 || got.CodeEvidence[0].Path != "internal/demo/demo.go" {
			t.Fatalf("code evidence = %#v", got.CodeEvidence)
		}
		if len(got.GraphPaths) != 1 || got.GraphPaths[0].From != primary.Path || got.GraphPaths[0].To != "internal/demo/demo.go" || got.GraphPaths[0].Relation != "doc_mentions" || len(got.GraphPaths[0].EvidenceRefs) != 1 {
			t.Fatalf("graph paths = %#v", got.GraphPaths)
		}
		if len(got.Drift) != 0 || len(got.Omissions) != 0 {
			t.Fatalf("unexpected drift/omissions: drift=%#v omissions=%#v", got.Drift, got.Omissions)
		}
		if i == 0 {
			digest = got.DeterminismDigest
		} else if got.DeterminismDigest != digest {
			t.Fatalf("iteration %d digest=%q want %q", i, got.DeterminismDigest, digest)
		}
	}
}

func TestBuildWikiCodeContextDoesNotDeadlockWithPinnedSingleConnection(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "single-connection-wiki-code", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/single-connection-wiki-code", Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeWikiCodeContextFixture(t, root, "go.mod", "module example.com/single-connection-wiki-code\n\ngo 1.23\n")
	writeWikiCodeContextFixture(t, root, "internal/demo/demo.go", "package demo\n\nfunc Value() int { return 1 }\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/00_gobierno_documental.md", "# Gobierno\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/04_RF/RF-GPH-007.md", "# RF-GPH-007\n\nImplementado en `internal/demo/demo.go`.\n")
	if _, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "index-v1"); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	docs, err := store.ListDocRecords(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	var primary model.DocRecord
	for _, doc := range docs {
		if doc.Path == ".docs/wiki/04_RF/RF-GPH-007.md" {
			primary = doc
			break
		}
	}
	if primary.Path == "" {
		t.Fatal("primary document not indexed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := BuildWikiCodeContext(ctx, db, primary, 10_000)
	if err != nil {
		t.Fatalf("BuildWikiCodeContext with one connection: %v", err)
	}
	if len(got.CodeEvidence) != 1 || got.CodeEvidence[0].Path != "internal/demo/demo.go" || len(got.GraphPaths) != 1 {
		t.Fatalf("graph evidence = %#v paths=%#v", got.CodeEvidence, got.GraphPaths)
	}
}

func TestBuildWikiCodeContextRetainsDocsFirstWhenGraphIsStale(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "stale-wiki-code", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo"}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/stale-wiki-code", Languages: []string{"go"}}}}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeWikiCodeContextFixture(t, root, "go.mod", "module example.com/stale-wiki-code\n\ngo 1.23\n")
	writeWikiCodeContextFixture(t, root, "main.go", "package main\nfunc main() {}\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/00_gobierno_documental.md", "# Gobierno\n")
	writeWikiCodeContextFixture(t, root, ".docs/wiki/04_RF/RF-GPH-007.md", "# RF-GPH-007\n")
	if _, err := indexer.IndexWorkspaceWithGeneration(context.Background(), root, true, "index-v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.IndexWorkspaceDocsOnlyWithGeneration(context.Background(), root, "docs-v2"); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.SetGraphRuntimeState(context.Background(), db, store.GraphRuntimeStale, ""); err != nil {
		t.Fatal(err)
	}
	docs, err := store.ListDocRecords(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		if doc.Path == ".docs/wiki/04_RF/RF-GPH-007.md" {
			got, err := BuildWikiCodeContext(context.Background(), db, doc, 2_000)
			if err != nil {
				t.Fatal(err)
			}
			if got.PrimaryDoc.Path != doc.Path || len(got.AuthorityChain) != 2 || len(got.Omissions) != 1 || got.Omissions[0].Code != "GPH_WIKI_GRAPH_STALE" || !got.Provenance.QueryOnly {
				t.Fatalf("stale graph did not retain docs-first authority: %#v", got)
			}
			return
		}
	}
	t.Fatal("primary document not indexed")
}

func writeWikiCodeContextFixture(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildWikiCodeContextRejectsRawPrimary(t *testing.T) {
	_, err := BuildWikiCodeContext(context.Background(), nil, model.DocRecord{Path: ".docs/raw/task.md"}, 100)
	var contextErr *model.WikiCodeContextError
	if !errors.As(err, &contextErr) || contextErr.Code != "GPH_WIKI_BACKEND_UNAVAILABLE" {
		t.Fatalf("error = %v, want backend validation before database access", err)
	}
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = BuildWikiCodeContext(context.Background(), db, model.DocRecord{Path: ".docs/raw/task.md"}, 100)
	if !errors.As(err, &contextErr) || contextErr.Code != "GPH_WIKI_PRIMARY_INVALID" {
		t.Fatalf("error = %v, want primary invalid", err)
	}
}

func TestBuildWikiCodeContextBudgetPreservesAuthority(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gov := model.DocRecord{Path: ".docs/wiki/00_gobierno_documental.md", Layer: "00"}
	primary := model.DocRecord{Path: ".docs/wiki/04_RF/RF-GPH-007.md", Layer: "04"}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{gov, primary}, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := BuildWikiCodeContext(context.Background(), db, primary, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.AuthorityChain) != 2 || got.PrimaryDoc.Path != primary.Path {
		t.Fatalf("budget dropped authority: truncated=%v chain=%#v primary=%#v", got.Truncated, got.AuthorityChain, got.PrimaryDoc)
	}
}

func TestBuildWikiCodeContextDeterministicDocsOnly(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	primary := model.DocRecord{Path: ".docs/wiki/04_RF/RF-GPH-007.md", DocID: "RF-GPH-007", Layer: "04"}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{primary}, nil, nil); err != nil {
		t.Fatal(err)
	}
	var digest string
	for i := 0; i < 30; i++ {
		got, err := BuildWikiCodeContext(context.Background(), db, primary, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			digest = got.DeterminismDigest
		} else if got.DeterminismDigest != digest {
			t.Fatalf("iteration %d digest=%q want %q", i, got.DeterminismDigest, digest)
		}
	}
}

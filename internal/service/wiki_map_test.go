package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestClassifyWikiMapHubDefaultsRemainLocked(t *testing.T) {
	profile := model.DocsReadProfile{}
	cases := map[string]string{
		"wiki/00-identidad-karen.md":          "persona",
		"wiki/10-chiamo.md":                   "proyectos",
		"wiki/30-dashboard.md":                "sistema",
		"wiki/20-proyectos-activos.md":        "sistema",
		"bibliotecas/memorias/ficha.md":       "materia",
		"wiki/31-workers/WORKER_CAFE.md":      "",
		"wiki/32-contratos/contrato.md":       "",
		"wiki/24-aprendizaje/learning-log.md": "",
		".docs/wiki/00_gobierno.md":           "",
	}
	for path, want := range cases {
		if got := classifyWikiMapHub(path, profile); got != want {
			t.Fatalf("classifyWikiMapHub(%q)=%q want %q", path, got, want)
		}
	}
}

func TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder(t *testing.T) {
	defaultProfile := model.DocsReadProfile{}
	defaultHubs := groupWikiMapDocs([]wikiMapDoc{
		{Path: "wiki/10-project.md"},
		{Path: "wiki/00-person.md"},
		{Path: "bibliotecas/topic.md"},
		{Path: "wiki/20-system.md"},
	}, &defaultProfile)
	if len(defaultHubs) != 4 || defaultHubs[0].ID != "persona" || defaultHubs[1].ID != "proyectos" || defaultHubs[2].ID != "sistema" || defaultHubs[3].ID != "materia" {
		t.Fatalf("default hubs=%#v", defaultHubs)
	}

	customProfile := model.DocsReadProfile{WikiMap: &model.WikiMapConfig{Hubs: []model.WikiMapHubConfig{
		{ID: "schemas", Title: "Schemas", Patterns: []string{"docs/schemas/**"}},
		{ID: "notes", Title: "Notes", Patterns: []string{"wiki/0*.md"}},
	}}}
	customHubs := groupWikiMapDocs([]wikiMapDoc{
		{Path: "wiki/00-person.md"},
		{Path: "docs/schemas/nested/a.md"},
		{Path: "wiki/10-unmatched.md"},
	}, &customProfile)
	if len(customHubs) != 2 || customHubs[0].ID != "schemas" || customHubs[1].ID != "notes" {
		t.Fatalf("custom hubs=%#v", customHubs)
	}
	if customHubs[0].TotalDocs != 1 || customHubs[1].TotalDocs != 1 {
		t.Fatalf("custom totals=%#v", customHubs)
	}
}

func TestWikiMapDisabledProducesNoClassification(t *testing.T) {
	disabled := false
	profile := model.DocsReadProfile{WikiMap: &model.WikiMapConfig{Enabled: &disabled}}
	if got := classifyWikiMapHub("wiki/00-person.md", profile); got != "" {
		t.Fatalf("disabled classification=%q", got)
	}
	if hubs := groupWikiMapDocs([]wikiMapDoc{{Path: "wiki/00-person.md"}}, &profile); len(hubs) != 0 {
		t.Fatalf("disabled hubs=%#v", hubs)
	}
}

func TestWikiMapConfigRejectsUnsafeValuesWithoutEchoingThem(t *testing.T) {
	roots, rootWarnings := model.SafeRoots([]string{"../secret", "C:/private", "safe"})
	if len(roots) != 1 || roots[0] != "safe/" || len(rootWarnings) != 2 {
		t.Fatalf("roots=%v warnings=%v", roots, rootWarnings)
	}
	patterns, patternWarnings := model.SafePatterns([]string{"../secret/**", "docs/[bad", "docs/**/*.md"})
	if len(patterns) != 1 || patterns[0] != "docs/**/*.md" || len(patternWarnings) != 2 {
		t.Fatalf("patterns=%v warnings=%v", patterns, patternWarnings)
	}
	for _, warning := range append(rootWarnings, patternWarnings...) {
		if strings.Contains(warning, "secret") || strings.Contains(warning, "private") {
			t.Fatalf("warning leaked input: %q", warning)
		}
	}
	profile := model.DocsReadProfile{}
	if got := profile.EffectiveWikiMapRoots(); len(got) != 2 || got[0] != "wiki/" || got[1] != "bibliotecas/" {
		t.Fatalf("default roots=%v", got)
	}
	profile.WikiMap = &model.WikiMapConfig{Roots: []string{"docs/notes"}}
	if got := profile.EffectiveWikiMapRoots(); len(got) != 3 || got[0] != "wiki/" || got[1] != "bibliotecas/" || got[2] != "docs/notes/" {
		t.Fatalf("additive roots=%v", got)
	}
}

func TestTrimWikiMapHubsIsFairAndHonest(t *testing.T) {
	hubs := []wikiMapHub{
		{ID: "a", Title: "A", Docs: makeWikiMapDocs("a", 5)},
		{ID: "b", Title: "B", Docs: makeWikiMapDocs("b", 1)},
		{ID: "c", Title: "C", Docs: makeWikiMapDocs("c", 3)},
	}
	trimmed, stats := trimWikiMapHubs(hubs, 5, 0)
	if len(trimmed) != 3 || wikiMapDocCount(trimmed) != 5 {
		t.Fatalf("trimmed=%#v", trimmed)
	}
	if len(trimmed[0].Docs) != 2 || len(trimmed[1].Docs) != 1 || len(trimmed[2].Docs) != 2 {
		t.Fatalf("unfair allocation=%#v", trimmed)
	}
	if stats.totalDocs != 9 || stats.totalReturned != 5 || stats.reason != "max_items" || !strings.Contains(stats.hint, "--max-items") {
		t.Fatalf("stats=%#v", stats)
	}
	for i, total := range []int{5, 1, 3} {
		if trimmed[i].TotalDocs != total {
			t.Fatalf("hub %d total_docs=%d want %d", i, trimmed[i].TotalDocs, total)
		}
	}

	all, allStats := trimWikiMapHubs(hubs, 0, 0)
	if wikiMapDocCount(all) != 9 || allStats.totalReturned != allStats.totalDocs || allStats.reason != "" {
		t.Fatalf("false truncation: hubs=%#v stats=%#v", all, allStats)
	}
}

func TestTrimWikiMapHubsUsesDeterministicTokenBinarySearch(t *testing.T) {
	hubs := []wikiMapHub{
		{ID: "a", Title: "A", Docs: makeWikiMapDocs("a", 10)},
		{ID: "b", Title: "B", Docs: makeWikiMapDocs("b", 10)},
	}
	budget := estimateWikiMapTokens(fairTrimByCount(hubs, 5))
	first, firstStats := trimWikiMapHubs(hubs, 0, budget)
	second, secondStats := trimWikiMapHubs(hubs, 0, budget)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) || *firstStats != *secondStats {
		t.Fatalf("nondeterministic result: %s / %s", firstJSON, secondJSON)
	}
	if estimateWikiMapTokens(first) > budget || firstStats.reason != "token_budget" || !strings.Contains(firstStats.hint, "--token-budget") {
		t.Fatalf("budget=%d tokens=%d stats=%#v", budget, estimateWikiMapTokens(first), firstStats)
	}
}

func TestWalkWikiMapDocsRespectsIgnoreSymlinkAndBoundedTitles(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"wiki/ignored", "bibliotecas"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("wiki/ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wiki", "00-person.md"), []byte("---\ntitle: Persona\n---\n# Ignored heading\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wiki", "ignored", "01-secret.md"), []byte("# Secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bibliotecas", "topic.md"), []byte("# Topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkCreated := os.Symlink(filepath.Join(root, "wiki", "00-person.md"), filepath.Join(root, "wiki", "01-link.md")) == nil

	docs, warnings, err := walkWikiMapDocs(context.Background(), root, model.DocsReadProfile{})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]wikiMapDoc{}
	for _, doc := range docs {
		paths[doc.Path] = doc
	}
	if paths["wiki/00-person.md"].Title != "Persona" || paths["bibliotecas/topic.md"].Title != "Topic" {
		t.Fatalf("titles=%#v", paths)
	}
	if _, ok := paths["wiki/ignored/01-secret.md"]; ok {
		t.Fatalf("ignored path indexed: %#v", paths)
	}
	if _, ok := paths["wiki/01-link.md"]; ok {
		t.Fatalf("symlink indexed: %#v", paths)
	}
	if symlinkCreated && !hasWikiMapWarning(warnings, "wiki_map_symlink_skipped") && !hasWikiMapWarning(warnings, "wiki_map_reparse_skipped") {
		t.Fatalf("symlink/reparse warning missing: %v", warnings)
	}
}

func TestWalkWikiMapDocsPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := walkWikiMapDocs(ctx, t.TempDir(), model.DocsReadProfile{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}

func makeWikiMapDocs(prefix string, count int) []wikiMapDoc {
	docs := make([]wikiMapDoc, count)
	for i := range docs {
		docs[i] = wikiMapDoc{Path: filepath.ToSlash(filepath.Join(prefix, string(rune('a'+i))+".md")), Title: strings.ToUpper(prefix)}
	}
	return docs
}

func hasWikiMapWarning(warnings []string, target string) bool {
	for _, warning := range warnings {
		if warning == target {
			return true
		}
	}
	return false
}

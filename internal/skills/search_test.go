package skills

import (
	"context"
	"path/filepath"
	"testing"
)

func TestLexicalSearchDatabaseQueryRanksDBCLI(t *testing.T) {
	root := filepath.Join("testdata", "skills-root")
	cat, err := BuildCatalog(context.Background(), IndexOptions{SkillsRoot: root})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	// Merge seed-classified skills so ranking is exercised on seed+fixture catalog.
	seed, err := LoadEmbeddedSeed()
	if err != nil {
		t.Fatalf("LoadEmbeddedSeed: %v", err)
	}
	present := map[string]struct{}{}
	for _, s := range cat.Skills {
		present[s.ID] = struct{}{}
	}
	for _, row := range seed {
		if _, ok := present[row.ID]; ok {
			continue
		}
		// Keep the catalog modest: only tool_infra + data-like rows that could compete.
		if row.Family == FamilyToolInfra || row.Family == FamilyData || row.ID == "db-cli" {
			cat.Skills = append(cat.Skills, Classify(row.ID, row, nil))
			present[row.ID] = struct{}{}
		}
	}
	if _, ok := FindByID(cat, "db-cli"); !ok {
		cat.Skills = append(cat.Skills, Classify("db-cli", SeedMap(seed)["db-cli"], nil))
	}

	hits, warnings := Search(context.Background(), cat, SearchOptions{
		Query: "database query",
		TopK:  8,
	})
	_ = warnings
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	// db-cli should be near top
	foundAt := -1
	for i, h := range hits {
		if h.Skill.ID == "db-cli" {
			foundAt = i
			break
		}
	}
	if foundAt < 0 {
		ids := make([]string, len(hits))
		for i, h := range hits {
			ids[i] = h.Skill.ID
		}
		t.Fatalf("db-cli not in hits: %v", ids)
	}
	if foundAt > 2 {
		t.Fatalf("db-cli rank = %d, want top-3; top=%s score=%.2f", foundAt, hits[0].Skill.ID, hits[0].Score)
	}
	// Alias surface for consumers of search hits.
	if !hasAlias(hits[foundAt].Skill, "mi-db-cli") {
		t.Fatalf("db-cli hit aliases = %v, want mi-db-cli", hits[foundAt].Skill.Aliases)
	}
}

func TestSearchLeafExcludesParentRouters(t *testing.T) {
	root := filepath.Join("testdata", "skills-root")
	cat, err := BuildCatalog(context.Background(), IndexOptions{SkillsRoot: root})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	hits, _ := Search(context.Background(), cat, SearchOptions{
		Query:    "ae work orchestration wiki",
		Audience: AudienceLeaf,
		TopK:     20,
	})
	for _, h := range hits {
		if h.Skill.ID == "ae-work" || h.Skill.Tier == TierParentRouter {
			t.Fatalf("leaf search returned parent router %s", h.Skill.ID)
		}
	}
}

func TestIndexFromFixture(t *testing.T) {
	root := filepath.Join("testdata", "skills-root")
	cat, err := BuildCatalog(context.Background(), IndexOptions{SkillsRoot: root})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	if len(cat.Skills) < 5 {
		t.Fatalf("indexed skills = %d, want >= 5", len(cat.Skills))
	}
	for _, id := range []string{"db-cli", "mi-lsp", "mi-key-cli"} {
		s, ok := FindByID(cat, id)
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if s.SourcePath == "" {
			t.Fatalf("%s missing source_path", id)
		}
		if s.ContentSHA256 == "" {
			t.Fatalf("%s missing content_sha256", id)
		}
		if s.IndexedText == "" {
			t.Fatalf("%s missing indexed_text", id)
		}
	}
}

func TestCatalogSaveLoadRoundTrip(t *testing.T) {
	root := filepath.Join("testdata", "skills-root")
	cat, err := BuildCatalog(context.Background(), IndexOptions{SkillsRoot: root})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	if err := SaveCatalog(path, cat); err != nil {
		t.Fatalf("SaveCatalog: %v", err)
	}
	loaded, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if loaded.Schema != SchemaCatalog {
		t.Fatalf("schema = %q", loaded.Schema)
	}
	if len(loaded.Skills) != len(cat.Skills) {
		t.Fatalf("skills len %d != %d", len(loaded.Skills), len(cat.Skills))
	}
}

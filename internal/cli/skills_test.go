package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/skills"
)

func fixtureSkillsRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/cli -> internal/skills/testdata/skills-root
	root := filepath.Join(filepath.Dir(file), "..", "skills", "testdata", "skills-root")
	return root
}

func testSkillsCatalog(t *testing.T) *skills.Catalog {
	t.Helper()
	cat, err := skills.BuildCatalog(context.Background(), skills.IndexOptions{
		SkillsRoot:     fixtureSkillsRoot(t),
		WithEmbeddings: false,
	})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	// Ensure key fixture skills are present for pure Plan/Search tests.
	need := []string{"db-cli", "mi-lsp", "mi-key-cli", "ae-work", "ps-asistente-wiki", "ceo-maestro"}
	for _, id := range need {
		if _, ok := skills.FindByID(cat, id); ok {
			continue
		}
		seed, err := skills.LoadEmbeddedSeed()
		if err != nil {
			t.Fatalf("LoadEmbeddedSeed: %v", err)
		}
		row, ok := skills.SeedMap(seed)[id]
		if !ok {
			t.Fatalf("fixture/seed missing skill %s", id)
		}
		cat.Skills = append(cat.Skills, skills.Classify(id, row, nil))
	}
	return cat
}

func TestSkillsCommandRegistered(t *testing.T) {
	root := NewRootCommand()
	skillsCmd, _, err := root.Find([]string{"skills"})
	if err != nil {
		t.Fatalf("find skills: %v", err)
	}
	if skillsCmd == nil || skillsCmd.Name() != "skills" {
		t.Fatal("skills command not registered")
	}
	want := map[string]bool{"index": false, "list": false, "search": false, "plan": false}
	for _, c := range skillsCmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("skills subcommand %q not registered", name)
		}
	}
}

func TestSkillsIndexAndSaveFixture(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	cat, path, err := skills.IndexAndSave(context.Background(), skills.IndexOptions{
		SkillsRoot:  fixtureSkillsRoot(t),
		CatalogPath: catalogPath,
	})
	if err != nil {
		t.Fatalf("IndexAndSave: %v", err)
	}
	if path != catalogPath {
		t.Fatalf("catalog path = %q, want %q", path, catalogPath)
	}
	if len(cat.Skills) < 5 {
		t.Fatalf("skill_count = %d, want >= 5", len(cat.Skills))
	}
	loaded, err := skills.LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if loaded.Schema != skills.SchemaCatalog {
		t.Fatalf("schema = %q, want %q", loaded.Schema, skills.SchemaCatalog)
	}
	if _, ok := skills.FindByID(loaded, "db-cli"); !ok {
		t.Fatal("loaded catalog missing db-cli")
	}
}

func TestSkillsSearchPlanPureFunctions(t *testing.T) {
	cat := testSkillsCatalog(t)

	hits, warnings := skills.Search(context.Background(), cat, skills.SearchOptions{
		Query: "database query",
		TopK:  8,
	})
	_ = warnings
	if len(hits) == 0 {
		t.Fatal("search returned no hits")
	}
	foundDB := false
	for i, h := range hits {
		if h.Skill.ID == "db-cli" {
			foundDB = true
			if i > 2 {
				t.Fatalf("db-cli rank = %d, want top-3", i)
			}
			break
		}
	}
	if !foundDB {
		t.Fatal("search did not rank db-cli")
	}

	leaf, err := skills.BuildPlan(context.Background(), cat, skills.PlanOptions{
		Role: skills.RoleLeaf,
		Task: "run database query and rotate vault secrets",
	})
	if err != nil {
		t.Fatalf("BuildPlan leaf: %v", err)
	}
	if leaf.Schema != skills.SchemaPlan {
		t.Fatalf("plan schema = %q", leaf.Schema)
	}
	if len(leaf.Routers) != 0 {
		t.Fatalf("leaf routers = %v, want empty", leaf.Routers)
	}
	hasDB, hasKey, hasMi := false, false, false
	for _, e := range leaf.Always {
		if e.ID == "mi-lsp" {
			hasMi = true
		}
	}
	for _, e := range leaf.Selected {
		if e.ID == "db-cli" {
			hasDB = true
			aliasOK := false
			for _, a := range e.Aliases {
				if a == "mi-db-cli" {
					aliasOK = true
				}
			}
			if !aliasOK {
				t.Fatalf("db-cli aliases = %v, want mi-db-cli", e.Aliases)
			}
		}
		if e.ID == "mi-key-cli" {
			hasKey = true
		}
		if e.ID == "ae-work" || e.Tier == skills.TierParentRouter {
			t.Fatalf("leaf selected parent router %s", e.ID)
		}
	}
	if !hasMi || !hasDB || !hasKey {
		t.Fatalf("leaf plan missing always/selected skills: mi-lsp=%v db-cli=%v mi-key-cli=%v", hasMi, hasDB, hasKey)
	}

	parent, err := skills.BuildPlan(context.Background(), cat, skills.PlanOptions{
		Role: skills.RoleParent,
		Task: "orchestrate ae workflow and wiki documentation harness",
	})
	if err != nil {
		t.Fatalf("BuildPlan parent: %v", err)
	}
	if len(parent.Routers) == 0 {
		t.Fatal("parent plan routers empty")
	}
}

func TestSkillsEnvelopeFormats(t *testing.T) {
	cat := testSkillsCatalog(t)
	plan, err := skills.BuildPlan(context.Background(), cat, skills.PlanOptions{
		Role: skills.RoleLeaf,
		Task: "database query on orders",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	planItem, err := skillPlanToItem(plan)
	if err != nil {
		t.Fatalf("skillPlanToItem: %v", err)
	}

	hits, _ := skills.Search(context.Background(), cat, skills.SearchOptions{
		Query: "database query",
		TopK:  3,
	})
	searchItems := make([]any, 0, len(hits))
	for _, h := range hits {
		searchItems = append(searchItems, map[string]any{
			"id":    h.Skill.ID,
			"score": h.Score,
			"why":   h.Why,
		})
	}
	listItems := make([]any, 0, len(cat.Skills))
	for _, s := range cat.Skills {
		listItems = append(listItems, map[string]any{
			"id":     s.ID,
			"family": s.Family,
			"tier":   s.Tier,
		})
	}

	cases := []struct {
		name string
		env  model.Envelope
	}{
		{
			name: "plan",
			env: model.Envelope{
				Ok:      true,
				Backend: "skills",
				Items:   []any{planItem},
			},
		},
		{
			name: "search",
			env: model.Envelope{
				Ok:      true,
				Backend: "skills",
				Items:   searchItems,
			},
		},
		{
			name: "list",
			env: model.Envelope{
				Ok:      true,
				Backend: "skills",
				Items:   listItems,
			},
		},
	}

	for _, tc := range cases {
		for _, format := range []string{"json", "toon", "yaml", "compact"} {
			t.Run(tc.name+"_"+format, func(t *testing.T) {
				body, err := output.Render(tc.env, format, false)
				if err != nil {
					t.Fatalf("Render(%s): %v", format, err)
				}
				if len(strings.TrimSpace(string(body))) == 0 {
					t.Fatal("empty render output")
				}
				if format == "json" {
					var decoded model.Envelope
					if err := json.Unmarshal(body, &decoded); err != nil {
						t.Fatalf("json.Unmarshal: %v\nbody=%s", err, body)
					}
					if !decoded.Ok || decoded.Backend != "skills" {
						t.Fatalf("decoded ok=%v backend=%q", decoded.Ok, decoded.Backend)
					}
				}
				if tc.name == "plan" && !strings.Contains(string(body), skills.SchemaPlan) && !strings.Contains(string(body), "mi-lsp-skill-plan") {
					// compact may shorten keys; require schema presence for structured formats
					if format == "json" || format == "yaml" || format == "toon" {
						t.Fatalf("plan render missing schema in %s output", format)
					}
				}
			})
		}
	}
}

func TestSkillsResolveCatalogEnvOverride(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "custom-catalog.json")
	t.Setenv(skills.EnvCatalogPath, want)
	got, err := skills.ResolveCatalogPath("")
	if err != nil {
		t.Fatalf("ResolveCatalogPath: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveCatalogPath = %q, want %q", got, want)
	}
}

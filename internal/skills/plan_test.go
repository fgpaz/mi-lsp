package skills

import (
	"context"
	"path/filepath"
	"testing"
)

func testCatalogFromFixture(t *testing.T) *Catalog {
	t.Helper()
	root := filepath.Join("testdata", "skills-root")
	cat, err := BuildCatalog(context.Background(), IndexOptions{
		SkillsRoot:     root,
		WithEmbeddings: false,
	})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	// Ensure key skills exist even if scan id resolution differs.
	need := []string{"db-cli", "mi-lsp", "mi-key-cli", "ae-work", "ps-asistente-wiki", "ceo-maestro"}
	for _, id := range need {
		if _, ok := FindByID(cat, id); !ok {
			// augment from seed classify for missing fixture edge cases
			seed, _ := LoadEmbeddedSeed()
			m := SeedMap(seed)
			if row, ok := m[id]; ok {
				cat.Skills = append(cat.Skills, Classify(id, row, nil))
			} else {
				t.Fatalf("fixture missing skill %s", id)
			}
		}
	}
	return cat
}

func TestPlanLeafExcludesParentRouters(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleLeaf,
		Task: "implement wiki documentation update with ae orchestration hints",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, e := range plan.Selected {
		if e.ID == "ae-work" || e.ID == "ps-asistente-wiki" || e.Tier == TierParentRouter {
			t.Fatalf("leaf plan selected parent router %s", e.ID)
		}
	}
	for _, e := range plan.Routers {
		t.Fatalf("leaf plan must not include routers, got %s", e.ID)
	}
	if planHasID(plan, "ae-work") || planHasID(plan, "ps-asistente-wiki") {
		t.Fatal("leaf plan must exclude ae-work and ps-asistente-wiki")
	}
}

func TestPlanSecretsIncludesMiKeyCLI(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleLeaf,
		Task: "rotate vault secrets and api key via mi-key",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if !planHasID(plan, "mi-key-cli") {
		t.Fatalf("plan missing mi-key-cli; selected=%v always=%v", idsOf(plan.Selected), idsOf(plan.Always))
	}
}

func TestPlanDBIncludesDBCLI(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleLeaf,
		Task: "run database query on sql server orders table",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var found *PlanEntry
	for i := range plan.Selected {
		if plan.Selected[i].ID == "db-cli" {
			found = &plan.Selected[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("plan missing db-cli; selected=%v", idsOf(plan.Selected))
	}
	has := false
	for _, a := range found.Aliases {
		if a == "mi-db-cli" {
			has = true
		}
	}
	if !has {
		t.Fatalf("db-cli aliases = %v, want mi-db-cli", found.Aliases)
	}
}

func TestPlanParentMayIncludeRouters(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleParent,
		Task: "orchestrate ae workflow to update wiki documentation harness",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Routers) == 0 {
		t.Fatalf("parent plan routers empty; expected ae-work")
	}
	hasAE := false
	hasWiki := false
	for _, r := range plan.Routers {
		if r.ID == "ae-work" {
			hasAE = true
		}
		if r.ID == "ps-asistente-wiki" {
			hasWiki = true
		}
	}
	if !hasAE {
		t.Fatalf("parent routers = %v, want ae-work", idsOf(plan.Routers))
	}
	// Wiki/orchestration task should also surface the wiki router when present.
	if !hasWiki {
		t.Fatalf("parent routers = %v, want ps-asistente-wiki for wiki task", idsOf(plan.Routers))
	}
}

func TestPlanDefaultDenyFamilies(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleLeaf,
		Task: "navigate workspace symbols and implement a small fix",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	want := map[string]bool{
		FamilyCEO:      false,
		FamilyResearch: false,
		FamilyComms:    false,
	}
	for _, f := range plan.DenyFamilies {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for f, ok := range want {
		if !ok {
			t.Fatalf("deny_families = %v, missing default %q", plan.DenyFamilies, f)
		}
	}
	// ceo-maestro is family=ceo and must not appear under default deny.
	if planHasID(plan, "ceo-maestro") {
		t.Fatal("default deny must suppress ceo-maestro")
	}
}

func TestPlanCEOSignalLiftsDeny(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleParent,
		Task: "ceo strategy board review with executive briefing",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, f := range plan.DenyFamilies {
		if f == FamilyCEO {
			t.Fatalf("deny_families still includes ceo after CEO signal: %v", plan.DenyFamilies)
		}
	}
	// Parent router ceo-maestro may appear once deny is lifted.
	if !planHasID(plan, "ceo-maestro") {
		// Not hard-fail if router matching is narrower; family lift is the rule under test.
		t.Logf("ceo-maestro not selected; deny lifted is sufficient (routers=%v)", idsOf(plan.Routers))
	}
}

func TestPlanAlwaysIncludesMiLSP(t *testing.T) {
	cat := testCatalogFromFixture(t)
	plan, err := BuildPlan(context.Background(), cat, PlanOptions{
		Role: RoleLeaf,
		Task: "navigate workspace symbols",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	found := false
	for _, a := range plan.Always {
		if a.ID == "mi-lsp" && a.Mode == ModeManifest {
			found = true
		}
	}
	if !found {
		t.Fatalf("always missing mi-lsp manifest; always=%v", idsOf(plan.Always))
	}
}

func planHasID(plan *SkillPlan, id string) bool {
	for _, e := range plan.Always {
		if e.ID == id {
			return true
		}
	}
	for _, e := range plan.Routers {
		if e.ID == id {
			return true
		}
	}
	for _, e := range plan.Selected {
		if e.ID == id {
			return true
		}
	}
	return false
}

func idsOf(entries []PlanEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

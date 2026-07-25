package skills

import (
	"testing"
)

func TestClassifyDBCLIAliases(t *testing.T) {
	seed, err := LoadEmbeddedSeed()
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	m := SeedMap(seed)
	row, ok := m["db-cli"]
	if !ok {
		t.Fatal("db-cli missing from seed")
	}
	rec := Classify("db-cli", row, nil)
	if !hasAlias(rec, "mi-db-cli") {
		t.Fatalf("db-cli aliases = %v, want mi-db-cli", rec.Aliases)
	}
	if rec.Family != FamilyToolInfra && rec.Family != FamilyData {
		// override forces tool_infra family
		t.Fatalf("db-cli family = %q, want tool_infra (or data before override)", rec.Family)
	}
	if rec.Family != FamilyToolInfra {
		t.Fatalf("db-cli family after override = %q, want %q", rec.Family, FamilyToolInfra)
	}
}

func TestClassifyAgentBrowserFamily(t *testing.T) {
	seed, err := LoadEmbeddedSeed()
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	row := SeedMap(seed)["agent-browser"]
	rec := Classify("agent-browser", row, nil)
	if rec.Family != FamilyToolInfra {
		t.Fatalf("agent-browser family = %q, want %q", rec.Family, FamilyToolInfra)
	}
}

func TestClassifyAEWorkParentRouter(t *testing.T) {
	seed, err := LoadEmbeddedSeed()
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	row := SeedMap(seed)["ae-work"]
	rec := Classify("ae-work", row, nil)
	if rec.Tier != TierParentRouter {
		t.Fatalf("ae-work tier = %q, want %q", rec.Tier, TierParentRouter)
	}
	if rec.Audience != AudienceParent {
		t.Fatalf("ae-work audience = %q, want %q", rec.Audience, AudienceParent)
	}
	if !IsParentRouter("ae-work") {
		t.Fatal("IsParentRouter(ae-work) = false")
	}
}

func TestClassifyCriticalAndHighToken(t *testing.T) {
	seed, err := LoadEmbeddedSeed()
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	m := SeedMap(seed)
	for _, id := range []string{"mi-lsp", "mi-key-cli", "ae-pre-push"} {
		rec := Classify(id, m[id], nil)
		if !rec.Critical {
			t.Fatalf("%s critical = false, want true", id)
		}
	}
	rec := Classify("mi-lsp", m["mi-lsp"], nil)
	if rec.TokenCostClass != TokenCostHigh {
		t.Fatalf("mi-lsp token_cost_class = %q, want high", rec.TokenCostClass)
	}
	imp := Classify("impeccable-polish", SeedRow{ID: "impeccable-polish", Family: FamilyFrontendDesign}, nil)
	if imp.TokenCostClass != TokenCostHigh {
		t.Fatalf("impeccable-polish token_cost_class = %q, want high", imp.TokenCostClass)
	}
}

func TestClassifyParentRouterPatterns(t *testing.T) {
	for _, id := range []string{
		"ae-adapter-codex",
		"ae-crear-politicas",
		"ae-crear-politicas-microservicios",
		"ps-asistente-wiki",
		"ceo-maestro",
	} {
		if !IsParentRouter(id) {
			t.Fatalf("IsParentRouter(%s) = false", id)
		}
		rec := Classify(id, SeedRow{ID: id, Family: "x", SuggestedTier: "optional", SuggestedAudience: "both"}, nil)
		if rec.Tier != TierParentRouter || rec.Audience != AudienceParent {
			t.Fatalf("%s => tier=%s audience=%s", id, rec.Tier, rec.Audience)
		}
	}
}

func TestLoadEmbeddedSeedCount(t *testing.T) {
	rows, err := LoadEmbeddedSeed()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) < 300 {
		t.Fatalf("seed rows = %d, want >= 300", len(rows))
	}
}

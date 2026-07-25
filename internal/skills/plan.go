package skills

import (
	"context"
	"fmt"
	"strings"
)

// Default denied families unless the task explicitly signals them.
var defaultDenyFamilies = []string{
	FamilyCEO,
	FamilyResearch,
	FamilyComms,
}

// BuildPlan constructs a skill_plan from a catalog + role/task.
func BuildPlan(ctx context.Context, cat *Catalog, opts PlanOptions) (*SkillPlan, error) {
	if cat == nil {
		return nil, fmt.Errorf("catalog is required")
	}
	role := strings.ToLower(strings.TrimSpace(opts.Role))
	if role != RoleParent && role != RoleLeaf {
		return nil, fmt.Errorf("role must be %q or %q", RoleParent, RoleLeaf)
	}
	task := strings.TrimSpace(opts.Task)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}
	maxSkills := opts.MaxSkills
	if maxSkills <= 0 {
		maxSkills = 8
	}
	tokenBudget := opts.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}

	plan := &SkillPlan{
		Schema: SchemaPlan,
		Role:   role,
		Task:   task,
		Budget: PlanBudget{
			MaxSkills:   maxSkills,
			TokenBudget: tokenBudget,
		},
		Always:          []PlanEntry{},
		Routers:         []PlanEntry{},
		Selected:        []PlanEntry{},
		BundlesOptional: []string{},
		DenyFamilies:    append([]string{}, defaultDenyFamilies...),
		Warnings:        []string{},
	}

	// Embeddings availability warning.
	if !catalogHasEmbeddings(cat) {
		plan.Warnings = append(plan.Warnings, "embeddings_unavailable")
	}

	taskLower := strings.ToLower(task)
	deny := map[string]struct{}{}
	for _, f := range plan.DenyFamilies {
		deny[f] = struct{}{}
	}
	// Lift deny when task clearly signals the family.
	if signalsCEO(taskLower) {
		delete(deny, FamilyCEO)
		plan.DenyFamilies = removeString(plan.DenyFamilies, FamilyCEO)
	}
	if signalsResearch(taskLower) {
		delete(deny, FamilyResearch)
		plan.DenyFamilies = removeString(plan.DenyFamilies, FamilyResearch)
	}
	if signalsComms(taskLower) {
		delete(deny, FamilyComms)
		plan.DenyFamilies = removeString(plan.DenyFamilies, FamilyComms)
	}

	// Suppress most of "other" unless task signals.
	suppressOther := !signalsOther(taskLower)

	// Always include mi-lsp when present.
	if milsp, ok := FindByID(cat, "mi-lsp"); ok {
		plan.Always = append(plan.Always, PlanEntry{
			ID:      milsp.ID,
			Aliases: milsp.Aliases,
			Family:  milsp.Family,
			Tier:    milsp.Tier,
			Mode:    ModeManifest,
			Why:     "always_on_manifest",
		})
	}

	// Intent-forced selections.
	forced := map[string]string{} // id -> why
	if isDBIntent(taskLower) {
		forced["db-cli"] = "db_intent"
	}
	if isSecretsIntent(taskLower) {
		forced["mi-key-cli"] = "secrets_intent"
	}

	// Search candidates with audience filter matching role.
	audience := role
	hits, searchWarnings := Search(ctx, cat, SearchOptions{
		Query:    task,
		Audience: audience,
		TopK:     maxSkills * 3,
	})
	plan.Warnings = append(plan.Warnings, searchWarnings...)

	selectedIDs := map[string]struct{}{}
	// mark always as selected for capacity accounting
	for _, a := range plan.Always {
		selectedIDs[a.ID] = struct{}{}
	}

	addSelected := func(s SkillRecord, score float64, mode, why string) {
		if _, ok := selectedIDs[s.ID]; ok {
			return
		}
		if role == RoleLeaf && (s.Tier == TierParentRouter || IsParentRouter(s.ID)) {
			return
		}
		if _, denied := deny[s.Family]; denied {
			return
		}
		if suppressOther && s.Family == FamilyOther && forced[s.ID] == "" {
			return
		}
		// max_skills budgets ranked selected entries; forced intents always fit.
		if len(plan.Selected) >= maxSkills {
			if _, isForced := forced[s.ID]; !isForced {
				return
			}
		}
		plan.Selected = append(plan.Selected, PlanEntry{
			ID:      s.ID,
			Aliases: s.Aliases,
			Family:  s.Family,
			Tier:    s.Tier,
			Score:   score,
			Mode:    mode,
			Why:     why,
		})
		selectedIDs[s.ID] = struct{}{}
	}

	// Forced intents first.
	for id, why := range forced {
		if s, ok := FindByID(cat, id); ok {
			// Ensure alias surface for db-cli.
			if id == "db-cli" {
				s.Aliases = mergeUnique(s.Aliases, []string{"mi-db-cli"})
			}
			addSelected(s, 10, ModeSelected, why)
		}
	}

	// Parent routers for parent role.
	if role == RoleParent {
		for _, s := range cat.Skills {
			if s.Tier != TierParentRouter && !IsParentRouter(s.ID) {
				continue
			}
			if _, denied := deny[s.Family]; denied {
				continue
			}
			if !routerMatchesTask(s, taskLower) {
				continue
			}
			if _, ok := selectedIDs[s.ID]; ok {
				continue
			}
			plan.Routers = append(plan.Routers, PlanEntry{
				ID:      s.ID,
				Aliases: s.Aliases,
				Family:  s.Family,
				Tier:    s.Tier,
				Mode:    ModeRouter,
				Why:     "parent_router_match",
			})
			selectedIDs[s.ID] = struct{}{}
		}
	}

	// Ranked search hits (skip weak hybrid noise once forced intents filled useful slots).
	const minHybridScore = 3.0
	for _, h := range hits {
		s := h.Skill
		if role == RoleLeaf && (s.Tier == TierParentRouter || IsParentRouter(s.ID)) {
			continue
		}
		// Skip if already a router entry.
		if _, ok := selectedIDs[s.ID]; ok {
			continue
		}
		if h.Score < minHybridScore && forced[s.ID] == "" {
			continue
		}
		addSelected(s, h.Score, ModeSelected, "hybrid_rank")
	}

	plan.WhyNotCheaper = whyNotCheaper(plan, taskLower)
	plan.Warnings = uniqueStrings(plan.Warnings)
	return plan, nil
}

func isDBIntent(task string) bool {
	needles := []string{
		"sql", "database", "postgres", "sql server", "query orders",
		"db-cli", "mi-db-cli", "query database", "mssql", "mysql",
	}
	for _, n := range needles {
		if strings.Contains(task, n) {
			return true
		}
	}
	return false
}

func isSecretsIntent(task string) bool {
	needles := []string{
		"secret", "secrets", "credential", "vault", "api key", "mi-key", "mi-key-cli",
	}
	for _, n := range needles {
		if strings.Contains(task, n) {
			return true
		}
	}
	return false
}

func signalsCEO(task string) bool {
	return strings.Contains(task, "ceo") || strings.Contains(task, "strategy") ||
		strings.Contains(task, "board") || strings.Contains(task, "executive")
}

func signalsResearch(task string) bool {
	return strings.Contains(task, "research") || strings.Contains(task, "paper") ||
		strings.Contains(task, "literature")
}

func signalsComms(task string) bool {
	return strings.Contains(task, "comms") || strings.Contains(task, "communication") ||
		strings.Contains(task, "whatsapp") || strings.Contains(task, "telegram post")
}

func signalsOther(task string) bool {
	// Allow other only when task is vague generic or mentions miscellaneous tools.
	return strings.Contains(task, "misc") || strings.Contains(task, "other family")
}

func routerMatchesTask(s SkillRecord, task string) bool {
	// Match routers when task mentions their domain.
	switch {
	case s.ID == "ae-work" || strings.HasPrefix(s.ID, "ae-"):
		return strings.Contains(task, "ae") || strings.Contains(task, "orchestr") ||
			strings.Contains(task, "workflow") || strings.Contains(task, "worker") ||
			strings.Contains(task, "policy") || strings.Contains(task, "harness") ||
			strings.Contains(task, "pre-push") || strings.Contains(task, "implement") ||
			strings.Contains(task, "build") || strings.Contains(task, "mutate") ||
			// parent role broad: include ae-work often
			s.ID == "ae-work"
	case strings.HasPrefix(s.ID, "ps-"):
		return strings.Contains(task, "wiki") || strings.Contains(task, "doc") ||
			strings.Contains(task, "canon") || strings.Contains(task, "trazabilidad") ||
			strings.Contains(task, "context") || strings.Contains(task, "prompt") ||
			strings.Contains(task, "gobierno") || strings.Contains(task, "sdd")
	case strings.HasPrefix(s.ID, "qa-"):
		return strings.Contains(task, "qa") || strings.Contains(task, "test") ||
			strings.Contains(task, "quality") || strings.Contains(task, "e2e")
	case s.ID == "ceo-maestro":
		return signalsCEO(task)
	default:
		// id token overlap
		for _, tok := range tokenize(strings.ReplaceAll(s.ID, "-", " ")) {
			if strings.Contains(task, tok) {
				return true
			}
		}
		return false
	}
}

func whyNotCheaper(plan *SkillPlan, task string) string {
	parts := []string{}
	if len(plan.Always) > 0 {
		parts = append(parts, "mi-lsp always_on")
	}
	if isDBIntent(task) {
		parts = append(parts, "db_intent requires db-cli")
	}
	if isSecretsIntent(task) {
		parts = append(parts, "secrets_intent requires mi-key-cli")
	}
	if plan.Role == RoleParent && len(plan.Routers) > 0 {
		parts = append(parts, "parent role includes matching routers")
	}
	if len(plan.Selected) > 0 {
		parts = append(parts, fmt.Sprintf("%d ranked skills selected", len(plan.Selected)))
	}
	if len(parts) == 0 {
		return "minimal lexical plan"
	}
	return strings.Join(parts, "; ")
}

func removeString(in []string, target string) []string {
	out := in[:0]
	for _, v := range in {
		if v == target {
			continue
		}
		out = append(out, v)
	}
	// if out shares backing array and shrunk to empty, return new empty
	if len(out) == 0 {
		return []string{}
	}
	return append([]string(nil), out...)
}

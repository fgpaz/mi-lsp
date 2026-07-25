package skills

import (
	"strings"
)

// Parent routers forced to tier=parent_router, audience=parent.
// Includes exact ids and prefix patterns (trailing *).
var parentRouterIDs = []string{
	"ae-work",
	"ae-adapter-*",
	"ae-pre-push",
	"ae-close",
	"ae-decide",
	"ae-setup",
	"ae-learn",
	"ae-crear-politicas*",
	"ae-projection-audit",
	"ae-harness-manifest",
	"ae-validacion-evidencia",
	"ae-worker-pi",
	"ps-asistente-wiki",
	"ps-prompt",
	"ps-contexto",
	"ps-asistente-negocio",
	"ps-trazabilidad",
	"ps-auditar-trazabilidad",
	"ps-canon-cross-ref-check",
	"qa-asistente",
	"qa-asistente-conversacional",
	"qa-asistente-decisional-tedi",
	"ceo-maestro",
}

// toolInfraIDs force family=tool_infra (tier remains tool_infra when appropriate).
var toolInfraIDs = map[string]struct{}{
	"mi-lsp":                    {},
	"mi-key-cli":                {},
	"mi-pi-docs":                {},
	"mi-telegram-cli":           {},
	"mi-cuenta":                 {},
	"mi-cuenta-td-notas":        {},
	"mi-asistente":              {},
	"mi-asistente-aprendizaje":  {},
	"mi-job":                    {},
	"mi-llm-ollama-gpu":         {},
	"mi-lsp-aprendizaje":        {},
	"db-cli":                    {},
	"ssh-remote":                {},
	"agent-browser":             {},
	"playwright-cli":            {},
	"buho-ai-proxy-codex-oauth": {},
	"html-to-pdf":               {},
}

// Hard alias overrides (merged with seed aliases).
var aliasOverrides = map[string][]string{
	"db-cli":     {"mi-db-cli"},
	"ssh-remote": {"mi-ssh-remote"},
}

// Critical force-set (union with seed critical_candidate).
var criticalForce = map[string]struct{}{
	"mi-lsp":      {},
	"mi-key-cli":  {},
	"ae-pre-push": {},
}

// highTokenCostIDs and prefixes for token_cost_class=high.
var highTokenCostExact = map[string]struct{}{
	"db-cli":      {},
	"ssh-remote":  {},
	"html-to-pdf": {},
	"mi-lsp":      {},
}

// Classify builds a SkillRecord from seed metadata and optional scan content.
// Seed may be the zero value when the skill is unscanned in seed; id must be set.
func Classify(id string, seed SeedRow, scan *ScanResult) SkillRecord {
	id = strings.TrimSpace(id)
	if id == "" && seed.ID != "" {
		id = seed.ID
	}
	if id == "" && scan != nil && scan.ID != "" {
		id = scan.ID
	}

	rec := SkillRecord{
		ID:             id,
		Family:         seed.Family,
		Tier:           seed.SuggestedTier,
		Audience:       seed.SuggestedAudience,
		Aliases:        append([]string(nil), seed.SuggestedAliases...),
		Critical:       seed.CriticalCandidate,
		TokenCostClass: TokenCostMed,
	}

	// Defaults when seed missing.
	if rec.Family == "" {
		rec.Family = inferFamily(id)
	}
	if rec.Tier == "" {
		rec.Tier = TierOptional
	}
	if rec.Audience == "" {
		rec.Audience = AudienceBoth
	}

	// Parent router override.
	if isParentRouter(id) {
		rec.Tier = TierParentRouter
		rec.Audience = AudienceParent
	}

	// tool_infra family override (agent-browser etc.).
	if _, ok := toolInfraIDs[id]; ok {
		rec.Family = FamilyToolInfra
		// Keep seed tier when it already is tool_infra; otherwise leave tier unless empty.
		if rec.Tier == "" || rec.Tier == FamilyToolMisc {
			rec.Tier = TierToolInfra
		}
		// html-to-pdf stays optional tier per design; playwright may stay optional.
		if id == "html-to-pdf" {
			rec.Tier = TierOptional
		}
		if id == "playwright-cli" && rec.Tier != TierToolInfra {
			// leave seed tier if present; default optional is fine
		}
	}

	// Alias overrides (ensure required aliases present).
	if extra, ok := aliasOverrides[id]; ok {
		rec.Aliases = mergeUnique(rec.Aliases, extra)
	}

	// Critical force.
	if _, ok := criticalForce[id]; ok {
		rec.Critical = true
	}

	// Token cost.
	rec.TokenCostClass = classifyTokenCost(id, rec.Family, rec.Tier)

	// Merge scan content.
	if scan != nil {
		if scan.SourcePath != "" {
			rec.SourcePath = scan.SourcePath
		}
		if scan.Description != "" {
			rec.Description = scan.Description
		}
		if scan.WhenToUse != "" {
			rec.WhenToUse = scan.WhenToUse
		}
		if scan.WhenNotToUse != "" {
			w := scan.WhenNotToUse
			rec.WhenNotToUse = &w
		}
		if scan.IndexedText != "" {
			rec.IndexedText = scan.IndexedText
		}
		if scan.ContentSHA != "" {
			rec.ContentSHA256 = scan.ContentSHA
		}
		// Fall back description from frontmatter name if empty.
		if rec.Description == "" && scan.Name != "" {
			rec.Description = scan.Name
		}
	}

	// Infer when_to_use from description if still empty.
	if rec.WhenToUse == "" && rec.Description != "" {
		rec.WhenToUse = rec.Description
	}

	return rec
}

// IsParentRouter reports whether id is a known parent router (after overrides).
func IsParentRouter(id string) bool {
	return isParentRouter(id)
}

func isParentRouter(id string) bool {
	id = strings.TrimSpace(id)
	for _, pat := range parentRouterIDs {
		if matchPattern(pat, id) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, id string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(id, prefix)
	}
	return pattern == id
}

func classifyTokenCost(id, family, tier string) string {
	if _, ok := highTokenCostExact[id]; ok {
		return TokenCostHigh
	}
	if strings.HasPrefix(id, "impeccable") {
		return TokenCostHigh
	}
	// Heavy routers / large trees.
	if tier == TierParentRouter {
		return TokenCostMed
	}
	if family == FamilyToolInfra || tier == TierToolInfra {
		return TokenCostMed
	}
	// Short optional helpers.
	if tier == TierOptional && (family == FamilyOther || family == FamilyTesting) {
		return TokenCostLow
	}
	return TokenCostMed
}

func inferFamily(id string) string {
	switch {
	case strings.HasPrefix(id, "ae-"):
		return FamilyAE
	case strings.HasPrefix(id, "ps-"):
		return FamilyPS
	case strings.HasPrefix(id, "qa-"):
		return FamilyQA
	case strings.HasPrefix(id, "ceo-"):
		return FamilyCEO
	case strings.HasPrefix(id, "mi-"):
		return FamilyMI
	case strings.HasPrefix(id, "impeccable"):
		return FamilyFrontendDesign
	default:
		return FamilyOther
	}
}

func mergeUnique(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, v := range base {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range extra {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ClassifyAll applies Classify to every seed row (no scan content).
func ClassifyAll(seed []SeedRow) []SkillRecord {
	out := make([]SkillRecord, 0, len(seed))
	for _, row := range seed {
		out = append(out, Classify(row.ID, row, nil))
	}
	return out
}

// TaxonomyCounts returns counts by family and tier for classified records.
func TaxonomyCounts(recs []SkillRecord) (byFamily, byTier map[string]int) {
	byFamily = map[string]int{}
	byTier = map[string]int{}
	for _, r := range recs {
		byFamily[r.Family]++
		byTier[r.Tier]++
	}
	return byFamily, byTier
}

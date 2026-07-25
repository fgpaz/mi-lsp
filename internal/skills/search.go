package skills

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/fgpaz/mi-lsp/internal/embed"
)

// Search ranks catalog skills for a query. Lexical always works; embeddings are optional.
func Search(ctx context.Context, cat *Catalog, opts SearchOptions) ([]SearchHit, []string) {
	if cat == nil {
		return nil, []string{"catalog_missing"}
	}
	if opts.TopK <= 0 {
		opts.TopK = 8
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, []string{"empty_query"}
	}

	var warnings []string
	qEmbed := opts.QueryEmbedding
	hasCatalogEmbeds := catalogHasEmbeddings(cat)
	if hasCatalogEmbeds && len(qEmbed) == 0 {
		if vec, warn := tryQueryEmbed(ctx, query); len(vec) > 0 {
			qEmbed = vec
		} else if warn != "" {
			warnings = append(warnings, warn)
		}
	} else if !hasCatalogEmbeds {
		// pure lexical — fine
	}

	terms := tokenize(query)
	hits := make([]SearchHit, 0, len(cat.Skills))
	for _, s := range cat.Skills {
		if opts.Family != "" && !strings.EqualFold(s.Family, opts.Family) {
			continue
		}
		if opts.Audience != "" && !audienceMatches(s.Audience, opts.Audience) {
			// parent_router only for parent audience already handled by audienceMatches
			// additionally: when audience=leaf, drop parent_router hard.
			if strings.EqualFold(opts.Audience, AudienceLeaf) && (s.Tier == TierParentRouter || IsParentRouter(s.ID)) {
				continue
			}
			if !audienceMatches(s.Audience, opts.Audience) {
				continue
			}
		}
		if strings.EqualFold(opts.Audience, AudienceLeaf) && (s.Tier == TierParentRouter || IsParentRouter(s.ID)) {
			continue
		}

		lex := lexicalScore(s, terms, query)
		embScore := 0.0
		if len(qEmbed) > 0 && len(s.Embedding) > 0 {
			embScore = embed.Cosine(qEmbed, s.Embedding)
		}

		// Hybrid: lexical primary, embed secondary.
		score := lex + 0.35*embScore

		// Critical allowlist boost when relevant.
		if s.Critical && criticalRelevant(s, terms, query) {
			score += 1.5
		}

		// Parent routers boost only for parent audience/role.
		if (s.Tier == TierParentRouter || IsParentRouter(s.ID)) &&
			strings.EqualFold(opts.Audience, AudienceParent) {
			score += 0.75
		}

		if score <= 0 {
			continue
		}
		hits = append(hits, SearchHit{
			Skill: s,
			Score: score,
			Why:   scoreWhy(s, lex, embScore),
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Skill.ID < hits[j].Skill.ID
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > opts.TopK {
		hits = hits[:opts.TopK]
	}
	return hits, uniqueStrings(warnings)
}

func catalogHasEmbeddings(cat *Catalog) bool {
	for _, s := range cat.Skills {
		if len(s.Embedding) > 0 {
			return true
		}
	}
	return false
}

func tryQueryEmbed(ctx context.Context, query string) ([]float32, string) {
	client, warning := bestEffortEmbedClient()
	if client == nil {
		if warning == "" {
			warning = "embeddings_unavailable"
		}
		return nil, warning
	}
	vec, err := client.EmbedOne(ctx, query)
	if err != nil {
		return nil, "embeddings_unavailable: " + err.Error()
	}
	return vec, ""
}

func lexicalScore(s SkillRecord, terms []string, rawQuery string) float64 {
	if len(terms) == 0 {
		return 0
	}
	idLower := strings.ToLower(s.ID)
	qLower := strings.ToLower(rawQuery)

	var score float64

	// Exact id / alias hits.
	if idLower == qLower || strings.Contains(qLower, idLower) {
		score += 5
	}
	for _, a := range s.Aliases {
		al := strings.ToLower(a)
		if al == qLower || strings.Contains(qLower, al) {
			score += 4.5
		}
	}

	// Field bags.
	fields := []struct {
		text   string
		weight float64
	}{
		{s.ID, 3.0},
		{strings.Join(s.Aliases, " "), 2.5},
		{s.Description, 2.0},
		{s.WhenToUse, 2.0},
		{s.Family, 1.0},
		{s.IndexedText, 1.0},
	}

	for _, term := range terms {
		for _, f := range fields {
			if f.text == "" {
				continue
			}
			hay := strings.ToLower(f.text)
			if hay == term {
				score += f.weight * 2
			} else if strings.Contains(hay, term) {
				score += f.weight
			}
		}
	}

	// Light id token overlap (db-cli ↔ database via aliases mostly).
	idTokens := tokenize(strings.ReplaceAll(s.ID, "-", " "))
	for _, t := range terms {
		for _, it := range idTokens {
			if t == it {
				score += 1.2
			}
		}
	}

	// Domain heuristics for common intents.
	score += domainBoost(s, terms, qLower)

	return score
}

func domainBoost(s SkillRecord, terms []string, qLower string) float64 {
	var boost float64
	// DB intent
	if hasAny(terms, "sql", "database", "postgres", "query", "db") ||
		strings.Contains(qLower, "sql server") ||
		strings.Contains(qLower, "query orders") ||
		strings.Contains(qLower, "database") {
		if s.ID == "db-cli" || hasAlias(s, "mi-db-cli") {
			// Prefer operational CLI over schema-designer lexical hits.
			boost += 6.5
		}
	}
	// Secrets intent
	if hasAny(terms, "secret", "secrets", "credential", "vault", "key") ||
		strings.Contains(qLower, "api key") || strings.Contains(qLower, "mi-key") {
		if s.ID == "mi-key-cli" {
			boost += 3.0
		}
	}
	// Wiki / docs routers
	if hasAny(terms, "wiki", "documentation", "docs", "canon", "trazabilidad") {
		if s.ID == "ps-asistente-wiki" || s.ID == "ps-trazabilidad" || s.ID == "ps-auditar-trazabilidad" {
			boost += 1.5
		}
	}
	// Orchestration
	if hasAny(terms, "orchestrat", "workflow", "ae", "worker", "plan") {
		if s.ID == "ae-work" {
			boost += 1.5
		}
	}
	return boost
}

func criticalRelevant(s SkillRecord, terms []string, rawQuery string) bool {
	q := strings.ToLower(rawQuery)
	switch s.ID {
	case "mi-lsp":
		return hasAny(terms, "lsp", "nav", "workspace", "semantic", "index", "mi-lsp") ||
			strings.Contains(q, "mi-lsp")
	case "mi-key-cli":
		return hasAny(terms, "secret", "secrets", "credential", "vault", "key") ||
			strings.Contains(q, "api key")
	case "ae-pre-push":
		return hasAny(terms, "push", "pre-push", "release", "main", "gate")
	default:
		// generic: any term matches id tokens
		for _, t := range terms {
			if strings.Contains(strings.ToLower(s.ID), t) {
				return true
			}
			for _, a := range s.Aliases {
				if strings.Contains(strings.ToLower(a), t) {
					return true
				}
			}
		}
		return false
	}
}

func scoreWhy(s SkillRecord, lex, emb float64) string {
	parts := []string{}
	if s.Critical {
		parts = append(parts, "critical")
	}
	if s.Tier == TierParentRouter {
		parts = append(parts, "parent_router")
	}
	if lex > 0 {
		parts = append(parts, "lexical")
	}
	if emb > 0 {
		parts = append(parts, "embedding")
	}
	return strings.Join(parts, "+")
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	var b strings.Builder
	var out []string
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := b.String()
		b.Reset()
		if len(tok) < 2 {
			return
		}
		out = append(out, tok)
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func hasAny(terms []string, needles ...string) bool {
	for _, t := range terms {
		for _, n := range needles {
			if t == n || strings.HasPrefix(t, n) || strings.Contains(t, n) {
				return true
			}
		}
	}
	return false
}

func hasAlias(s SkillRecord, alias string) bool {
	for _, a := range s.Aliases {
		if a == alias {
			return true
		}
	}
	return false
}

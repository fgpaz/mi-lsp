package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	docRankerOwner  = "owner"
	docRankerLegacy = "legacy"
)

type ownerHintMatch struct {
	hint  model.DocsOwnerHint
	terms []string
}

func docRankingMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MI_LSP_DOC_RANKING"))) {
	case docRankerLegacy:
		return docRankerLegacy
	default:
		return docRankerOwner
	}
}

func rankDocs(question string, family string, docs []model.DocRecord, ftsScores map[string]float64, profile model.DocsReadProfile, recent []model.ReentryMemoryChange) []scoredDoc {
	if docRankingMode() == docRankerLegacy {
		return legacyRankDocs(question, family, docs, ftsScores)
	}
	return ownerAwareRankDocs(question, family, docs, ftsScores, profile, recent)
}

func legacyRankDocs(question string, family string, docs []model.DocRecord, ftsScores map[string]float64) []scoredDoc {
	tokens := docgraph.QuestionTokens(question)
	normalizedQuestion := normalizeRankingText(question)
	items := make([]scoredDoc, 0, len(docs))
	for _, doc := range docs {
		if doc.IsSnapshot {
			continue
		}
		score := 0
		reasons := make([]string, 0, 4)

		if ftsScores != nil {
			if ftsScore, ok := ftsScores[doc.Path]; ok {
				score += int(ftsScore)
				reasons = append(reasons, "fts5=match")
			}
		}

		if doc.Family == family {
			score += 30
			reasons = append(reasons, "family="+family)
		}
		score += layerWeight(family, doc.Layer)
		if doc.DocID != "" && strings.Contains(normalizedQuestion, normalizeRankingText(doc.DocID)) {
			score += 40
			reasons = append(reasons, "doc_id="+doc.DocID)
		}

		searchText := strings.ToLower(doc.SearchText)
		titleText := strings.ToLower(doc.Title)
		for _, token := range tokens {
			if strings.Contains(titleText, token) {
				score += 10
			}
			if strings.Contains(searchText, token) {
				score += 5
			}
		}

		if score > 0 {
			items = append(items, scoredDoc{record: doc, score: score, reason: reasons})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].record.Path < items[j].record.Path
		}
		return items[i].score > items[j].score
	})
	return items
}

func ownerAwareRankDocs(question string, family string, docs []model.DocRecord, ftsScores map[string]float64, profile model.DocsReadProfile, recent []model.ReentryMemoryChange) []scoredDoc {
	tokens := docgraph.QuestionTokens(question)
	normalizedQuestion := normalizeRankingText(question)
	hints := matchingOwnerHints(profile.OwnerHints, normalizedQuestion)
	recentRank := recentChangeRank(recent)

	type provisionalDoc struct {
		doc      model.DocRecord
		score    int
		reasons  []string
		positive bool
	}

	provisional := make([]provisionalDoc, 0, len(docs))
	hasCanonicalPositive := false
	hasCanonicalWikiPositive := false
	for _, doc := range docs {
		if doc.IsSnapshot {
			continue
		}
		score, reasons, positive := scoreOwnerAwareDoc(doc, family, normalizedQuestion, tokens, ftsScores, hints, recentRank)
		if positive && doc.Family != "generic" {
			hasCanonicalPositive = true
		}
		if positive && isCanonicalWikiDoc(doc.Path) {
			hasCanonicalWikiPositive = true
		}
		provisional = append(provisional, provisionalDoc{
			doc:      doc,
			score:    score,
			reasons:  reasons,
			positive: positive,
		})
	}

	items := make([]scoredDoc, 0, len(provisional))
	for _, candidate := range provisional {
		score := candidate.score
		reasons := append([]string{}, candidate.reasons...)
		if hasCanonicalPositive && candidate.doc.Family == "generic" && candidate.positive {
			score -= 60
			reasons = append(reasons, "generic_penalty")
		}
		if hasCanonicalWikiPositive && candidate.positive && isSupportArtifactDoc(candidate.doc.Path) {
			score -= 90
			reasons = append(reasons, "support_artifact_penalty")
		}
		if score <= 0 {
			continue
		}
		items = append(items, scoredDoc{
			record: candidate.doc,
			score:  score,
			reason: reasons,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			if items[i].record.Family == items[j].record.Family {
				return items[i].record.Path < items[j].record.Path
			}
			if items[i].record.Family == "generic" {
				return false
			}
			if items[j].record.Family == "generic" {
				return true
			}
			return items[i].record.Path < items[j].record.Path
		}
		return items[i].score > items[j].score
	})
	return items
}

func scoreOwnerAwareDoc(doc model.DocRecord, family string, normalizedQuestion string, tokens []string, ftsScores map[string]float64, hints []ownerHintMatch, recentRank map[string]int) (int, []string, bool) {
	score := 0
	reasons := make([]string, 0, 8)
	positive := false

	if ftsScores != nil {
		if ftsScore, ok := ftsScores[doc.Path]; ok {
			score += int(ftsScore)
			reasons = append(reasons, "fts5=match")
			positive = true
		}
	}

	if doc.Family == family {
		score += 30
		reasons = append(reasons, "family="+family)
	}
	score += layerWeight(family, doc.Layer)

	if doc.DocID != "" {
		docIDNormalized := normalizeRankingText(doc.DocID)
		if strings.Contains(normalizedQuestion, docIDNormalized) {
			score += 60
			reasons = append(reasons, "doc_id="+doc.DocID)
			positive = true
		}
	}

	titleText := normalizeRankingText(doc.Title)
	searchText := normalizeRankingText(doc.SearchText)
	pathTokens := rankingPathTokens(doc.Path)
	titleMatches := 0
	searchMatches := 0
	pathMatches := 0
	for _, token := range tokens {
		if strings.Contains(titleText, token) {
			score += 12
			titleMatches++
			positive = true
		}
		if strings.Contains(searchText, token) {
			score += 5
			searchMatches++
			positive = true
		}
		if _, ok := pathTokens[token]; ok {
			score += 9
			pathMatches++
			positive = true
		}
	}
	if titleMatches > 0 {
		reasons = append(reasons, "title_overlap")
	}
	if searchMatches > 0 {
		reasons = append(reasons, "search_overlap")
	}
	if pathMatches > 0 {
		reasons = append(reasons, "path_overlap")
	}

	if reason, bonus := ownerPrefixBoost(normalizedQuestion, doc); bonus > 0 {
		score += bonus
		reasons = append(reasons, reason)
		positive = true
	}

	for _, hint := range hints {
		if matched, reason, bonus := applyOwnerHint(doc, hint); matched {
			score += bonus
			reasons = append(reasons, reason)
			positive = true
		}
	}

	if positive && doc.Family != "generic" {
		score += 8
		reasons = append(reasons, "canonical_match")
	}
	if positive {
		if rank, ok := recentRank[doc.Path]; ok {
			bonus := max(1, 4-rank)
			score += bonus
			reasons = append(reasons, "recent_change_tiebreak")
		}
	}
	return score, dedupeStrings(reasons), positive
}

func ownerPrefixBoost(normalizedQuestion string, doc model.DocRecord) (string, int) {
	if doc.DocID == "" {
		return "", 0
	}
	prefix := strings.ToUpper(strings.SplitN(doc.DocID, "-", 2)[0])
	switch prefix {
	case "CT":
		if hasAnyTerm(normalizedQuestion, "contract", "contracts", "contrato", "contratos", "api", "envelope", "protocol", "nav ") {
			return "owner_prefix=CT", 22
		}
	case "FL":
		if hasAnyTerm(normalizedQuestion, "flow", "flows", "flujo", "flujos", "journey", "workflow") {
			return "owner_prefix=FL", 22
		}
	case "RF":
		if hasAnyTerm(normalizedQuestion, "requirement", "requirements", "requerimiento", "requerimientos", "behavior", "comportamiento") {
			return "owner_prefix=RF", 22
		}
	case "RS":
		if hasAnyTerm(normalizedQuestion, "outcome", "outcomes", "resultado", "resultados", "solution", "solutions", "solucion", "soluciones") {
			return "owner_prefix=RS", 22
		}
	case "TECH":
		if hasAnyTerm(normalizedQuestion, "technical", "tecnica", "tecnico", "baseline", "architecture", "arquitectura") {
			return "owner_prefix=TECH", 20
		}
	case "DB":
		if hasAnyTerm(normalizedQuestion, "database", "db", "schema", "telemetry", "sqlite") {
			return "owner_prefix=DB", 20
		}
	}
	return "", 0
}

func matchingOwnerHints(hints []model.DocsOwnerHint, normalizedQuestion string) []ownerHintMatch {
	if len(hints) == 0 {
		return nil
	}
	matches := make([]ownerHintMatch, 0, len(hints))
	for _, hint := range hints {
		matchedTerms := make([]string, 0, len(hint.Terms))
		for _, term := range hint.Terms {
			term = normalizeRankingText(term)
			if term == "" {
				continue
			}
			if strings.Contains(normalizedQuestion, term) {
				matchedTerms = append(matchedTerms, term)
			}
		}
		if len(matchedTerms) == 0 {
			continue
		}
		matches = append(matches, ownerHintMatch{hint: hint, terms: dedupeStrings(matchedTerms)})
	}
	return matches
}

func applyOwnerHint(doc model.DocRecord, hint ownerHintMatch) (bool, string, int) {
	reasons := make([]string, 0, 2)
	score := 0
	if containsFold(hint.hint.PreferDocIDs, doc.DocID) {
		score += 110
		reasons = append(reasons, "doc_id")
	}
	if pathMatchesAnyHint(doc.Path, hint.hint.PreferPaths) {
		score += 80
		reasons = append(reasons, "path")
	}
	if containsFold(hint.hint.PreferFamilies, doc.Family) {
		score += 35
		reasons = append(reasons, "family")
	}
	if containsFold(hint.hint.PreferLayers, doc.Layer) {
		score += 30
		reasons = append(reasons, "layer")
	}
	if score == 0 {
		return false, "", 0
	}
	return true, "owner_hint=" + strings.Join(reasons, "+"), score
}

func recentChangeRank(changes []model.ReentryMemoryChange) map[string]int {
	if len(changes) == 0 {
		return nil
	}
	ranks := make(map[string]int, len(changes))
	for i, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" {
			continue
		}
		ranks[path] = i + 1
	}
	return ranks
}

func rankingPathTokens(path string) map[string]struct{} {
	normalized := normalizeRankingText(filepath.ToSlash(path))
	tokens := strings.Fields(normalized)
	items := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if len(token) < 3 {
			continue
		}
		items[token] = struct{}{}
	}
	return items
}

func isCanonicalWikiDoc(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	return strings.HasPrefix(normalized, ".docs/wiki/")
}

func isSupportArtifactDoc(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	return strings.HasPrefix(normalized, ".docs/raw/")
}

func pathMatchesAnyHint(path string, patterns []string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(normalizedPath, strings.TrimSuffix(pattern, "/")+"/") {
			return true
		}
		if strings.ContainsAny(pattern, "*?[") {
			if matched, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(normalizedPath)); err == nil && matched {
				return true
			}
			continue
		}
		if strings.EqualFold(normalizedPath, pattern) || strings.HasPrefix(normalizedPath, pattern+"/") {
			return true
		}
	}
	return false
}

func containsFold(items []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}

func hasAnyTerm(normalizedQuestion string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(normalizedQuestion, normalizeRankingText(term)) {
			return true
		}
	}
	return false
}

func normalizeRankingText(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(
		"\r", " ",
		"\n", " ",
		"_", " ",
		"-", " ",
		"/", " ",
		"\\", " ",
		".", " ",
		":", " ",
	)
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func layerWeight(family string, layer string) int {
	switch family {
	case "functional":
		switch layer {
		case "01":
			return 18
		case "RS":
			return 17
		case "02":
			return 16
		case "03":
			return 14
		case "04":
			return 12
		case "05":
			return 10
		case "06":
			return 8
		}
	case "technical":
		switch layer {
		case "07":
			return 18
		case "08":
			return 14
		case "09":
			return 12
		}
	case "ux":
		switch layer {
		case "10":
			return 18
		case "11":
			return 16
		case "12":
			return 14
		case "13":
			return 12
		case "14":
			return 10
		case "15":
			return 8
		case "16":
			return 6
		}
	}
	if layer == "generic" {
		return 2
	}
	return 0
}

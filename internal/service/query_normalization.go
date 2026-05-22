package service

import "strings"

func queryRankingTask(task string) (string, bool) {
	original := strings.TrimSpace(task)
	if original == "" {
		return original, false
	}
	tokens := strings.Fields(normalizeRankingText(original))
	if !looksLikeSDDAnchorIntent(tokens) {
		return original, false
	}
	filtered := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if isSDDMetaToken(token) || isAnchorIntentStopToken(token) {
			continue
		}
		filtered = append(filtered, token)
	}
	if len(filtered) < 3 {
		return original, false
	}
	normalized := strings.Join(filtered, " ")
	if normalized == normalizeRankingText(original) {
		return original, false
	}
	return normalized, true
}

func looksLikeSDDAnchorIntent(tokens []string) bool {
	hasLayer := false
	hasAnchorIntent := false
	for _, token := range tokens {
		if isSDDMetaToken(token) {
			hasLayer = true
		}
		if token == "ancla" || token == "anclas" || token == "anchor" || token == "anchors" ||
			token == "aplica" || token == "aplican" || token == "aplicable" || token == "aplicables" {
			hasAnchorIntent = true
		}
	}
	return hasLayer && hasAnchorIntent
}

func isSDDMetaToken(token string) bool {
	switch token {
	case "rs", "rf", "fl", "ct", "tech", "db", "tp":
		return true
	default:
		return false
	}
}

func isAnchorIntentStopToken(token string) bool {
	switch token {
	case "que", "cual", "cuales", "ancla", "anclas", "anchor", "anchors",
		"aplica", "aplican", "aplicable", "aplicables", "son", "es":
		return true
	default:
		return false
	}
}

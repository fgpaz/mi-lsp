package service

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	coachTriggerRepoSelectorInvalid    = "repo_selector_invalid"
	coachTriggerRegexAutoHealed        = "regex_auto_healed"
	coachTriggerNoMatchesRefinable     = "no_matches_refinable"
	coachTriggerPreviewTrimmed         = "preview_trimmed"
	coachTriggerLowConfidence          = "low_confidence"
	coachTriggerTextFallback           = "text_fallback"
	coachTriggerScopeNarrowingAvailabe = "scope_narrowing_available"
)

var coachSearchStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "does": {}, "for": {}, "how": {}, "in": {}, "is": {}, "of": {}, "or": {},
	"the": {}, "this": {}, "to": {}, "what": {}, "where": {}, "with": {}, "work": {},
}

func applyCoachPolicy(env model.Envelope, opts model.QueryOptions) model.Envelope {
	if env.Coach == nil {
		return applyContinuationPolicy(env, opts)
	}
	coach := *env.Coach
	coach.Actions = trimSlice(coach.Actions, 2)
	if isAXIPreview(opts) {
		coach.Actions = trimSlice(coach.Actions, 1)
	}
	if len(coach.Actions) == 0 {
		coach.Actions = nil
	}
	env.Coach = &coach
	return applyContinuationPolicy(env, opts)
}

func applyContinuationPolicy(env model.Envelope, opts model.QueryOptions) model.Envelope {
	if env.Continuation == nil {
		return env
	}
	continuation := *env.Continuation
	if strings.TrimSpace(continuation.Next.Op) == "" {
		env.Continuation = nil
		return env
	}
	if isAXIPreview(opts) {
		continuation.Alternate = nil
	}
	env.Continuation = &continuation
	return env
}

func coachAction(kind string, label string, command string) model.CoachAction {
	return model.CoachAction{Kind: kind, Label: label, Command: command}
}

func searchCommand(alias string, pattern string, includeContent bool, repo string, useRegex bool, full bool) string {
	parts := []string{"mi-lsp", "nav", "search", fmt.Sprintf("%q", pattern), "--workspace", alias}
	if repo != "" {
		parts = append(parts, "--repo", repo)
	}
	if includeContent {
		parts = append(parts, "--include-content")
	}
	if useRegex {
		parts = append(parts, "--regex")
	}
	if full {
		parts = append(parts, "--full")
	}
	return strings.Join(parts, " ")
}

func askCommand(alias string, question string, full bool) string {
	parts := []string{"mi-lsp", "nav", "ask", fmt.Sprintf("%q", question), "--workspace", alias}
	if full {
		parts = append(parts, "--full")
	}
	return strings.Join(parts, " ")
}

func buildSearchScopeCoach(alias string, pattern string, includeContent bool, useRegex bool, env model.Envelope) *model.Coach {
	repo := firstSearchCandidateRepo(env.Items)
	if repo == "" {
		return nil
	}
	return &model.Coach{
		Trigger: coachTriggerRepoSelectorInvalid,
		Message: fmt.Sprintf("The repo selector could not be resolved cleanly; %q is the closest rerun target.", repo),
		Actions: []model.CoachAction{
			coachAction("rerun", "Retry with repo "+repo, searchCommand(alias, pattern, includeContent, repo, useRegex, false)),
		},
	}
}

func buildSearchCoach(alias string, project model.ProjectFile, pattern string, includeContent bool, repoSelector string, useRegex bool, regexAutoHealed bool, items []map[string]any, opts model.QueryOptions) *model.Coach {
	if regexAutoHealed {
		return &model.Coach{
			Trigger: coachTriggerRegexAutoHealed,
			Message: "The supplied regex was invalid, so mi-lsp retried the query as a literal search.",
			Actions: []model.CoachAction{
				coachAction("rerun", "Run as literal search", searchCommand(alias, pattern, includeContent, repoSelector, false, false)),
			},
		}
	}

	if len(items) == 0 {
		actions := make([]model.CoachAction, 0, 2)
		if keyword := strongestSearchCoachToken(pattern); keyword != "" && !strings.EqualFold(keyword, pattern) {
			actions = append(actions, coachAction("refine", "Retry with "+keyword, searchCommand(alias, keyword, includeContent, repoSelector, false, false)))
		}
		if looksRegexLikePattern(pattern) && !useRegex {
			actions = append(actions, coachAction("rerun", "Retry as regex", searchCommand(alias, pattern, includeContent, repoSelector, true, false)))
		}
		if len(actions) > 0 {
			return &model.Coach{
				Trigger: coachTriggerNoMatchesRefinable,
				Message: "No matches were found, but this query still has a clear refinement path.",
				Actions: actions,
			}
		}
	}

	if repoSelector == "" && len(project.Repos) != 1 {
		if repo := searchSingleVisibleRepo(project, items); repo != "" {
			return &model.Coach{
				Trigger: coachTriggerScopeNarrowingAvailabe,
				Message: fmt.Sprintf("The visible matches are all under repo %q; narrowing the scope should speed up follow-up queries.", repo),
				Actions: []model.CoachAction{
					coachAction("narrow", "Scope to repo "+repo, searchCommand(alias, pattern, includeContent, repo, useRegex, false)),
				},
			}
		}
	}

	if isAXIPreview(opts) && opts.MaxItems > 0 && len(items) >= opts.MaxItems {
		return &model.Coach{
			Trigger: coachTriggerPreviewTrimmed,
			Message: "Preview mode may be hiding additional matches for this query.",
			Actions: []model.CoachAction{
				coachAction("expand", "Rerun with --full", searchCommand(alias, pattern, includeContent, repoSelector, useRegex, true)),
			},
		}
	}

	return nil
}

func buildAskCoach(alias string, project model.ProjectFile, question string, result model.AskResult, warnings []string, opts model.QueryOptions, previewTrimmed bool) *model.Coach {
	confidence := askCoachConfidence(result, warnings)
	repoScope := askRepoScope(project, result.CodeEvidence)
	searchQuery := bestAskCoachSearchQuery(question, result)
	searchCmd := searchCommand(alias, searchQuery, true, repoScope, false, false)

	if containsWarning(warnings, "documentation index is empty; using code fallback") {
		return &model.Coach{
			Trigger:    coachTriggerTextFallback,
			Message:    "This answer relied on textual fallback instead of strong indexed evidence.",
			Confidence: "low",
			Actions: []model.CoachAction{
				coachAction("refine", "Inspect supporting code", searchCmd),
			},
		}
	}

	if previewTrimmed && isAXIPreview(opts) {
		return &model.Coach{
			Trigger:    coachTriggerPreviewTrimmed,
			Message:    "Preview mode trimmed the evidence behind this answer.",
			Confidence: confidence,
			Actions: []model.CoachAction{
				coachAction("expand", "Rerun with --full", askCommand(alias, question, true)),
			},
		}
	}

	if confidence == "low" {
		return &model.Coach{
			Trigger:    coachTriggerLowConfidence,
			Message:    "The answer is usable, but the supporting evidence is still thin.",
			Confidence: confidence,
			Actions: []model.CoachAction{
				coachAction("refine", "Search supporting code", searchCmd),
			},
		}
	}

	return nil
}

func askCoachConfidence(result model.AskResult, warnings []string) string {
	if containsWarning(warnings, "documentation index is empty; using code fallback") ||
		containsWarning(warnings, "code evidence came from text fallback") ||
		result.PrimaryDoc.Path == "" ||
		len(result.DocEvidence) == 0 {
		return "low"
	}
	if len(result.DocEvidence) >= 2 && len(result.CodeEvidence) >= 1 {
		return "high"
	}
	return "medium"
}

func askResultWouldTrimForAXIPreview(result model.AskResult) bool {
	return len(result.DocEvidence) > 2 || len(result.CodeEvidence) > 2 || len(result.Why) > 3 || len(result.NextQueries) > 3
}

func bestAskCoachSearchQuery(question string, result model.AskResult) string {
	if result.PrimaryDoc.DocID != "" {
		return result.PrimaryDoc.DocID
	}
	if keyword := strongestSearchCoachToken(question); keyword != "" {
		return keyword
	}
	return question
}

func buildAskAllWorkspacesCoach(question string, topWorkspace string, opts model.QueryOptions) *model.Coach {
	if strings.TrimSpace(topWorkspace) == "" {
		return nil
	}
	return &model.Coach{
		Trigger: coachTriggerScopeNarrowingAvailabe,
		Message: fmt.Sprintf("One workspace stands out as the best match for this question: %q.", topWorkspace),
		Actions: []model.CoachAction{
			coachAction("narrow", "Rerun in workspace "+topWorkspace, askCommand(topWorkspace, question, opts.Full)),
		},
	}
}

func searchSingleVisibleRepo(project model.ProjectFile, items []map[string]any) string {
	repos := map[string]struct{}{}
	for _, item := range items {
		repo, _ := item["repo"].(string)
		repo = strings.TrimSpace(repo)
		if repo == "" {
			file, _ := item["file"].(string)
			repo = repoForSearchItem(project, file)
		}
		if repo == "" {
			return ""
		}
		repos[repo] = struct{}{}
	}
	if len(repos) != 1 {
		return ""
	}
	for repo := range repos {
		return repo
	}
	return ""
}

func repoForSearchItem(project model.ProjectFile, file string) string {
	normalized := strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, "\\", "/")), "/")
	if normalized == "" {
		return ""
	}
	var fallback string
	for _, repo := range project.Repos {
		root := strings.Trim(strings.TrimSpace(strings.ReplaceAll(repo.Root, "\\", "/")), "/")
		if root == "" || root == "." {
			if fallback == "" {
				fallback = repo.Name
			}
			continue
		}
		if normalized == root || strings.HasPrefix(normalized, root+"/") {
			return repo.Name
		}
	}
	if len(project.Repos) == 1 {
		return project.Repos[0].Name
	}
	return fallback
}

func firstSearchCandidateRepo(items any) string {
	candidates, ok := items.([]map[string]any)
	if !ok || len(candidates) == 0 {
		return ""
	}
	repo, _ := candidates[0]["repo"].(string)
	return strings.TrimSpace(repo)
}

func strongestSearchCoachToken(pattern string) string {
	fields := strings.FieldsFunc(pattern, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-'
	})
	type candidate struct {
		raw   string
		score int
	}
	candidates := make([]candidate, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if len(trimmed) < 4 {
			continue
		}
		if _, blocked := coachSearchStopwords[strings.ToLower(trimmed)]; blocked {
			continue
		}
		candidates = append(candidates, candidate{raw: trimmed, score: len(trimmed)})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return strings.ToLower(candidates[i].raw) < strings.ToLower(candidates[j].raw)
		}
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].raw
}

func containsWarning(warnings []string, fragment string) bool {
	fragment = strings.ToLower(strings.TrimSpace(fragment))
	if fragment == "" {
		return false
	}
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning), fragment) {
			return true
		}
	}
	return false
}

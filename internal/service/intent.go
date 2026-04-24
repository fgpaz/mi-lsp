package service

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type intentMatch struct {
	Symbol   model.SymbolRecord
	Score    float64
	Evidence string
}

var (
	intentDocIDPattern  = regexp.MustCompile(`\b(?:FL|RS|RF|TP|TECH|CT|DB)-[A-Z0-9-]+\b`)
	intentSymbolPattern = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]{2,}\b`)
)

func (a *App) intent(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	question, _ := request.Payload["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return model.Envelope{}, fmt.Errorf("question is required for nav intent")
	}

	topN := intFromAny(request.Payload["top"], 10)
	if topN <= 0 {
		topN = 10
	}
	offset := intFromAny(request.Payload["offset"], 0)

	scopedRepo, scopeWarnings, scopeEnvelope := resolveCatalogRepoScope(registration, project, request.Payload)
	if scopeEnvelope != nil {
		return *scopeEnvelope, nil
	}

	profile, _, _ := docgraph.LoadProfile(registration.Root)
	mode := classifyIntentMode(question, profile)
	if mode == "docs" {
		return a.intentDocs(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings)
	}

	db, err := openWorkspaceDB(registration, "nav.intent")
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	tokens := docgraph.QuestionTokens(question)
	if len(tokens) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Mode: "code", Items: []map[string]any{}, Warnings: []string{"query produced no tokens after normalization"}}, nil
	}

	queryLimit := topN * 5
	sqlOffset := offset
	if scopedRepo != nil {
		queryLimit = max((offset+topN)*10, 100)
		sqlOffset = 0
	}
	candidates, err := store.IntentSearch(ctx, db, tokens, queryLimit, sqlOffset)
	if err != nil {
		return model.Envelope{}, err
	}
	candidates = filterSymbolsByRepo(candidates, scopedRepo)
	if offset > 0 {
		if offset >= len(candidates) {
			candidates = []model.SymbolRecord{}
		} else {
			candidates = candidates[offset:]
		}
	}

	if len(candidates) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Mode: "code", Items: []map[string]any{}, Warnings: []string{"no symbols matched intent tokens"}}, nil
	}

	scored := scoreBM25(candidates, tokens)
	if len(scored) > topN {
		scored = scored[:topN]
	}

	items := make([]map[string]any, len(scored))
	for i, match := range scored {
		items[i] = map[string]any{
			"file":           match.Symbol.FilePath,
			"line":           match.Symbol.StartLine,
			"symbol":         match.Symbol.Name,
			"kind":           match.Symbol.Kind,
			"qualified_name": match.Symbol.QualifiedName,
			"score":          fmt.Sprintf("%.2f", match.Score),
			"evidence":       match.Evidence,
			"snippet":        intentSnippet(match.Symbol),
		}
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "intent",
		Mode:      "code",
		Items:     items,
		Warnings:  scopeWarnings,
		Stats:     model.Stats{Symbols: len(items)},
	}
	return applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint), nil
}

func (a *App) intentDocs(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, question string, topN int, offset int, scopedRepo *model.WorkspaceRepo, scopeWarnings []string) (model.Envelope, error) {
	query := loadDocQueryContext(ctx, registration, question)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	route := query.canonicalRoute(request.Context, false)
	items := buildIntentDocItems(registration.Name, question, route, query.ranked, topN, offset)
	warnings := append([]string{}, scopeWarnings...)
	warnings = append(warnings, query.profileWarnings...)
	if scopedRepo != nil {
		warnings = append(warnings, "repo selector applies only to code mode; ignored after docs classification")
	}
	if len(items) == 0 {
		warnings = append(warnings, "no docs matched intent query")
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "intent",
		Mode:      "docs",
		Items:     items,
		Warnings:  dedupeStrings(warnings),
		Stats:     model.Stats{Files: len(items)},
	}
	return applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint), nil
}

func scoreBM25(symbols []model.SymbolRecord, tokens []string) []intentMatch {
	// Compute document frequency per token
	docFreq := make(map[string]int)
	for _, sym := range symbols {
		seen := make(map[string]struct{})
		searchLower := strings.ToLower(sym.SearchText)
		for _, token := range tokens {
			if strings.Contains(searchLower, token) {
				if _, ok := seen[token]; !ok {
					docFreq[token]++
					seen[token] = struct{}{}
				}
			}
		}
	}

	totalDocs := float64(len(symbols))
	scored := make([]intentMatch, 0, len(symbols))

	for _, sym := range symbols {
		score := 0.0
		evidence := ""
		searchLower := strings.ToLower(sym.SearchText)
		nameLower := strings.ToLower(sym.Name)

		for _, token := range tokens {
			if !strings.Contains(searchLower, token) {
				continue
			}

			count := float64(strings.Count(searchLower, token))

			// IDF: log(N / df)
			idf := 1.0
			if df, ok := docFreq[token]; ok && df > 0 {
				idf = 1.0 + math.Log(totalDocs/float64(df))
			}

			termScore := count * idf

			// Positional boosts
			if strings.Contains(nameLower, token) {
				termScore *= 3.0
				if evidence == "" {
					evidence = "name_match"
				}
			}
			kindLower := strings.ToLower(sym.Kind)
			if strings.Contains(kindLower, token) {
				termScore *= 2.0
			}
			if sym.Parent != "" && strings.Contains(strings.ToLower(sym.Parent), token) {
				termScore *= 1.5
			}

			score += termScore
		}

		if score > 0 {
			if evidence == "" {
				evidence = "search_text_match"
			}
			scored = append(scored, intentMatch{
				Symbol:   sym,
				Score:    score,
				Evidence: evidence,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}

func intentSnippet(sym model.SymbolRecord) string {
	if sym.Signature != "" {
		return sym.Signature
	}
	if sym.Parent != "" {
		return sym.Parent + "." + sym.Name
	}
	return sym.Name
}

func classifyIntentMode(question string, profile model.DocsReadProfile) string {
	normalized := normalizeRankingText(question)
	tokens := docgraph.QuestionTokens(question)
	if normalized == "" {
		return "code"
	}
	if intentDocIDPattern.MatchString(strings.ToUpper(question)) {
		return "docs"
	}
	if looksLikeCodeIntent(normalized, question, tokens) {
		return "code"
	}
	if matchesIntentOwnerHint(normalized, profile.OwnerHints) {
		return "docs"
	}
	if hasAnyTerm(normalized,
		"how", "what", "why", "when", "where", "understand", "explain",
		"contract", "contracts", "contrato", "contratos", "flow", "flows", "flujo", "flujos",
		"requirement", "requirements", "requerimiento", "requerimientos",
		"governance", "workspace status", "read model", "continuation", "memory pointer",
		"memory_pointer", "stale", "preview", "full", "mode", "nav ask", "nav route", "nav pack") {
		return "docs"
	}
	if len(tokens) >= 4 {
		return "docs"
	}
	if docgraph.MatchFamily(question, profile) != "technical" && len(tokens) >= 3 {
		return "docs"
	}
	return "code"
}

func looksLikeCodeIntent(normalized string, raw string, tokens []string) bool {
	if strings.ContainsAny(raw, "/\\(){}[]") || strings.Contains(raw, "::") {
		return true
	}
	if strings.Contains(raw, ".cs") || strings.Contains(raw, ".ts") || strings.Contains(raw, ".go") {
		return true
	}
	if intentSymbolPattern.MatchString(raw) && len(tokens) <= 3 && !hasAnyTerm(normalized, "how", "what", "why", "where", "when", "understand", "explain") {
		return true
	}
	if hasAnyTerm(normalized, "class", "method", "function", "interface", "symbol", "implementation", "implementacion", "handler", "service", "repository") && len(tokens) <= 4 {
		return true
	}
	return false
}

func matchesIntentOwnerHint(normalized string, hints []model.DocsOwnerHint) bool {
	for _, hint := range hints {
		for _, term := range hint.Terms {
			if term = normalizeRankingText(term); term != "" && strings.Contains(normalized, term) {
				return true
			}
		}
	}
	return false
}

func buildIntentDocItems(workspaceName string, question string, route model.RouteResult, ranked []scoredDoc, topN int, offset int) []map[string]any {
	if topN <= 0 {
		topN = 10
	}
	if offset < 0 {
		offset = 0
	}
	if len(ranked) > 0 {
		if offset >= len(ranked) {
			return []map[string]any{}
		}
		end := min(len(ranked), offset+topN)
		items := make([]map[string]any, 0, end-offset)
		for _, candidate := range ranked[offset:end] {
			items = append(items, map[string]any{
				"doc_path":     candidate.record.Path,
				"doc_id":       candidate.record.DocID,
				"title":        candidate.record.Title,
				"family":       candidate.record.Family,
				"layer":        candidate.record.Layer,
				"score":        candidate.score,
				"evidence":     append([]string{}, candidate.reason...),
				"next_queries": buildIntentDocNextQueries(workspaceName, question, candidate.record.Path, candidate.record.DocID),
			})
		}
		return items
	}

	routeDocs := make([]model.RouteDoc, 0, 1+len(route.Canonical.PreviewPack))
	if route.Canonical.AnchorDoc.Path != "" {
		routeDocs = append(routeDocs, route.Canonical.AnchorDoc)
	}
	routeDocs = append(routeDocs, route.Canonical.PreviewPack...)
	if offset >= len(routeDocs) {
		return []map[string]any{}
	}
	end := min(len(routeDocs), offset+topN)
	items := make([]map[string]any, 0, end-offset)
	for idx, doc := range routeDocs[offset:end] {
		items = append(items, map[string]any{
			"doc_path":     doc.Path,
			"doc_id":       doc.DocID,
			"title":        doc.Title,
			"family":       doc.Family,
			"layer":        doc.Layer,
			"score":        max(1, len(routeDocs)-idx),
			"evidence":     []string{"tier1_canonical_route", doc.Why},
			"next_queries": buildIntentDocNextQueries(workspaceName, question, doc.Path, doc.DocID),
		})
	}
	return items
}

func buildIntentDocNextQueries(workspaceName string, question string, path string, docID string) []string {
	queries := []string{
		fmt.Sprintf("mi-lsp nav ask %q --workspace %s --full", question, workspaceName),
		fmt.Sprintf("mi-lsp nav pack %q --workspace %s", question, workspaceName),
	}
	if strings.TrimSpace(docID) != "" {
		queries = append(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s", docID, workspaceName))
	}
	if strings.TrimSpace(path) != "" {
		queries = append(queries, fmt.Sprintf("mi-lsp nav multi-read %s:1-120 --workspace %s", filepath.ToSlash(path), workspaceName))
	}
	return queries
}

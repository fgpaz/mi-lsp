package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

	if planned, handled, planErr := a.intentPlan(ctx, request, registration, project, question, scopedRepo, scopeWarnings); handled {
		return planned, planErr
	}

	profile, _, _ := docgraph.LoadProfile(registration.Root)
	mode := classifyIntentMode(question, profile)
	if mode == "docs" {
		return a.intentDocs(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings)
	}

	db, err := openWorkspaceDB(registration, "nav.intent", true)
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

// intentRoute is the small, deterministic route decision made before any
// backend is consulted. The planner never guesses an ambiguous selector.
type intentRoute struct {
	Intent     string
	Operation  string
	Arguments  map[string]string
	Confidence float64
}

func (a *App) intentPlan(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, project model.ProjectFile, question string, scopedRepo *model.WorkspaceRepo, scopeWarnings []string) (model.Envelope, bool, error) {
	route, ok := classifySupportedIntent(question, request.Payload)
	if !ok {
		return model.Envelope{}, false, nil
	}

	plan := model.IntentPlan{
		Intent:     route.Intent,
		Operation:  route.Operation,
		Arguments:  cloneStringMap(route.Arguments),
		Confidence: route.Confidence,
		Freshness:  "catalog-current",
		Preview:    []model.IntentPreview{},
		Omissions:  []model.IntentOmission{},
		Fallbacks:  []model.IntentFallback{},
		Expansions: []model.Expansion{},
		Telemetry:  model.IntentTelemetry{PlannerVersion: "intent-v1", Operation: route.Operation},
	}
	warnings := append([]string{"automatic intent routing selected a local deterministic planner"}, scopeWarnings...)

	if route.Operation == "explain-change" {
		a.planExplainChange(ctx, request, registration, &plan, &warnings)
	} else {
		a.planGraphIntent(ctx, request, registration, project, scopedRepo, &plan, &warnings)
	}

	plan.Telemetry.CandidateCount = len(plan.Candidates)
	plan.Telemetry.SectionCount = len(plan.Preview)
	plan.Telemetry.OmissionCount = len(plan.Omissions)
	plan.Telemetry.Fallback = len(plan.Fallbacks) > 0
	if plan.Arguments["from"] != "" || plan.Arguments["to"] != "" {
		plan.Telemetry.SelectorKind = "path_endpoints"
	} else if plan.Arguments["edge"] != "" {
		plan.Telemetry.SelectorKind = "edge"
	} else if plan.Arguments["selector"] != "" {
		plan.Telemetry.SelectorKind = "symbol"
	}
	if plan.GenerationID != "" {
		plan.Freshness = "graph-generation-bound"
	} else if plan.Operation == "explain-change" {
		plan.Freshness = "working-tree-snapshot"
	}
	plan.DeterminismDigest = model.IntentPlanDigest(plan)
	plan.Telemetry.CandidateCount = len(plan.Candidates)

	env := model.Envelope{
		Ok:                true,
		Workspace:         registration.Name,
		Backend:           "planner",
		Mode:              "preview",
		Items:             []model.IntentPlan{plan},
		Warnings:          dedupeStrings(warnings),
		Truncated:         plan.Truncated,
		GenerationID:      plan.GenerationID,
		DeterminismDigest: plan.DeterminismDigest,
		Stats:             model.Stats{Files: len(plan.Preview)},
	}
	if len(plan.Omissions) > 0 {
		for _, omission := range plan.Omissions {
			env.Warnings = appendStringIfMissing(env.Warnings, omission.Code+": "+omission.Reason)
		}
	}
	return applyAXIPreviewHints(env, request.Context, "preview mode: use an expansion command for the next bounded section"), true, nil
}

func classifySupportedIntent(question string, payload map[string]any) (intentRoute, bool) {
	explicit := strings.ToLower(strings.TrimSpace(stringPayload(payload, "intent")))
	normalized := strings.ToLower(strings.TrimSpace(question))
	operation := ""
	switch explicit {
	case "callers", "caller":
		operation = "callers"
	case "callees", "callee":
		operation = "callees"
	case "affected-change", "affected", "impact":
		operation = "affected-change"
	case "path-between", "path":
		operation = "path-between"
	case "explain-edge", "edge":
		operation = "explain-edge"
	case "neighborhood", "related":
		operation = "neighborhood"
	case "explain-change", "change":
		operation = "explain-change"
	}
	if operation == "" {
		switch {
		case hasAnyTerm(normalized, "explain change", "explain the change", "what changed", "impact of this change", "impact of the change"):
			operation = "explain-change"
		case hasAnyTerm(normalized, "path between", "path from") && (strings.Contains(normalized, " to ") || strings.Contains(normalized, " and ")):
			operation = "path-between"
		case hasAnyTerm(normalized, "explain edge", "explain relationship", "edge between"):
			operation = "explain-edge"
		case hasAnyTerm(normalized, "affected change", "affected by this change", "change impact", "impact of"):
			operation = "affected-change"
		case hasAnyTerm(normalized, "callers of", "who calls", "incoming callers"):
			operation = "callers"
		case hasAnyTerm(normalized, "callees of", "what does", "calls from") && strings.Contains(normalized, "call"):
			operation = "callees"
		case hasAnyTerm(normalized, "neighborhood of", "around", "related to"):
			operation = "neighborhood"
		}
	}
	if operation == "" {
		return intentRoute{}, false
	}
	args := extractIntentArguments(question, payload, operation)
	confidence := 0.9
	if explicit != "" {
		confidence = 1.0
	}
	if operation != "explain-change" && len(args) == 0 {
		confidence = 0.65
	}
	return intentRoute{Intent: operation, Operation: operation, Arguments: args, Confidence: confidence}, true
}

func extractIntentArguments(question string, payload map[string]any, operation string) map[string]string {
	args := map[string]string{}
	for _, key := range []string{"selector", "symbol", "from", "to", "edge", "generation", "ref"} {
		if value := strings.TrimSpace(stringPayload(payload, key)); value != "" {
			args[key] = filepath.ToSlash(value)
		}
	}
	if paths := affectedPathsFromPayload(payload["paths"]); len(paths) > 0 {
		args["paths"] = strings.Join(paths, ",")
	}
	if operation == "path-between" && (args["from"] == "" || args["to"] == "") {
		matches := regexp.MustCompile(`(?i)\b(?:between|from)\s+([A-Za-z0-9_:.\/\\-]+)\s+(?:and|to)\s+([A-Za-z0-9_:.\/\\-]+)`).FindStringSubmatch(question)
		if len(matches) == 3 {
			args["from"], args["to"] = matches[1], matches[2]
		}
	}
	if operation == "explain-edge" && args["edge"] == "" {
		matches := regexp.MustCompile(`(?i)\bedge(?:\s+selector)?[:= ]+([A-Za-z0-9_:.\/\\-]+)`).FindStringSubmatch(question)
		if len(matches) == 2 {
			args["edge"] = matches[1]
		}
	}
	if operation != "explain-change" && args["selector"] == "" && args["symbol"] == "" {
		args["selector"] = extractIntentSelector(question)
	}
	if args["selector"] == "" && args["symbol"] != "" {
		args["selector"] = args["symbol"]
	}
	return args
}

func extractIntentSelector(question string) string {
	matches := intentSymbolPattern.FindAllString(question, -1)
	ignored := map[string]struct{}{"Callers": {}, "Caller": {}, "Callees": {}, "Callee": {}, "Explain": {}, "Edge": {}, "Path": {}, "Between": {}, "Neighborhood": {}, "Related": {}, "Change": {}, "Impact": {}, "What": {}, "Who": {}}
	for i := len(matches) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(matches[i])
		if _, skip := ignored[candidate]; skip {
			continue
		}
		return candidate
	}
	return ""
}

func (a *App) planGraphIntent(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, project model.ProjectFile, scopedRepo *model.WorkspaceRepo, plan *model.IntentPlan, warnings *[]string) {
	selector := strings.TrimSpace(plan.Arguments["selector"])
	if plan.Operation == "path-between" {
		from, to := strings.TrimSpace(plan.Arguments["from"]), strings.TrimSpace(plan.Arguments["to"])
		if from == "" || to == "" {
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_ARGUMENT_MISSING", Section: "path", Reason: "path-between requires deterministic from and to selectors"})
			plan.Incomplete = true
			plan.Expansions = append(plan.Expansions, model.Expansion{Command: intentPathDiscoveryExpansion(registration.Name, from, to), Reason: "discover valid endpoint selectors before executing nav path; incomplete endpoints must not be sent to nav path"})
			return
		}
		fromResolved, fromCandidates, fromWarning := a.resolveIntentSelector(ctx, registration, project, scopedRepo, from)
		toResolved, toCandidates, toWarning := a.resolveIntentSelector(ctx, registration, project, scopedRepo, to)
		if fromWarning != "" {
			*warnings = appendStringIfMissing(*warnings, fromWarning)
		}
		if toWarning != "" {
			*warnings = appendStringIfMissing(*warnings, toWarning)
		}
		if len(fromCandidates) > 1 || len(toCandidates) > 1 {
			plan.Candidates = append(plan.Candidates, fromCandidates...)
			plan.Candidates = append(plan.Candidates, toCandidates...)
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_SELECTOR_AMBIGUOUS", Section: "path", Reason: "path endpoint selector is ambiguous; no endpoint was auto-selected", Candidates: candidateSelectors(append(fromCandidates, toCandidates...))})
			return
		}
		if fromResolved != "" {
			plan.Arguments["from"] = fromResolved
		}
		if toResolved != "" {
			plan.Arguments["to"] = toResolved
		}
		a.executePlannedGraph(ctx, request, registration, plan, warnings, "nav.path", map[string]any{"from": plan.Arguments["from"], "to": plan.Arguments["to"]}, "path")
		return
	}

	if plan.Operation == "affected-change" {
		child := request
		child.Operation = "nav.affected"
		child.Payload = cloneIntentPayload(request.Payload)
		if len(affectedPathsFromPayload(child.Payload["paths"])) == 0 {
			child.Payload["from_git_diff"] = true
		}
		child.Payload["include_tests"] = true
		child.Payload["include_docs"] = true
		env, err := a.affected(ctx, child)
		if err != nil {
			plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: "affected", Operation: "nav.affected", Reason: "affected-change could not query the local catalog/graph"})
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_AFFECTED_UNAVAILABLE", Section: "affected", Reason: sanitizeIntentError(err)})
			return
		}
		items, truncated := boundIntentItems(intentAnyItems(env.Items), request.Context)
		plan.Preview = append(plan.Preview, model.IntentPreview{Section: "affected", Items: items, Count: len(items), Truncated: env.Truncated || truncated})
		plan.Truncated = env.Truncated || truncated
		plan.GenerationID = env.GenerationID
		if strings.Contains(env.Backend, "heuristic") {
			plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: "affected", Operation: "nav.affected", Reason: "graph generation unavailable; affected output is explicitly heuristic"})
		}
		for _, warning := range env.Warnings {
			*warnings = appendStringIfMissing(*warnings, warning)
		}
		plan.Expansions = append(plan.Expansions, model.Expansion{Command: intentAffectedExpansion(registration.Name, plan.GenerationID), Reason: "expand the affected-change section with the same changed-path snapshot"})
		return
	}

	if plan.Operation == "explain-edge" {
		edge := strings.TrimSpace(plan.Arguments["edge"])
		if edge == "" {
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_ARGUMENT_MISSING", Section: "edge", Reason: "explain-edge requires an edge selector"})
			return
		}
		a.executePlannedGraph(ctx, request, registration, plan, warnings, "nav.explain", map[string]any{"selector": edge}, "edge")
		return
	}

	if selector == "" {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_ARGUMENT_MISSING", Section: plan.Operation, Reason: plan.Operation + " requires a symbol selector"})
		return
	}
	resolved, candidates, candidateWarning := a.resolveIntentSelector(ctx, registration, project, scopedRepo, selector)
	if candidateWarning != "" {
		*warnings = appendStringIfMissing(*warnings, candidateWarning)
	}
	if len(candidates) > 1 {
		plan.Candidates = candidates
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_SELECTOR_AMBIGUOUS", Section: plan.Operation, Reason: "selector matched multiple symbols; no symbol was auto-selected", Candidates: candidateSelectors(candidates)})
		return
	}
	if resolved != "" {
		plan.Arguments["selector"] = resolved
	}
	graphOp := map[string]string{"callers": "nav.callers", "callees": "nav.callees", "neighborhood": "nav.neighbors"}[plan.Operation]
	if graphOp == "" {
		return
	}
	a.executePlannedGraph(ctx, request, registration, plan, warnings, graphOp, map[string]any{"selector": plan.Arguments["selector"]}, plan.Operation)
}

func (a *App) resolveIntentSelector(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, scopedRepo *model.WorkspaceRepo, selector string) (string, []model.IntentCandidate, string) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", nil, ""
	}
	db, err := openWorkspaceDB(registration, "nav.intent.plan", true)
	if err != nil {
		return selector, nil, "catalog unavailable; planner retained the explicit selector and will attempt graph resolution"
	}
	defer db.Close()
	symbols, err := store.FindSymbols(ctx, db, selector, "", false, 20, 0)
	if err != nil {
		return selector, nil, "catalog selector lookup failed; planner retained the explicit selector"
	}
	symbols = filterSymbolsByRepo(symbols, scopedRepo)
	candidates := make([]model.IntentCandidate, 0, len(symbols))
	for _, symbol := range symbols {
		candidates = append(candidates, model.IntentCandidate{Selector: symbol.Name, File: symbol.FilePath, Line: symbol.StartLine, Kind: symbol.Kind, QualifiedName: symbol.QualifiedName, Score: 1.0})
	}
	if len(candidates) == 1 {
		return candidates[0].Selector, candidates, ""
	}
	if len(candidates) > 1 {
		return "", candidates, "selector resolution produced candidates; explicit disambiguation is required"
	}
	return selector, nil, "selector was not found in the catalog; graph resolution may report an omission"
}

func (a *App) executePlannedGraph(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, plan *model.IntentPlan, warnings *[]string, operation string, args map[string]any, section string) {
	payload := cloneIntentPayload(request.Payload)
	for key, value := range args {
		payload[key] = value
	}
	payload["generation"] = plan.Arguments["generation"]
	child := request
	child.Operation = operation
	child.Payload = payload
	env, err := a.graphQuery(ctx, child)
	if err != nil {
		if graphErr, ok := err.(*model.GraphQueryError); ok && len(graphErr.Candidates) > 0 {
			candidates := make([]model.IntentCandidate, 0, len(graphErr.Candidates))
			for _, item := range graphErr.Candidates {
				candidates = append(candidates, model.IntentCandidate{Selector: item.Display, File: item.OwnerPath, Kind: item.SymbolKind, QualifiedName: item.NodeKey, Score: 1.0})
			}
			plan.Candidates = candidates
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_SELECTOR_AMBIGUOUS", Section: section, Reason: "graph selector is ambiguous; no node was auto-selected", Candidates: candidateSelectors(candidates)})
			return
		}
		plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: section, Operation: operation, Reason: "graph backend unavailable or stale; retained the planner preview without claiming graph precision"})
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_GRAPH_UNAVAILABLE", Section: section, Reason: sanitizeIntentError(err)})
		return
	}
	if env.GenerationID != "" && plan.GenerationID == "" {
		plan.GenerationID = env.GenerationID
	}
	if plan.GenerationID != "" && env.GenerationID != "" && plan.GenerationID != env.GenerationID {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_GENERATION_MISMATCH", Section: section, Reason: "composed graph sections were read from different generations"})
	}
	if env.Truncated {
		plan.Truncated = true
	}
	items := intentAnyItems(env.Items)
	items, truncated := boundIntentItems(items, request.Context)
	if truncated {
		plan.Truncated = true
	}
	preview := model.IntentPreview{Section: section, Items: items, Count: len(items), Truncated: env.Truncated || truncated}
	if len(items) == 0 && env.Hint != "" {
		preview.Omission = env.Hint
	}
	plan.Preview = append(plan.Preview, preview)
	for _, warning := range env.Warnings {
		*warnings = appendStringIfMissing(*warnings, warning)
	}
	for _, omission := range env.Omissions {
		code := omission.ErrorCode
		if code == "" {
			code = "INTENT_GRAPH_OMISSION"
		}
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: code, Section: section, Reason: omission.Reason})
	}
	plan.Expansions = append(plan.Expansions, model.Expansion{Command: intentExpansionCommandForPlan(registration.Name, *plan), Reason: "expand the selected graph section with the same selector and generation"})
}

func intentAdoptGeneration(plan *model.IntentPlan, section, observed string) bool {
	observed = strings.TrimSpace(observed)
	if observed == "" {
		return true
	}
	if plan.GenerationID == "" {
		plan.GenerationID = observed
		return true
	}
	if plan.GenerationID == observed {
		return true
	}
	plan.Incomplete = true
	plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_GENERATION_MISMATCH", Section: section, Reason: "composed graph sections were read from different generations; section withheld"})
	return false
}

func (a *App) planExplainChange(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, plan *model.IntentPlan, warnings *[]string) {
	sections := map[string]*model.IntentPreview{}
	for _, name := range []string{"change", "affected", "callers", "callees", "tests", "contracts", "wiki"} {
		sections[name] = &model.IntentPreview{Section: name, Items: []any{}}
	}

	diffRequest := request
	diffRequest.Operation = "nav.diff-context"
	diffRequest.Payload = cloneIntentPayload(request.Payload)
	diffEnv, diffErr := a.diffContext(ctx, diffRequest)
	var diff *DiffContextResult
	if diffErr != nil {
		plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: "change", Operation: "nav.diff-context", Reason: "git diff context was unavailable; affected paths may still be read explicitly"})
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_DIFF_UNAVAILABLE", Section: "change", Reason: sanitizeIntentError(diffErr)})
	} else if results, ok := diffEnv.Items.([]DiffContextResult); ok && len(results) > 0 {
		diff = &results[0]
		sections["change"].Items = intentAnyItems(diff.ChangedSymbols)
		sections["change"].Count = len(diff.ChangedSymbols)
		sections["change"].Truncated = diffEnv.Truncated
		if diffEnv.GenerationID != "" {
			intentAdoptGeneration(plan, "change", diffEnv.GenerationID)
		}
		for _, warning := range diffEnv.Warnings {
			*warnings = appendStringIfMissing(*warnings, warning)
		}
	}

	affectedRequest := request
	affectedRequest.Operation = "nav.affected"
	affectedRequest.Payload = cloneIntentPayload(request.Payload)
	if diff != nil && len(diff.ChangedPaths) > 0 && len(affectedPathsFromPayload(affectedRequest.Payload["paths"])) == 0 {
		affectedRequest.Payload["paths"] = diff.ChangedPaths
	}
	if len(affectedPathsFromPayload(affectedRequest.Payload["paths"])) == 0 {
		affectedRequest.Payload["from_git_diff"] = true
	}
	affectedRequest.Payload["include_tests"] = true
	affectedRequest.Payload["include_docs"] = true
	if plan.GenerationID != "" {
		affectedRequest.Payload["generation"] = plan.GenerationID
	}
	affectedEnv, affectedErr := a.affected(ctx, affectedRequest)
	var affectedItems []AffectedItem
	if affectedErr != nil {
		plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: "affected", Operation: "nav.affected", Reason: "affected-change fell back because the catalog or graph was unavailable"})
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_AFFECTED_UNAVAILABLE", Section: "affected", Reason: sanitizeIntentError(affectedErr)})
	} else {
		generationMatches := intentAdoptGeneration(plan, "affected", affectedEnv.GenerationID)
		if typed, ok := affectedEnv.Items.([]AffectedItem); ok && generationMatches {
			affectedItems = typed
			sections["affected"].Items = intentAnyItems(typed)
			sections["affected"].Count = len(typed)
		}
		if strings.Contains(affectedEnv.Backend, "heuristic") {
			plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: "affected", Operation: "nav.affected", Reason: "graph generation was unavailable; affected output is explicitly heuristic"})
		}
		if affectedEnv.Truncated {
			plan.Truncated = true
		}
		for _, warning := range affectedEnv.Warnings {
			*warnings = appendStringIfMissing(*warnings, warning)
		}
	}

	changedPaths := []string{}
	if diff != nil {
		changedPaths = append(changedPaths, diff.ChangedPaths...)
	}
	if len(changedPaths) == 0 {
		for _, item := range affectedItems {
			if item.TriggerPath != "" {
				changedPaths = append(changedPaths, item.TriggerPath)
			}
		}
	}
	changedPaths = normalizeAffectedPaths(changedPaths)
	if len(sections["change"].Items) == 0 {
		for _, path := range changedPaths {
			sections["change"].Items = append(sections["change"].Items, map[string]any{"path": path, "change_type": "explicit", "reason": "explicit changed path supplied to explain-change"})
		}
		sections["change"].Count = len(sections["change"].Items)
	}
	wiki := buildIntentWikiPlan(changedPaths, registration.Name)
	plan.Wiki = wiki
	sections["contracts"].Items = intentAnyItems(wiki.MustRead)
	sections["contracts"].Count = len(wiki.MustRead)
	sections["wiki"].Items = []any{map[string]any{"must_read": wiki.MustRead, "may_read": wiki.MayRead}}
	sections["wiki"].Count = 1
	if len(changedPaths) == 0 {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_NO_CHANGED_PATHS", Section: "change", Reason: "no working-tree or explicit changed paths were detected"})
	}

	// Use the extracted changed symbols as graph seeds. A missing symbol is an
	// explicit omission; never invent a caller/callee from a file name.
	if diff != nil {
		for _, symbol := range diff.ChangedSymbols[:minInt(len(diff.ChangedSymbols), 3)] {
			for _, graphOperation := range []struct{ op, section string }{{"nav.callers", "callers"}, {"nav.callees", "callees"}} {
				child := request
				child.Operation = graphOperation.op
				child.Payload = cloneIntentPayload(request.Payload)
				child.Payload["selector"] = symbol.Name
				if plan.GenerationID != "" {
					child.Payload["generation"] = plan.GenerationID
				}
				env, err := a.graphQuery(ctx, child)
				if err != nil {
					plan.Fallbacks = append(plan.Fallbacks, model.IntentFallback{Section: graphOperation.section, Operation: graphOperation.op, Reason: "changed symbol graph expansion was unavailable"})
					plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_GRAPH_UNAVAILABLE", Section: graphOperation.section, Reason: sanitizeIntentError(err)})
					continue
				}
				items := intentAnyItems(env.Items)
				if intentAdoptGeneration(plan, graphOperation.section, env.GenerationID) {
					sections[graphOperation.section].Items = append(sections[graphOperation.section].Items, items...)
				}
				if env.GenerationID != "" && plan.GenerationID == "" {
					plan.GenerationID = env.GenerationID
				}
				if env.Truncated {
					sections[graphOperation.section].Truncated = true
					plan.Truncated = true
				}
				for _, warning := range env.Warnings {
					*warnings = appendStringIfMissing(*warnings, warning)
				}
			}
		}
	}
	if len(sections["callers"].Items) == 0 {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_CALLERS_OMITTED", Section: "callers", Reason: "no changed symbol had a resolvable caller neighborhood"})
	}
	if len(sections["callees"].Items) == 0 {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_CALLEES_OMITTED", Section: "callees", Reason: "no changed symbol had a resolvable callee neighborhood"})
	}
	for _, item := range affectedItems {
		if item.Kind == "test" {
			sections["tests"].Items = append(sections["tests"].Items, item)
		}
	}
	sections["tests"].Count = len(sections["tests"].Items)
	if sections["tests"].Count == 0 {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_TESTS_OMITTED", Section: "tests", Reason: "no test evidence or focused test suggestion was found"})
	}

	for _, name := range []string{"change", "affected", "callers", "callees", "tests", "contracts", "wiki"} {
		section := sections[name]
		section.Items, section.Truncated = boundIntentItems(section.Items, request.Context)
		section.Count = len(section.Items)
		if section.Truncated {
			plan.Truncated = true
		}
		plan.Preview = append(plan.Preview, *section)
	}
	plan.Expansions = append(plan.Expansions,
		model.Expansion{Command: intentExplainChangeExpansion(registration.Name, *plan), Reason: "rerun the same local planner with the complete progressive preview and original normalized inputs"},
		model.Expansion{Command: intentDiffExpansion(registration.Name, plan.GenerationID), Reason: "expand changed files and changed symbols from the same git snapshot"},
		model.Expansion{Command: intentAffectedExpansion(registration.Name, plan.GenerationID), Reason: "expand affected code, tests, and documentation with explicit heuristic labels"},
	)
	if len(wiki.MustRead) == 0 {
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_WIKI_EVIDENCE_OMITTED", Section: "wiki", Reason: "wiki relevance could not cite an evidence path"})
	}
}

func buildIntentWikiPlan(changedPaths []string, workspaceName string) model.IntentWikiPlan {
	result := model.IntentWikiPlan{MustRead: []model.IntentWikiRead{}, MayRead: []model.IntentWikiRead{}}
	evidence := append([]string(nil), changedPaths...)
	if len(evidence) > 0 {
		result.MustRead = append(result.MustRead, model.IntentWikiRead{Path: ".docs/wiki/00_gobierno_documental.md", Layer: "00", Reason: "governance context is mandatory before interpreting change impact", EvidencePaths: evidence})
	}
	seenMust, seenMay := map[string]bool{}, map[string]bool{}
	if len(result.MustRead) > 0 {
		seenMust[result.MustRead[0].Path] = true
	}
	for _, path := range changedPaths {
		for _, suggestion := range affectedDocSuggestions(path, workspaceName) {
			read := model.IntentWikiRead{Path: suggestion.Path, Reason: suggestion.Reason, EvidencePaths: []string{path}}
			if strings.HasPrefix(suggestion.Path, ".docs/wiki/09_") || strings.Contains(suggestion.Path, "/09_contratos/") || strings.Contains(filepath.Base(suggestion.Path), "CT-") {
				if !seenMust[read.Path] {
					result.MustRead = append(result.MustRead, read)
					seenMust[read.Path] = true
				}
			} else if !seenMay[read.Path] && !seenMust[read.Path] {
				result.MayRead = append(result.MayRead, read)
				seenMay[read.Path] = true
			}
		}
	}
	if len(result.MayRead) == 0 && len(changedPaths) > 0 {
		result.MayRead = append(result.MayRead, model.IntentWikiRead{Path: ".docs/wiki/06_matriz_pruebas_RF.md", Layer: "06", Reason: "test-matrix review is optional unless the change alters a requirement boundary", EvidencePaths: evidence})
	}
	return result
}

func intentExpansionCommand(workspaceName, operation, selector string) string {
	workspaceName = strings.TrimSpace(workspaceName)
	base := fmt.Sprintf("mi-lsp nav %s", strings.ReplaceAll(operation, "-", " "))
	if selector != "" {
		base += fmt.Sprintf(" %q", selector)
	}
	return fmt.Sprintf("%s --workspace %s --format toon --full", base, workspaceName)
}

func intentPathDiscoveryExpansion(workspaceName, from, to string) string {
	known := strings.TrimSpace(from)
	if known == "" {
		known = strings.TrimSpace(to)
	}
	if known == "" {
		known = "path endpoint symbol"
	}
	return fmt.Sprintf("mi-lsp nav search %q --workspace %s --format toon --include-content", known, strings.TrimSpace(workspaceName))
}

func intentExpansionCommandForPlan(workspaceName string, plan model.IntentPlan) string {
	workspaceName = strings.TrimSpace(workspaceName)
	var base string
	switch plan.Operation {
	case "neighborhood":
		base = fmt.Sprintf("mi-lsp nav neighbors %q", plan.Arguments["selector"])
	case "path-between":
		base = fmt.Sprintf("mi-lsp nav path %q %q", plan.Arguments["from"], plan.Arguments["to"])
	case "explain-edge":
		base = fmt.Sprintf("mi-lsp nav explain %q", plan.Arguments["edge"])
	case "callers", "callees":
		base = fmt.Sprintf("mi-lsp nav %s %q", plan.Operation, plan.Arguments["selector"])
	default:
		return intentExpansionCommand(workspaceName, plan.Operation, plan.Arguments["selector"])
	}
	command := fmt.Sprintf("%s --workspace %s --format toon --full", base, workspaceName)
	if strings.TrimSpace(plan.GenerationID) != "" {
		command += fmt.Sprintf(" --generation %s", plan.GenerationID)
	}
	return command
}

func intentExplainChangeExpansion(workspaceName string, plan model.IntentPlan) string {
	command := fmt.Sprintf("mi-lsp nav explain-change --workspace %s --format toon --full", strings.TrimSpace(workspaceName))
	for _, path := range strings.Split(plan.Arguments["paths"], ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			command += " --path " + strconv.Quote(path)
		}
	}
	if ref := strings.TrimSpace(plan.Arguments["ref"]); ref != "" {
		command += " --ref " + strconv.Quote(ref)
	}
	return command
}

func intentDiffExpansion(workspaceName, generation string) string {
	command := fmt.Sprintf("mi-lsp nav diff-context --workspace %s --format toon", workspaceName)
	if strings.TrimSpace(generation) != "" {
		command += fmt.Sprintf(" --generation %s", generation)
	}
	return command
}

func intentAffectedExpansion(workspaceName, generation string) string {
	command := fmt.Sprintf("mi-lsp nav affected --from-git-diff --include-tests --include-docs --workspace %s --format toon --full", workspaceName)
	if strings.TrimSpace(generation) != "" {
		command += fmt.Sprintf(" --generation %s", generation)
	}
	return command
}

func intentAnyItems(value any) []any {
	if value == nil {
		return []any{}
	}
	if items, ok := value.([]any); ok {
		return append([]any(nil), items...)
	}
	if items, ok := value.([]model.GraphQueryItem); ok {
		out := make([]any, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	}
	if items, ok := value.([]DiffSymbol); ok {
		out := make([]any, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	}
	if items, ok := value.([]AffectedItem); ok {
		out := make([]any, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	}
	if items, ok := value.([]model.IntentWikiRead); ok {
		out := make([]any, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	}
	b, err := json.Marshal(value)
	if err != nil {
		return []any{value}
	}
	var decoded any
	if json.Unmarshal(b, &decoded) != nil {
		return []any{value}
	}
	if list, ok := decoded.([]any); ok {
		return list
	}
	return []any{decoded}
}

func boundIntentItems(items []any, opts model.QueryOptions) ([]any, bool) {
	limit := intentPreviewLimit(opts)
	if len(items) <= limit {
		return items, false
	}
	return append([]any(nil), items[:limit]...), true
}

func intentPreviewLimit(opts model.QueryOptions) int {
	limit := 5
	if opts.Full {
		limit = 20
	}
	if opts.MaxItems > 0 && opts.MaxItems < limit {
		limit = opts.MaxItems
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}

func candidateSelectors(candidates []model.IntentCandidate) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, candidate := range candidates {
		value := candidate.Selector
		if value == "" {
			value = candidate.QualifiedName
		}
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	sort.Strings(result)
	return result
}

func cloneIntentPayload(payload map[string]any) map[string]any {
	clone := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func sanitizeIntentError(err error) string {
	if err == nil {
		return ""
	}
	if graphErr, ok := err.(*model.GraphQueryError); ok {
		return graphErr.Code + ": " + graphErr.Message
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		message = message[:240] + "..."
	}
	return message
}

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/nav"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func (a *App) wikiSearch(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.search"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	// Check if --all-workspaces mode is requested
	allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
	if allWorkspaces {
		return a.wikiSearchAllWorkspaces(ctx, request)
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	queryText := strings.TrimSpace(firstNonEmpty(
		stringPayload(request.Payload, "query"),
		stringPayload(request.Payload, "pattern"),
		stringPayload(request.Payload, "task"),
	))
	if queryText == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	query := loadDocQueryContext(ctx, registration, queryText)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	warnings := append([]string{}, query.profileWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", query.profileSource))
	if len(query.docs) == 0 {
		hint := fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before wiki search", registration.Name)
		warnings = appendStringIfMissing(warnings, hint)
		env := model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "wiki.search",
			Items:     []map[string]any{},
			Warnings:  warnings,
			Hint:      hint,
		}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	layerFilter, unknownLayers := parseWikiLayerFilter(stringPayload(request.Payload, "layer"))
	for _, layer := range unknownLayers {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("unknown wiki layer %q ignored; valid layers: RS, RF, FL, TP, CT, TECH, DB", layer))
	}

	topFromPayload := intFromAny(request.Payload["top"], 0)
	top := topFromPayload
	if top <= 0 {
		if request.Context.Full {
			top = len(query.ranked)
			if exactDocs, err := store.FindDocRecordsBySourceID(ctx, query.db, queryText); err == nil && len(exactDocs) > top {
				top = len(exactDocs)
			}
		} else {
			top = request.Context.MaxItems
		}
	}
	if top <= 0 {
		top = 10
	}
	offset := intFromAny(request.Payload["offset"], request.Context.Offset)
	includeContent, _ := request.Payload["include_content"].(bool)

	candidates := make([]model.WikiSearchResult, 0, min(top, len(query.ranked)))
	seenPaths := map[string]struct{}{}
	exactDocPaths := map[string]struct{}{}
	if exactDocs, err := store.FindDocRecordsBySourceID(ctx, query.db, queryText); err == nil {
		for _, doc := range exactDocs {
			layer := wikiLayerForDoc(doc)
			if len(layerFilter) > 0 {
				if _, ok := layerFilter[layer]; !ok {
					continue
				}
			}
			item := wikiSearchResult(registration.Name, registration.Root, queryText, scoredDoc{record: doc, score: 1000, reason: []string{"source_id_exact"}}, layer, includeContent, request.Context.MaxChars)
			candidates = append(candidates, item)
			seenPaths[doc.Path] = struct{}{}
			exactDocPaths[doc.Path] = struct{}{}
			if len(candidates) >= top {
				break
			}
		}
	}
	skipped := 0
	for _, candidate := range query.ranked {
		if _, seen := seenPaths[candidate.record.Path]; seen {
			continue
		}
		layer := wikiLayerForDoc(candidate.record)
		if len(layerFilter) > 0 {
			if _, ok := layerFilter[layer]; !ok {
				continue
			}
		}
		if skipped < offset {
			skipped++
			continue
		}
		item := wikiSearchResult(registration.Name, registration.Root, queryText, candidate, layer, includeContent, request.Context.MaxChars)
		candidates = append(candidates, item)
		if len(candidates) >= top {
			break
		}
	}
	totalMatches := countWikiSearchMatches(query.ranked, layerFilter, exactDocPaths)
	nextHint := wikiExpansionHint("nav.wiki.search", registration.Name, queryText, len(candidates), totalMatches)
	sourceIdentity, hasSourceIdentity, _ := sourceIdentityForQuery(ctx, query.db, queryText)
	for i := range candidates {
		status := wikiLookupStatusForDoc(registration.Name, queryText, model.DocRecord{
			Path:   candidates[i].Path,
			Title:  candidates[i].Title,
			DocID:  candidates[i].DocID,
			Layer:  candidates[i].Layer,
			Family: candidates[i].Family,
		}, candidates[i].Why, totalMatches, len(candidates), nextHint)
		if hasSourceIdentity && candidates[i].Path == sourceIdentity.path {
			applySourceIdentity(&status, sourceIdentity)
		}
		candidates[i].LookupStatus = &status
	}

	hint := ""
	if len(candidates) == 0 {
		if len(layerFilter) > 0 {
			hint = "0 wiki matches for selected layers; broaden --layer or try nav wiki route"
		} else {
			hint = "0 wiki matches; try a doc id like RS-*, RF-*, FL-*, CT-*, TECH-*, DB-*, AE-* or run nav wiki route"
		}
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.search",
		Items:     candidates,
		Warnings:  warnings,
		Hint:      hint,
		Stats:     model.Stats{Files: len(candidates)},
	}
	if nextHint != "" {
		env.NextHint = &nextHint
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

type wikiSearchEvidence struct {
	line      int
	startLine int
	endLine   int
	text      string
}

func wikiSearchLineEvidence(root string, relPath string, queryText string) (wikiSearchEvidence, bool) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || strings.Contains(relPath, "..") {
		return wikiSearchEvidence{}, false
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		return wikiSearchEvidence{}, false
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return wikiSearchEvidence{}, false
	}
	tokens := docgraph.QuestionTokens(queryText)
	queryLower := strings.ToLower(strings.TrimSpace(queryText))
	lineNo := 1
	for i, line := range lines {
		lineLower := strings.ToLower(line)
		if queryLower != "" && strings.Contains(lineLower, queryLower) {
			lineNo = i + 1
			break
		}
		if len(tokens) > 0 && containsAnyToken(lineLower, tokens) {
			lineNo = i + 1
			break
		}
	}
	text := strings.TrimSpace(lines[lineNo-1])
	if text == "" {
		text = strings.TrimSpace(lines[0])
		lineNo = 1
	}
	if len(text) > 240 {
		text = text[:240]
	}
	return wikiSearchEvidence{line: lineNo, startLine: lineNo, endLine: lineNo, text: text}, true
}

func countWikiSearchMatches(ranked []scoredDoc, layerFilter map[string]struct{}, exactDocPaths map[string]struct{}) int {
	total := len(exactDocPaths)
	for _, candidate := range ranked {
		if _, seen := exactDocPaths[candidate.record.Path]; seen {
			continue
		}
		layer := wikiLayerForDoc(candidate.record)
		if len(layerFilter) > 0 {
			if _, ok := layerFilter[layer]; !ok {
				continue
			}
		}
		total++
	}
	return total
}

func wikiSearchResult(workspaceName string, root string, queryText string, candidate scoredDoc, layer string, includeContent bool, maxChars int) model.WikiSearchResult {
	doc := candidate.record
	item := model.WikiSearchResult{
		DocID:       doc.DocID,
		Path:        doc.Path,
		Title:       doc.Title,
		Layer:       layer,
		Family:      doc.Family,
		Stage:       wikiStageForDoc(doc),
		Score:       candidate.score,
		Why:         append([]string{}, candidate.reason...),
		Snippet:     doc.Snippet,
		NextQueries: buildWikiSearchNextQueries(workspaceName, queryText, doc, layer),
	}
	if includeContent {
		item.Content = readWikiSearchContent(root, doc.Path, maxChars)
	}
	if evidence, ok := wikiSearchLineEvidence(root, doc.Path, queryText); ok {
		item.Line = evidence.line
		item.StartLine = evidence.startLine
		item.EndLine = evidence.endLine
		item.Evidence = evidence.text
	}
	return item
}

func parseWikiLayerFilter(raw string) (map[string]struct{}, []string) {
	filter := map[string]struct{}{}
	unknown := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		layer := strings.ToUpper(strings.TrimSpace(part))
		if layer == "" {
			continue
		}
		switch layer {
		case "RS", "RF", "FL", "TP", "CT", "TECH", "DB", "AE":
			filter[layer] = struct{}{}
		default:
			unknown = append(unknown, layer)
		}
	}
	if len(filter) == 0 {
		return nil, unknown
	}
	return filter, unknown
}

func wikiLayerForDoc(doc model.DocRecord) string {
	docID := strings.ToUpper(strings.TrimSpace(doc.DocID))
	path := filepath.ToSlash(doc.Path)
	switch {
	case strings.HasPrefix(docID, "RS-") || doc.Layer == "RS" || strings.Contains(path, "/02_resultados/") || strings.HasSuffix(path, "/02_resultados_soluciones_usuario.md"):
		return "RS"
	case strings.HasPrefix(docID, "FL-") || doc.Layer == "03" || strings.Contains(path, "/03_FL/"):
		return "FL"
	case strings.HasPrefix(docID, "RF-") || doc.Layer == "04" || strings.Contains(path, "/04_RF/"):
		return "RF"
	case strings.HasPrefix(docID, "TP-") || doc.Layer == "06" || strings.Contains(path, "/06_pruebas/"):
		return "TP"
	case strings.HasPrefix(docID, "CT-") || doc.Layer == "09" || strings.Contains(path, "/09_contratos/"):
		return "CT"
	case strings.HasPrefix(docID, "TECH-") || doc.Layer == "07" || strings.Contains(path, "/07_tech/"):
		return "TECH"
	case strings.HasPrefix(docID, "DB-") || doc.Layer == "08" || strings.Contains(path, "/08_db/"):
		return "DB"
	case strings.HasPrefix(docID, "AE-") || doc.Layer == "AE" || strings.Contains(path, "/ae/"):
		return "AE"
	default:
		return strings.ToUpper(strings.TrimSpace(doc.Layer))
	}
}

func wikiStageForDoc(doc model.DocRecord) string {
	path := filepath.ToSlash(doc.Path)
	switch {
	case strings.Contains(path, "/02_resultados/") || strings.HasSuffix(path, "/02_resultados_soluciones_usuario.md") || doc.Layer == "RS":
		return "outcome"
	case strings.Contains(path, "/03_FL/") || doc.Layer == "03":
		return "flow"
	case strings.Contains(path, "/04_RF/") || doc.Layer == "04":
		return "requirements"
	case strings.Contains(path, "/06_pruebas/") || doc.Layer == "06":
		return "tests"
	case strings.Contains(path, "/07_tech/"):
		return "technical_detail"
	case doc.Layer == "07":
		return "technical_baseline"
	case strings.Contains(path, "/08_db/") || doc.Layer == "08":
		return "physical_data"
	case strings.Contains(path, "/09_contratos/") || doc.Layer == "09":
		return "contracts"
	case strings.Contains(path, "/ae/") || doc.Layer == "AE":
		return "technical_detail"
	case doc.Layer == "01":
		return "scope"
	case doc.Layer == "02":
		return "architecture"
	default:
		return ""
	}
}

func buildWikiSearchNextQueries(workspaceName string, queryText string, doc model.DocRecord, layer string) []string {
	queries := make([]string, 0, 4)
	queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki pack %q --workspace %s --doc %s --format toon", queryText, workspaceName, doc.Path))
	if (layer == "RS" && strings.HasPrefix(strings.ToUpper(doc.DocID), "RS-")) ||
		(layer == "RF" && strings.HasPrefix(strings.ToUpper(doc.DocID), "RF-")) {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki trace %s --workspace %s --format toon", doc.DocID, workspaceName))
	}
	if doc.DocID != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki search %q --workspace %s --format toon", doc.DocID, workspaceName))
	}
	if doc.Path != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav multi-read %s:1-120 --workspace %s --format toon", doc.Path, workspaceName))
	}
	queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav ask %q --workspace %s --format toon", queryText, workspaceName))
	if len(queries) > 4 {
		return queries[:4]
	}
	return queries
}

func readWikiSearchContent(root string, relPath string, maxChars int) string {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || strings.Contains(relPath, "..") {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		return ""
	}
	limit := 4096
	if maxChars > 0 && maxChars < limit {
		limit = maxChars
	}
	text := string(content)
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

// wikiSearchAllWorkspaces handles --all-workspaces fan-out using FanOutWiki.
func (a *App) wikiSearchAllWorkspaces(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	queryText := strings.TrimSpace(firstNonEmpty(
		stringPayload(request.Payload, "query"),
		stringPayload(request.Payload, "pattern"),
		stringPayload(request.Payload, "task"),
	))
	if queryText == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	// Extract search parameters from payload
	layerFilterRaw := stringPayload(request.Payload, "layer")
	layerFilter, _ := parseWikiLayerFilter(layerFilterRaw)
	topGlobalFromPayload := intFromAny(request.Payload["top_global"], 50)
	topGlobal := topGlobalFromPayload
	if topGlobal <= 0 {
		topGlobal = 50
	}
	searchTop := intFromAny(request.Payload["top"], 0)
	searchOffset := intFromAny(request.Payload["offset"], 0)
	includeContent, _ := request.Payload["include_content"].(bool)

	// Fan-out across workspaces
	fanOutOpts := nav.WikiFanOutOptions{
		Timeout:  0, // Use default (30s)
		Parallel: 0, // Use default (4)
	}

	fanOutResult, err := nav.FanOutWiki(ctx, fanOutOpts, func(subCtx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		// Query the doc index for this workspace
		query := loadDocQueryContext(subCtx, ws, queryText)
		defer query.Close()
		if query.dbErr != nil {
			return nil, map[string]any{}, query.dbErr
		}

		if len(query.docs) == 0 {
			return []any{}, map[string]any{}, nil
		}

		// Collect candidates for this workspace (reuse single-workspace logic)
		candidates := make([]model.WikiSearchResult, 0)
		seenPaths := map[string]struct{}{}
		exactDocPaths := map[string]struct{}{}

		// Exact matches
		if exactDocs, err := store.FindDocRecordsBySourceID(subCtx, query.db, queryText); err == nil {
			for _, doc := range exactDocs {
				layer := wikiLayerForDoc(doc)
				if len(layerFilter) > 0 {
					if _, ok := layerFilter[layer]; !ok {
						continue
					}
				}
				item := wikiSearchResult(ws.Name, ws.Root, queryText, scoredDoc{record: doc, score: 1000, reason: []string{"source_id_exact"}}, layer, includeContent, request.Context.MaxChars)
				item.Workspace = ws.Name // Add workspace label
				item.Host = ""
				candidates = append(candidates, item)
				seenPaths[doc.Path] = struct{}{}
				exactDocPaths[doc.Path] = struct{}{}
				if len(candidates) >= searchTop || searchTop <= 0 && len(candidates) >= 10 {
					break
				}
			}
		}

		// Ranked matches
		skipped := 0
		for _, candidate := range query.ranked {
			if _, seen := seenPaths[candidate.record.Path]; seen {
				continue
			}
			layer := wikiLayerForDoc(candidate.record)
			if len(layerFilter) > 0 {
				if _, ok := layerFilter[layer]; !ok {
					continue
				}
			}
			if skipped < searchOffset {
				skipped++
				continue
			}
			item := wikiSearchResult(ws.Name, ws.Root, queryText, candidate, layer, includeContent, request.Context.MaxChars)
			item.Workspace = ws.Name // Add workspace label
			item.Host = ""
			candidates = append(candidates, item)
			if (searchTop > 0 && len(candidates) >= searchTop) || (searchTop <= 0 && len(candidates) >= 10) {
				break
			}
		}

		// Convert candidates to []any
		itemsAny := make([]any, len(candidates))
		for i := range candidates {
			itemsAny[i] = candidates[i]
		}

		stats := map[string]any{}
		return itemsAny, stats, nil
	})

	if err != nil {
		return model.Envelope{}, err
	}

	// Aggregate results from all workspaces
	var allResults []model.WikiSearchResult
	for _, wsResult := range fanOutResult.Items {
		if wsResult.Err != nil {
			// Skip workspaces with errors but continue with others
			continue
		}
		for _, item := range wsResult.Items {
			if wsItem, ok := item.(model.WikiSearchResult); ok {
				allResults = append(allResults, wsItem)
			}
		}
	}

	// Sort by (score DESC, workspace ASC, doc_id ASC)
	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].Score != allResults[j].Score {
			return allResults[i].Score > allResults[j].Score
		}
		if allResults[i].Workspace != allResults[j].Workspace {
			return allResults[i].Workspace < allResults[j].Workspace
		}
		return allResults[i].DocID < allResults[j].DocID
	})

	// Truncate to topGlobal
	if len(allResults) > topGlobal {
		allResults = allResults[:topGlobal]
	}

	// Build envelope with federated stats
	warnings := []string{}
	failureStrs := []string{}
	if len(fanOutResult.WorkspacesFailed) > 0 {
		for _, f := range fanOutResult.WorkspacesFailed {
			failureStrs = append(failureStrs, fmt.Sprintf("%s: %s", f.Alias, f.Reason))
		}
		warnings = append(warnings, fmt.Sprintf("%d workspace(s) failed during query", len(fanOutResult.WorkspacesFailed)))
	}

	stats := model.Stats{
		Files:                 len(allResults),
		WorkspacesQueried:     fanOutResult.WorkspacesQueried,
		WorkspacesFailed:      failureStrs,
		TruncatedPerWorkspace: fanOutResult.TruncatedPerWS,
	}

	hint := ""
	if len(allResults) == 0 {
		if len(layerFilter) > 0 {
			hint = "0 wiki matches across workspaces for selected layers; broaden --layer or try nav wiki route"
		} else {
			hint = "0 wiki matches across workspaces; try a doc id like RS-*, RF-*, FL-*, CT-*, TECH-*, DB-*, AE-*"
		}
	}

	// Convert results to []any for envelope
	itemsAny := make([]any, len(allResults))
	for i := range allResults {
		itemsAny[i] = allResults[i]
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: "all",
		Backend:   "wiki.search",
		Items:     itemsAny,
		Warnings:  warnings,
		Hint:      hint,
		Stats:     stats,
	}
	return env, nil
}

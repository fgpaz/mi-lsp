package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type scoredDoc struct {
	record model.DocRecord
	score  int
	reason []string
}

func (a *App) ask(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
	if allWorkspaces {
		return a.askAllWorkspaces(ctx, request)
	}
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.ask"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)
	question, _ := request.Payload["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return model.Envelope{}, fmt.Errorf("question is required")
	}

	query := loadDocQueryContext(ctx, registration, question)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}
	profile := query.profile
	profileSource := query.profileSource
	profileWarnings := query.profileWarnings
	docs := query.docs
	if len(docs) == 0 {
		items, searchErr := searchPattern(ctx, registration.Root, project, question, false, askLimit(request.Context.MaxItems, 5, 5))
		if searchErr != nil {
			return model.Envelope{}, searchErr
		}
		// Use Tier 1 canonical routing instead of hardcoded README.md
		// when governance/wiki exists (RF-QRY-015).
		canonical, tier1Why := docgraph.Tier1CanonicalRoute(question, profile, registration.Root)
		anchor := canonical.AnchorDoc
		fallback := model.AskResult{
			Question: question,
			Summary:  "No encontre documentacion indexada; resolucion canonica tier1 + evidencia textual del workspace como fallback.",
			PrimaryDoc: model.AskDocEvidence{
				Path:   anchor.Path,
				Title:  anchor.Title,
				DocID:  anchor.DocID,
				Family: anchor.Family,
				Layer:  anchor.Layer,
			},
			Why:          append([]string{"doc_index_empty", "fallback=tier1_canonical"}, tier1Why...),
			CodeEvidence: searchItemsToEvidence(items),
			NextQueries:  []string{fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s", question, registration.Name)},
		}
		previewTrimmed := askResultWouldTrimForAXIPreview(fallback)
		if isAXIPreview(request.Context) {
			fallback = trimAskResultForAXIPreview(fallback)
		}
		warnings := append([]string{}, profileWarnings...)
		warnings = append(warnings, "documentation index is empty; using code fallback")
		warnings = append(warnings, fmt.Sprintf("read_model=%s", profileSource))
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "ask", Items: []model.AskResult{fallback}, Warnings: warnings}
		env.Coach = buildAskCoach(registration.Name, project, question, fallback, warnings, request.Context, previewTrimmed)
		env = applyWikiRepoCompatHint(env, request, "nav.ask", registration.Name, question)
		env = attachMemoryPointer(env, memory)
		env.Continuation = buildAskContinuation(question, project, fallback, warnings, request.Context, previewTrimmed, memory)
		env = applyAXIPreviewHints(env, request.Context, "preview mode: rerun with --full for more evidence")
		return applyCoachPolicy(env, request.Context), nil
	}

	routeResult := query.canonicalRoute(request.Context, false)
	ranked := query.ranked
	primary, ok := query.primaryDoc(routeResult)
	if !ok {
		warnings := append([]string{}, profileWarnings...)
		warnings = append(warnings, "no wiki match found")
		warnings = append(warnings, fmt.Sprintf("read_model=%s", profileSource))
		result := model.AskResult{Question: question, Summary: "No encontre una pista fuerte en la wiki para esta pregunta.", Why: []string{"no_doc_match"}}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "ask", Items: []model.AskResult{result}, Warnings: warnings}
		env.Coach = buildAskCoach(registration.Name, project, question, result, warnings, request.Context, false)
		env = applyWikiRepoCompatHint(env, request, "nav.ask", registration.Name, question)
		env = attachMemoryPointer(env, memory)
		env.Continuation = buildAskContinuation(question, project, result, warnings, request.Context, false, memory)
		return applyCoachPolicy(env, request.Context), nil
	}
	docEvidence, reasons, docWarnings := buildDocEvidence(ctx, query.db, primary, ranked, query.docByPath, askDocEvidenceLimit(request.Context))
	codeEvidence, codeWarnings := a.buildAskCodeEvidence(ctx, query.db, registration, project, primary.record, docEvidence, question, askCodeEvidenceLimit(request.Context))
	warnings := append([]string{}, profileWarnings...)
	warnings = append(warnings, docWarnings...)
	warnings = append(warnings, codeWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", profileSource))

	result := model.AskResult{
		Question:     question,
		Summary:      buildAskSummary(primary.record, codeEvidence),
		PrimaryDoc:   docRecordToEvidence(primary.record),
		DocEvidence:  docEvidence,
		CodeEvidence: codeEvidence,
		Why:          append(append([]string{}, routeResult.Why...), append(primary.reason, reasons...)...),
		NextQueries:  buildAskNextQueries(registration.Name, project, primary.record, question, codeEvidence, request.Context),
	}
	previewTrimmed := askResultWouldTrimForAXIPreview(result)
	if isAXIPreview(request.Context) {
		result = trimAskResultForAXIPreview(result)
	}
	env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "ask", Items: []model.AskResult{result}, Warnings: warnings, Stats: model.Stats{Files: len(codeEvidence)}}
	env.Coach = buildAskCoach(registration.Name, project, question, result, warnings, request.Context, previewTrimmed)
	env = applyWikiRepoCompatHint(env, request, "nav.ask", registration.Name, question)
	env = attachMemoryPointer(env, memory)
	env.Continuation = buildAskContinuation(question, project, result, warnings, request.Context, previewTrimmed, memory)
	env = applyAXIPreviewHints(env, request.Context, "preview mode: rerun with --full for more evidence")
	return applyCoachPolicy(env, request.Context), nil
}

func buildDocEvidence(ctx context.Context, db *sql.DB, primary scoredDoc, ranked []scoredDoc, docByPath map[string]model.DocRecord, maxDocs int) ([]model.AskDocEvidence, []string, []string) {
	if maxDocs <= 0 {
		maxDocs = 4
	}
	seen := map[string]struct{}{primary.record.Path: {}}
	items := []model.AskDocEvidence{docRecordToEvidence(primary.record)}
	reasons := []string{"primary_doc=" + primary.record.Path}
	warnings := []string{}
	queue := []string{primary.record.Path}
	visited := map[string]struct{}{}

	addDoc := func(doc model.DocRecord, reason string) bool {
		if len(items) >= maxDocs {
			return false
		}
		if doc.Path == "" || doc.Path == primary.record.Path {
			return false
		}
		if _, ok := seen[doc.Path]; ok {
			return false
		}
		seen[doc.Path] = struct{}{}
		items = append(items, docRecordToEvidence(doc))
		reasons = append(reasons, reason)
		queue = append(queue, doc.Path)
		return true
	}

	for len(queue) > 0 && len(items) < maxDocs {
		source := queue[0]
		queue = queue[1:]
		if _, ok := visited[source]; ok {
			continue
		}
		visited[source] = struct{}{}
		edges, err := store.DocEdgesFrom(ctx, db, source)
		if err != nil {
			warnings = appendStringIfMissing(warnings, fmt.Sprintf("doc edges unavailable for %s", source))
			continue
		}
		for _, edge := range edges {
			if edge.ToPath == "" || edge.ToPath == source {
				continue
			}
			doc, ok := docByPath[edge.ToPath]
			if !ok {
				continue
			}
			reason := fmt.Sprintf("linked_doc=%s", doc.Path)
			if edge.Kind != "" {
				reason = fmt.Sprintf("linked_doc[%s]=%s", edge.Kind, doc.Path)
			}
			if addDoc(doc, reason) && len(items) >= maxDocs {
				break
			}
		}
	}

	for _, candidate := range ranked[1:] {
		if len(items) >= maxDocs {
			break
		}
		addDoc(candidate.record, "supporting_doc="+candidate.record.Path)
	}
	return items, reasons, warnings
}

func (a *App) buildAskCodeEvidence(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, project model.ProjectFile, primary model.DocRecord, docs []model.AskDocEvidence, question string, limit int) ([]model.AskCodeEvidence, []string) {
	_ = project
	limit = askLimit(limit, 6, 6)
	warnings := make([]string, 0)
	items := make([]model.AskCodeEvidence, 0, limit)
	seen := map[string]struct{}{}
	addEvidence := func(item model.AskCodeEvidence) {
		key := fmt.Sprintf("%s|%s|%s|%d", item.Type, item.File, item.Name, item.Line)
		if _, ok := seen[key]; ok || len(items) >= limit {
			return
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}

	paths := []string{}
	symbols := []string{}
	for _, doc := range docs {
		mentions, err := store.DocMentionsForPath(ctx, db, doc.Path)
		if err != nil {
			warnings = appendStringIfMissing(warnings, fmt.Sprintf("doc mentions unavailable for %s", doc.Path))
			continue
		}
		for _, mention := range mentions {
			switch mention.MentionType {
			case "file_path":
				paths = append(paths, mention.MentionValue)
			case "symbol":
				symbols = append(symbols, mention.MentionValue)
			}
		}
	}

	for _, path := range paths {
		symbolsByFile, err := store.SymbolsByFile(ctx, db, path, 3, 0)
		if err == nil && len(symbolsByFile) > 0 {
			for _, symbol := range symbolsByFile {
				addEvidence(model.AskCodeEvidence{Type: "symbol", File: symbol.FilePath, Line: symbol.StartLine, Name: symbol.Name, Kind: symbol.Kind, Snippet: symbol.Signature})
			}
			continue
		}
		absPath := filepath.Join(registration.Root, filepath.FromSlash(path))
		snippet, _, err := readFileLineRange(absPath, 1, 20)
		if err == nil {
			addEvidence(model.AskCodeEvidence{Type: "file", File: path, Line: 1, Snippet: snippet})
		}
	}

	for _, symbolName := range symbols {
		found, err := store.FindSymbols(ctx, db, symbolName, "", true, 3, 0)
		if err != nil {
			continue
		}
		for _, symbol := range found {
			addEvidence(model.AskCodeEvidence{Type: "symbol", File: symbol.FilePath, Line: symbol.StartLine, Name: symbol.Name, Kind: symbol.Kind, Snippet: symbol.Signature})
		}
	}

	if len(items) == 0 {
		keywords := docgraph.QuestionTokens(primary.Title + " " + question)
		for _, keyword := range keywords {
			if len(items) >= limit {
				break
			}
			matches, err := searchPattern(ctx, registration.Root, project, keyword, false, 3)
			if err != nil {
				continue
			}
			for _, match := range matches {
				file, _ := match["file"].(string)
				line := intFromAny(match["line"], 0)
				text, _ := match["text"].(string)
				addEvidence(model.AskCodeEvidence{Type: "text", File: file, Line: line, Snippet: text})
			}
		}
		if len(items) > 0 {
			warnings = appendStringIfMissing(warnings, "code evidence came from text fallback")
		}
	}

	return items, warnings
}

func buildAskSummary(primary model.DocRecord, codeEvidence []model.AskCodeEvidence) string {
	label := primary.Title
	if primary.DocID != "" {
		label = primary.DocID + " - " + primary.Title
	}
	if len(codeEvidence) == 0 {
		return fmt.Sprintf("La mejor pista esta en %s. No encontre todavia evidencia de codigo fuerte, asi que la respuesta queda guiada por documentacion.", label)
	}
	first := codeEvidence[0]
	if first.Name != "" {
		return fmt.Sprintf("La mejor pista esta en %s y el codigo mas relacionado aparece alrededor de %s en %s.", label, first.Name, first.File)
	}
	return fmt.Sprintf("La mejor pista esta en %s y la evidencia de codigo mas cercana esta en %s.", label, first.File)
}

func docRecordToEvidence(doc model.DocRecord) model.AskDocEvidence {
	return model.AskDocEvidence{Path: doc.Path, Title: doc.Title, DocID: doc.DocID, Layer: doc.Layer, Family: doc.Family, Snippet: doc.Snippet}
}

func searchItemsToEvidence(items []map[string]any) []model.AskCodeEvidence {
	result := make([]model.AskCodeEvidence, 0, len(items))
	for _, item := range items {
		result = append(result, model.AskCodeEvidence{Type: "text", File: fmt.Sprintf("%v", item["file"]), Line: intFromAny(item["line"], 0), Snippet: fmt.Sprintf("%v", item["text"])})
	}
	return result
}

func buildAskNextQueries(workspaceName string, project model.ProjectFile, primary model.DocRecord, question string, codeEvidence []model.AskCodeEvidence, opts model.QueryOptions) []string {
	queries := []string{}
	repoFlag := ""
	if repoName := askRepoScope(project, codeEvidence); repoName != "" && project.Project.Kind == model.WorkspaceKindContainer {
		repoFlag = " --repo " + repoName
	}
	if primary.Family == "generic" || primary.Layer == "generic" || askCodeEvidenceIsTextOnly(codeEvidence) {
		if searchPhrase := askSearchPhrase(question); searchPhrase != "" {
			queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s%s", searchPhrase, workspaceName, repoFlag))
		}
	}
	if primary.DocID != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s%s", primary.DocID, workspaceName, repoFlag))
	}
	if len(codeEvidence) > 0 {
		first := codeEvidence[0]
		if first.File != "" && first.Line > 0 {
			queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav context %s %d --workspace %s%s", first.File, first.Line, workspaceName, repoFlag))
		}
		if first.Name != "" {
			queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav related %s --workspace %s%s", first.Name, workspaceName, repoFlag))
		}
	}
	if len(queries) == 0 {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s%s", primary.Title, workspaceName, repoFlag))
	}
	return queries
}

func askCodeEvidenceIsTextOnly(items []model.AskCodeEvidence) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.Type != "text" {
			return false
		}
	}
	return true
}

func askSearchPhrase(question string) string {
	rawTokens := strings.FieldsFunc(question, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	})
	for _, token := range rawTokens {
		if len(token) < 2 {
			continue
		}
		hasLetter := false
		for _, r := range token {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
		if hasLetter && token == strings.ToUpper(token) {
			return token
		}
	}

	generic := map[string]struct{}{
		"how": {}, "where": {}, "what": {}, "when": {}, "which": {},
		"como": {}, "donde": {}, "que": {}, "cual": {},
		"mode": {}, "implemented": {}, "implementation": {}, "works": {}, "work": {},
		"details": {}, "detail": {}, "handle": {}, "handled": {}, "using": {},
	}
	best := ""
	for _, token := range rawTokens {
		token = strings.ToLower(token)
		if len(token) < 3 {
			continue
		}
		if _, ok := generic[token]; ok {
			continue
		}
		if len(token) > len(best) {
			best = token
		}
	}
	return best
}

func appendUniqueQuery(queries []string, query string) []string {
	for _, existing := range queries {
		if existing == query {
			return queries
		}
	}
	return append(queries, query)
}

func askRepoScope(project model.ProjectFile, codeEvidence []model.AskCodeEvidence) string {
	for _, evidence := range codeEvidence {
		if repoName := askRepoForPath(project, evidence.File); repoName != "" {
			return repoName
		}
	}
	return ""
}

func askRepoForPath(project model.ProjectFile, file string) string {
	normalized := strings.Trim(strings.TrimSpace(filepath.ToSlash(file)), "/")
	if normalized == "" {
		return ""
	}
	var fallback string
	for _, repo := range project.Repos {
		root := strings.Trim(strings.TrimSpace(filepath.ToSlash(repo.Root)), "/")
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
	if project.Project.Kind == model.WorkspaceKindSingle {
		return fallback
	}
	return ""
}

func askLimit(value int, fallback int, ceiling int) int {
	if value <= 0 {
		value = fallback
	}
	if ceiling > 0 && value > ceiling {
		return ceiling
	}
	return value
}

func askDocEvidenceLimit(opts model.QueryOptions) int {
	if isAXIPreview(opts) {
		return 2
	}
	return 4
}

func askCodeEvidenceLimit(opts model.QueryOptions) int {
	if isAXIPreview(opts) {
		return 1
	}
	return askLimit(opts.MaxItems, 6, 6)
}

func (a *App) askAllWorkspaces(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return model.Envelope{}, fmt.Errorf("failed to list workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		return model.Envelope{Ok: true, Backend: "ask", Items: []model.AskResult{}, Warnings: []string{"no workspaces registered"}}, nil
	}

	question, _ := request.Payload["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return model.Envelope{}, fmt.Errorf("question is required")
	}

	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = 1
	}

	type askResult struct {
		ws       model.WorkspaceRegistration
		envelope model.Envelope
		err      error
	}

	results := make(chan askResult, len(workspaces))
	var wg sync.WaitGroup
	const maxConcurrent = 4
	semaphore := make(chan struct{}, maxConcurrent)

	for _, ws := range workspaces {
		wg.Add(1)
		go func(wsReg model.WorkspaceRegistration) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			subRequest := model.CommandRequest{
				Context: request.Context,
				Payload: clonePayload(request.Payload),
			}
			subRequest.Context.Workspace = wsReg.Name
			delete(subRequest.Payload, "all_workspaces")

			env, err := a.ask(ctx, subRequest)
			results <- askResult{ws: wsReg, envelope: env, err: err}
		}(ws)
	}

	wg.Wait()
	close(results)

	type scoredResult struct {
		result model.AskResult
		score  int
		wsName string
	}

	var scored []scoredResult
	var allWarnings []string

	for result := range results {
		if result.err != nil {
			allWarnings = append(allWarnings, fmt.Sprintf("%s: ask failed: %v", result.ws.Name, result.err))
			continue
		}
		allWarnings = append(allWarnings, result.envelope.Warnings...)
		if askItems, ok := result.envelope.Items.([]model.AskResult); ok {
			for _, askItem := range askItems {
				// Score by number of doc evidence + code evidence as proxy for confidence
				score := len(askItem.DocEvidence)*10 + len(askItem.CodeEvidence)*5
				if len(askItem.Why) > 0 {
					score += 5
				}
				scored = append(scored, scoredResult{result: askItem, score: score, wsName: result.ws.Name})
			}
		}
	}

	// Sort by score descending, break ties by workspace name for determinism
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].wsName < scored[j].wsName
		}
		return scored[i].score > scored[j].score
	})

	items := make([]model.AskResult, 0, len(scored))
	for i, s := range scored {
		if i >= maxItems {
			break
		}
		items = append(items, s.result)
	}
	env := model.Envelope{Ok: true, Backend: "ask", Items: items, Warnings: allWarnings, Stats: model.Stats{Files: len(items)}}
	if len(scored) > 0 && (len(scored) == 1 || scored[0].score > scored[1].score) {
		env.Coach = buildAskAllWorkspacesCoach(question, scored[0].wsName, request.Context)
	}
	env = applyWikiRepoCompatHint(env, request, "nav.ask", request.Context.Workspace, question)
	return applyCoachPolicy(env, request.Context), nil
}

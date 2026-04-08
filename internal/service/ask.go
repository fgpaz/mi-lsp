package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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

	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	question, _ := request.Payload["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return model.Envelope{}, fmt.Errorf("question is required")
	}

	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	profile, profileSource, profileWarnings := docgraph.LoadProfile(registration.Root)
	if len(docs) == 0 {
		items, searchErr := searchPattern(ctx, registration.Root, project, question, false, askLimit(request.Context.MaxItems, 5, 5))
		if searchErr != nil {
			return model.Envelope{}, searchErr
		}
		fallback := model.AskResult{
			Question: question,
			Summary:  "No encontre documentacion indexada; devolvi evidencia textual del workspace como fallback.",
			PrimaryDoc: model.AskDocEvidence{
				Path:   "README.md",
				Title:  "Generic fallback",
				Family: "generic",
				Layer:  "generic",
			},
			Why:          []string{"doc_index_empty", "fallback=text-search"},
			CodeEvidence: searchItemsToEvidence(items),
			NextQueries:  []string{fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s --format compact", question, registration.Name)},
		}
		warnings := append([]string{}, profileWarnings...)
		warnings = append(warnings, "documentation index is empty; using code fallback")
		warnings = append(warnings, fmt.Sprintf("read_model=%s", profileSource))
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "ask", Items: []model.AskResult{fallback}, Warnings: warnings}, nil
	}

	docByPath := make(map[string]model.DocRecord, len(docs))
	for _, doc := range docs {
		docByPath[doc.Path] = doc
	}

	family := docgraph.MatchFamily(question, profile)

	// FTS5 primary search - gracefully degrades to nil if table unavailable
	_, ftsScores, _ := store.FTSSearchDocs(ctx, db, question, 20)

	ranked := rankDocs(question, family, docs, ftsScores)
	if len(ranked) == 0 {
		warnings := append([]string{}, profileWarnings...)
		warnings = append(warnings, "no wiki match found")
		warnings = append(warnings, fmt.Sprintf("read_model=%s", profileSource))
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "ask", Items: []model.AskResult{{Question: question, Summary: "No encontre una pista fuerte en la wiki para esta pregunta.", Why: []string{"no_doc_match"}}}, Warnings: warnings}, nil
	}

	primary := ranked[0]
	docEvidence, reasons, docWarnings := buildDocEvidence(ctx, db, primary, ranked, docByPath)
	codeEvidence, codeWarnings := a.buildAskCodeEvidence(ctx, db, registration, project, primary.record, docEvidence, question, request.Context.MaxItems)
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
		Why:          append(primary.reason, reasons...),
		NextQueries:  buildAskNextQueries(registration.Name, project, primary.record, codeEvidence),
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "ask", Items: []model.AskResult{result}, Warnings: warnings, Stats: model.Stats{Files: len(codeEvidence)}}, nil
}

func rankDocs(question string, family string, docs []model.DocRecord, ftsScores map[string]float64) []scoredDoc {
	tokens := docgraph.QuestionTokens(question)
	items := make([]scoredDoc, 0, len(docs))
	for _, doc := range docs {
		score := 0
		reasons := make([]string, 0, 4)
		if doc.Family == family {
			score += 30
			reasons = append(reasons, "family="+family)
		}
		score += layerWeight(family, doc.Layer)
		if doc.DocID != "" && strings.Contains(strings.ToLower(question), strings.ToLower(doc.DocID)) {
			score += 40

		// FTS5 BM25 score is the primary signal when available
		if ftsScores != nil {
			if ftsScore, ok := ftsScores[doc.Path]; ok {
				score += int(ftsScore)
				reasons = append(reasons, "fts5=match")
			}
		}

			reasons = append(reasons, "doc_id="+doc.DocID)
		}

		// Manual token overlap - fallback when FTS5 is unavailable or as supplement
		if ftsScores == nil {
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

func layerWeight(family string, layer string) int {
	switch family {
	case "functional":
		switch layer {
		case "01":
			return 18
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

func buildDocEvidence(ctx context.Context, db *sql.DB, primary scoredDoc, ranked []scoredDoc, docByPath map[string]model.DocRecord) ([]model.AskDocEvidence, []string, []string) {
	const maxDocs = 4
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
		symbolsByFile, err := store.SymbolsByFile(ctx, db, path, 3)
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
		found, err := store.FindSymbols(ctx, db, symbolName, "", true, 3)
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

func buildAskNextQueries(workspaceName string, project model.ProjectFile, primary model.DocRecord, codeEvidence []model.AskCodeEvidence) []string {
	queries := []string{}
	repoFlag := ""
	if repoName := askRepoScope(project, codeEvidence); repoName != "" && project.Project.Kind == model.WorkspaceKindContainer {
		repoFlag = " --repo " + repoName
	}
	if primary.DocID != "" {
		queries = append(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s%s --format compact", primary.DocID, workspaceName, repoFlag))
	}
	if len(codeEvidence) > 0 {
		first := codeEvidence[0]
		if first.File != "" && first.Line > 0 {
			queries = append(queries, fmt.Sprintf("mi-lsp nav context %s %d --workspace %s%s --format compact", first.File, first.Line, workspaceName, repoFlag))
		}
		if first.Name != "" {
			queries = append(queries, fmt.Sprintf("mi-lsp nav related %s --workspace %s%s --format compact", first.Name, workspaceName, repoFlag))
		}
	}
	if len(queries) == 0 {
		queries = append(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s%s --format compact", primary.Title, workspaceName, repoFlag))
	}
	return queries
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
				Context: model.QueryOptions{
					Workspace: wsReg.Name,
					MaxItems:  request.Context.MaxItems,
				},
				Payload: clonePayload(request.Payload),
			}
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

	return model.Envelope{Ok: true, Backend: "ask", Items: items, Warnings: allWarnings, Stats: model.Stats{Files: len(items)}}, nil
}

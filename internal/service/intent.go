package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// intentMatch is one file-level code match: Symbol is the best symbol of the
// file, Matched/Total count the distinct query terms found in the file versus
// the terms known to the catalog.
type intentMatch struct {
	Symbol   model.SymbolRecord
	Score    float64
	Evidence string
	Matched  int
	Total    int
	Exact    bool
	// Named is true when a multi-part file name (workflow-settlement.ts) is
	// fully covered by query terms.
	Named bool
}

var (
	intentDocIDPattern  = regexp.MustCompile(`\b(?:FL|RS|RF|TP|TECH|CT|DB)-[A-Z0-9-]+\b`)
	intentSymbolPattern = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]{2,}\b`)
	intentPathPattern   = regexp.MustCompile(`(?:^|[\s"'(\[])((?:[A-Za-z]:[\\/]|[\\/])?[^\s"')\]]+[\\/][^\s"')\]]+)`)
)

func (a *App) intent(ctx context.Context, request model.CommandRequest) (env model.Envelope, err error) {
	var root string
	defer func() {
		if err != nil || root == "" {
			return
		}
		env = annotatePublishedQuery(root, env)
	}()

	registration, project, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, model.NewStableError("intent_workspace_invalid")
	}
	root = registration.Root

	question, _ := request.Payload["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return model.Envelope{}, model.NewStableError("intent_question_required")
	}
	if pair, kinetic := kineticProject(registration.Root); kinetic {
		return a.kineticIntent(ctx, request, registration, question, pair), nil
	}
	if decision, ok := decisionIntentEnvelope(registration, question); ok {
		return decision, nil
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
		env, docsErr := a.intentDocs(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings)
		if docsErr != nil {
			if hasIntentCodeSignals(question) {
				return intentCatalogUnavailable(intentEmptyDocsEnvelope(registration, scopeWarnings), question, true, withCatalogRoot(registration.Root, docsErr)), nil
			}
			return model.Envelope{}, model.NewStableError(sanitizeIntentError(docsErr))
		}
		return a.intentMixWithCode(ctx, registration, question, topN, offset, scopedRepo, env), nil
	}

	if ready, readyErr := store.WorkspaceCatalogReady(ctx, registration.Root); readyErr != nil || !ready {
		cause := readyErr
		if cause == nil {
			cause = errIntentCatalogNotPublished
		}
		return a.intentCodeFallback(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings, cause), nil
	}
	db, err := openWorkspaceDB(registration, "nav.intent", true)
	if err != nil {
		return a.intentCodeFallback(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings, err), nil
	}
	defer db.Close()

	terms := intentTerms(question)
	if len(terms) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Mode: "code", Items: []map[string]any{}, Warnings: []string{"query produced no tokens after normalization"}}, nil
	}

	scored, err := intentCodeSearch(ctx, db, terms, intentQuestionIdentifiers(question), topN, offset, scopedRepo)
	if err != nil {
		if isIndexSchemaBrokenError(err) {
			return a.intentCodeFallback(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings, err), nil
		}
		return model.Envelope{}, model.NewStableError("intent_search_failed")
	}
	if len(scored) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Mode: "code", Items: []map[string]any{}, Warnings: []string{"no symbols matched intent tokens"}}, nil
	}

	items := make([]map[string]any, len(scored))
	for i, match := range scored {
		items[i] = intentCodeItem(match)
	}

	env = model.Envelope{
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

const (
	intentTermFetchLimit = 600
	intentRescoreFiles   = 40
	intentFileWeightBase = 2.0
	intentFileWeightPath = 1.2
	intentFileWeightName = 1.0
	intentFileWeightRel  = 0.6
	intentStemCoverBonus = 1.4
)

// intentCodeSearch ranks catalog files for the question terms. Pass 1 gathers
// candidate symbols per term and computes each term's IDF over files; pass 2
// reloads every symbol of the best files and scores them at file level (see
// scoreIntentFiles). The result has one match per file, best first.
func intentCodeSearch(ctx context.Context, db *sql.DB, terms []intentTerm, identifiers []string, topN int, offset int, scopedRepo *model.WorkspaceRepo) ([]intentMatch, error) {
	var totalFiles int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT file_path) FROM symbols").Scan(&totalFiles); err != nil {
		return nil, err
	}
	if totalFiles == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	candidates := []model.SymbolRecord{}
	known := make([]intentTerm, 0, len(terms))
	idfs := make([]float64, 0, len(terms))
	for _, term := range terms {
		rows, truncated, err := fetchIntentTermSymbols(ctx, db, term, intentTermFetchLimit)
		if err != nil {
			return nil, err
		}
		files := map[string]struct{}{}
		for _, row := range rows {
			if !intentSymbolMatchesTerm(row, term) {
				continue
			}
			files[row.FilePath] = struct{}{}
			key := fmt.Sprintf("%s|%s|%d", row.FilePath, row.QualifiedName, row.StartLine)
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				candidates = append(candidates, row)
			}
		}
		df := len(files)
		if truncated {
			df = max(df, countIntentTermFiles(ctx, db, term))
		}
		if df == 0 {
			continue
		}
		known = append(known, term)
		idfs = append(idfs, math.Log(1+(float64(totalFiles)-float64(df)+0.5)/(float64(df)+0.5)))
	}
	candidates = filterSymbolsByRepo(candidates, scopedRepo)
	if len(known) == 0 || len(candidates) == 0 {
		return nil, nil
	}
	first := scoreIntentFiles(candidates, known, idfs, identifiers)
	if len(first) > intentRescoreFiles {
		first = first[:intentRescoreFiles]
	}
	paths := make([]string, len(first))
	for i, match := range first {
		paths[i] = match.Symbol.FilePath
	}
	full, err := fetchIntentFileSymbols(ctx, db, paths)
	if err != nil {
		return nil, err
	}
	scored := scoreIntentFiles(filterSymbolsByRepo(full, scopedRepo), known, idfs, identifiers)
	if offset > 0 {
		if offset >= len(scored) {
			return nil, nil
		}
		scored = scored[offset:]
	}
	if len(scored) > topN {
		scored = scored[:topN]
	}
	return scored, nil
}

const intentSymbolSelect = `SELECT file_path, COALESCE(repo_id, ''), COALESCE(repo_name, ''), name, kind, start_line, COALESCE(qualified_name, ''), COALESCE(parent, ''), COALESCE(signature, '') FROM symbols`

func scanIntentSymbols(rows *sql.Rows) ([]model.SymbolRecord, error) {
	defer rows.Close()
	symbols := []model.SymbolRecord{}
	for rows.Next() {
		var sym model.SymbolRecord
		if err := rows.Scan(&sym.FilePath, &sym.RepoID, &sym.RepoName, &sym.Name, &sym.Kind, &sym.StartLine, &sym.QualifiedName, &sym.Parent, &sym.Signature); err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
}

func intentTermWhere(term intentTerm) (string, []any) {
	clauses := make([]string, 0, len(term.Patterns))
	args := make([]any, 0, len(term.Patterns)*3)
	for _, pattern := range term.Patterns {
		clauses = append(clauses, "(lower(name) LIKE ? OR lower(file_path) LIKE ? OR lower(COALESCE(parent, '')) LIKE ?)")
		like := "%" + pattern + "%"
		args = append(args, like, like, like)
	}
	return strings.Join(clauses, " OR "), args
}

// fetchIntentTermSymbols returns up to limit symbols whose name, path or parent
// contains one of the term patterns; truncated reports that more rows exist.
func fetchIntentTermSymbols(ctx context.Context, db *sql.DB, term intentTerm, limit int) ([]model.SymbolRecord, bool, error) {
	where, args := intentTermWhere(term)
	rows, err := db.QueryContext(ctx, intentSymbolSelect+" WHERE "+where+" ORDER BY file_path, start_line LIMIT ?", append(args, limit+1)...)
	if err != nil {
		return nil, false, err
	}
	symbols, err := scanIntentSymbols(rows)
	if err != nil {
		return nil, false, err
	}
	if len(symbols) > limit {
		return symbols[:limit], true, nil
	}
	return symbols, false, nil
}

func countIntentTermFiles(ctx context.Context, db *sql.DB, term intentTerm) int {
	where, args := intentTermWhere(term)
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT file_path) FROM symbols WHERE "+where, args...).Scan(&count); err != nil {
		return 0
	}
	return count
}

func fetchIntentFileSymbols(ctx context.Context, db *sql.DB, paths []string) ([]model.SymbolRecord, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	args := make([]any, len(paths))
	for i, path := range paths {
		args[i] = path
	}
	rows, err := db.QueryContext(ctx, intentSymbolSelect+" WHERE file_path IN ("+strings.TrimRight(strings.Repeat("?,", len(paths)), ",")+") ORDER BY file_path, start_line LIMIT 50000", args...)
	if err != nil {
		return nil, err
	}
	return scanIntentSymbols(rows)
}

// scoreIntentFiles scores symbols grouped by file. For every term matched in a
// file the file earns idf x weight (file name 2.0, other path segment 1.2,
// symbol name 1.0, parent/qualified 0.6; the best one counts), multiplied by a coverage bonus of
// 1 + matched/total, halved for test files and non-code files and boosted for an
// exact symbol-name match or when the file name is fully covered by terms. The item symbol is the best matching symbol of the
// file. Ties break by file path so the order is deterministic.
func scoreIntentFiles(symbols []model.SymbolRecord, terms []intentTerm, idfs []float64, identifiers []string) []intentMatch {
	byFile := map[string][]model.SymbolRecord{}
	order := []string{}
	for _, sym := range symbols {
		if _, ok := byFile[sym.FilePath]; !ok {
			order = append(order, sym.FilePath)
		}
		byFile[sym.FilePath] = append(byFile[sym.FilePath], sym)
	}
	identifierSet := stringSet(identifiers...)
	matches := make([]intentMatch, 0, len(order))
	for _, file := range order {
		fileLower := strings.ToLower(file)
		fileParts := intentFieldParts(file)
		stem := fileLower[strings.LastIndex(fileLower, "/")+1:]
		if dot := strings.Index(stem, "."); dot > 0 {
			stem = stem[:dot]
		}
		stemParts := intentFieldParts(stem)
		weights := make([]float64, len(terms))
		var best model.SymbolRecord
		bestScore, bestExact, haveBest := -1.0, false, false
		for _, sym := range byFile[file] {
			nameLower := strings.ToLower(sym.Name)
			nameParts := intentFieldParts(sym.Name)
			relatedText := sym.Parent + " " + intentQualifiedTail(sym.QualifiedName)
			related := strings.ToLower(relatedText)
			relatedParts := intentFieldParts(relatedText)
			symScore := 0.0
			for i, term := range terms {
				switch {
				case intentTermMatches(term, nameLower, nameParts):
					symScore += idfs[i] * intentFileWeightName
					weights[i] = math.Max(weights[i], intentFileWeightName)
				case intentTermMatches(term, related, relatedParts):
					symScore += idfs[i] * intentFileWeightRel
					weights[i] = math.Max(weights[i], intentFileWeightRel)
				}
			}
			_, exact := identifierSet[nameLower]
			if !haveBest || (exact && !bestExact) || (exact == bestExact && symScore > bestScore) {
				best, bestScore, bestExact, haveBest = sym, symScore, exact, true
			}
		}
		matched, score := 0, 0.0
		words := []string{}
		for i, term := range terms {
			if intentTermMatches(term, stem, stemParts) {
				weights[i] = math.Max(weights[i], intentFileWeightBase)
			} else if intentTermMatches(term, fileLower, fileParts) {
				weights[i] = math.Max(weights[i], intentFileWeightPath)
			}
			if weights[i] == 0 {
				continue
			}
			matched++
			score += idfs[i] * weights[i]
			words = append(words, term.Word)
		}
		if matched == 0 {
			continue
		}
		score *= 1 + float64(matched)/float64(len(terms))
		if isIntentTestPath(fileLower) || !isIntentCodePath(fileLower) {
			score *= 0.5
		}
		named := intentStemFullyCovered(terms, stemParts)
		if named {
			score *= intentStemCoverBonus
		}
		if bestExact {
			score *= 1.5
		}
		matches = append(matches, intentMatch{Symbol: best, Score: score, Evidence: "terms=" + strings.Join(words, ","), Matched: matched, Total: len(terms), Exact: bestExact, Named: named && len(stemParts) >= 2})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Symbol.FilePath < matches[j].Symbol.FilePath
	})
	return matches
}

// intentStemFullyCovered reports that every part of the file name ("workflow",
// "settlement" in workflow-settlement.ts) is matched by some query term: the
// file is named after what the question asks for.
func intentStemFullyCovered(terms []intentTerm, stemParts []string) bool {
	if len(stemParts) == 0 {
		return false
	}
	for _, part := range stemParts {
		covered := false
		for _, term := range terms {
			if intentTermMatches(term, part, []string{part}) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func intentQualifiedTail(qualified string) string {
	if index := strings.LastIndex(qualified, "::"); index >= 0 {
		return qualified[index+2:]
	}
	return qualified
}

func isIntentTestPath(lowerPath string) bool {
	base := lowerPath[strings.LastIndex(lowerPath, "/")+1:]
	if strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	padded := "/" + lowerPath
	for _, segment := range []string{"/tests/", "/test/", "/__tests__/"} {
		if strings.Contains(padded, segment) {
			return true
		}
	}
	return strings.Contains(padded, ".tests/")
}

var intentCodeExtensions = stringSet(".go", ".cs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".rs", ".java", ".kt", ".rb", ".php", ".swift", ".c", ".cc", ".cpp", ".h", ".hpp")

func isIntentCodePath(lowerPath string) bool {
	index := strings.LastIndex(lowerPath, ".")
	if index < 0 {
		return false
	}
	_, ok := intentCodeExtensions[lowerPath[index:]]
	return ok
}

// intentCodeItem is the stable code item of nav.intent (primitives-v2):
// result_kind discriminates code vs doc items; kind keeps the catalog symbol kind.
func intentCodeItem(match intentMatch) map[string]any {
	return map[string]any{
		"result_kind":    "code",
		"origin":         model.ItemOriginCatalog,
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

// intentMixWithCode upgrades a docs answer to mode "mixed" when the question is
// about code: it also runs the catalog search and merges strong code matches
// before the docs and weak ones after. Without code signals in the question
// only strong matches are merged. Without a usable catalog or code hits it
// returns the docs envelope untouched.
func (a *App) intentMixWithCode(ctx context.Context, registration model.WorkspaceRegistration, question string, topN int, offset int, scopedRepo *model.WorkspaceRepo, docs model.Envelope) model.Envelope {
	terms := intentTerms(question)
	if len(terms) == 0 {
		return docs
	}
	lexical := hasIntentCodeSignals(question)
	if lexical {
		if ready, readyErr := store.WorkspaceCatalogReady(ctx, registration.Root); readyErr != nil {
			return intentCatalogUnavailable(docs, question, lexical, withCatalogRoot(registration.Root, readyErr))
		} else if !ready {
			return intentCatalogUnavailable(docs, question, lexical, errIntentCatalogNotPublished)
		}
	}
	db, err := openWorkspaceDB(registration, "nav.intent", true)
	if err != nil {
		return intentCatalogUnavailable(docs, question, lexical, err)
	}
	defer db.Close()
	lexical = lexical || intentCatalogNameSignal(ctx, db, question)
	scored, err := intentCodeSearch(ctx, db, terms, intentQuestionIdentifiers(question), topN, offset, scopedRepo)
	if err != nil {
		return intentCatalogUnavailable(docs, question, lexical, withCatalogRoot(registration.Root, err))
	}
	if len(scored) == 0 {
		return docs
	}
	docItems, _ := docs.Items.([]map[string]any)
	strong, weak := []map[string]any{}, []map[string]any{}
	strongLimit := max(1, topN*2/3)
	for _, match := range scored {
		item := intentCodeItem(match)
		if len(strong) < strongLimit && isStrongIntentCodeMatch(match) {
			strong = append(strong, item)
		} else if lexical {
			weak = append(weak, item)
		}
	}
	if len(strong) == 0 && len(weak) == 0 {
		return docs
	}
	merged := make([]map[string]any, 0, len(strong)+len(docItems)+len(weak))
	merged = append(merged, strong...)
	merged = append(merged, docItems...)
	merged = append(merged, weak...)
	if len(merged) > topN {
		merged = merged[:topN]
	}
	docs.Items = merged
	docs.Mode = "mixed"
	docs.Stats = model.Stats{Files: len(docItems), Symbols: len(strong) + len(weak)}
	return docs
}

var errIntentCatalogNotPublished = errors.New("catalog not published")

func intentEmptyDocsEnvelope(registration model.WorkspaceRegistration, warnings []string) model.Envelope {
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Mode: "docs", Items: []map[string]any{}, Warnings: append([]string{}, warnings...)}
}

// intentCodeFallback answers a code question whose catalog cannot be used with
// the docs it can still find (or none), ok:true and marked degraded.
func (a *App) intentCodeFallback(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, question string, topN int, offset int, scopedRepo *model.WorkspaceRepo, scopeWarnings []string, cause error) model.Envelope {
	env, err := a.intentDocs(ctx, request, registration, question, topN, offset, scopedRepo, scopeWarnings)
	if err != nil {
		env = intentEmptyDocsEnvelope(registration, scopeWarnings)
	}
	return intentCatalogUnavailable(env, question, true, withCatalogRoot(registration.Root, cause))
}

// intentCatalogUnavailable keeps the docs answer but, for a code question,
// marks it degraded (classified reason, no code fallback) so the caller does not
// read the missing code hits as "nothing found".
func intentCatalogUnavailable(docs model.Envelope, question string, codeQuestion bool, err error) model.Envelope {
	if !codeQuestion {
		return docs
	}
	docs.MarkDegraded(classifyCatalogUnavailable(err), "")
	search := "mi-lsp nav search <identifier>"
	for _, field := range strings.Fields(question) {
		field = strings.Trim(field, ".,;:!?()[]{}\"'`")
		if (intentCamelCasePattern.MatchString(field) || strings.Contains(field, "_")) && intentSafeCLIValue(field) && !strings.ContainsAny(field, "/:") {
			search = "mi-lsp nav search " + field
			break
		}
	}
	docs.Warnings = dedupeStrings(append(docs.Warnings, "code catalog unavailable ("+docs.Reason+"); only wiki docs are shown, try `"+search+"` for code hits"))
	return docs
}

func (a *App) intentDocs(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, question string, topN int, offset int, scopedRepo *model.WorkspaceRepo, scopeWarnings []string) (model.Envelope, error) {
	query := loadDocQueryContext(ctx, registration, question)
	defer func() { _ = query.Close() }()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	route := query.canonicalRoute(request.Context, false)
	refreshWarnings := []string{}
	if paths := routeDocCandidatePaths(query, route, min(max(topN, 1), 5)); len(paths) > 0 {
		refreshed, refreshErr := RefreshQueryPaths(ctx, registration.Root, paths, 500*time.Millisecond)
		if refreshErr != nil {
			refreshWarnings = append(refreshWarnings, "intent document refresh unavailable; using the currently published snapshot: "+sanitizeIntentError(refreshErr))
		} else if refreshed {
			freshQuery := loadDocQueryContext(ctx, registration, question)
			if freshQuery.dbErr == nil {
				_ = query.Close()
				query = freshQuery
				route = query.canonicalRoute(request.Context, false)
				refreshWarnings = append(refreshWarnings, "intent document candidates refreshed from the published snapshot")
			} else {
				_ = freshQuery.Close()
				refreshWarnings = append(refreshWarnings, "intent document refresh committed but the published snapshot could not be reopened; using prior query results")
			}
		}
	}
	items := buildIntentDocItems(registration.Name, question, route, query.ranked, topN, offset)
	warnings := append([]string{}, scopeWarnings...)
	warnings = append(warnings, query.profileWarnings...)
	warnings = append(warnings, refreshWarnings...)
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

func intentSnippet(sym model.SymbolRecord) string {
	if sym.Signature != "" {
		return sym.Signature
	}
	if sym.Parent != "" {
		return sym.Parent + "." + sym.Name
	}
	return sym.Name
}

const intentDocOrigin = "wiki"

var (
	intentWordPattern      = regexp.MustCompile(`[A-Za-z0-9]+`)
	intentCamelBoundary    = regexp.MustCompile(`([a-z0-9])([A-Z])|([A-Z]+)([A-Z][a-z])`)
	intentCamelCasePattern = regexp.MustCompile(`\b[A-Za-z]*[a-z][A-Z][A-Za-z0-9]*\b|\b[A-Z][a-z0-9]+[A-Z][A-Za-z0-9]*\b`)
	intentSnakeCasePattern = regexp.MustCompile(`\b[A-Za-z0-9]+_[A-Za-z0-9_]+\b`)
	intentDottedPattern    = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*\b`)
	intentFilePathPattern  = regexp.MustCompile(`[\w.-]+/[\w./-]+|\b[\w-]+\.(?:go|cs|ts|tsx|js|jsx|py|rs|java)\b`)
	intentCodeNounPattern  = regexp.MustCompile(`\b(?:function|func|method|struct|class|handler|interface|type)\b`)
	intentCodeVerbPattern  = regexp.MustCompile(`\b(?:where|donde|who|quien)\b.*\b(?:implemented|defined|called|declared|invoked|implementa|implementado|define|definido|llama|llamado)\b`)
	intentCodeStopwords    = stringSet(
		"where", "what", "which", "who", "when", "why", "how", "does", "did", "are", "was", "were", "the", "and", "for", "with", "this", "that", "from", "into", "can", "get", "all", "any",
		"function", "func", "method", "struct", "class", "type", "implemented", "defined", "called", "declared", "invoked",
		"donde", "como", "cual", "cuales", "que", "quien", "por", "para", "con", "del", "los", "las", "una", "uno", "unos", "unas", "esta", "este", "esto", "hay", "son", "ser", "sus",
		"implementa", "implementado", "define", "definido", "llama", "llamado",
	)
)

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// intentCodeWords splits a natural-language question into lowercase words,
// breaking CamelCase, snake_case, dotted names and path segments, and dropping
// stopwords (en+es) and one/two-letter noise.
func intentCodeWords(question string) []string {
	spaced := intentCamelBoundary.ReplaceAllString(question, "$1$3 $2$4")
	seen := map[string]struct{}{}
	words := []string{}
	for _, word := range intentWordPattern.FindAllString(spaced, -1) {
		word = strings.ToLower(word)
		if len(word) < 3 {
			continue
		}
		if _, stop := intentCodeStopwords[word]; stop {
			continue
		}
		if _, dup := seen[word]; dup {
			continue
		}
		seen[word] = struct{}{}
		words = append(words, word)
	}
	return words
}

// intentTerm is one distinct query term with the lowercase patterns that match
// it in names and paths (stem and cheap aliases). Patterns shorter than four
// letters only match a whole identifier part.
type intentTerm struct {
	Word     string
	Patterns []string
}

var intentTermAliases = map[string][]string{
	"deduplicate": {"dedup"}, "deduplicated": {"dedup"}, "deduplication": {"dedup"}, "dedupe": {"dedup"}, "duplicate": {"dedup", "duplicat"},
	"garbage": {"gc"}, "built": {"build"}, "settled": {"settle", "settl"},
}

// intentTerms returns the distinct, stemmed terms of the question.
func intentTerms(question string) []intentTerm {
	seen := map[string]struct{}{}
	terms := []intentTerm{}
	for _, word := range intentCodeWords(question) {
		patterns := intentTermPatterns(word)
		if _, dup := seen[patterns[0]]; dup {
			continue
		}
		seen[patterns[0]] = struct{}{}
		terms = append(terms, intentTerm{Word: word, Patterns: patterns})
	}
	return terms
}

func intentTermPatterns(word string) []string {
	patterns := []string{intentStem(word)}
	for _, alias := range intentTermAliases[word] {
		if !intentContainsString(patterns, alias) {
			patterns = append(patterns, alias)
		}
	}
	return patterns
}

func intentContainsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// intentStem strips common English suffixes ("collected" -> "collect",
// "entries" -> "entry", "submitted" -> "submit"). Stems act as substring
// patterns, so over-stemming ("settled" -> "settl") still matches "settle".
func intentStem(word string) string {
	if len(word) >= 7 && strings.HasSuffix(word, "ies") {
		return strings.TrimSuffix(word, "ies") + "y"
	}
	if len(word) >= 9 && strings.HasSuffix(word, "ation") {
		return strings.TrimSuffix(word, "ation")
	}
	for _, suffix := range []string{"ing", "ed", "es", "s"} {
		if len(word) < len(suffix)+4 || !strings.HasSuffix(word, suffix) || strings.HasSuffix(word, "ss") {
			continue
		}
		stem := strings.TrimSuffix(word, suffix)
		if suffix == "es" && !strings.HasSuffix(stem, "s") && !strings.HasSuffix(stem, "x") && !strings.HasSuffix(stem, "z") && !strings.HasSuffix(stem, "ch") && !strings.HasSuffix(stem, "sh") {
			continue // "scores" -> "score" through the plain "s" rule
		}
		if suffix == "ing" || suffix == "ed" {
			if n := len(stem); n >= 2 && stem[n-1] == stem[n-2] && !strings.ContainsAny(stem[n-1:], "lsz") {
				stem = stem[:n-1]
			}
		}
		return stem
	}
	return word
}

// intentFieldParts splits an identifier or path into lowercase parts
// (CamelCase, snake_case, separators).
func intentFieldParts(value string) []string {
	spaced := intentCamelBoundary.ReplaceAllString(value, "$1$3 $2$4")
	return intentWordPattern.FindAllString(strings.ToLower(spaced), -1)
}

// intentTermMatches reports whether a lowercase field (with its parts) matches
// any pattern of the term: patterns of six or more letters match as a
// substring, four or five letters as the prefix of a part ("turn" does not hit
// "returned"), and shorter ones only as a whole part.
func intentTermMatches(term intentTerm, lower string, parts []string) bool {
	for _, pattern := range term.Patterns {
		switch {
		case len(pattern) >= 6:
			if strings.Contains(lower, pattern) {
				return true
			}
		case len(pattern) >= 4:
			for _, part := range parts {
				if strings.HasPrefix(part, pattern) {
					return true
				}
			}
		default:
			if intentContainsString(parts, pattern) {
				return true
			}
		}
	}
	return false
}

func intentSymbolMatchesTerm(sym model.SymbolRecord, term intentTerm) bool {
	for _, field := range []string{sym.Name, sym.FilePath, sym.Parent} {
		if field != "" && intentTermMatches(term, strings.ToLower(field), intentFieldParts(field)) {
			return true
		}
	}
	return false
}

// hasIntentCodeSignals reports lexical evidence that a question is about code:
// identifier-like tokens, paths, code nouns or "where is X implemented".
func hasIntentCodeSignals(question string) bool {
	if intentCamelCasePattern.MatchString(question) || intentSnakeCasePattern.MatchString(question) ||
		intentDottedPattern.MatchString(question) || intentFilePathPattern.MatchString(question) {
		return true
	}
	normalized := strings.ToLower(question)
	return intentCodeNounPattern.MatchString(normalized) || intentCodeVerbPattern.MatchString(normalized)
}

// intentCatalogNameSignal reports whether a question word equals a catalog
// symbol name or a file basename ("registry" -> internal/workspace/registry.go).
func intentCatalogNameSignal(ctx context.Context, db *sql.DB, question string) bool {
	words := intentCodeWords(question)
	candidates := make([]string, 0, 8)
	for _, word := range words {
		if len(word) >= 4 && len(candidates) < 8 {
			candidates = append(candidates, word)
		}
	}
	if len(candidates) == 0 {
		return false
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(candidates)), ",")
	args := make([]any, len(candidates))
	for i, word := range candidates {
		args[i] = word
	}
	var found int
	if err := db.QueryRowContext(ctx, "SELECT 1 FROM symbols WHERE lower(name) IN ("+placeholders+") LIMIT 1", args...).Scan(&found); err == nil {
		return true
	}
	clauses := make([]string, 0, len(candidates))
	fileArgs := make([]any, 0, len(candidates)*2)
	for _, word := range candidates {
		clauses = append(clauses, "lower(file_path) LIKE ? OR lower(file_path) LIKE ?")
		fileArgs = append(fileArgs, word+".%", "%/"+word+".%")
	}
	return db.QueryRowContext(ctx, "SELECT 1 FROM files WHERE "+strings.Join(clauses, " OR ")+" LIMIT 1", fileArgs...).Scan(&found) == nil
}

// intentQuestionIdentifiers returns the lowercase identifier-like words of the
// question (CamelCase, snake_case, dotted or path names, or a capitalized word that
// is not the first one; for paths their last segment)
// used for exact symbol-name matches. In a one- or two-word question every word
// counts.
func intentQuestionIdentifiers(question string) []string {
	fields := strings.Fields(question)
	identifiers := []string{}
	for position, field := range fields {
		field = strings.Trim(field, ".,;:!?()[]{}\"'`")
		if index := strings.LastIndexAny(field, "./\\"); index >= 0 && index < len(field)-1 {
			field = field[index+1:]
		} else if len(fields) > 2 && !intentCamelCasePattern.MatchString(field) && !strings.Contains(field, "_") && !(position > 0 && startsWithUpper(field)) {
			continue
		}
		if len(field) >= 3 {
			identifiers = append(identifiers, strings.ToLower(field))
		}
	}
	return identifiers
}

func startsWithUpper(value string) bool {
	return value != "" && unicode.IsUpper([]rune(value)[0])
}

// isStrongIntentCodeMatch is true for an exact symbol-name match, a file named
// after the question (multi-part name fully covered) or when the file covers at least half of the distinct catalog-known terms and at least
// two of them.
func isStrongIntentCodeMatch(match intentMatch) bool {
	if match.Exact || match.Named {
		return true
	}
	return match.Matched >= 2 && match.Matched*2 >= match.Total
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
				"result_kind":  "doc",
				"origin":       intentDocOrigin,
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
			"result_kind":  "doc",
			"origin":       intentDocOrigin,
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

func buildIntentDocNextQueries(workspaceName string, _question string, path string, docID string) []string {
	// Legacy document results must never turn the user's raw question into a
	// reversible command. Continue only from canonical indexed identifiers and
	// paths, or fall back to a fixed workspace diagnostic.
	workspace := intentLegacySafeValue(workspaceName)
	queries := []string{}
	if docID = strings.TrimSpace(docID); docID != "" && intentSafeCLIValue(docID) && workspace != "" {
		queries = append(queries, fmt.Sprintf("mi-lsp nav search %s --include-content --workspace %s", docID, workspace))
	}
	if path = strings.TrimSpace(path); path != "" && workspace != "" {
		pathToken := filepath.ToSlash(path) + ":1-120"
		if intentSafeCLIValue(pathToken) {
			queries = append(queries, fmt.Sprintf("mi-lsp nav multi-read %s --workspace %s", pathToken, workspace))
		}
	}
	if len(queries) == 0 {
		if workspace == "" {
			queries = append(queries, "mi-lsp nav governance --format toon")
		} else {
			queries = append(queries, fmt.Sprintf("mi-lsp nav governance --workspace %s --format toon", workspace))
		}
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
	unsafeRepo := false
	if scopedRepo != nil && strings.TrimSpace(scopedRepo.Name) != "" {
		if intentSafeRepoName(scopedRepo.Name) {
			// Keep the plan and every derived expansion tied to the canonical repo,
			// not to a fuzzy selector that was only used for discovery.
			plan.Arguments["repo"] = scopedRepo.Name
		} else {
			// A configured hostile repo name cannot be selected or published.
			// Keep only stable diagnostics in the public plan and stop execution.
			plan.Arguments["repo"] = intentPlaceholder("repo")
			plan.Incomplete = true
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_REPO_SELECTOR_UNSAFE", Section: "repo", Reason: "repo selector cannot be published or executed safely"})
			if fallback, fallbackErr := model.NewIntentFallback("repo", "repo_selector", model.IntentFallbackExplicitIncomplete); fallbackErr == nil {
				plan.Fallbacks = append(plan.Fallbacks, fallback)
			}
			unsafeRepo = true
		}
	}
	warnings := append([]string{"automatic intent routing selected a local deterministic planner"}, scopeWarnings...)

	if !unsafeRepo {
		switch route.Operation {
		case "explain-change":
			a.planExplainChange(ctx, request, registration, &plan, &warnings)
		case "flow-slice", "change-pack":
			a.planHarnessPacket(ctx, request, registration, &plan, &warnings)
		default:
			a.planGraphIntent(ctx, request, registration, project, scopedRepo, &plan, &warnings)
		}
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
			reason := omission.Reason
			if omission.Code == "INTENT_GRAPH_OMISSION" || strings.HasPrefix(omission.Code, "GPH_") || omission.Code == "graph_unresolved" {
				reason = sanitizeIntentOmissionReason(omission.Code, omission.Reason)
			}
			env.Warnings = appendStringIfMissing(env.Warnings, omission.Code+": "+reason)
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
	case "flow-slice", "flow":
		operation = "flow-slice"
	case "change-pack", "pr-impact", "diff-pack":
		operation = "change-pack"
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
		case hasAnyTerm(normalized, "flow slice", "code flow", "trace the flow", "follow the flow"):
			operation = "flow-slice"
		case hasAnyTerm(normalized, "change pack", "pr impact", "diff pack", "review this pr", "review this diff"):
			operation = "change-pack"
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
	if operation != "explain-change" && operation != "change-pack" && operation != "flow-slice" && len(args) == 0 {
		confidence = 0.65
	}
	return intentRoute{Intent: operation, Operation: operation, Arguments: args, Confidence: confidence}, true
}

func extractIntentArguments(question string, payload map[string]any, operation string) map[string]string {
	args := map[string]string{}
	for _, key := range []string{"selector", "symbol", "from", "to", "edge", "generation", "ref", "changed_ref"} {
		if value := strings.TrimSpace(stringPayload(payload, key)); value != "" {
			args[key] = filepath.ToSlash(value)
		}
	}
	if repo := strings.TrimSpace(stringPayload(payload, "repo")); repo != "" {
		args["repo"] = repo
	}
	if fromGitDiff, _ := payload["from_git_diff"].(bool); fromGitDiff {
		args["from_git_diff"] = "true"
	}
	if paths := affectedPathsFromPayload(payload["paths"]); len(paths) > 0 {
		args["paths"] = strings.Join(paths, ",")
	}
	if operation == "explain-change" && args["ref"] != "" {
		args["changed_ref"] = args["ref"]
		if args["paths"] == "" {
			args["from_git_diff"] = "true"
		}
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
	if path, pathSupplied := extractIntentPathSelector(question); pathSupplied {
		return path
	}
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

func extractIntentPathSelector(question string) (string, bool) {
	for _, token := range strings.Fields(question) {
		raw := strings.Trim(token, "\"'()[]")
		if !strings.ContainsAny(raw, `/\\`) {
			continue
		}
		normalized, ok := intentSafeWorkspaceRelativePath(raw)
		if !ok || strings.Contains(normalized, "://") {
			return "", true
		}
	}
	matches := intentPathPattern.FindAllStringSubmatch(question, -1)
	if len(matches) == 0 {
		return "", false
	}
	candidates := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		raw := strings.TrimSpace(match[1])
		normalized, ok := intentSafeWorkspaceRelativePath(raw)
		if !ok || strings.Contains(normalized, "://") {
			return "", true
		}
		candidates[normalized] = struct{}{}
	}
	if len(candidates) != 1 {
		return "", true
	}
	for candidate := range candidates {
		return candidate, true
	}
	return "", true
}

func (a *App) planGraphIntent(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, project model.ProjectFile, scopedRepo *model.WorkspaceRepo, plan *model.IntentPlan, warnings *[]string) {
	selector := strings.TrimSpace(plan.Arguments["selector"])
	if plan.Operation == "path-between" {
		from, to := strings.TrimSpace(plan.Arguments["from"]), strings.TrimSpace(plan.Arguments["to"])
		if from == "" || to == "" {
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_ARGUMENT_MISSING", Section: "path", Reason: "path-between requires deterministic from and to selectors"})
			plan.Incomplete = true
			plan.Expansions = append(plan.Expansions, func() model.Expansion {
				discovery := intentPathDiscoveryExpansionForPlan(registration.Name, from, to, plan.Arguments)
				discovery.Reason = "discover valid endpoint selectors before executing nav path; incomplete endpoints must not be sent to nav path"
				return discovery
			}())
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
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_AFFECTED_UNAVAILABLE", Section: "affected", Reason: sanitizeIntentError(err)})
			return
		}
		items, truncated := boundIntentItems(intentAnyItems(env.Items), request.Context)
		plan.Preview = append(plan.Preview, model.IntentPreview{Section: "affected", Items: items, Count: len(items), Truncated: env.Truncated || truncated})
		plan.Truncated = env.Truncated || truncated
		plan.GenerationID = env.GenerationID
		if strings.Contains(env.Backend, "heuristic") {
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_AFFECTED_HEURISTIC", Section: "affected", Reason: "graph generation unavailable; affected output is explicitly heuristic"})
		}
		for _, warning := range env.Warnings {
			*warnings = appendStringIfMissing(*warnings, sanitizeIntentWarning(warning))
		}
		plan.Expansions = append(plan.Expansions, func() model.Expansion {
			affectedExpansion := intentAffectedExpansionForPlan(registration.Name, *plan)
			affectedExpansion.Reason = "expand the affected-change section with the same changed-path snapshot"
			return affectedExpansion
		}())
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
		*warnings = appendStringIfMissing(*warnings, sanitizeIntentWarning(warning))
	}
	for _, omission := range env.Omissions {
		code := omission.ErrorCode
		if code == "" {
			code = "INTENT_GRAPH_OMISSION"
		}
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: code, Section: section, Reason: sanitizeIntentOmissionReason(code, omission.Reason)})
	}
	plan.Expansions = append(plan.Expansions, func() model.Expansion {
		graphExpansion := intentExpansionForPlan(registration.Name, *plan)
		graphExpansion.Reason = "expand the selected graph section with the same selector and generation"
		return graphExpansion
	}())
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
			*warnings = appendStringIfMissing(*warnings, sanitizeIntentWarning(warning))
		}
	}

	affectedRequest := request
	affectedRequest.Operation = "nav.affected"
	affectedRequest.Payload = cloneIntentPayload(request.Payload)
	if changedRef := strings.TrimSpace(plan.Arguments["changed_ref"]); changedRef != "" {
		affectedRequest.Payload["changed_ref"] = changedRef
	}
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
		plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_AFFECTED_UNAVAILABLE", Section: "affected", Reason: sanitizeIntentError(affectedErr)})
	} else {
		generationMatches := intentAdoptGeneration(plan, "affected", affectedEnv.GenerationID)
		if typed, ok := affectedEnv.Items.([]AffectedItem); ok && generationMatches {
			affectedItems = typed
			sections["affected"].Items = intentAnyItems(typed)
			sections["affected"].Count = len(typed)
		}
		if strings.Contains(affectedEnv.Backend, "heuristic") {
			plan.Omissions = append(plan.Omissions, model.IntentOmission{Code: "INTENT_AFFECTED_HEURISTIC", Section: "affected", Reason: "graph generation unavailable; affected output is explicitly heuristic"})
		}
		if affectedEnv.Truncated {
			plan.Truncated = true
		}
		for _, warning := range affectedEnv.Warnings {
			*warnings = appendStringIfMissing(*warnings, sanitizeIntentWarning(warning))
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
					*warnings = appendStringIfMissing(*warnings, sanitizeIntentWarning(warning))
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
	explainExpansion := intentExplainChangeExpansionForPlan(registration.Name, *plan)
	explainExpansion.Reason = "rerun the same local planner with the complete progressive preview and original normalized inputs"
	diffExpansion := intentDiffExpansionForPlan(registration.Name, plan.GenerationID)
	diffExpansion.Reason = "expand changed files and changed symbols from the same git snapshot"
	affectedExpansion := intentAffectedExpansionForPlan(registration.Name, *plan)
	affectedExpansion.Reason = "expand affected code, tests, and documentation with explicit heuristic labels"
	plan.Expansions = append(plan.Expansions, explainExpansion, diffExpansion, affectedExpansion)
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
	plan := model.IntentPlan{Operation: operation, Arguments: map[string]string{"selector": selector}}
	return intentExpansionForPlan(workspaceName, plan).Command
}

func intentPathDiscoveryExpansion(workspaceName, from, to string) string {
	return intentPathDiscoveryExpansionForPlan(workspaceName, from, to, nil).Command
}

func intentPathDiscoveryExpansionForPlan(workspaceName, from, to string, planArguments map[string]string) model.Expansion {
	arguments := map[string]any{}
	known := strings.TrimSpace(from)
	if known == "" {
		known = strings.TrimSpace(to)
	}
	if known == "" {
		known = "path endpoint symbol"
	}
	command := "mi-lsp nav search " + intentExpansionValue(filepath.ToSlash(known), "selector", arguments)
	command += " --workspace " + intentExpansionValue(workspaceName, "workspace", arguments)
	command += " --format toon --include-content"
	command = appendIntentRepoScope(command, planArguments, arguments)
	return model.Expansion{Command: command, Arguments: emptyIntentExpansionArguments(arguments)}
}

func intentExpansionCommandForPlan(workspaceName string, plan model.IntentPlan) string {
	return intentExpansionForPlan(workspaceName, plan).Command
}

func intentExpansionForPlan(workspaceName string, plan model.IntentPlan) model.Expansion {
	arguments := map[string]any{}
	workspace := intentExpansionValue(workspaceName, "workspace", arguments)
	var base string
	switch plan.Operation {
	case "neighborhood":
		base = "mi-lsp nav neighbors " + intentExpansionValue(plan.Arguments["selector"], "selector", arguments)
	case "path-between":
		base = "mi-lsp nav path " + intentExpansionValue(plan.Arguments["from"], "from", arguments) + " " + intentExpansionValue(plan.Arguments["to"], "to", arguments)
	case "explain-edge":
		base = "mi-lsp nav explain " + intentExpansionValue(plan.Arguments["edge"], "edge", arguments)
	case "callers", "callees":
		base = "mi-lsp nav " + plan.Operation + " " + intentExpansionValue(plan.Arguments["selector"], "selector", arguments)
	default:
		base = "mi-lsp nav " + safeIntentOperation(plan.Operation) + " " + intentExpansionValue(plan.Arguments["selector"], "selector", arguments)
	}
	command := base + " --workspace " + workspace + " --format toon --full"
	command = appendIntentRepoScope(command, plan.Arguments, arguments)
	if generation := strings.TrimSpace(plan.GenerationID); generation != "" {
		command += " --generation " + intentExpansionValue(generation, "generation", arguments)
	}
	return model.Expansion{Command: command, Arguments: emptyIntentExpansionArguments(arguments)}
}

func intentExplainChangeExpansion(workspaceName string, plan model.IntentPlan) string {
	return intentExplainChangeExpansionForPlan(workspaceName, plan).Command
}

func intentExplainChangeExpansionForPlan(workspaceName string, plan model.IntentPlan) model.Expansion {
	arguments := map[string]any{}
	command := "mi-lsp nav explain-change --workspace " + intentExpansionValue(workspaceName, "workspace", arguments) + " --format toon --full"
	paths := intentExpansionPaths(plan.Arguments["paths"])
	if safePaths, ok := intentSafeWorkspaceRelativePaths(paths); ok {
		for _, path := range safePaths {
			command += " --path " + path
		}
	} else if len(paths) > 0 {
		command += " --path " + intentPlaceholder("paths")
		arguments["paths"] = append([]string(nil), paths...)
	}
	if ref := strings.TrimSpace(plan.Arguments["ref"]); ref != "" {
		command += " --ref " + intentExpansionValue(ref, "ref", arguments)
	}
	command = appendIntentRepoScope(command, plan.Arguments, arguments)
	return model.Expansion{Command: command, Arguments: emptyIntentExpansionArguments(arguments)}
}

func intentDiffExpansion(workspaceName, generation string) string {
	return intentDiffExpansionForPlan(workspaceName, generation).Command
}

func intentDiffExpansionForPlan(workspaceName, generation string) model.Expansion {
	arguments := map[string]any{}
	command := "mi-lsp nav diff-context --workspace " + intentExpansionValue(workspaceName, "workspace", arguments)
	if generation = strings.TrimSpace(generation); generation != "" {
		command += " --generation " + intentExpansionValue(generation, "generation", arguments)
	}
	command += " --format toon"
	return model.Expansion{Command: command, Arguments: emptyIntentExpansionArguments(arguments)}
}

func intentAffectedExpansion(workspaceName string, plan model.IntentPlan) string {
	return intentAffectedExpansionForPlan(workspaceName, plan).Command
}

func intentAffectedExpansionForPlan(workspaceName string, plan model.IntentPlan) model.Expansion {
	arguments := map[string]any{}
	command := "mi-lsp nav affected"
	paths := intentExpansionPaths(plan.Arguments["paths"])
	if safePaths, ok := intentSafeWorkspaceRelativePaths(paths); ok {
		for _, path := range safePaths {
			command += " " + path
		}
	} else if len(paths) > 0 {
		command += " " + intentPlaceholder("paths")
		arguments["paths"] = append([]string(nil), paths...)
	}
	if plan.Arguments["from_git_diff"] == "true" || len(paths) == 0 {
		command += " --from-git-diff"
	}
	command += " --include-tests --include-docs --workspace " + intentExpansionValue(workspaceName, "workspace", arguments) + " --format toon --full"
	if changedRef := strings.TrimSpace(plan.Arguments["changed_ref"]); changedRef != "" && plan.Arguments["from_git_diff"] == "true" {
		command += " --changed-ref " + intentExpansionValue(changedRef, "changed_ref", arguments)
	}
	command = appendIntentRepoScope(command, plan.Arguments, arguments)
	if generation := strings.TrimSpace(plan.GenerationID); generation != "" {
		command += " --generation " + intentExpansionValue(generation, "generation", arguments)
	}
	return model.Expansion{Command: command, Arguments: emptyIntentExpansionArguments(arguments)}
}

func appendIntentRepoScope(command string, planArguments map[string]string, arguments map[string]any) string {
	if repo := strings.TrimSpace(planArguments["repo"]); repo != "" {
		if intentSafeRepoName(repo) {
			command += " --repo " + repo
		} else {
			// Do not retain an unpublishable configured name in public structured
			// arguments. The inert placeholder cannot select a repo by itself.
			command += " --repo " + intentPlaceholder("repo")
			arguments["repo"] = intentPlaceholder("repo")
		}
	}
	return command
}

var intentSafeCLIValuePattern = regexp.MustCompile(`^[A-Za-z0-9._/:+_-]+$`)
var intentSafeWorkspaceAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func intentSafeRepoName(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && intentSafeWorkspaceAliasPattern.MatchString(value)
}

func intentSafeCLIValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if intentAbsolutePath(normalized) || intentPathTraversal(normalized) {
		return false
	}
	return intentSafeCLIValuePattern.MatchString(value)
}

func intentAbsolutePath(value string) bool {
	return strings.HasPrefix(value, "/") || regexp.MustCompile(`^[A-Za-z]:($|/)`).MatchString(value)
}

func intentPathTraversal(value string) bool {
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func intentSafeWorkspaceRelativePath(value string) (string, bool) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", false
	}
	normalized := strings.ReplaceAll(raw, "\\", "/")
	if intentAbsolutePath(normalized) || intentPathTraversal(normalized) || !intentSafeCLIValuePattern.MatchString(normalized) {
		return "", false
	}
	if normalized == "." {
		return normalized, true
	}
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "" || normalized == "." {
		return ".", true
	}
	return normalized, true
}

func intentSafeWorkspaceRelativePaths(paths []string) ([]string, bool) {
	if len(paths) == 0 {
		return nil, true
	}
	safe := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized, ok := intentSafeWorkspaceRelativePath(path)
		if !ok {
			return nil, false
		}
		safe = append(safe, normalized)
	}
	return safe, true
}

func intentLegacySafeValue(value string) string {
	value = strings.TrimSpace(value)
	if intentSafeCLIValue(value) {
		return value
	}
	return ""
}

func intentPlaceholder(key string) string {
	key = strings.ToUpper(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	return "__MI_LSP_ARG_" + key + "__"
}

func intentExpansionValue(value, key string, arguments map[string]any) string {
	value = strings.TrimSpace(value)
	if intentSafeCLIValue(value) {
		return value
	}
	if value != "" {
		arguments[key] = value
	}
	return intentPlaceholder(key)
}

func emptyIntentExpansionArguments(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return nil
	}
	return arguments
}

func intentExpansionPaths(value string) []string {
	paths := []string{}
	for _, path := range strings.Split(value, ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			// Keep the caller's value byte-for-byte in structured arguments when
			// the path cannot be safely embedded in the command.
			paths = append(paths, path)
		}
	}
	return paths
}

func safeIntentOperation(operation string) string {
	switch operation {
	case "callers", "callees", "neighborhood", "path-between", "explain-edge":
		return strings.ReplaceAll(operation, "-", " ")
	default:
		return "intent"
	}
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

func sanitizeIntentOmissionReason(code, reason string) string {
	code = strings.TrimSpace(code)
	if code != "" && intentSafeCLIValue(code) {
		return code
	}
	if strings.TrimSpace(reason) == "" {
		return "graph_omission"
	}
	return "graph_omission"
}

func sanitizeIntentWarning(warning string) string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return ""
	}
	if strings.HasPrefix(warning, "catalog unavailable; symbol evidence omitted:") {
		return "catalog_unavailable"
	}
	if strings.HasPrefix(warning, "graph generation unavailable") {
		return "graph_generation_unavailable"
	}
	if strings.HasPrefix(warning, "git hunk parsing failed") {
		return "git_hunk_parsing_failed"
	}
	if strings.HasPrefix(warning, "stdin was parsed") {
		return "stdin_path_parse_fallback"
	}
	if idx := strings.IndexByte(warning, ':'); idx > 0 {
		code := warning[:idx]
		if strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") == "" {
			return code
		}
	}
	return "intent_warning"
}

func sanitizeIntentError(err error) string {
	if err == nil {
		return ""
	}

	var dbErr *workspaceDBOpenError
	if errors.As(err, &dbErr) {
		return workspaceDBOpenErrorCode
	}

	var graphErr *model.GraphQueryError
	if errors.As(err, &graphErr) {
		switch graphErr.Code {
		case "GPH_QUERY_BACKEND_UNAVAILABLE",
			"GPH_QUERY_BUDGET_INVALID",
			"GPH_QUERY_CURSOR_STALE",
			"GPH_QUERY_GENERATION_INVALID",
			"GPH_QUERY_GENERATION_NOT_FOUND",
			"GPH_QUERY_GRAPH_INVALID",
			"GPH_QUERY_SELECTOR_AMBIGUOUS",
			"GPH_QUERY_SELECTOR_INVALID",
			"GPH_QUERY_UTILITY_INVALID":
			return graphErr.Code
		default:
			return "graph_query_error"
		}
	}
	return "operation_error"
}

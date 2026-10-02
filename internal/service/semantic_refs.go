package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/language"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const (
	maxRefsContextLines = 5
	maxRefsContextBytes = 2 << 20
	maxRefsContextWidth = 400
)

var identifierPattern = regexp.MustCompile(`^\w+$`)

// findRefs answers find_refs without ever returning a false empty result:
// semantic failures and empty semantic answers fall back to word-bounded text
// search, and every item carries its origin and (when the catalog knows it) caller.
func (a *App) findRefs(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	symbol, _ := request.Payload["symbol"].(string)
	symbol = strings.TrimSpace(symbol)
	contextLines := clampRefsContext(intFromAny(request.Payload["context"], 0))

	if request.Context.BackendHint == "" && symbol != "" {
		if file, _ := request.Payload["file"].(string); strings.TrimSpace(file) == "" {
			anchored, early := a.anchorRefsRequest(ctx, registration, request, symbol)
			if early != nil {
				return a.finalizeRefs(ctx, registration, *early, symbol, contextLines, started), nil
			}
			request = anchored
		}
	}

	env, err := a.semanticCore(ctx, registration, project, request, "find_refs")
	switch {
	case err != nil:
		fallback, fallbackErr := a.textReferenceFallback(ctx, registration, request, []string{semanticBackendWarning(backendLabel(registration, request), err)})
		if fallbackErr != nil {
			return model.Envelope{}, err
		}
		fallback.MarkDegraded(semanticFailureReason(err), model.FallbackText)
		env = fallback
	case env.Backend == "router":
		return env, nil
	case env.Backend != "text" && env.Backend != "catalog" && (!env.Ok || len(refsItems(env)) == 0):
		fallback, fallbackErr := a.textReferenceFallback(ctx, registration, request, nil)
		switch {
		case fallbackErr != nil:
			env.Warnings = append(env.Warnings, "text verification of an empty semantic result failed: "+fallbackErr.Error())
			if !env.Ok {
				return env, nil
			}
			return a.finalizeRefs(ctx, registration, env, symbol, contextLines, started), nil
		case len(refsItems(fallback)) > 0:
			fallback.Warnings = append(env.Warnings, fallback.Warnings...)
			if env.Ok {
				fallback.MarkDegraded(model.ReasonSemanticEmptyTextHits, model.FallbackText)
			} else {
				fallback.MarkDegraded(model.ReasonLSPError, model.FallbackText)
			}
			env = fallback
		case !env.Ok:
			fallback.Warnings = append(env.Warnings, fallback.Warnings...)
			env = fallback
		}
	}
	return a.finalizeRefs(ctx, registration, env, symbol, contextLines, started), nil
}

// anchorRefsRequest picks the symbol's language when the caller gave no file:
// first from the catalog definition, then from the first code-file text hit.
// It returns an early envelope when only non-code files mention the symbol, or
// when nothing mentions it at all.
func (a *App) anchorRefsRequest(ctx context.Context, registration model.WorkspaceRegistration, request model.CommandRequest, symbol string) (model.CommandRequest, *model.Envelope) {
	def, found, ambiguous := a.catalogDefinition(ctx, registration, request, symbol)
	if ambiguous {
		// Defined in several repos: keep the router's ambiguity handling.
		return request, nil
	}
	if found {
		return withRefsAnchor(request, def.FilePath, def.StartLine), nil
	}
	fallback, err := a.textReferenceFallback(ctx, registration, request, nil)
	if err != nil {
		return request, nil
	}
	items := refsItems(fallback)
	if len(items) == 0 {
		return request, &fallback
	}
	for _, item := range items {
		file, _ := item["file"].(string)
		if _, ok := backendForSourcePath(file); ok {
			return withRefsAnchor(request, file, intFromAny(item["line"], 1)), nil
		}
	}
	fallback.MarkDegraded(model.ReasonLanguageUnsupported, model.FallbackText)
	return request, &fallback
}

func (a *App) catalogDefinition(ctx context.Context, registration model.WorkspaceRegistration, request model.CommandRequest, symbol string) (def model.SymbolRecord, found bool, ambiguous bool) {
	db, err := openWorkspaceDB(registration, "semantic.refs-anchor", true)
	if err != nil {
		return model.SymbolRecord{}, false, false
	}
	defer db.Close()
	name := symbol
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		name = name[idx+1:]
	}
	symbols, err := store.FindSymbols(ctx, db, name, "", true, 20, 0)
	if err != nil {
		return model.SymbolRecord{}, false, false
	}
	repo, _ := request.Payload["repo"].(string)
	var first *model.SymbolRecord
	repos := map[string]bool{}
	for i := range symbols {
		if _, ok := backendForSourcePath(symbols[i].FilePath); !ok {
			continue
		}
		if repo != "" && strings.EqualFold(symbols[i].RepoName, repo) {
			return symbols[i], true, false
		}
		repos[symbols[i].RepoName] = true
		if first == nil {
			first = &symbols[i]
		}
	}
	if first == nil {
		return model.SymbolRecord{}, false, false
	}
	return *first, true, repo == "" && len(repos) > 1
}

func withRefsAnchor(request model.CommandRequest, file string, line int) model.CommandRequest {
	request.Payload = clonePayload(request.Payload)
	request.Payload["file"] = file
	if line > 0 {
		request.Payload["line"] = line
	}
	return request
}

// finalizeRefs normalizes items (relative file, origin, caller, context) and
// settles the no_matches contract for an answer without items.
func (a *App) finalizeRefs(ctx context.Context, registration model.WorkspaceRegistration, env model.Envelope, symbol string, contextLines int, started time.Time) model.Envelope {
	items := refsItems(env)
	origin := model.ItemOriginSemantic
	switch env.Backend {
	case "text":
		origin = model.ItemOriginText
	case "catalog":
		origin = model.ItemOriginCatalog
	}
	for _, item := range items {
		if file, _ := item["file"].(string); file != "" {
			if relative, err := makeRelative(registration.Root, file); err == nil && !strings.HasPrefix(relative, "..") {
				item["file"] = relative
			}
		}
		if _, ok := item["origin"]; !ok {
			item["origin"] = origin
		}
	}
	a.attachRefCallers(ctx, registration, items, symbol)
	attachRefContext(registration.Root, items, contextLines)
	env.Items = items
	if len(items) == 0 {
		env.Ok = true
		env.Degraded = false
		env.FallbackUsed = ""
		env.Reason = model.ReasonNoMatches
	}
	env.Stats.Ms = time.Since(started).Milliseconds()
	return env
}

func refsItems(env model.Envelope) []map[string]any {
	items, _ := env.Items.([]map[string]any)
	if items == nil {
		return []map[string]any{}
	}
	return items
}

// attachRefCallers sets item["caller"] to the innermost catalog symbol that
// contains the reference line. It skips silently when the catalog is unavailable.
func (a *App) attachRefCallers(ctx context.Context, registration model.WorkspaceRegistration, items []map[string]any, symbol string) {
	if len(items) == 0 {
		return
	}
	db, err := openWorkspaceDB(registration, "semantic.refs-callers", true)
	if err != nil {
		return
	}
	defer db.Close()
	byFile := map[string][]model.SymbolRecord{}
	for _, item := range items {
		file, _ := item["file"].(string)
		if file == "" {
			continue
		}
		symbols, loaded := byFile[file]
		if !loaded {
			symbols = fileSymbols(ctx, db, file)
			byFile[file] = symbols
		}
		line := intFromAny(item["line"], 0)
		if caller, ok := innermostSymbol(symbols, line, symbol); ok {
			item["caller"] = map[string]any{"name": caller.Name, "kind": caller.Kind, "line": caller.StartLine}
		}
	}
}

func fileSymbols(ctx context.Context, db *sql.DB, file string) []model.SymbolRecord {
	symbols, err := store.SymbolsByFile(ctx, db, file, 5000, 0)
	if err != nil {
		return nil
	}
	return symbols
}

// innermostSymbol returns the smallest symbol whose range contains line. The
// definition of the queried symbol itself is not its own caller.
func innermostSymbol(symbols []model.SymbolRecord, line int, queried string) (model.SymbolRecord, bool) {
	var best model.SymbolRecord
	found := false
	bestSpan := 0
	for _, symbol := range symbols {
		end := symbol.EndLine
		if end < symbol.StartLine {
			end = symbol.StartLine
		}
		if line < symbol.StartLine || line > end {
			continue
		}
		if symbol.StartLine == line && symbol.Name == queried {
			continue
		}
		span := end - symbol.StartLine
		if !found || span < bestSpan || (span == bestSpan && symbol.StartLine > best.StartLine) {
			best, bestSpan, found = symbol, span, true
		}
	}
	return best, found
}

func clampRefsContext(value int) int {
	if value < 0 {
		return 0
	}
	if value > maxRefsContextLines {
		return maxRefsContextLines
	}
	return value
}

// attachRefContext adds item["context"] with n lines before and after the
// reference, read from disk. Unreadable or oversized files are skipped.
func attachRefContext(root string, items []map[string]any, n int) {
	if n <= 0 {
		return
	}
	cache := map[string][]string{}
	for _, item := range items {
		file, _ := item["file"].(string)
		line := intFromAny(item["line"], 0)
		if file == "" || line <= 0 {
			continue
		}
		lines, loaded := cache[file]
		if !loaded {
			lines = readBoundedLines(filepath.Join(root, filepath.FromSlash(file)))
			cache[file] = lines
		}
		if line > len(lines) {
			continue
		}
		from, to := max(line-n, 1), min(line+n, len(lines))
		item["context"] = strings.Join(lines[from-1:to], "\n")
	}
}

func readBoundedLines(path string) []string {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxRefsContextBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for i, line := range lines {
		if len(line) > maxRefsContextWidth {
			lines[i] = line[:maxRefsContextWidth]
		}
	}
	return lines
}

// refsTextPattern builds the text fallback pattern: a word-bounded regex for
// plain identifiers (rg -w semantics), a literal match for anything else.
func refsTextPattern(symbol string) (string, bool) {
	if identifierPattern.MatchString(symbol) {
		return `\b` + regexp.QuoteMeta(symbol) + `\b`, true
	}
	return symbol, false
}

// backendForSourcePath maps a source file to its semantic backend.
func backendForSourcePath(path string) (string, bool) {
	lang, ok := language.ForPath(path)
	if !ok {
		return "", false
	}
	return backendForLanguage(lang)
}

func backendForLanguage(lang string) (string, bool) {
	switch strings.ToLower(lang) {
	case "go":
		return "gopls", true
	case "typescript", "javascript":
		return "tsserver", true
	case "python":
		return "pyright", true
	case "csharp":
		return "roslyn", true
	default:
		return "", false
	}
}

// backendForRegistration picks the backend of the first registered language
// that has one; text when none does, so roslyn never starts by default.
func backendForRegistration(registration model.WorkspaceRegistration) string {
	for _, lang := range registration.Languages {
		if backendType, ok := backendForLanguage(lang); ok {
			return backendType
		}
	}
	return "text"
}

func backendLabel(registration model.WorkspaceRegistration, request model.CommandRequest) string {
	return resolveBackendType(registration, request, "find_refs")
}

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	nativeFindCandidateLimit = 8
	nativeContextLimit       = 5
	nativeContextChars       = 5000
	nativeRenameFileLimit    = 32
	nativeRenameEditLimit    = 5000
	nativeWarmupLimit        = 2
	nativeWarmupTimeout      = 8 * time.Second
)

func (a *App) findSymbol(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	query := strings.TrimSpace(stringPayload(request.Payload, "query"))
	if query == "" || len(query) > 512 {
		return model.Envelope{}, errors.New("query is required and must be at most 512 bytes")
	}
	request.Context.NoIndexRefresh = true
	identifier := identifierPattern.MatchString(query)
	intent := model.Envelope{}
	var intentErr error
	if !identifier {
		intentRequest := request
		intentRequest.Operation = "nav.intent"
		intentRequest.Payload = map[string]any{"question": query, "top": nativeFindCandidateLimit}
		if repo := stringPayload(request.Payload, "repo"); repo != "" {
			intentRequest.Payload["repo"] = repo
		}
		intent, intentErr = a.intent(ctx, intentRequest)
	}
	pattern := query
	if !identifier {
		candidates := intentCodeCandidates(intent.Items)
		if len(candidates) != 1 {
			return model.Envelope{Ok: true, Backend: "catalog", Partial: true, Degraded: true, Reason: "explicit_incomplete", Items: map[string]any{
				"query": query, "approximate": true, "source": "intent_catalog", "semantic_status": "not_started",
				"definitions": candidates, "usages": []any{}, "ranges": []any{},
				"ambiguity": len(candidates) > 1,
			}, Warnings: append([]string{"question query resolved only to bounded catalog candidates; select a symbol for semantic references"}, intent.Warnings...)}, nil
		}
		pattern = candidates[0].Name
	}

	findRequest := request
	findRequest.Operation = "nav.find"
	findRequest.Context.MaxItems = nativeFindCandidateLimit
	findRequest.Payload = map[string]any{"pattern": pattern, "exact": true}
	if repo := stringPayload(request.Payload, "repo"); repo != "" {
		findRequest.Payload["repo"] = repo
	}
	found, err := a.find(ctx, findRequest)
	if err != nil {
		return model.Envelope{}, err
	}
	candidates, err := symbolRecords(found.Items)
	if err != nil {
		return model.Envelope{}, errors.New("symbol catalog response was invalid")
	}
	catalogCandidateCount := len(candidates)
	validCandidates := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Name != "" && candidate.FilePath != "" && candidate.StartLine > 0 {
			validCandidates = append(validCandidates, candidate)
		}
	}
	candidates = validCandidates
	if identifier && found.Ok && found.Backend == "catalog" && catalogCandidateCount == 0 {
		return a.findSymbolTextFallback(ctx, request, query, found.Warnings)
	}
	if found.Backend == "text" {
		return model.Envelope{Ok: true, Workspace: found.Workspace, Backend: found.Backend, Partial: true, Degraded: true, Reason: "explicit_incomplete", Items: map[string]any{
			"query": query, "approximate": true, "source": found.Backend, "semantic_status": "not_started",
			"definitions": found.Items, "usages": []any{}, "ranges": []any{},
		}, Warnings: append(found.Warnings, "text fallback is lexical evidence, not a definition or semantic reference")}, nil
	}
	if !identifier {
		intentCandidates := intentCodeCandidates(intent.Items)
		if len(intentCandidates) == 1 {
			want := intentCandidates[0]
			filtered := candidates[:0]
			for _, candidate := range candidates {
				if filepath.ToSlash(candidate.FilePath) == filepath.ToSlash(want.FilePath) && candidate.Name == want.Name {
					filtered = append(filtered, candidate)
				}
			}
			candidates = filtered
			if len(candidates) == 0 {
				candidates = []model.SymbolRecord{want}
			}
		}
	}
	candidates = uniqueSymbolRecords(candidates)
	if len(candidates) != 1 {
		ambiguity := len(candidates) > 1
		warning := "catalog did not provide a usable candidate; semantic references were not selected"
		if ambiguity {
			warning = "multiple catalog candidates; semantic references were not selected"
		}
		return model.Envelope{Ok: true, Workspace: found.Workspace, Backend: found.Backend, Partial: true, Degraded: true, Reason: "explicit_incomplete", Items: map[string]any{
			"query": query, "approximate": true, "source": "catalog", "semantic_status": "not_started",
			"definitions": candidates, "usages": []any{}, "ranges": []any{}, "ambiguity": ambiguity,
		}, Warnings: append(found.Warnings, warning)}, nil
	}
	candidate := candidates[0]
	registration, project, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, err
	}
	file := candidate.FilePath
	if !filepath.IsAbs(file) {
		file = filepath.Join(registration.Root, filepath.FromSlash(file))
	}
	semanticRequest := request
	semanticRequest.Operation = "nav.refs"
	semanticRequest.Payload = map[string]any{"symbol": candidate.Name, "file": file, "line": candidate.StartLine}
	backend := resolveBackendType(registration, semanticRequest, "find_refs")
	target, ambiguity, targetErr := a.resolveSemanticTarget(ctx, registration, project, semanticRequest, "find_refs", backend)
	if targetErr != nil || ambiguity != nil {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Partial: true, Degraded: true, Reason: "explicit_incomplete", Items: map[string]any{
			"query": query, "approximate": true, "source": "catalog", "semantic_status": "unavailable",
			"definitions": []model.SymbolRecord{candidate}, "usages": []any{}, "ranges": []any{},
		}, Warnings: []string{"semantic target could not be uniquely resolved; catalog candidate retained"}}, nil
	}
	if backend == "catalog" || backend == "text" {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Partial: true, Degraded: true, Reason: "explicit_incomplete", Items: map[string]any{
			"query": query, "approximate": true, "source": backend, "semantic_status": "unsupported",
			"definitions": []model.SymbolRecord{candidate}, "usages": []any{}, "ranges": []any{},
		}, Warnings: []string{"selected backend has no semantic definition/reference capability; no semantic warm-up was attempted"}}, nil
	}
	if !semanticWorkerReady(a.Semantic.Status(), registration, target, backend) {
		workerRequest := semanticWorkerRequest(registration, target, backend, semanticRequest, "find_refs")
		workerRequest.Payload["symbol"] = candidate.Name
		workerRequest.Payload["file"] = file
		workerRequest.Payload["line"] = candidate.StartLine
		warmup := a.scheduleNativeWarmup(ctx, registration, workerRequest)
		warning := "semantic worker cold; returned the bounded catalog result"
		if warmup == "pending" {
			warning += " and scheduled/coalesced one bounded warm-up"
		} else {
			warning += "; bounded warm-up capacity is currently full"
		}
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Partial: true, Degraded: true, Reason: "explicit_incomplete", Items: map[string]any{
			"query": query, "approximate": true, "source": "catalog", "semantic_status": warmup,
			"definitions": []model.SymbolRecord{candidate}, "usages": []any{}, "ranges": []any{},
		}, Warnings: []string{warning}}, nil
	}

	definitionEnv, definitionErr := a.semantic(ctx, semanticRequest, "find_definition")
	refsEnv, refsErr := a.findRefs(ctx, registration, project, semanticRequest)
	definitionExact := definitionErr == nil && definitionEnv.Ok && definitionEnv.Backend == backend && !definitionEnv.Partial && !definitionEnv.Degraded && definitionEnv.FallbackUsed == ""
	refsExact := refsErr == nil && refsEnv.Ok && refsEnv.Backend == backend && !refsEnv.Partial && !refsEnv.Degraded && refsEnv.FallbackUsed == "" && !refsEnv.Truncated
	semanticOK := definitionExact && refsExact
	definitions := []map[string]any{}
	if definitionExact {
		definitions, _ = envelopeMapItems(definitionEnv.Items)
	}
	if len(definitions) == 0 {
		definitions = []map[string]any{{"file": candidate.FilePath, "line": candidate.StartLine, "symbol": candidate.Name, "source": "catalog"}}
		definitionExact = false
		semanticOK = false
	}
	usageCandidates := []map[string]any{}
	if refsErr == nil {
		usageCandidates = refsItems(refsEnv)
	}
	usages := []map[string]any{}
	if refsExact {
		usages = usageCandidates
	}
	ranges := []any{}
	readWarnings := []string{}
	readRequest := request
	readRequest.Operation = "nav.multi-read"
	readRequest.Context.Workspace = registration.Root
	readRequest.Context.MaxItems = nativeContextLimit
	readRequest.Context.MaxChars = nativeContextChars
	readRequest.Payload = map[string]any{"args": nativeRanges(registration.Root, definitions, usages, nativeContextLimit)}
	if args, _ := readRequest.Payload["args"].([]string); len(args) > 0 {
		readEnv, readErr := a.multiRead(ctx, readRequest)
		if readErr == nil {
			readItems, ok := readEnv.Items.([]multiReadItem)
			if !ok {
				return model.Envelope{}, fmt.Errorf("invalid_response: multi-read returned items of type %T", readEnv.Items)
			}
			ranges = make([]any, len(readItems))
			for i := range readItems {
				ranges[i] = readItems[i]
			}
			if readEnv.Partial || len(readEnv.Omissions) > 0 {
				readWarnings = append(readWarnings, "source context was partial; inspect omissions before relying on omitted ranges")
			}
		} else {
			readWarnings = append(readWarnings, "source context unavailable: "+readErr.Error())
		}
	}
	warnings := append([]string{}, found.Warnings...)
	if intentErr != nil {
		warnings = append(warnings, "intent ranking unavailable; direct catalog lookup used")
	}
	if !semanticOK {
		warnings = append(warnings, "semantic definition/references unavailable or degraded; catalog candidates are not asserted as exact")
	}
	warnings = append(warnings, readWarnings...)
	resultItems := map[string]any{
		"query": query, "approximate": !semanticOK, "source": backend, "semantic_status": statusIf(semanticOK, "checked", "partial"),
		"definitions": definitions, "usages": usages, "ranges": ranges,
	}
	if !refsExact {
		resultItems["usage_candidates"] = usageCandidates
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Partial: !semanticOK, Degraded: !semanticOK, Reason: reasonIf(!semanticOK, "explicit_incomplete"), Items: resultItems, Warnings: warnings, Stats: model.Stats{Symbols: len(usages), Files: len(ranges)}}, nil
}

func (a *App) findSymbolTextFallback(ctx context.Context, request model.CommandRequest, query string, catalogWarnings []string) (model.Envelope, error) {
	matcher := newTextDeclarationMatcher(query, true)
	searchCtx, cancel := withSearchTimeout(ctx, a.Config.SearchTimeout)
	defer cancel()

	search := func(pattern string, limit int) (model.Envelope, []map[string]any, error) {
		searchRequest := request
		searchRequest.Operation = "nav.search"
		searchRequest.Context.MaxItems = limit
		searchRequest.Payload = map[string]any{"pattern": pattern, "regex": true}
		if repo := stringPayload(request.Payload, "repo"); repo != "" {
			searchRequest.Payload["repo"] = repo
		}
		envelope, err := a.search(searchCtx, searchRequest)
		if err != nil || !envelope.Ok {
			return envelope, nil, err
		}
		hits, ok := envelope.Items.([]map[string]any)
		if !ok {
			return model.Envelope{}, nil, fmt.Errorf("invalid_response: native text search returned items of type %T", envelope.Items)
		}
		return envelope, hits, nil
	}

	declarationsEnv, declarationHits, err := search(matcher.searchRe, 500)
	if err != nil || !declarationsEnv.Ok {
		return declarationsEnv, err
	}
	candidates := matcher.declarationItems(declarationHits)
	warnings := append([]string{}, catalogWarnings...)
	warnings = append(warnings, declarationsEnv.Warnings...)
	searchIncomplete := declarationsEnv.Truncated || len(declarationHits) == 500 || searchCtx.Err() != nil || (declarationsEnv.NextHint != nil && *declarationsEnv.NextHint != axiPreviewExpansionHint)
	var nextHint *string
	if declarationsEnv.NextHint != nil {
		nextHint = declarationsEnv.NextHint
	}

	candidateKind := "declaration"
	if len(candidates) == 0 && !searchIncomplete {
		occurrencesEnv, occurrenceHits, searchErr := search(wordBoundaryPattern(query), 300)
		if searchErr != nil || !occurrencesEnv.Ok {
			return occurrencesEnv, searchErr
		}
		candidates = matcher.occurrenceItems(occurrenceHits)
		warnings = append(warnings, occurrencesEnv.Warnings...)
		searchIncomplete = occurrencesEnv.Truncated || len(occurrenceHits) == 300 || searchCtx.Err() != nil || (occurrencesEnv.NextHint != nil && *occurrencesEnv.NextHint != axiPreviewExpansionHint)
		if occurrencesEnv.NextHint != nil {
			nextHint = occurrencesEnv.NextHint
		}
		candidateKind = "occurrence"
	}

	truncated := searchIncomplete || len(candidates) > nativeFindCandidateLimit
	if len(candidates) > nativeFindCandidateLimit {
		candidates = candidates[:nativeFindCandidateLimit]
	}
	ambiguity := len(candidates) > 1
	warnings = append(warnings, "catalog returned zero candidates; bounded text evidence is lexical only and no semantic target was selected or warmed")
	switch {
	case len(candidates) == 0 && searchIncomplete:
		warnings = append(warnings, "bounded text search was incomplete; zero observed candidates is not evidence of an empty workspace")
	case len(candidates) == 0:
		warnings = append(warnings, "bounded text search found no declaration or occurrence; zero candidates is not ambiguity")
	case ambiguity:
		warnings = append(warnings, "multiple lexical "+candidateKind+" candidates remain approximate; semantic identity was not established")
	default:
		warnings = append(warnings, "lexical "+candidateKind+" candidate is approximate; it is not a semantic definition/reference and was not selected")
	}

	reason := "explicit_incomplete"
	if len(candidates) == 0 && !searchIncomplete {
		reason = model.ReasonNoMatches
	}
	return model.Envelope{
		Ok: true, Workspace: declarationsEnv.Workspace, Backend: "text", Partial: true, Degraded: true,
		Reason: reason, Truncated: truncated, NextHint: nextHint,
		Items: map[string]any{
			"query": query, "approximate": true, "source": "text", "semantic_status": "not_started",
			"definitions": candidates, "usages": []any{}, "ranges": []any{}, "ambiguity": ambiguity,
		},
		Warnings: dedupeStrings(warnings), Stats: model.Stats{Symbols: len(candidates)},
	}, nil
}

func (a *App) getErrors(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	files, err := nativeFilesPayload(request.Payload["files"])
	if err != nil {
		return model.Envelope{}, err
	}
	if len(files) > 0 && strings.TrimSpace(stringPayload(request.Payload, "base")) != "" {
		return nativeOperationFailure("invalid_input", "--base is valid only when using the changed-file default"), nil
	}
	payload := clonePayload(request.Payload)
	if len(files) == 0 {
		payload = map[string]any{"changed": true, "base": stringPayload(request.Payload, "base"), "max_files": intFromAny(request.Payload["max_files"], 32), "ready_only": true}
	} else {
		payload = map[string]any{"files": files, "max_files": intFromAny(request.Payload["max_files"], 32), "ready_only": true}
	}
	diagnosticRequest := request
	diagnosticRequest.Operation = "nav.diagnostics"
	diagnosticRequest.Payload = payload
	diagnostics, err := a.diagnostics(ctx, diagnosticRequest)
	if err != nil {
		return model.Envelope{}, err
	}
	summary := diagnostics.Diagnostics
	status := "not_checked"
	detailsComplete := false
	if summary != nil && summary.FilesChecked > 0 {
		detailsComplete = !diagnostics.Partial && !diagnostics.Truncated && diagnostics.NextHint == nil && len(diagnostics.Omissions) == 0 && summary.FilesChecked == summary.FilesRequested
		status = "partial"
		if detailsComplete {
			status = "checked"
		}
	}
	errorCountComplete := summary != nil && summary.FilesChecked > 0 && !diagnostics.Partial && len(diagnostics.Omissions) == 0 && summary.FilesChecked == summary.FilesRequested
	unchecked := make([]string, 0, len(diagnostics.Omissions))
	for _, omission := range diagnostics.Omissions {
		if omission.Path != "" {
			unchecked = append(unchecked, omission.Path)
		}
	}
	result := map[string]any{
		"status": status, "compilation_state": "not_checked", "files_requested": countDiagnosticsRequested(summary),
		"files_checked": countDiagnosticsChecked(summary), "not_checked_files": unchecked,
		"diagnostics": diagnostics.Items, "details_complete": detailsComplete, "error_count_complete": errorCountComplete,
		"truncated": diagnostics.Truncated,
	}
	if diagnostics.NextHint != nil {
		result["next_hint"] = *diagnostics.NextHint
	}
	resultSummary := summary
	if countDiagnosticsChecked(summary) > 0 {
		result["errors_observed"] = countDiagnosticErrors(summary)
	} else {
		resultSummary = nil
	}
	partial := diagnostics.Partial || diagnostics.Truncated || diagnostics.NextHint != nil || (summary != nil && summary.FilesChecked < summary.FilesRequested)
	reason := diagnostics.Reason
	if partial && reason == "" {
		reason = "explicit_incomplete"
	}
	return model.Envelope{
		ContractVersion: "get_errors/v0", Ok: diagnostics.Ok, Workspace: diagnostics.Workspace, Backend: diagnostics.Backend,
		Partial: partial, Degraded: diagnostics.Degraded || partial, Reason: reason, Truncated: diagnostics.Truncated, NextHint: diagnostics.NextHint,
		Diagnostics: resultSummary, Warnings: diagnostics.Warnings, Omissions: diagnostics.Omissions, Error: diagnostics.Error, Stats: diagnostics.Stats,
		Items: result,
	}, nil
}

func (a *App) renameSymbol(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	symbolInput := strings.TrimSpace(stringPayload(request.Payload, "symbol"))
	newName := strings.TrimSpace(stringPayload(request.Payload, "new_name"))
	file := strings.TrimSpace(stringPayload(request.Payload, "file"))
	line := intFromAny(request.Payload["line"], 0)
	column := intFromAny(request.Payload["column"], 0)
	_, hasFile := request.Payload["file"]
	_, hasLine := request.Payload["line"]
	_, hasColumn := request.Payload["column"]
	coordinatePosition := hasFile || hasLine || hasColumn
	selector, selectorInput, selectorErr := model.DecodeNativeSymbolSelector(symbolInput)
	if selectorErr != nil || (selectorInput && coordinatePosition) {
		return nativeOperationFailure("invalid_input", "symbol selector is invalid or conflicts with explicit coordinates"), nil
	}
	symbol := symbolInput
	if selectorInput {
		symbol = selector.Name
		line, column = selector.Line, selector.Column
	}
	explicitPosition := coordinatePosition || selectorInput
	if symbol == "" || newName == "" || symbol == newName || !token.IsIdentifier(symbol) || !token.IsIdentifier(newName) || (coordinatePosition && (!hasFile || !hasLine || !hasColumn || file == "" || line < 1 || column < 1)) {
		return nativeOperationFailure("invalid_input", "rename requires distinct Go identifiers and --new-name; explicit selection requires file, 1-based line, and UTF-16 column together"), nil
	}
	if request.Context.BackendHint != "" && request.Context.BackendHint != "gopls" {
		return nativeOperationFailure("unsupported_operation", "rename_symbol is currently supported only by gopls"), nil
	}
	registration, project, err := diagnosticsWorkspace(ctx, request)
	if err != nil {
		return nativeOperationFailure("invalid_workspace", "physical workspace resolution unavailable; select the intended worktree explicitly"), nil
	}
	absolute := ""
	if selectorInput {
		workspacePhysical, pathErr := filepath.EvalSymlinks(registration.Root)
		if pathErr != nil || filepath.Clean(workspacePhysical) != selector.WorkspaceRoot || registration.Name != selector.Workspace {
			return nativeOperationFailure("invalid_path", "symbol selector belongs to a different physical workspace; no files changed"), nil
		}
		var selectorRepo model.WorkspaceRepo
		for _, candidate := range project.Repos {
			if candidate.ID == selector.RepoID {
				selectorRepo = candidate
				break
			}
		}
		if selectorRepo.ID == "" {
			return nativeOperationFailure("invalid_path", "symbol selector repo is not present in the selected workspace; no files changed"), nil
		}
		repoPath := filepath.Join(registration.Root, filepath.FromSlash(selectorRepo.Root))
		repoPhysical, pathErr := filepath.EvalSymlinks(repoPath)
		if pathErr != nil || filepath.Clean(repoPhysical) != selector.RepoRoot || !nativePathWithin(workspacePhysical, repoPhysical) {
			return nativeOperationFailure("invalid_path", "symbol selector repo physical identity changed; no files changed"), nil
		}
		file = filepath.Join(repoPath, filepath.FromSlash(selector.File))
		var relative string
		var pathInfo os.FileInfo
		absolute, relative, pathInfo, pathErr = nativeRenamePath(registration.Root, file)
		if pathErr != nil || pathInfo.Size() > 2<<20 || !strings.EqualFold(filepath.Ext(relative), ".go") || !nativeRenamePathWithinRepo(registration.Root, selectorRepo.Root, absolute) {
			return nativeOperationFailure("invalid_path", "symbol selector file is no longer a regular source inside the selected repo; no files changed"), nil
		}
		if snapshotErr := nativeVerifyRenameSnapshot(absolute, selector.SHA256); snapshotErr != nil {
			return nativeOperationFailure("explicit_incomplete", "symbol selector source snapshot is stale or changed; no files changed"), nil
		}
	} else if coordinatePosition {
		var relative string
		var pathInfo os.FileInfo
		var pathErr error
		absolute, relative, pathInfo, pathErr = nativeRenamePath(registration.Root, file)
		if pathErr != nil || pathInfo.Size() > 2<<20 || !strings.EqualFold(filepath.Ext(relative), ".go") {
			return nativeOperationFailure("invalid_path", "rename target must be a regular Go source file inside the selected workspace and outside .git"), nil
		}
	}
	if !explicitPosition && registration.Kind == model.WorkspaceKindContainer && len(project.Repos) > 1 && !hasRenameRouteSelector(request.Payload) {
		return nativeRenameResolutionFailure("ambiguous_repo", "name-only rename in a multi-repo workspace requires --repo or another explicit semantic selector", repoCandidates(project.Repos)), nil
	}
	semanticRequest := request
	semanticRequest.Operation = "nav.refs"
	semanticRequest.Context.BackendHint = "gopls"
	semanticRequest.Payload = clonePayload(request.Payload)
	semanticRequest.Payload["symbol"] = symbol
	semanticRequest.Payload["new_name"] = newName
	if selectorInput {
		semanticRequest.Payload["repo"] = selector.RepoID
	}
	if explicitPosition {
		semanticRequest.Payload["file"] = absolute
		semanticRequest.Payload["line"] = line
		semanticRequest.Payload["column"] = column
		semanticRequest.Payload["strict_position"] = true
	}
	target, ambiguity, targetErr := a.resolveSemanticTarget(ctx, registration, project, semanticRequest, "rename_symbol", "gopls")
	if targetErr != nil {
		return nativeOperationFailure("unavailable", "semantic rename target could not be resolved; no files changed"), nil
	}
	if ambiguity != nil {
		candidates, _ := envelopeMapItems(ambiguity.Items)
		return nativeRenameResolutionFailure("ambiguous_target", "semantic rename target is unresolved; select a valid unique repo or entrypoint; no files changed", candidates), nil
	}
	if selectorInput && target.Repo.ID != selector.RepoID {
		return nativeOperationFailure("invalid_path", "symbol selector does not match the selected semantic repo; no files changed"), nil
	}
	if !semanticWorkerReady(a.Semantic.Status(), registration, target, "gopls") {
		method := "rename_symbol"
		if !explicitPosition {
			method = "resolve_rename_symbol"
		}
		warmRequest := semanticWorkerRequest(registration, target, "gopls", semanticRequest, method)
		warmup := a.scheduleNativeWarmup(ctx, registration, warmRequest)
		envelope := nativeOperationFailure("explicit_incomplete", "gopls worker is cold; rename was not attempted and no files changed")
		envelope.Items = map[string]any{"status": warmup, "semantic_status": warmup, "candidates": []any{}, "files_touched": []string{}, "compilation_state": "not_checked"}
		return envelope, nil
	}
	if !explicitPosition {
		resolveRequest := semanticWorkerRequest(registration, target, "gopls", semanticRequest, "resolve_rename_symbol")
		resolveCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		resolved, resolveErr := a.Semantic.Call(resolveCtx, registration, resolveRequest)
		cancel()
		if resolveErr != nil {
			return nativeOperationFailure(nativeRenameErrorCode(resolveErr), "live gopls symbol resolution failed; no files changed: "+resolveErr.Error()), nil
		}
		if !resolved.Ok {
			code := resolved.ErrorCode
			if code == "" {
				code = "explicit_incomplete"
			}
			detail := "gopls did not provide a selectable rename candidate; no files changed"
			if code == "selection_required" {
				detail = "workspace/symbol completeness is unproven; select one prepareRename-validated candidate.symbol and retry; no files changed"
			} else if code == "ambiguous_symbol" {
				detail = "multiple live gopls symbols passed prepareRename; select one candidate.symbol; no files changed"
			}
			return nativeRenameResolutionFailure(code, detail, resolved.Items), nil
		}
		if resolved.RenameTarget == nil {
			return nativeOperationFailure("invalid_response", "gopls returned no unique rename target; no files changed"), nil
		}
		file = resolved.RenameTarget.File
		line, column = resolved.RenameTarget.Line, resolved.RenameTarget.Column
		var relative string
		var pathInfo os.FileInfo
		var pathErr error
		absolute, relative, pathInfo, pathErr = nativeRenamePath(registration.Root, file)
		if pathErr != nil || pathInfo.Size() > 2<<20 || !strings.EqualFold(filepath.Ext(relative), ".go") {
			return nativeOperationFailure("invalid_path", "live gopls rename target escaped the selected Go workspace; no files changed"), nil
		}
		semanticRequest.Payload = map[string]any{"symbol": symbol, "new_name": newName, "file": absolute, "line": line, "column": column, "strict_position": true}
	}
	if !nativeRenamePathWithinRepo(registration.Root, target.Repo.Root, absolute) {
		return nativeOperationFailure("invalid_path", "rename source is outside the selected semantic repo; no files changed"), nil
	}
	payload := map[string]any{"symbol": symbol, "new_name": newName, "file": absolute, "line": line, "column": column, "files": []string{absolute}}
	workerRequest := semanticWorkerRequest(registration, target, "gopls", semanticRequest, "rename_symbol")
	if selectorInput {
		payload["selector_sha256"] = selector.SHA256
	}
	workerRequest.Payload = payload
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	response, callErr := a.Semantic.Call(callCtx, registration, workerRequest)
	cancel()
	if callErr != nil {
		code := nativeRenameErrorCode(callErr)
		if errors.Is(callErr, context.DeadlineExceeded) || errors.Is(callErr, context.Canceled) {
			code = "timeout"
		}
		return nativeOperationFailure(code, "gopls rename planning failed; no files changed: "+callErr.Error()), nil
	}
	if !response.Ok || len(response.WorkspaceEdit) == 0 {
		return nativeOperationFailure("unsupported_operation", "gopls did not provide a complete versioned semantic rename plan; no files changed"), nil
	}
	files := make([]string, 0, len(response.Items))
	seenFiles := map[string]bool{}
	for _, item := range response.Items {
		refFile, ok := item["file"].(string)
		if !ok || refFile == "" || !filepath.IsAbs(refFile) {
			return nativeOperationFailure("invalid_response", "gopls rename plan omitted a verified local reference path; no files changed"), nil
		}
		validated, _, pathInfo, pathErr := nativeRenamePath(registration.Root, refFile)
		if pathErr != nil || pathInfo == nil || pathInfo.Size() > 2<<20 {
			return nativeOperationFailure("invalid_path", "gopls rename reference escaped the selected workspace; no files changed"), nil
		}
		key := filepath.Clean(validated)
		if seenFiles[key] {
			return nativeOperationFailure("invalid_response", "gopls rename plan repeated a verified reference path; no files changed"), nil
		}
		seenFiles[key] = true
		files = append(files, validated)
	}
	if !seenFiles[filepath.Clean(absolute)] {
		return nativeOperationFailure("explicit_incomplete", "gopls rename plan omitted the selected source from verified references; no files changed"), nil
	}
	if len(files) > nativeRenameFileLimit {
		return nativeOperationFailure("work_limit", "rename touches more than 32 files; no files changed"), nil
	}
	touched, applyErr := applyNativeRenamePlan(registration.Root, symbol, newName, files, response)
	if applyErr != nil {
		var recoveryErr *nativeRenameApplyError
		if errors.As(applyErr, &recoveryErr) {
			envelope := nativeOperationFailure(recoveryErr.Code, applyErr.Error())
			mutationState := "not_applied"
			if recoveryErr.Applied {
				mutationState = "fully_applied"
			} else if len(touched) > 0 {
				mutationState = "partial_or_conflicted"
			}
			envelope.Items = map[string]any{
				"status": "applied_with_cleanup_or_recovery_warning", "files_touched": touched,
				"mutation_state": mutationState, "partial_mutation": !recoveryErr.Applied && len(touched) > 0, "compilation_state": "not_checked",
				"recovery_artifacts": recoveryErr.Recovery,
			}
			return envelope, nil
		}
		if len(touched) > 0 {
			envelope := nativeOperationFailure(nativeRenameErrorCode(applyErr), applyErr.Error())
			envelope.Items = map[string]any{"status": "applied_with_cleanup_or_recovery_warning", "files_touched": touched, "partial_mutation": true, "compilation_state": "not_checked"}
			return envelope, nil
		}
		return nativeOperationFailure(nativeRenameErrorCode(applyErr), applyErr.Error()), nil
	}
	post := request
	post.Operation = "get_errors"
	post.Context.Workspace = registration.Root
	post.Payload = map[string]any{"files": touched, "max_files": nativeRenameFileLimit}
	postDiagnostics, diagErr := a.getErrors(ctx, post)
	diagnosticStatus := "not_checked"
	var diagnosticItems any = []any{}
	warnings := []string{}
	if diagErr == nil {
		diagnosticItems = postDiagnostics.Items
		if result, ok := postDiagnostics.Items.(map[string]any); ok {
			if value, ok := result["status"].(string); ok {
				diagnosticStatus = value
			}
		}
		warnings = append(warnings, postDiagnostics.Warnings...)
	} else {
		warnings = append(warnings, "post-rename diagnostics unavailable; compilation not checked")
	}
	partial := diagnosticStatus != "checked"
	return model.Envelope{ContractVersion: "rename_symbol/v0", Ok: true, Workspace: registration.Name, Backend: "gopls", Partial: partial, Degraded: partial, Reason: reasonIf(partial, "explicit_incomplete"), Warnings: warnings, Items: map[string]any{
		"symbol": symbol, "new_name": newName, "files_touched": touched, "diagnostics_status": diagnosticStatus,
		"compilation_state": "not_checked", "application": "per_file_atomic_with_cross_file_rollback_on_detected_failure", "diagnostics": diagnosticItems,
	}, Stats: model.Stats{Files: len(touched)}}, nil
}

type nativeRecoveryArtifact struct {
	File   string `json:"file"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type nativeRenameApplyError struct {
	Code     string
	Message  string
	Touched  []string
	Recovery []nativeRecoveryArtifact
	Applied  bool
	Cause    error
}

func (e *nativeRenameApplyError) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *nativeRenameApplyError) Unwrap() error { return e.Cause }

func nativeRenamePath(root, path string) (string, string, os.FileInfo, error) {
	if root == "" || path == "" || strings.ContainsRune(path, 0) {
		return "", "", nil, errors.New("rename path is empty or invalid")
	}
	if !(runtime.GOOS == "windows" && filepath.IsAbs(path) && filepath.VolumeName(path) != "") {
		schemeEnd := strings.IndexByte(path, ':')
		pathSeparator := strings.IndexAny(path, `/\\`)
		if schemeEnd > 0 && (pathSeparator < 0 || schemeEnd < pathSeparator) {
			path = uriToNativePath(path)
			if path == "" {
				return "", "", nil, errors.New("rename path URI must resolve to a local file")
			}
		}
	}
	rootLexical, err := filepath.Abs(root)
	if err != nil {
		return "", "", nil, errors.New("rename workspace path is unavailable")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootLexical, filepath.FromSlash(path))
	}
	absolute := filepath.Clean(path)
	lexicalRelative, err := filepath.Rel(rootLexical, absolute)
	if err != nil || filepath.IsAbs(lexicalRelative) || lexicalRelative == ".." || strings.HasPrefix(lexicalRelative, ".."+string(filepath.Separator)) {
		return "", "", nil, errors.New("rename path is outside the lexical workspace")
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", "", nil, errors.New("rename target must be a regular non-symlink file")
	}
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", nil, errors.New("rename target physical path is unavailable")
	}
	physicalRoot, err := filepath.EvalSymlinks(rootLexical)
	if err != nil || !diagnosticsInside(physicalRoot, physical) {
		return "", "", nil, errors.New("rename target escaped the physical workspace")
	}
	physicalRelative, err := filepath.Rel(physicalRoot, physical)
	if err != nil || pathContainsGit(lexicalRelative) || pathContainsGit(physicalRelative) {
		return "", "", nil, errors.New("rename target is inside .git")
	}
	return absolute, filepath.ToSlash(lexicalRelative), info, nil
}

func nativeVerifyRenameSnapshot(filePath, expectedDigest string) error {
	before, err := os.Lstat(filePath)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() > 2<<20 {
		return errors.New("selector source is not a bounded regular file")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return errors.New("selector source identity changed before read")
	}
	content, err := io.ReadAll(io.LimitReader(file, before.Size()))
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(opened, after) || after.Size() != before.Size() || int64(len(content)) != before.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return errors.New("selector source changed during read")
	}
	pathAfter, err := os.Lstat(filePath)
	if err != nil || pathAfter.Mode()&os.ModeSymlink != 0 || !os.SameFile(after, pathAfter) {
		return errors.New("selector source path identity changed during read")
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != expectedDigest {
		return errors.New("selector source digest changed")
	}
	return nil
}

func removeRenameArtifact(path string, identity os.FileInfo) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if identity == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !os.SameFile(identity, info) {
		return errors.New("temporary artifact identity changed; left untouched")
	}
	return os.Remove(path)
}

func nativeRenameOriginalUnchanged(root string, file stagedRenameFile) error {
	_, _, info, err := nativeRenamePath(root, file.path)
	if err != nil || file.identity == nil || !os.SameFile(file.identity, info) {
		return errors.New("rename target identity changed")
	}
	current, err := os.ReadFile(file.path)
	if err != nil || nativeDigest(current) != file.snapshot {
		return errors.New("rename source bytes changed")
	}
	return nil
}

func applyNativeRenamePlan(root, symbol, newName string, expectedFiles []string, response model.WorkerResponse) (touched []string, retErr error) {
	if response.RenamePlaceholder != symbol {
		return nil, errors.New("explicit_incomplete: gopls did not confirm the selected symbol; no files changed")
	}
	var plan struct {
		Changes         map[string]json.RawMessage `json:"changes"`
		DocumentChanges []json.RawMessage          `json:"documentChanges"`
	}
	if err := json.Unmarshal(response.WorkspaceEdit, &plan); err != nil {
		return nil, fmt.Errorf("invalid_response: workspace edit is not valid JSON")
	}
	if len(plan.Changes) != 0 || len(plan.DocumentChanges) == 0 || len(plan.DocumentChanges) > nativeRenameFileLimit {
		return nil, errors.New("unsupported_operation: rename must be a complete versioned documentChanges plan; no files changed")
	}
	files := make([]stagedRenameFile, 0, len(plan.DocumentChanges))
	totalEdits := 0
	seen := map[string]bool{}
	expected := map[string]bool{}
	for _, file := range expectedFiles {
		validated, _, pathInfo, pathErr := nativeRenamePath(root, file)
		if pathErr != nil || pathInfo.Size() > 2<<20 {
			return nil, errors.New("invalid_path: expected reference path is not a regular non-symlink file inside the selected workspace")
		}
		expected[filepath.Clean(validated)] = true
	}
	if len(expected) == 0 {
		return nil, errors.New("explicit_incomplete: semantic references did not identify files to rename")
	}
	defer func() {
		var cleanupRecovery []nativeRecoveryArtifact
		for i := range files {
			file := &files[i]
			if file.stage != "" {
				if err := removeRenameArtifact(file.stage, file.stageIdentity); err != nil {
					cleanupRecovery = append(cleanupRecovery, nativeRecoveryArtifact{File: file.relative, Path: file.stage, Kind: "stage", State: "cleanup_failed", Detail: err.Error()})
				} else {
					file.stage = ""
				}
			}
			if file.backup != "" && !file.preserveBackup {
				if err := removeRenameArtifact(file.backup, file.backupIdentity); err != nil {
					file.preserveBackup = true
					cleanupRecovery = append(cleanupRecovery, nativeRecoveryArtifact{File: file.relative, Path: file.backup, Kind: "backup", State: "cleanup_failed", Detail: err.Error()})
				} else {
					file.backup = ""
				}
			}
		}
		if len(cleanupRecovery) == 0 {
			return
		}
		var applyErr *nativeRenameApplyError
		if errors.As(retErr, &applyErr) {
			applyErr.Recovery = append(applyErr.Recovery, cleanupRecovery...)
			return
		}
		retErr = &nativeRenameApplyError{Code: "mutation_recovery_failed", Message: "rename artifacts could not be cleaned; inspect recovery paths", Touched: touched, Recovery: cleanupRecovery, Applied: retErr == nil, Cause: retErr}
	}()
	for _, raw := range plan.DocumentChanges {
		var doc nativeDocumentEdit
		if err := json.Unmarshal(raw, &doc); err != nil || doc.TextDocument == nil || doc.TextDocument.URI == "" || doc.TextDocument.Version == nil || len(doc.Edits) == 0 {
			return nil, errors.New("unsupported_operation: rename plan contains an unversioned edit or resource operation; no files changed")
		}
		uri := doc.TextDocument.URI
		if seen[uri] {
			return nil, errors.New("invalid_response: duplicate document in rename plan")
		}
		seen[uri] = true
		if response.DocumentVersions[uri] != *doc.TextDocument.Version || response.DocumentSnapshots[uri] == "" {
			return nil, errors.New("explicit_incomplete: source version or snapshot is missing/stale; no files changed")
		}
		path := uriToNativePath(uri)
		if path == "" {
			return nil, errors.New("invalid_path: rename plan contains an invalid or non-local file URI")
		}
		absolute, relative, info, pathErr := nativeRenamePath(root, path)
		if pathErr != nil || info.Size() > 2<<20 {
			return nil, errors.New("invalid_path: rename plan escaped the selected workspace, targets .git, or uses a symlink")
		}
		if !expected[filepath.Clean(absolute)] {
			return nil, errors.New("explicit_incomplete: rename plan contains an unreferenced file; no files changed")
		}
		delete(expected, filepath.Clean(absolute))
		if totalEdits+len(doc.Edits) > nativeRenameEditLimit {
			return nil, errors.New("work_limit: rename plan exceeds 5000 edits; no files changed")
		}
		totalEdits += len(doc.Edits)
		original, err := os.ReadFile(absolute)
		if err != nil || !utf8.Valid(original) {
			return nil, errors.New("unavailable: rename source could not be read as UTF-8")
		}
		_, _, currentInfo, identityErr := nativeRenamePath(root, absolute)
		if identityErr != nil || !os.SameFile(info, currentInfo) {
			return nil, errors.New("explicit_incomplete: rename target identity changed while staging; no files changed")
		}
		if digest := sha256.Sum256(original); hex.EncodeToString(digest[:]) != response.DocumentSnapshots[uri] {
			return nil, errors.New("explicit_incomplete: source changed after the semantic plan; no files changed")
		}
		converted := make([]nativeByteEdit, 0, len(doc.Edits))
		for _, edit := range doc.Edits {
			start, okStart := utf16ByteOffset(original, edit.Range.Start.Line, edit.Range.Start.Character)
			end, okEnd := utf16ByteOffset(original, edit.Range.End.Line, edit.Range.End.Character)
			if !okStart || !okEnd || start > end || edit.NewText != newName || !bytes.Equal(original[start:end], []byte(symbol)) {
				return nil, errors.New("explicit_incomplete: rename edit does not match the selected symbol exactly; no files changed")
			}
			converted = append(converted, nativeByteEdit{start: start, end: end, text: edit.NewText})
		}
		sort.Slice(converted, func(i, j int) bool { return converted[i].start > converted[j].start })
		lastStart := len(original) + 1
		updated := append([]byte(nil), original...)
		for _, edit := range converted {
			if edit.end > lastStart {
				return nil, errors.New("invalid_response: overlapping rename edits; no files changed")
			}
			updated = append(updated[:edit.start], append([]byte(edit.text), updated[edit.end:]...)...)
			lastStart = edit.start
		}
		files = append(files, stagedRenameFile{path: absolute, relative: relative, original: original, updated: updated, mode: info.Mode().Perm(), snapshot: response.DocumentSnapshots[uri], identity: info})
	}
	if totalEdits == 0 {
		return nil, errors.New("unsupported_operation: rename plan contains no edits")
	}
	if len(expected) != 0 {
		return nil, errors.New("explicit_incomplete: rename plan omitted one or more referenced files; no files changed")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].relative < files[j].relative })
	for i := range files {
		stage, err := writeRenameTemp(filepath.Dir(files[i].path), ".mi-lsp-rename-stage-*", files[i].updated, files[i].mode)
		files[i].stage = stage
		if err != nil {
			return nil, fmt.Errorf("unavailable: could not stage rename: %w", err)
		}
		files[i].stageIdentity, err = os.Lstat(stage)
		if err != nil {
			return nil, fmt.Errorf("unavailable: could not verify staged rename file: %w", err)
		}
		backup, err := writeRenameTemp(filepath.Dir(files[i].path), ".mi-lsp-rename-backup-*", files[i].original, files[i].mode)
		files[i].backup = backup
		if err != nil {
			return nil, fmt.Errorf("unavailable: could not stage rollback copy: %w", err)
		}
		files[i].backupIdentity, err = os.Lstat(backup)
		if err != nil {
			return nil, fmt.Errorf("unavailable: could not verify rollback copy: %w", err)
		}
	}
	for _, file := range files {
		if err := nativeRenameOriginalUnchanged(root, file); err != nil {
			return nil, errors.New("explicit_incomplete: source or target identity changed before commit; no files changed")
		}
	}
	committed := 0
	rollback := func(message string, cause error) ([]string, error) {
		recovery := rollbackNativeRename(root, files, committed)
		if len(recovery) != 0 {
			touched = nativeTouched(files[:committed])
			return touched, &nativeRenameApplyError{Code: "mutation_recovery_failed", Message: message, Touched: touched, Recovery: recovery, Cause: cause}
		}
		return nil, cause
	}
	for i := range files {
		if err := nativeRenameOriginalUnchanged(root, files[i]); err != nil {
			return rollback("source changed during commit; inspect recovery artifacts", errors.New("explicit_incomplete: source changed during commit; prior files were rolled back"))
		}
		stageInfo, stageErr := os.Lstat(files[i].stage)
		stageBytes, readStageErr := os.ReadFile(files[i].stage)
		if stageErr != nil || readStageErr != nil || files[i].stageIdentity == nil || !os.SameFile(files[i].stageIdentity, stageInfo) || nativeDigest(stageBytes) != nativeDigest(files[i].updated) {
			return rollback("staged target changed before commit; inspect recovery artifacts", errors.New("explicit_incomplete: staged rename file identity or bytes changed"))
		}
		if err := os.Rename(files[i].stage, files[i].path); err != nil {
			return rollback("rename commit failed; inspect recovery artifacts", fmt.Errorf("unavailable: rename commit failed and prior files were rolled back: %w", err))
		}
		files[i].stage = ""
		committed++
		info, err := os.Lstat(files[i].path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || files[i].stageIdentity == nil || !os.SameFile(files[i].stageIdentity, info) {
			return rollback("committed target identity is unavailable; inspect recovery artifacts", errors.New("mutation_recovery_failed: committed target identity could not be verified"))
		}
		files[i].committedIdentity = info
	}
	return nativeTouched(files), nil
}

type nativeTextPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type nativeTextRange struct {
	Start nativeTextPosition `json:"start"`
	End   nativeTextPosition `json:"end"`
}
type nativeTextEdit struct {
	Range   nativeTextRange `json:"range"`
	NewText string          `json:"newText"`
}
type nativeDocumentEdit struct {
	TextDocument *struct {
		URI     string `json:"uri"`
		Version *int   `json:"version"`
	} `json:"textDocument"`
	Edits []nativeTextEdit `json:"edits"`
}
type nativeByteEdit struct {
	start int
	end   int
	text  string
}
type stagedRenameFile struct {
	path              string
	relative          string
	original          []byte
	updated           []byte
	mode              os.FileMode
	snapshot          string
	stage             string
	backup            string
	identity          os.FileInfo
	backupIdentity    os.FileInfo
	stageIdentity     os.FileInfo
	committedIdentity os.FileInfo
	preserveBackup    bool
}

func utf16ByteOffset(data []byte, line, character int) (int, bool) {
	if line < 0 || character < 0 || !utf8.Valid(data) {
		return 0, false
	}
	start, currentLine := 0, 0
	for currentLine < line {
		next := bytes.IndexByte(data[start:], '\n')
		if next < 0 {
			return 0, false
		}
		start += next + 1
		currentLine++
	}
	end := len(data)
	if next := bytes.IndexByte(data[start:], '\n'); next >= 0 {
		end = start + next
	}
	if end > start && data[end-1] == '\r' {
		end--
	}
	units, offset := 0, start
	for offset < end {
		if units == character {
			return offset, true
		}
		r, size := utf8.DecodeRune(data[offset:end])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > character {
			return 0, false
		}
		units += width
		offset += size
	}
	return offset, units == character
}

func writeRenameTemp(dir, pattern string, data []byte, mode os.FileMode) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	cleanup := func(cause error) (string, error) {
		identity, identityErr := file.Stat()
		closeErr := file.Close()
		if identityErr != nil {
			return path, fmt.Errorf("%w; temporary artifact identity unavailable at %s: %v", cause, path, identityErr)
		}
		if err := removeRenameArtifact(path, identity); err != nil {
			return path, fmt.Errorf("%w; temporary artifact retained at %s: %v", cause, path, err)
		}
		if closeErr != nil {
			return "", fmt.Errorf("%w; close failed: %v", cause, closeErr)
		}
		return "", cause
	}
	if err = file.Chmod(mode); err != nil {
		return cleanup(err)
	}
	if _, err = file.Write(data); err != nil {
		return cleanup(err)
	}
	if err = file.Sync(); err != nil {
		return cleanup(err)
	}
	identity, identityErr := file.Stat()
	closeErr := file.Close()
	if closeErr != nil {
		if identityErr != nil {
			return path, fmt.Errorf("%w; temporary artifact identity unavailable at %s: %v", closeErr, path, identityErr)
		}
		if removeErr := removeRenameArtifact(path, identity); removeErr != nil {
			return path, fmt.Errorf("%w; temporary artifact retained at %s: %v", closeErr, path, removeErr)
		}
		return "", closeErr
	}
	if identityErr != nil {
		return path, fmt.Errorf("temporary artifact identity unavailable at %s: %w", path, identityErr)
	}
	return path, nil
}

func rollbackNativeRename(root string, files []stagedRenameFile, committed int) []nativeRecoveryArtifact {
	var recovery []nativeRecoveryArtifact
	for i := committed - 1; i >= 0; i-- {
		file := &files[i]
		state, detail := "", ""
		_, _, currentInfo, pathErr := nativeRenamePath(root, file.path)
		switch {
		case file.backup == "":
			state, detail = "backup_unavailable", "the rollback copy is missing"
		case file.backupIdentity == nil:
			state, detail = "backup_conflict", "rollback copy identity is unavailable; left untouched"
		case pathErr != nil:
			state, detail = "rollback_conflict", "target path is no longer a regular in-workspace file; left untouched"
		case file.committedIdentity == nil || !os.SameFile(file.committedIdentity, currentInfo):
			state, detail = "rollback_conflict", "target identity no longer matches this rename; left untouched"
		default:
			backupInfo, backupStatErr := os.Lstat(file.backup)
			backupBytes, backupReadErr := os.ReadFile(file.backup)
			if backupStatErr != nil || backupReadErr != nil || backupInfo.Mode()&os.ModeSymlink != 0 || !backupInfo.Mode().IsRegular() || !os.SameFile(file.backupIdentity, backupInfo) || nativeDigest(backupBytes) != nativeDigest(file.original) {
				state, detail = "backup_conflict", "rollback copy identity or bytes changed; left untouched"
			} else {
				current, err := os.ReadFile(file.path)
				if err != nil || nativeDigest(current) != nativeDigest(file.updated) {
					state, detail = "rollback_conflict", "target bytes no longer match this rename; left untouched"
				} else if err := os.Rename(file.backup, file.path); err != nil {
					state, detail = "restore_failed", err.Error()
				} else {
					file.backup = ""
					file.backupIdentity = nil
				}
			}
		}
		if state != "" {
			file.preserveBackup = file.backup != ""
			recovery = append(recovery, nativeRecoveryArtifact{File: file.relative, Path: file.backup, Kind: "backup", State: state, Detail: detail})
		}
	}
	return recovery
}

func nativeTouched(files []stagedRenameFile) []string {
	result := make([]string, len(files))
	for i, file := range files {
		result[i] = file.relative
	}
	return result
}

func nativeDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// uriToNativePath accepts only local file authorities for native rename.
func uriToNativePath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" || parsed.User != nil || (parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost")) {
		return ""
	}
	path := parsed.Path
	if path == "" {
		return ""
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

func pathContainsGit(path string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.EqualFold(segment, ".git") {
			return true
		}
	}
	return false
}

func nativeFilesPayload(raw any) ([]string, error) {
	var values []string
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []string:
		values = v
	case []any:
		for _, item := range v {
			value, ok := item.(string)
			if !ok {
				return nil, errors.New("files must contain only strings")
			}
			values = append(values, value)
		}
	default:
		return nil, errors.New("files must be an array of paths")
	}
	if len(values) > diagnosticsMaxPaths {
		return nil, fmt.Errorf("files may contain at most %d raw paths before canonical deduplication", diagnosticsMaxPaths)
	}
	result, seen := make([]string, 0, len(values)), map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("files cannot contain an empty path")
		}
		key := filepath.Clean(value)
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func symbolRecords(raw any) ([]model.SymbolRecord, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var records []model.SymbolRecord
	if err := json.Unmarshal(encoded, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func uniqueSymbolRecords(records []model.SymbolRecord) []model.SymbolRecord {
	seen := map[string]bool{}
	result := make([]model.SymbolRecord, 0, len(records))
	for _, record := range records {
		key := strings.Join([]string{filepath.ToSlash(record.FilePath), record.Name, fmt.Sprint(record.StartLine), record.QualifiedName}, "\x00")
		if !seen[key] {
			seen[key] = true
			result = append(result, record)
		}
	}
	return result
}

func intentCodeCandidates(raw any) []model.SymbolRecord {
	items, ok := raw.([]map[string]any)
	if !ok {
		return nil
	}
	result := make([]model.SymbolRecord, 0, len(items))
	for _, item := range items {
		if item["result_kind"] != "code" {
			continue
		}
		file, _ := item["file"].(string)
		name, _ := item["symbol"].(string)
		line := intFromAny(item["line"], 0)
		if file == "" || name == "" || line < 1 {
			continue
		}
		kind, _ := item["kind"].(string)
		qualified, _ := item["qualified_name"].(string)
		result = append(result, model.SymbolRecord{FilePath: file, Name: name, Kind: kind, StartLine: line, QualifiedName: qualified, Origin: "catalog"})
	}
	return uniqueSymbolRecords(result)
}

func envelopeMapItems(raw any) ([]map[string]any, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func nativeRanges(root string, definitions, usages []map[string]any, limit int) []string {
	result, seen := []string{}, map[string]bool{}
	for _, item := range append(append([]map[string]any{}, definitions...), usages...) {
		file, _ := item["file"].(string)
		if file == "" {
			file, _ = item["path"].(string)
		}
		line := intFromAny(item["line"], 0)
		if file == "" || line < 1 {
			continue
		}
		if filepath.IsAbs(file) {
			rel, err := filepath.Rel(root, file)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			file = filepath.ToSlash(rel)
		}
		start, end := max(1, line-2), line+2
		rangeArg := fmt.Sprintf("%s:%d-%d", filepath.ToSlash(file), start, end)
		if !seen[rangeArg] {
			seen[rangeArg] = true
			result = append(result, rangeArg)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func semanticWorkerRequest(registration model.WorkspaceRegistration, target semanticTarget, backend string, request model.CommandRequest, method string) model.WorkerRequest {
	payload := clonePayload(request.Payload)
	if target.Entrypoint.Path != "" {
		switch target.Entrypoint.Kind {
		case model.EntrypointKindSolution:
			payload["solution"] = target.Entrypoint.Path
		case model.EntrypointKindProject:
			payload["project_path"] = target.Entrypoint.Path
		}
	}
	return model.WorkerRequest{
		ProtocolVersion: model.ProtocolVersion, Method: method, Workspace: registration.Root, WorkspaceName: registration.Name,
		BackendType: backend, RepoID: target.Repo.ID, RepoName: target.Repo.Name,
		RepoRoot:     filepath.Join(registration.Root, filepath.FromSlash(target.Repo.Root)),
		EntrypointID: target.Entrypoint.ID, EntrypointPath: target.Entrypoint.Path, EntrypointType: target.Entrypoint.Kind,
		Payload: payload,
	}
}

func semanticWorkerReady(statuses []model.WorkerStatus, registration model.WorkspaceRegistration, target semanticTarget, backend string) bool {
	repoRoot := filepath.Clean(filepath.Join(registration.Root, filepath.FromSlash(target.Repo.Root)))
	for _, status := range statuses {
		if status.BackendType != backend || status.PID <= 0 || filepath.Clean(status.RepoRoot) != repoRoot {
			continue
		}
		if status.WorkspaceRoot != "" && filepath.Clean(status.WorkspaceRoot) != filepath.Clean(registration.Root) {
			continue
		}
		return true
	}
	return false
}

func (a *App) scheduleNativeWarmup(parent context.Context, registration model.WorkspaceRegistration, request model.WorkerRequest) string {
	key := strings.Join([]string{registration.Root, request.RepoRoot, request.EntrypointID, request.BackendType}, "\x00")
	a.nativeWarmupMu.Lock()
	if a.nativeWarmups == nil {
		a.nativeWarmups = map[string]struct{}{}
	}
	if _, exists := a.nativeWarmups[key]; exists {
		a.nativeWarmupMu.Unlock()
		return "pending"
	}
	if len(a.nativeWarmups) >= nativeWarmupLimit {
		a.nativeWarmupMu.Unlock()
		return "not_started"
	}
	a.nativeWarmups[key] = struct{}{}
	a.nativeWarmupMu.Unlock()
	go func() {
		defer func() {
			a.nativeWarmupMu.Lock()
			delete(a.nativeWarmups, key)
			a.nativeWarmupMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), nativeWarmupTimeout)
		defer cancel()
		_, _ = a.Semantic.Call(ctx, registration, request)
	}()
	return "pending"
}

func countDiagnosticsRequested(summary *model.DiagnosticsSummary) int {
	if summary == nil {
		return 0
	}
	return summary.FilesRequested
}
func countDiagnosticsChecked(summary *model.DiagnosticsSummary) int {
	if summary == nil {
		return 0
	}
	return summary.FilesChecked
}
func countDiagnosticErrors(summary *model.DiagnosticsSummary) int {
	if summary == nil {
		return 0
	}
	return summary.Errors
}
func reasonIf(condition bool, reason string) string {
	if condition {
		return reason
	}
	return ""
}
func statusIf(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

func hasRenameRouteSelector(payload map[string]any) bool {
	for _, key := range []string{"repo", "entrypoint", "solution", "project_path"} {
		if strings.TrimSpace(stringPayload(payload, key)) != "" {
			return true
		}
	}
	return false
}

func nativeRenamePathWithinRepo(workspaceRoot, repoRoot, file string) bool {
	if workspaceRoot == "" || file == "" {
		return false
	}
	root := filepath.Join(workspaceRoot, filepath.FromSlash(repoRoot))
	rootReal, rootErr := filepath.EvalSymlinks(root)
	fileReal, fileErr := filepath.EvalSymlinks(file)
	return rootErr == nil && fileErr == nil && nativePathWithin(rootReal, fileReal)
}

func nativePathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func nativeRenameResolutionFailure(code, detail string, candidates []map[string]any) model.Envelope {
	envelope := nativeOperationFailure(code, detail)
	envelope.ContractVersion = "rename_symbol/v0"
	items := map[string]any{"status": code, "candidates": candidates, "files_touched": []string{}, "compilation_state": "not_checked"}
	if code == "selection_required" || code == "work_limit" {
		items["candidates_complete"] = false
		items["uniqueness"] = "not_proven"
	}
	envelope.Items = items
	return envelope
}

func nativeOperationFailure(code, detail string) model.Envelope {
	return model.Envelope{Ok: false, Partial: true, Degraded: true, Reason: code, Items: map[string]any{"status": "not_checked", "compilation_state": "not_checked", "files_touched": []string{}}, Error: &model.EnvelopeError{Kind: "native_operation", Code: code, ReasonCode: code, Message: detail, Detail: detail}, Warnings: []string{detail}}
}

func nativeRenameErrorCode(err error) string {
	if err == nil {
		return "unavailable"
	}
	var semanticTimeout *semanticTimeoutError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &semanticTimeout) {
		return "timeout"
	}
	message := err.Error()
	for _, code := range []string{"unsupported_operation", "invalid_response", "invalid_path", "explicit_incomplete", "work_limit", "mutation_recovery_failed"} {
		if strings.HasPrefix(message, code+":") {
			return code
		}
	}
	return "unavailable"
}

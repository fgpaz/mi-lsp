package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const affectedHeuristicWarning = "nav.affected uses conservative path, catalog, docs, and test heuristics; confidence is advisory and not complete impact proof"

type AffectedItem struct {
	Kind             string   `json:"kind"`
	Path             string   `json:"path"`
	Reason           string   `json:"reason"`
	Confidence       float64  `json:"confidence"`
	SuggestedCommand string   `json:"suggested_command,omitempty"`
	Evidence         []string `json:"evidence,omitempty"`
}

type affectedInput struct {
	Path       string
	ChangeType string
	Source     string
}

func (a *App) affected(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	fromGitDiff, _ := request.Payload["from_git_diff"].(bool)
	includeTests, _ := request.Payload["include_tests"].(bool)
	includeDocs, _ := request.Payload["include_docs"].(bool)
	quiet, _ := request.Payload["quiet"].(bool)
	changedRef := strings.TrimSpace(stringPayload(request.Payload, "changed_ref"))
	testCommand := strings.TrimSpace(stringPayload(request.Payload, "test_command"))

	inputs := map[string]affectedInput{}
	for _, path := range affectedPathsFromPayload(request.Payload["paths"]) {
		addAffectedInput(inputs, path, "", "explicit_path")
	}
	stdinPaths, stdinWarnings := affectedPathsFromStdinPayload(request.Payload["stdin"])
	for _, path := range stdinPaths {
		addAffectedInput(inputs, path, "", "stdin")
	}
	if len(inputs) == 0 && !fromGitDiff {
		fromGitDiff = true
	}

	changedLines := map[string][]int{}
	warnings := append([]string{}, stdinWarnings...)
	if fromGitDiff {
		gitRef := changedRef
		if gitRef == "" || gitRef == "HEAD" {
			gitRef = ""
		}
		fileChangeTypes, err := getGitFileChangeTypes(ctx, registration.Root, gitRef)
		if err != nil {
			return model.Envelope{}, fmt.Errorf("git affected path discovery failed: %w", err)
		}
		for path, changeType := range fileChangeTypes {
			addAffectedInput(inputs, path, changeType, "git_diff")
		}
		if lineMap, err := getGitDiffChanges(ctx, registration.Root, gitRef); err == nil {
			changedLines = lineMap
		} else {
			warnings = appendStringIfMissing(warnings, "git hunk parsing failed; symbol evidence falls back to file-level catalog heuristics")
		}
	}

	warnings = appendStringIfMissing(warnings, affectedHeuristicWarning)
	if len(inputs) == 0 {
		warnings = appendStringIfMissing(warnings, "no affected paths detected")
		env := model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "git+catalog+heuristic",
			Items:     []AffectedItem{},
			Warnings:  warnings,
			Stats:     model.Stats{Files: 0, Symbols: 0},
		}
		if !quiet {
			env.Hint = "no affected paths found; pass paths, --stdin, or create a git diff"
		}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	var db *sql.DB
	db, err = openWorkspaceDB(registration, "nav.affected")
	if err != nil {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("catalog unavailable; symbol evidence omitted: %s", err))
	} else {
		defer db.Close()
	}

	orderedInputs := make([]affectedInput, 0, len(inputs))
	for _, input := range inputs {
		orderedInputs = append(orderedInputs, input)
	}
	sort.Slice(orderedInputs, func(i, j int) bool {
		return orderedInputs[i].Path < orderedInputs[j].Path
	})

	var items []AffectedItem
	seenItems := map[string]struct{}{}
	symbolEvidenceCount := 0
	for _, input := range orderedInputs {
		evidence := []string{"source:" + input.Source}
		if input.ChangeType != "" {
			evidence = append(evidence, "change_type:"+input.ChangeType)
		}
		symbolEvidence := affectedSymbolEvidence(ctx, db, input.Path, changedLines[input.Path])
		symbolEvidenceCount += len(symbolEvidence)
		evidence = append(evidence, symbolEvidence...)

		kind := "code"
		reason := "changed source path selected from " + input.Source
		confidence := 1.0
		if isWikiPath(input.Path) {
			kind = "doc"
			reason = "changed canonical documentation path selected from " + input.Source
		} else if isTestFile(input.Path) {
			kind = "test"
			reason = "changed test path selected from " + input.Source
		}

		addAffectedItem(&items, seenItems, AffectedItem{
			Kind:       kind,
			Path:       input.Path,
			Reason:     reason,
			Confidence: confidence,
			Evidence:   evidence,
		})

		if includeTests {
			if testItem, ok := affectedTestSuggestion(input.Path, testCommand); ok {
				addAffectedItem(&items, seenItems, testItem)
			}
		}
		if includeDocs {
			for _, docItem := range affectedDocSuggestions(input.Path, registration.Name) {
				addAffectedItem(&items, seenItems, docItem)
			}
		}
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "git+catalog+heuristic",
		Items:     items,
		Warnings:  warnings,
		Stats: model.Stats{
			Files:   len(orderedInputs),
			Symbols: symbolEvidenceCount,
		},
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func affectedPathsFromPayload(value any) []string {
	switch typed := value.(type) {
	case []string:
		return normalizeAffectedPaths(typed)
	case []any:
		paths := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				paths = append(paths, text)
			}
		}
		return normalizeAffectedPaths(paths)
	case string:
		return normalizeAffectedPaths(splitAffectedPathText(typed))
	default:
		return nil
	}
}

func affectedPathsFromStdinPayload(value any) ([]string, []string) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		var paths []string
		if err := json.Unmarshal([]byte(trimmed), &paths); err == nil {
			return normalizeAffectedPaths(paths), nil
		}
		var object struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal([]byte(trimmed), &object); err == nil && len(object.Paths) > 0 {
			return normalizeAffectedPaths(object.Paths), nil
		}
		return normalizeAffectedPaths(splitAffectedPathText(trimmed)), []string{"stdin was parsed as newline/comma separated paths, not JSON"}
	default:
		return affectedPathsFromPayload(value), nil
	}
}

func splitAffectedPathText(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ','
	})
	return fields
}

func normalizeAffectedPaths(paths []string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, path := range paths {
		clean := strings.Trim(strings.TrimSpace(path), "\"'")
		clean = filepath.ToSlash(clean)
		clean = strings.TrimPrefix(clean, "./")
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
	}
	sort.Strings(normalized)
	return normalized
}

func addAffectedInput(inputs map[string]affectedInput, path string, changeType string, source string) {
	normalized := normalizeAffectedPaths([]string{path})
	if len(normalized) == 0 {
		return
	}
	path = normalized[0]
	if shouldIgnoreAffectedPath(path) {
		return
	}
	if existing, ok := inputs[path]; ok {
		if existing.ChangeType == "" {
			existing.ChangeType = changeType
		} else {
			existing.ChangeType = mergeChangeType(existing.ChangeType, changeType)
		}
		if !strings.Contains(existing.Source, source) {
			existing.Source += "+" + source
		}
		inputs[path] = existing
		return
	}
	inputs[path] = affectedInput{Path: path, ChangeType: changeType, Source: source}
}

func addAffectedItem(items *[]AffectedItem, seen map[string]struct{}, item AffectedItem) {
	key := item.Kind + "\x00" + item.Path + "\x00" + item.SuggestedCommand
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*items = append(*items, item)
}

func affectedSymbolEvidence(ctx context.Context, db *sql.DB, path string, changedLines []int) []string {
	if db == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var evidence []string
	for _, line := range changedLines {
		symbol, found, err := store.SymbolContainingLine(ctx, db, path, line)
		if err != nil || !found {
			continue
		}
		key := symbol.Name + ":" + symbol.Kind
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		evidence = append(evidence, "symbol:"+symbol.Name+"("+symbol.Kind+")")
		if len(evidence) >= 5 {
			return evidence
		}
	}
	if len(evidence) > 0 {
		return evidence
	}
	symbols, err := store.SymbolsByFile(ctx, db, path, 5, 0)
	if err != nil {
		return nil
	}
	for _, symbol := range symbols {
		key := symbol.Name + ":" + symbol.Kind
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		evidence = append(evidence, "symbol:"+symbol.Name+"("+symbol.Kind+")")
	}
	return evidence
}

func affectedTestSuggestion(path string, overrideCommand string) (AffectedItem, bool) {
	if isWikiPath(path) {
		return AffectedItem{}, false
	}
	command := strings.TrimSpace(overrideCommand)
	testPath := filepath.ToSlash(filepath.Dir(path))
	if testPath == "." {
		testPath = ""
	}
	reason := "include_tests requested; package-level test command inferred from changed path"
	confidence := 0.75
	if command == "" {
		switch {
		case strings.HasSuffix(path, ".go"):
			if testPath == "" {
				command = "go test ."
			} else {
				command = "go test ./" + testPath
			}
		case strings.HasSuffix(strings.ToLower(path), ".cs"):
			command = "dotnet test worker-dotnet/MiLsp.Worker.sln"
			reason = "include_tests requested; .NET solution test command inferred from C# path"
			confidence = 0.65
		case strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx"):
			if testPath == "" {
				command = "npm test"
			} else {
				command = "npm test -- " + testPath
			}
			confidence = 0.55
		default:
			return AffectedItem{}, false
		}
	} else {
		reason = "include_tests requested; command supplied by --test-command"
		confidence = 0.9
	}
	if testPath == "" {
		testPath = "."
	}
	return AffectedItem{
		Kind:             "test",
		Path:             testPath,
		Reason:           reason,
		Confidence:       confidence,
		SuggestedCommand: command,
		Evidence:         []string{"source_path:" + path},
	}, true
}

func affectedDocSuggestions(path string, workspaceName string) []AffectedItem {
	triggerPath := path
	doc := func(docPath string, reason string, confidence float64, command string) AffectedItem {
		return AffectedItem{
			Kind:             "doc",
			Path:             docPath,
			Reason:           reason,
			Confidence:       confidence,
			SuggestedCommand: command,
			Evidence:         []string{"trigger_path:" + triggerPath},
		}
	}
	validateCommand := fmt.Sprintf("mi-lsp nav wiki validate-harness --workspace %s --format toon", workspaceName)
	traceCommand := fmt.Sprintf("mi-lsp nav wiki trace RF-QRY-017 --workspace %s --format toon", workspaceName)

	var items []AffectedItem
	add := func(item AffectedItem) {
		items = append(items, item)
	}
	switch {
	case strings.HasPrefix(path, ".docs/wiki/04_RF/"):
		add(doc(".docs/wiki/04_RF.md", "RF index may need catalog sync for changed RF doc", 0.85, traceCommand))
		add(doc(".docs/wiki/06_matriz_pruebas_RF.md", "RF change may need test matrix sync", 0.8, validateCommand))
		add(doc(".docs/wiki/06_pruebas/TP-QRY.md", "query RF change may need TP-QRY coverage sync", 0.8, validateCommand))
	case strings.HasPrefix(path, ".docs/wiki/06_"):
		add(doc(".docs/wiki/06_matriz_pruebas_RF.md", "test-plan change may need matrix sync", 0.8, validateCommand))
	case isWikiPath(path):
		add(doc(path, "changed canonical wiki path should be validated", 1.0, validateCommand))
	case strings.HasPrefix(path, "internal/cli/"):
		add(doc(".docs/wiki/09_contratos_tecnicos.md", "CLI surface changed; root technical contract should be reviewed", 0.85, validateCommand))
		add(doc(".docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md", "CLI command/envelope boundary changed", 0.85, validateCommand))
	case strings.HasPrefix(path, "internal/store/"):
		add(doc(".docs/wiki/08_modelo_fisico_datos.md", "store/catalog persistence changed; physical model should be reviewed", 0.85, validateCommand))
		add(doc(".docs/wiki/08_db/DB-SYMBOL-EDGE-GRAPH.md", "symbol graph persistence proposal may be affected by store changes", 0.65, validateCommand))
	case strings.HasPrefix(path, "internal/daemon/"):
		add(doc(".docs/wiki/07_baseline_tecnica.md", "daemon runtime behavior changed; technical baseline should be reviewed", 0.8, validateCommand))
		add(doc(".docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md", "daemon/governance detail may need sync", 0.7, validateCommand))
	case strings.HasPrefix(path, "worker-dotnet/"):
		add(doc(".docs/wiki/09_contratos/CT-DAEMON-WORKER.md", "worker protocol/runtime boundary changed", 0.85, validateCommand))
	case strings.HasPrefix(path, "internal/service/"):
		add(doc(".docs/wiki/04_RF/RF-QRY-017.md", "query service behavior changed; RF-QRY-017 should be reviewed for affected selector scope", 0.75, traceCommand))
		add(doc(".docs/wiki/06_pruebas/TP-QRY.md", "query service behavior changed; TP-QRY tests may need sync", 0.75, validateCommand))
		add(doc(".docs/wiki/09_contratos_tecnicos.md", "nav service output/envelope changed; technical contracts should be reviewed", 0.75, validateCommand))
	}
	return items
}

func isWikiPath(path string) bool {
	return strings.HasPrefix(path, ".docs/wiki/")
}

func shouldIgnoreAffectedPath(path string) bool {
	return path == ".mi-lsp" ||
		strings.HasPrefix(path, ".mi-lsp/") ||
		path == ".git" ||
		strings.HasPrefix(path, ".git/") ||
		strings.HasPrefix(path, ".docs/auditoria/") ||
		strings.HasPrefix(path, ".docs/raw/")
}

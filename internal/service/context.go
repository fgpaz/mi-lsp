package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const (
	defaultContextRadius   = 2
	defaultContextMaxChars = 500
)

func (a *App) contextQuery(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	file, _ := request.Payload["file"].(string)
	if strings.TrimSpace(file) == "" {
		return model.Envelope{}, fmt.Errorf("file is required")
	}
	line := intFromAny(request.Payload["line"], 1)
	item, warnings, err := buildContextSliceItem(registration.Root, project, file, line, request.Context.MaxChars)
	if err != nil {
		return model.Envelope{}, err
	}

	backend := "text"
	stats := model.Stats{Files: 1}
	backendType := resolveContextBackendType(request)
	if backendType == "text" {
		stats.Ms = time.Since(started).Milliseconds()
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Items: []map[string]any{item}, Warnings: warnings, Stats: stats}, nil
	}

	if backendType == "catalog" {
		if mergeCatalogContextItem(ctx, registration, file, line, item) {
			backend = "catalog"
			warnings = append(warnings, "served from catalog fallback")
			stats.Symbols = 1
		}
		stats.Ms = time.Since(started).Milliseconds()
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Items: []map[string]any{item}, Warnings: warnings, Stats: stats}, nil
	}

	target, targetEnvelope, err := a.resolveSemanticTarget(ctx, registration, project, request, "get_context", backendType)
	if err != nil {
		return model.Envelope{}, err
	}
	if targetEnvelope != nil {
		warnings = append(warnings, targetEnvelope.Warnings...)
		stats.Ms = time.Since(started).Milliseconds()
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Items: []map[string]any{item}, Warnings: warnings, Stats: stats, NextHint: targetEnvelope.NextHint}, nil
	}

	payload := clonePayload(request.Payload)
	if target.Entrypoint.Path != "" {
		switch target.Entrypoint.Kind {
		case model.EntrypointKindSolution:
			payload["solution"] = target.Entrypoint.Path
		case model.EntrypointKindProject:
			payload["project_path"] = target.Entrypoint.Path
		}
	}
	workerRequest := model.WorkerRequest{
		ProtocolVersion: model.ProtocolVersion,
		Method:          "get_context",
		Workspace:       registration.Root,
		WorkspaceName:   registration.Name,
		BackendType:     backendType,
		RepoID:          target.Repo.ID,
		RepoName:        target.Repo.Name,
		RepoRoot:        filepath.Join(registration.Root, filepath.FromSlash(target.Repo.Root)),
		EntrypointID:    target.Entrypoint.ID,
		EntrypointPath:  target.Entrypoint.Path,
		EntrypointType:  target.Entrypoint.Kind,
		Payload:         payload,
	}

	response, semErr := a.Semantic.Call(ctx, registration, workerRequest)
	if semErr == nil {
		backend = response.Backend
		if len(response.Items) > 0 {
			mergeContextItem(item, response.Items[0])
			stats.Symbols = max(stats.Symbols, len(response.Items))
		}
		warnings = append(warnings, response.Warnings...)
		if stats.Symbols == 0 && mergeCatalogContextItem(ctx, registration, file, line, item) {
			warnings = append(warnings, "served from catalog fallback")
			stats.Symbols = 1
		}
		stats.Ms = time.Since(started).Milliseconds()
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Items: []map[string]any{item}, Warnings: warnings, Stats: stats}, nil
	}

	warnings = append(warnings, semanticBackendWarning(backendType, semErr))
	if mergeCatalogContextItem(ctx, registration, file, line, item) {
		backend = "catalog"
		warnings = append(warnings, "served from catalog fallback")
		stats.Symbols = 1
	}
	stats.Ms = time.Since(started).Milliseconds()
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: backend, Items: []map[string]any{item}, Warnings: warnings, Stats: stats}, nil
}

func resolveContextBackendType(request model.CommandRequest) string {
	file, _ := request.Payload["file"].(string)
	if !isSemanticContextFile(file) {
		return "text"
	}
	if explicit := strings.ToLower(strings.TrimSpace(request.Context.BackendHint)); explicit != "" {
		return explicit
	}
	if isTypeScriptFile(file) {
		return "tsserver"
	}
	if isPythonFile(file) {
		return "pyright"
	}
	return "roslyn"
}

func isSemanticContextFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cs", ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".py", ".pyi":
		return true
	default:
		return false
	}
}

func buildContextSliceItem(workspaceRoot string, project model.ProjectFile, file string, line int, maxChars int) (map[string]any, []string, error) {
	absoluteFile := file
	if !filepath.IsAbs(absoluteFile) {
		absoluteFile = filepath.Join(workspaceRoot, filepath.FromSlash(file))
	}
	focusLine, startLine, endLine, sliceText, err := readContextWindow(absoluteFile, line, defaultContextRadius)
	if err != nil {
		return nil, nil, err
	}
	warnings := []string{}
	if maxChars <= 0 {
		maxChars = defaultContextMaxChars
	}
	if len(sliceText) > maxChars {
		if maxChars > 3 {
			sliceText = sliceText[:maxChars-3] + "..."
		} else {
			sliceText = sliceText[:maxChars]
		}
		warnings = append(warnings, "context slice truncated to max_chars")
	}

	displayFile, relErr := makeRelative(workspaceRoot, absoluteFile)
	if relErr != nil {
		displayFile = filepath.Clean(absoluteFile)
	}
	item := map[string]any{
		"file":             displayFile,
		"line":             focusLine,
		"focus_line":       focusLine,
		"slice_start_line": startLine,
		"slice_end_line":   endLine,
		"slice_text":       sliceText,
	}
	if repo, ok := workspace.FindRepoByFile(project, workspaceRoot, absoluteFile); ok {
		item["repo"] = repo.Name
	}
	return item, warnings, nil
}

func readContextWindow(path string, targetLine int, radius int) (int, int, int, string, error) {
	if targetLine < 1 {
		targetLine = 1
	}
	if radius < 0 {
		radius = 0
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, "", err
	}
	defer file.Close()

	startTarget := targetLine - radius
	if startTarget < 1 {
		startTarget = 1
	}
	endTarget := targetLine + radius
	reader := bufio.NewReaderSize(file, 64*1024)
	var window []string
	var tail []string
	lineNo := 0
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			line = strings.TrimRight(strings.ReplaceAll(line, "\r\n", "\n"), "\n")
			line = strings.TrimRight(line, "\r")
			if lineNo >= startTarget && lineNo <= endTarget {
				window = append(window, line)
			}
			tail = append(tail, line)
			if maxTail := radius*2 + 1; len(tail) > maxTail {
				tail = tail[1:]
			}
			if lineNo > endTarget {
				break
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return 0, 0, 0, "", readErr
		}
	}
	if lineNo == 0 {
		return 1, 1, 1, "", nil
	}
	focusLine := targetLine
	if focusLine > lineNo {
		focusLine = lineNo
		window = tail
	}
	startLine := focusLine - radius
	if startLine < 1 {
		startLine = 1
	}
	endLine := focusLine + radius
	if endLine > lineNo {
		endLine = lineNo
	}
	if targetLine > lineNo {
		startLine = endLine - len(window) + 1
		if startLine < 1 {
			startLine = 1
		}
	}
	return focusLine, startLine, endLine, strings.Join(window, "\n"), nil
}

func mergeCatalogContextItem(ctx context.Context, registration model.WorkspaceRegistration, file string, line int, item map[string]any) bool {
	relativeFile, err := makeRelative(registration.Root, file)
	if err != nil {
		return false
	}
	db, err := openWorkspaceDB(registration, "nav.context")
	if err != nil {
		return false
	}
	defer db.Close()

	symbols, err := store.SymbolsByFile(ctx, db, relativeFile, DefaultConfig().DefaultSearchLimit, 0)
	if err != nil || len(symbols) == 0 {
		return false
	}
	best := symbols[0]
	for _, symbol := range symbols {
		if symbol.StartLine <= line {
			best = symbol
		}
	}
	mergeContextItem(item, map[string]any{
		"name":           best.Name,
		"kind":           best.Kind,
		"scope":          best.Scope,
		"signature":      best.Signature,
		"qualified_name": best.QualifiedName,
		"repo":           best.RepoName,
	})
	return true
}

func mergeContextItem(item map[string]any, overlay map[string]any) {
	protected := map[string]struct{}{
		"file":             {},
		"line":             {},
		"focus_line":       {},
		"slice_start_line": {},
		"slice_end_line":   {},
		"slice_text":       {},
	}
	for key, value := range overlay {
		if _, skip := protected[key]; skip {
			continue
		}
		if value == nil {
			continue
		}
		item[key] = value
	}
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

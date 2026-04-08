package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type symbolWithContent struct {
	File        string `json:"file"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Line        int    `json:"line"`
	EndLine     int    `json:"end_line,omitempty"`
	Content     string `json:"content,omitempty"`
	ContentMode string `json:"content_mode,omitempty"`
}

type symbolNeighborhood struct {
	Symbol       string              `json:"symbol"`
	Definition   *symbolWithContent  `json:"definition,omitempty"`
	Implementors []symbolWithContent `json:"implementors,omitempty"`
	Callers      []symbolWithContent `json:"callers,omitempty"`
	Tests        []symbolWithContent `json:"tests,omitempty"`
}

func (a *App) related(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	symbolName, _ := request.Payload["symbol"].(string)
	if symbolName == "" {
		return model.Envelope{}, fmt.Errorf("symbol name is required")
	}

	depthStr, _ := request.Payload["depth"].(string)
	depth := parseDepth(depthStr)

	warnings := []string{}
	neighborhood := symbolNeighborhood{Symbol: symbolName}
	backend := "catalog"

	// Open catalog DB
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, fmt.Errorf("opening catalog: %w", err)
	}
	defer db.Close()

	// 1. Find definition in catalog
	symbols, err := store.FindSymbols(ctx, db, symbolName, "", true, 5, 0)
	if err == nil && len(symbols) > 0 {
		def := symbolToContent(registration.Root, symbols[0])
		neighborhood.Definition = &def
	}

	// 2. Find implementors/callers via semantic backend (if requested and available)
	if depth.implementors || depth.callers {
		refsItems, refsBackend, refsWarnings := a.findRefsForRelated(ctx, registration, project, request, symbolName, symbols)
		warnings = append(warnings, refsWarnings...)
		if refsBackend != "" {
			backend = refsBackend
		}

		if depth.callers {
			neighborhood.Callers = filterRefsByRole(refsItems, registration.Root, "caller")
		}
		if depth.implementors {
			neighborhood.Implementors = filterRefsByRole(refsItems, registration.Root, "implementor")
		}
	}

	// 3. Find tests via text search (if requested)
	if depth.tests {
		testItems := a.findTestsForSymbol(ctx, registration, project, symbolName)
		neighborhood.Tests = testItems
	}

	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   backend,
		Items:     []symbolNeighborhood{neighborhood},
		Warnings:  warnings,
		Stats:     model.Stats{Ms: time.Since(started).Milliseconds()},
	}, nil
}

type relatedDepth struct {
	definition   bool
	implementors bool
	callers      bool
	tests        bool
}

func parseDepth(s string) relatedDepth {
	if s == "" {
		return relatedDepth{definition: true, implementors: true, callers: true, tests: true}
	}
	d := relatedDepth{}
	for _, part := range strings.Split(s, ",") {
		switch strings.TrimSpace(part) {
		case "definition":
			d.definition = true
		case "implementors":
			d.implementors = true
		case "callers":
			d.callers = true
		case "tests":
			d.tests = true
		}
	}
	return d
}

func symbolToContent(workspaceRoot string, sym model.SymbolRecord) symbolWithContent {
	sc := symbolWithContent{
		File:    sym.FilePath,
		Name:    sym.Name,
		Kind:    sym.Kind,
		Line:    sym.StartLine,
		EndLine: sym.EndLine,
	}

	// Try to read the symbol body
	if sym.StartLine > 0 && sym.EndLine > 0 {
		absFile := sym.FilePath
		if !filepath.IsAbs(absFile) {
			absFile = filepath.Join(workspaceRoot, filepath.FromSlash(sym.FilePath))
		}
		content, _, err := readFileLineRange(absFile, sym.StartLine, sym.EndLine)
		if err == nil {
			sc.Content = content
			sc.ContentMode = "symbol"
		}
	}
	return sc
}

func (a *App) findRefsForRelated(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, origRequest model.CommandRequest, symbolName string, catalogSymbols []model.SymbolRecord) ([]map[string]any, string, []string) {
	// Build a semantic refs request
	payload := map[string]any{"symbol": symbolName}

	// If we found the symbol in catalog, provide file/line hint
	if len(catalogSymbols) > 0 {
		payload["file"] = catalogSymbols[0].FilePath
		payload["line"] = catalogSymbols[0].StartLine
	}

	// Copy semantic selectors from original request
	for _, key := range []string{"repo", "entrypoint", "solution", "project_path"} {
		if v, ok := origRequest.Payload[key]; ok {
			payload[key] = v
		}
	}

	refsRequest := model.CommandRequest{
		Operation: "nav.refs",
		Context:   origRequest.Context,
		Payload:   payload,
	}

	envelope, err := a.semantic(ctx, refsRequest, "find_refs")
	if err != nil {
		return nil, "", []string{fmt.Sprintf("semantic backend unavailable for refs: %s", err)}
	}

	items, ok := envelope.Items.([]map[string]any)
	if !ok {
		return nil, envelope.Backend, envelope.Warnings
	}
	return items, envelope.Backend, envelope.Warnings
}

func filterRefsByRole(items []map[string]any, workspaceRoot string, role string) []symbolWithContent {
	// Without semantic classification, all refs are treated as callers
	// Implementors would need type hierarchy info from Roslyn
	result := make([]symbolWithContent, 0)
	for _, item := range items {
		file, _ := item["file"].(string)
		name, _ := item["name"].(string)
		kind, _ := item["kind"].(string)
		line := intFromAny(item["line"], 0)
		endLine := intFromAny(item["end_line"], 0)

		if role == "implementor" {
			// Skip unless this looks like an implementation (class/struct containing the interface name)
			if kind != "class" && kind != "struct" && kind != "record" {
				continue
			}
		}

		sc := symbolWithContent{
			File:    file,
			Name:    name,
			Kind:    kind,
			Line:    line,
			EndLine: endLine,
		}

		// Read content for the ref
		if line > 0 {
			absFile := file
			if !filepath.IsAbs(absFile) {
				absFile = filepath.Join(workspaceRoot, filepath.FromSlash(file))
			}
			start := line
			end := endLine
			if end <= 0 {
				end = line + 20 // context fallback
			}
			content, _, err := readFileLineRange(absFile, start, end)
			if err == nil {
				sc.Content = content
				sc.ContentMode = "symbol"
			}
		}

		result = append(result, sc)
	}
	return result
}

func (a *App) findTestsForSymbol(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, symbolName string) []symbolWithContent {
	// Search for symbol name in test files
	items, err := searchPattern(ctx, registration.Root, project, symbolName, false, 20)
	if err != nil || len(items) == 0 {
		return nil
	}

	tests := make([]symbolWithContent, 0)
	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		file, _ := item["file"].(string)
		if !isTestFile(file) {
			continue
		}
		line := intFromAny(item["line"], 0)
		text, _ := item["text"].(string)

		tests = append(tests, symbolWithContent{
			File:        file,
			Name:        text,
			Kind:        "test_reference",
			Line:        line,
			ContentMode: "lines",
		})
	}
	return tests
}

func isTestFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "test") ||
		strings.Contains(lower, ".spec.") ||
		strings.Contains(lower, "_test.") ||
		strings.HasSuffix(lower, "tests.cs") ||
		strings.HasSuffix(lower, "test.ts") ||
		strings.HasSuffix(lower, "test.tsx")
}

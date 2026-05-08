package service

import (
	"bufio"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func enrichSearchResultsWithContent(ctx context.Context, registration model.WorkspaceRegistration, items []map[string]any, contextLines int, contextMode string) []string {
	var warnings []string
	var sqlDB *sql.DB

	// Open catalog DB for symbol lookup (hybrid/symbol modes)
	if contextMode != "lines" {
		opened, err := openWorkspaceDB(registration, "nav.search content")
		if err != nil {
			warnings = append(warnings, "catalog unavailable for symbol-based content; falling back to line-based")
			contextMode = "lines"
		} else {
			sqlDB = opened
			defer sqlDB.Close()
		}
	}

	lineItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		if tryEnrichSearchResultWithSymbol(ctx, registration.Root, sqlDB, item, contextMode) {
			continue
		}
		if contextMode != "symbol" {
			lineItems = append(lineItems, item)
		}
	}
	if len(lineItems) > 0 && ctx.Err() == nil {
		enrichLineSearchResultsWithContent(registration.Root, lineItems, contextLines)
	}
	return warnings
}

func enrichSingleSearchResult(ctx context.Context, workspaceRoot string, db *sql.DB, item map[string]any, contextLines int, contextMode string) {
	if tryEnrichSearchResultWithSymbol(ctx, workspaceRoot, db, item, contextMode) || contextMode == "symbol" {
		return
	}
	enrichLineSearchResultsWithContent(workspaceRoot, []map[string]any{item}, contextLines)
}

func tryEnrichSearchResultWithSymbol(ctx context.Context, workspaceRoot string, db *sql.DB, item map[string]any, contextMode string) bool {
	fileRel, _ := item["file"].(string)
	lineNum, _ := item["line"].(int)
	if fileRel == "" || lineNum == 0 {
		return true
	}

	absFile := fileRel
	if !filepath.IsAbs(absFile) {
		absFile = filepath.Join(workspaceRoot, filepath.FromSlash(fileRel))
	}

	// Try symbol-based content first (hybrid or symbol mode)
	if contextMode == "hybrid" || contextMode == "symbol" {
		if db != nil {
			symbol, found, err := store.SymbolContainingLine(ctx, db, fileRel, lineNum)
			if err == nil && found && symbol.StartLine > 0 && symbol.EndLine > 0 {
				content, lineCount, err := readFileLineRange(absFile, symbol.StartLine, symbol.EndLine)
				if err == nil {
					item["content"] = content
					item["content_mode"] = "symbol"
					item["symbol_name"] = symbol.Name
					item["symbol_kind"] = symbol.Kind
					item["content_start_line"] = symbol.StartLine
					item["content_end_line"] = symbol.EndLine
					item["content_line_count"] = lineCount
					return true
				}
			}
		}
	}
	return false
}

type searchContentRequest struct {
	item      map[string]any
	startLine int
	endLine   int
}

func enrichLineSearchResultsWithContent(workspaceRoot string, items []map[string]any, contextLines int) {
	requestsByFile := map[string][]searchContentRequest{}
	for _, item := range items {
		fileRel, _ := item["file"].(string)
		lineNum, _ := item["line"].(int)
		if fileRel == "" || lineNum == 0 {
			continue
		}
		absFile := fileRel
		if !filepath.IsAbs(absFile) {
			absFile = filepath.Join(workspaceRoot, filepath.FromSlash(fileRel))
		}
		startLine := lineNum - contextLines
		if startLine < 1 {
			startLine = 1
		}
		requestsByFile[absFile] = append(requestsByFile[absFile], searchContentRequest{
			item:      item,
			startLine: startLine,
			endLine:   lineNum + contextLines,
		})
	}

	for absFile, requests := range requestsByFile {
		enrichFileLineRanges(absFile, requests, contextLines)
	}
}

func enrichFileLineRanges(absFile string, requests []searchContentRequest, contextLines int) {
	if len(requests) == 0 {
		return
	}
	file, err := os.Open(absFile)
	if err != nil {
		return
	}
	defer file.Close()

	maxEndLine := 0
	for _, request := range requests {
		if request.endLine > maxEndLine {
			maxEndLine = request.endLine
		}
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := map[int]string{}
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine > maxEndLine {
			break
		}
		lines[currentLine] = scanner.Text()
	}
	if scanner.Err() != nil {
		return
	}

	for _, request := range requests {
		var builder strings.Builder
		lineCount := 0
		for line := request.startLine; line <= request.endLine; line++ {
			text, ok := lines[line]
			if !ok {
				continue
			}
			if lineCount > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(text)
			lineCount++
		}
		if lineCount == 0 {
			continue
		}
		request.item["content"] = builder.String()
		request.item["content_mode"] = "lines"
		request.item["context_lines"] = contextLines
		request.item["content_start_line"] = request.startLine
		request.item["content_end_line"] = request.startLine + lineCount - 1
		request.item["content_line_count"] = lineCount
	}
}

func readFileLineRange(absPath string, startLine, endLine int) (string, int, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var builder strings.Builder
	currentLine := 0
	collectedLines := 0

	for scanner.Scan() {
		currentLine++
		if currentLine < startLine {
			continue
		}
		if endLine > 0 && currentLine > endLine {
			break
		}

		if collectedLines > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(scanner.Text())
		collectedLines++
	}

	return builder.String(), collectedLines, scanner.Err()
}

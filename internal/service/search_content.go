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
		opened, err := store.Open(registration.Root)
		if err != nil {
			warnings = append(warnings, "catalog unavailable for symbol-based content; falling back to line-based")
			contextMode = "lines"
		} else {
			sqlDB = opened
			defer sqlDB.Close()
		}
	}

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		enrichSingleSearchResult(ctx, registration.Root, sqlDB, item, contextLines, contextMode)
	}
	return warnings
}

func enrichSingleSearchResult(ctx context.Context, workspaceRoot string, db *sql.DB, item map[string]any, contextLines int, contextMode string) {
	fileRel, _ := item["file"].(string)
	lineNum, _ := item["line"].(int)
	if fileRel == "" || lineNum == 0 {
		return
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
					return
				}
			}
		}
		// If symbol mode only and no symbol found, skip content
		if contextMode == "symbol" {
			return
		}
	}

	// Fallback to line-based context
	startLine := lineNum - contextLines
	if startLine < 1 {
		startLine = 1
	}
	endLine := lineNum + contextLines

	content, lineCount, err := readFileLineRange(absFile, startLine, endLine)
	if err != nil {
		return
	}

	item["content"] = content
	item["content_mode"] = "lines"
	item["context_lines"] = contextLines
	item["content_start_line"] = startLine
	item["content_end_line"] = startLine + lineCount - 1
	item["content_line_count"] = lineCount
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

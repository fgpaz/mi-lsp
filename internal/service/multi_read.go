package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type fileRange struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type multiReadItem struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	LineCount int    `json:"line_count"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (a *App) multiRead(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	ranges, err := parseMultiReadPayload(request)
	if err != nil {
		return model.Envelope{}, err
	}
	if len(ranges) == 0 {
		return model.Envelope{}, errors.New("at least one file range is required")
	}

	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = DefaultConfig().DefaultMaxItems
	}
	maxChars := request.Context.MaxChars
	if maxChars <= 0 {
		maxChars = 200_000 // generous default for multi-read
	}
	if maxChars > 1_000_000 {
		maxChars = 1_000_000
	}

	items := make([]multiReadItem, 0, len(ranges))
	totalChars := 0
	truncated := false

	for i, fr := range ranges {
		if i >= maxItems {
			truncated = true
			break
		}
		if ctx.Err() != nil {
			break
		}

		absFile := fr.File
		if !filepath.IsAbs(absFile) {
			absFile = filepath.Join(registration.Root, filepath.FromSlash(fr.File))
		}
		absFile = filepath.Clean(absFile)
		if !strings.HasPrefix(absFile, filepath.Clean(registration.Root)+string(os.PathSeparator)) && absFile != filepath.Clean(registration.Root) {
			items = append(items, multiReadItem{File: fr.File, Content: "error: path outside workspace root", StartLine: fr.StartLine, EndLine: fr.EndLine})
			continue
		}

		content, lineCount, itemTruncated, readErr := readFileRange(absFile, fr.StartLine, fr.EndLine, maxChars-totalChars)
		if readErr != nil {
			// Include error as content so caller knows which file failed
			items = append(items, multiReadItem{
				File:      fr.File,
				StartLine: fr.StartLine,
				EndLine:   fr.EndLine,
				Content:   fmt.Sprintf("error: %s", readErr),
				LineCount: 0,
			})
			continue
		}

		totalChars += len(content)
		items = append(items, multiReadItem{
			File:      fr.File,
			StartLine: fr.StartLine,
			EndLine:   fr.EndLine,
			Content:   content,
			LineCount: lineCount,
			Truncated: itemTruncated,
		})

		if totalChars >= maxChars {
			truncated = true
			break
		}
	}

	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "text",
		Items:     items,
		Truncated: truncated,
		Stats:     model.Stats{Files: len(items), Ms: time.Since(started).Milliseconds()},
	}, nil
}

func parseMultiReadPayload(request model.CommandRequest) ([]fileRange, error) {
	// Support stdin reading
	if stdinFlag, _ := request.Payload["stdin"].(bool); stdinFlag {
		limReader := io.LimitReader(os.Stdin, 10*1024*1024) // 10MB max
		data, err := io.ReadAll(limReader)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		var items []string
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing stdin JSON array: %w", err)
		}
		ranges := make([]fileRange, 0, len(items))
		for _, s := range items {
			fr, err := parseFileRangeString(s)
			if err != nil {
				return nil, err
			}
			ranges = append(ranges, fr)
		}
		return ranges, nil
	}

	// Support items as JSON array in payload
	if rawItems, ok := request.Payload["items"]; ok {
		switch v := rawItems.(type) {
		case []any:
			return parseFileRangesFromSlice(v)
		case []string:
			ranges := make([]fileRange, 0, len(v))
			for _, s := range v {
				fr, err := parseFileRangeString(s)
				if err != nil {
					return nil, err
				}
				ranges = append(ranges, fr)
			}
			return ranges, nil
		}
	}

	// Support args as positional arguments
	if rawArgs, ok := request.Payload["args"]; ok {
		switch v := rawArgs.(type) {
		case []any:
			return parseFileRangesFromSlice(v)
		case []string:
			ranges := make([]fileRange, 0, len(v))
			for _, s := range v {
				fr, err := parseFileRangeString(s)
				if err != nil {
					return nil, err
				}
				ranges = append(ranges, fr)
			}
			return ranges, nil
		}
	}

	return nil, errors.New("items or args required: provide file ranges as file:startLine-endLine")
}

func parseFileRangesFromSlice(items []any) ([]fileRange, error) {
	ranges := make([]fileRange, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("expected string file range, got %T", item)
		}
		fr, err := parseFileRangeString(s)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, fr)
	}
	return ranges, nil
}

func parseFileRangeString(s string) (fileRange, error) {
	// Format: file:startLine-endLine or file:startLine or file (whole file)
	// Handles Windows paths like C:\path\file.go:10-20
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, "\n\r") {
		return fileRange{}, fmt.Errorf("invalid path: contains newline in %q", s)
	}
	colonIdx := strings.LastIndex(s, ":")
	if colonIdx == -1 || colonIdx == len(s)-1 {
		// Whole file
		file := s
		if colonIdx == len(s)-1 {
			file = s[:colonIdx]
		}
		return fileRange{File: file, StartLine: 1, EndLine: 0}, nil // 0 = to end
	}

	// Check for Windows drive letter (single letter before first colon)
	if colonIdx == 1 && len(s) > 2 && (s[0] >= 'A' && s[0] <= 'Z' || s[0] >= 'a' && s[0] <= 'z') {
		// This is a Windows drive letter (C:), not a file:line separator
		nextColon := strings.Index(s[2:], ":")
		if nextColon == -1 {
			// No range specified, whole file
			return fileRange{File: s, StartLine: 1, EndLine: 0}, nil
		}
		file := s[:2+nextColon]
		rangeStr := s[2+nextColon+1:]
		return parseRangeString(file, rangeStr, s)
	}

	file := s[:colonIdx]
	rangeStr := s[colonIdx+1:]
	return parseRangeString(file, rangeStr, s)
}

func parseRangeString(file, rangeStr, fullStr string) (fileRange, error) {
	dashIdx := strings.Index(rangeStr, "-")
	if dashIdx == -1 {
		// Single line
		line, err := strconv.Atoi(rangeStr)
		if err != nil {
			return fileRange{}, fmt.Errorf("invalid line number in %q: %w", fullStr, err)
		}
		return fileRange{File: file, StartLine: line, EndLine: line}, nil
	}

	startStr := rangeStr[:dashIdx]
	endStr := rangeStr[dashIdx+1:]
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return fileRange{}, fmt.Errorf("invalid start line in %q: %w", fullStr, err)
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		return fileRange{}, fmt.Errorf("invalid end line in %q: %w", fullStr, err)
	}
	return fileRange{File: file, StartLine: start, EndLine: end}, nil
}

func readFileRange(absPath string, startLine int, endLine int, charBudget int) (content string, lineCount int, truncated bool, err error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", 0, false, err
	}
	defer file.Close()

	if charBudget <= 0 {
		return "", 0, true, nil
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var builder strings.Builder
	currentLine := 0
	collectedLines := 0

	for scanner.Scan() {
		currentLine++
		if startLine > 0 && currentLine < startLine {
			continue
		}
		if endLine > 0 && currentLine > endLine {
			break
		}

		if collectedLines > 0 {
			builder.WriteByte('\n')
		}
		line := scanner.Text()

		if builder.Len()+len(line)+1 > charBudget {
			remaining := charBudget - builder.Len()
			if remaining > 0 {
				builder.WriteString(line[:min(remaining, len(line))])
			}
			collectedLines++
			return builder.String(), collectedLines, true, nil
		}

		builder.WriteString(line)
		collectedLines++
	}

	if err := scanner.Err(); err != nil {
		return builder.String(), collectedLines, false, err
	}

	return builder.String(), collectedLines, false, nil
}

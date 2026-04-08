package service

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type DiffSymbol struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	ChangeType string `json:"change_type"` // "modified", "added", "deleted"
	Line       int    `json:"line"`
	EndLine    int    `json:"end_line,omitempty"`
	Content    string `json:"content,omitempty"`
}

type DiffContextResult struct {
	Ref            string       `json:"ref"`
	ChangedFiles   int          `json:"changed_files"`
	ChangedSymbols []DiffSymbol `json:"changed_symbols"`
	Impact         *DiffImpact  `json:"impact,omitempty"`
}

type DiffImpact struct {
	FilesAffected   int `json:"files_affected"`
	SymbolsAffected int `json:"symbols_affected"`
}

func (a *App) diffContext(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	// 1. Resolve workspace
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	// 2. Get git ref from payload (default empty = working tree changes)
	ref, _ := request.Payload["ref"].(string)
	includeContent, _ := request.Payload["include_content"].(bool)

	// 3. Get changed files and line ranges from git
	changedMap, err := getGitDiffChanges(ctx, registration.Root, ref)
	if err != nil {
		return model.Envelope{}, fmt.Errorf("git diff failed: %w", err)
	}

	// Also fetch per-file change types (added/modified/deleted)
	fileChangeTypes, err := getGitFileChangeTypes(ctx, registration.Root, ref)
	if err != nil {
		// Non-fatal: fall back to all "modified"
		fileChangeTypes = make(map[string]string)
	}

	if len(changedMap) == 0 {
		// No changes
		return model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "git",
			Items: []DiffContextResult{{
				Ref:            ref,
				ChangedFiles:   0,
				ChangedSymbols: []DiffSymbol{},
				Impact: &DiffImpact{
					FilesAffected:   0,
					SymbolsAffected: 0,
				},
			}},
			Stats: model.Stats{Files: 0, Symbols: 0},
		}, nil
	}

	// 4. Open database for symbol lookup
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	// 5. For each changed file+line: lookup enclosing symbol
	symbolMap := make(map[string]*DiffSymbol) // key: "file:line:name" for dedup
	var changingFiles []string

	for relFile, lineRanges := range changedMap {
		changingFiles = append(changingFiles, relFile)

		for _, lineNum := range lineRanges {
			// Query for symbol containing this line
			symbol, found, err := store.SymbolContainingLine(ctx, db, relFile, lineNum)
			if err != nil {
				continue
			}
			if !found {
				continue
			}

			changeType := fileChangeTypes[relFile]
			if changeType == "" {
				changeType = "modified"
			}

			key := fmt.Sprintf("%s:%d:%s", relFile, symbol.StartLine, symbol.Name)
			if _, exists := symbolMap[key]; exists {
				continue // Already added
			}

			diffSymbol := &DiffSymbol{
				File:       relFile,
				Name:       symbol.Name,
				Kind:       symbol.Kind,
				ChangeType: changeType,
				Line:       symbol.StartLine,
				EndLine:    symbol.EndLine,
			}

			// Optionally read content
			if includeContent {
				absFile := relFile
				if !filepath.IsAbs(absFile) {
					absFile = filepath.Join(registration.Root, filepath.FromSlash(relFile))
				}
				content, _, err := readFileLineRange(absFile, symbol.StartLine, symbol.EndLine)
				if err == nil {
					diffSymbol.Content = content
				}
			}

			symbolMap[key] = diffSymbol
		}
	}

	// 6. Build result
	var symbols []DiffSymbol
	for _, sym := range symbolMap {
		symbols = append(symbols, *sym)
	}

	// Sort for deterministic output
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File != symbols[j].File {
			return symbols[i].File < symbols[j].File
		}
		return symbols[i].Line < symbols[j].Line
	})

	result := DiffContextResult{
		Ref:            ref,
		ChangedFiles:   len(changingFiles),
		ChangedSymbols: symbols,
		Impact: &DiffImpact{
			FilesAffected:   len(changingFiles),
			SymbolsAffected: len(symbols),
		},
	}

	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "git+catalog",
		Items:     []DiffContextResult{result},
		Stats: model.Stats{
			Files:   len(changingFiles),
			Symbols: len(symbols),
		},
	}, nil
}

// getGitDiffChanges returns a map of file -> list of changed line numbers.
// If ref is empty, compares working tree against HEAD.
// If ref is specified, compares ref against HEAD.
func getGitDiffChanges(ctx context.Context, workspaceRoot string, ref string) (map[string][]int, error) {
	// First, get list of changed files
	changedFiles, err := getGitChangedFiles(ctx, workspaceRoot, ref)
	if err != nil {
		return nil, err
	}

	if len(changedFiles) == 0 {
		return make(map[string][]int), nil
	}

	// Then get line-level changes via git diff --unified=0
	lineMap, err := getGitDiffHunks(ctx, workspaceRoot, ref)
	if err != nil {
		return nil, err
	}

	// Filter to only files we know changed
	result := make(map[string][]int)
	for file, lines := range lineMap {
		result[file] = lines
	}

	return result, nil
}

// getGitChangedFiles returns list of files changed between ref and HEAD (or working tree).
func getGitChangedFiles(ctx context.Context, workspaceRoot string, ref string) ([]string, error) {
	fileTypes, err := getGitFileChangeTypes(ctx, workspaceRoot, ref)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(fileTypes))
	for f := range fileTypes {
		files = append(files, f)
	}
	return files, nil
}

// getGitFileChangeTypes returns a map of relative file path -> change type string
// by running git diff --name-status.
func getGitFileChangeTypes(ctx context.Context, workspaceRoot string, ref string) (map[string]string, error) {
	args := []string{"diff", "--name-status"}
	if ref != "" {
		args = append(args, ref)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	processutil.ConfigureNonInteractiveCommand(cmd)
	cmd.Dir = workspaceRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status failed: %w", err)
	}

	return parseNameStatus(string(output)), nil
}

// parseNameStatus parses the output of git diff --name-status into a map of
// relative file path -> change type ("modified", "added", "deleted").
// For rename (R) and copy (C) entries the new path is used as the key.
func parseNameStatus(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		statusCode := fields[0]
		var changeType string
		var filePath string

		switch {
		case statusCode == "A":
			changeType = "added"
			filePath = filepath.ToSlash(fields[1])
		case statusCode == "D":
			changeType = "deleted"
			filePath = filepath.ToSlash(fields[1])
		case strings.HasPrefix(statusCode, "R"):
			// Rename: fields[1]=old, fields[2]=new
			changeType = "modified"
			if len(fields) >= 3 {
				filePath = filepath.ToSlash(fields[2])
			} else {
				filePath = filepath.ToSlash(fields[1])
			}
		case strings.HasPrefix(statusCode, "C"):
			// Copy: fields[1]=source, fields[2]=dest
			changeType = "added"
			if len(fields) >= 3 {
				filePath = filepath.ToSlash(fields[2])
			} else {
				filePath = filepath.ToSlash(fields[1])
			}
		default:
			// M, T, U, X, B and anything else -> modified
			changeType = "modified"
			filePath = filepath.ToSlash(fields[1])
		}

		if filePath != "" {
			result[filePath] = changeType
		}
	}
	return result
}

// getGitDiffHunks parses git diff --unified=0 output to extract changed line numbers.
func getGitDiffHunks(ctx context.Context, workspaceRoot string, ref string) (map[string][]int, error) {
	args := []string{"diff", "--unified=0"}
	if ref != "" {
		args = append(args, ref)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	processutil.ConfigureNonInteractiveCommand(cmd)
	cmd.Dir = workspaceRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --unified=0 failed: %w", err)
	}

	// Parse hunks: @@ -startA,countA +startB,countB @@
	lineMap := make(map[string][]int)
	diffOutput := string(output)
	lines := strings.Split(diffOutput, "\n")

	var currentFile string
	hunkRegex := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

	for _, line := range lines {
		// Check for file markers
		if strings.HasPrefix(line, "diff --git a/") {
			// Extract file path from "diff --git a/path b/path"
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				filePath := strings.TrimPrefix(parts[2], "a/")
				currentFile = filepath.ToSlash(filePath)
			}
		} else if strings.HasPrefix(line, "@@") {
			// Parse hunk header
			matches := hunkRegex.FindStringSubmatch(line)
			if len(matches) >= 2 && currentFile != "" {
				startLine := parseLineNum(matches[1])
				countStr := matches[2]
				count := 1
				if countStr != "" {
					count = parseLineNum(countStr)
				}

				// Add all lines in this hunk
				if _, exists := lineMap[currentFile]; !exists {
					lineMap[currentFile] = []int{}
				}

				for i := 0; i < count; i++ {
					lineMap[currentFile] = append(lineMap[currentFile], startLine+i)
				}
			}
		}
	}

	return lineMap, nil
}

func parseLineNum(s string) int {
	var num int
	_, _ = fmt.Sscanf(s, "%d", &num)
	return num
}

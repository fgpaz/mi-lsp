package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

var (
	rgPath     string
	rgResolved bool
	rgOnce     sync.Once
	rgCommand  = exec.CommandContext
)

func resolveRgBinary() string {
	rgOnce.Do(func() {
		if envPath := os.Getenv("MI_LSP_RG"); envPath != "" {
			if _, err := os.Stat(envPath); err == nil {
				rgPath = envPath
				rgResolved = true
				return
			}
		}
		if path, err := exec.LookPath("rg"); err == nil {
			rgPath = path
			rgResolved = true
			return
		}
		rgResolved = true
	})
	return rgPath
}

var binaryExtensions = map[string]struct{}{
	".exe": {}, ".dll": {}, ".pdb": {}, ".bin": {}, ".obj": {},
	".o": {}, ".a": {}, ".so": {}, ".dylib": {},
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".ico": {}, ".bmp": {}, ".webp": {},
	".zip": {}, ".gz": {}, ".tar": {}, ".rar": {}, ".7z": {},
	".pdf": {}, ".doc": {}, ".docx": {}, ".xls": {}, ".xlsx": {},
	".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {},
	".mp3": {}, ".mp4": {}, ".avi": {}, ".mov": {},
	".db": {}, ".sqlite": {}, ".sqlite3": {},
	".nupkg": {}, ".snk": {}, ".pfx": {},
}

func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := binaryExtensions[ext]
	return ok
}

type searchMatch struct {
	File string
	Line int
	Text string
}

type searchPatternDiagnostics struct {
	RipgrepFallbackCode string
}

func searchPattern(ctx context.Context, root string, project model.ProjectFile, pattern string, useRegex bool, limit int) ([]map[string]any, error) {
	return searchPatternScoped(ctx, root, root, project, pattern, useRegex, limit)
}

func searchPatternScoped(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int) ([]map[string]any, error) {
	return searchPatternScopedWithDiagnostics(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit, nil)
}

func searchPatternScopedWithDiagnostics(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int, diagnostics *searchPatternDiagnostics) ([]map[string]any, error) {
	rgBin := resolveRgBinary()
	if rgBin != "" {
		return searchPatternRgWithDiagnostics(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit, rgBin, diagnostics)
	}
	return searchPatternFallback(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit)
}

func searchPatternRg(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int, rgBin string) ([]map[string]any, error) {
	return searchPatternRgWithDiagnostics(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit, rgBin, nil)
}

func searchPatternRgWithDiagnostics(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int, rgBin string, diagnostics *searchPatternDiagnostics) ([]map[string]any, error) {
	if limit <= 0 {
		limit = DefaultConfig().DefaultSearchLimit
	}

	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	command := rgCommand(searchCtx, rgBin, buildRipgrepArgs(pattern, useRegex, searchRoot)...)
	processutil.ConfigureNonInteractiveCommand(command)

	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		if isTransientRipgrepAccessError(err) {
			recordRipgrepFallback(diagnostics, err)
			return searchPatternFallback(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit)
		}
		return nil, err
	}

	resultPattern := regexp.MustCompile(`^(.*):([0-9]+):(.*)$`)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	items := make([]map[string]any, 0, min(limit, 16))
	reachedLimit := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		match := resultPattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		lineNumber, _ := strconv.Atoi(match[2])
		relativeFile, relErr := makeRelative(workspaceRoot, match[1])
		if relErr != nil {
			relativeFile = filepath.ToSlash(match[1])
		}
		item := map[string]any{
			"file": relativeFile,
			"line": lineNumber,
			"text": match[3],
		}
		if repo, ok := workspace.FindRepoByFile(project, workspaceRoot, match[1]); ok {
			item["repo"] = repo.Name
		}
		items = append(items, item)
		if len(items) >= limit {
			reachedLimit = true
			cancel()
			break
		}
	}

	scanErr := scanner.Err()
	waitErr := command.Wait()

	if reachedLimit {
		return items, nil
	}
	if scanErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, scanErr
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 && strings.TrimSpace(stderr.String()) == "" && len(items) == 0 {
			return []map[string]any{}, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			combinedErr := fmt.Errorf("%w: %s", waitErr, msg)
			if isTransientRipgrepAccessError(combinedErr) {
				recordRipgrepFallback(diagnostics, combinedErr)
				return searchPatternFallback(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit)
			}
			return nil, fmt.Errorf("%w: %s", waitErr, msg)
		}
		if isTransientRipgrepAccessError(waitErr) {
			recordRipgrepFallback(diagnostics, waitErr)
			return searchPatternFallback(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit)
		}
		return nil, waitErr
	}
	return items, nil
}

func recordRipgrepFallback(diagnostics *searchPatternDiagnostics, err error) {
	if diagnostics == nil || diagnostics.RipgrepFallbackCode != "" {
		return
	}
	if code := classifySearchRuntimeFailure(err); code != "" {
		diagnostics.RipgrepFallbackCode = code
		return
	}
	diagnostics.RipgrepFallbackCode = "process_spawn_failed"
}

func isTransientRipgrepAccessError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "access is denied") ||
		strings.Contains(message, "access denied") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "temporarily unavailable") ||
		strings.Contains(message, "resource busy") ||
		strings.Contains(message, "database is locked") ||
		strings.Contains(message, "file is locked") ||
		strings.Contains(message, "sharing violation") ||
		strings.Contains(message, "used by another process")
}

func isRegexParseError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "regex parse error") ||
		strings.Contains(message, "error parsing regexp") ||
		strings.Contains(message, "missing closing") ||
		strings.Contains(message, "missing terminating") ||
		strings.Contains(message, "invalid escape") ||
		strings.Contains(message, "invalid repeat") ||
		strings.Contains(message, "unexpected )") ||
		strings.Contains(message, "unexpected ]")
}

func buildRipgrepArgs(pattern string, useRegex bool, searchRoot string) []string {
	args := []string{
		"--line-number", "--no-heading", "--color", "never", "--hidden",
		"--glob", "!.mi-lsp/index.db",
		"--glob", "!.mi-lsp/index.db-wal",
		"--glob", "!.mi-lsp/index.db-shm",
	}
	if !useRegex {
		args = append(args, "-F")
	}
	args = append(args, pattern, searchRoot)
	return args
}

func searchPatternFallback(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int) ([]map[string]any, error) {
	rawMatches, err := searchPatternGo(ctx, searchRoot, pattern, useRegex, limit)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rawMatches))
	for _, m := range rawMatches {
		relativeFile, relErr := makeRelative(workspaceRoot, m.File)
		if relErr != nil {
			relativeFile = filepath.ToSlash(m.File)
		}
		item := map[string]any{
			"file": relativeFile,
			"line": m.Line,
			"text": m.Text,
		}
		if repo, ok := workspace.FindRepoByFile(project, workspaceRoot, m.File); ok {
			item["repo"] = repo.Name
		}
		items = append(items, item)
	}
	return items, nil
}

func searchPatternGo(ctx context.Context, root string, pattern string, useRegex bool, limit int) ([]searchMatch, error) {
	if limit <= 0 {
		limit = DefaultConfig().DefaultSearchLimit
	}

	matcher, _ := workspace.LoadIgnoreMatcher(root, nil)

	var re *regexp.Regexp
	if useRegex {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
	}

	matches := make([]searchMatch, 0, 64)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(matches) >= limit {
			return fs.SkipAll
		}

		if matcher != nil && matcher.ShouldIgnore(root, path) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if isBinaryFile(path) {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			var found bool
			if useRegex {
				found = re.MatchString(line)
			} else {
				found = strings.Contains(line, pattern)
			}
			if found {
				matches = append(matches, searchMatch{
					File: path,
					Line: lineNum,
					Text: line,
				})
				if len(matches) >= limit {
					return fs.SkipAll
				}
			}
		}
		return nil
	})

	if err != nil && err != fs.SkipAll {
		return matches, err
	}
	return matches, nil
}

func looksRegexLikePattern(pattern string) bool {
	return strings.ContainsAny(pattern, "|()[]{}+?^\\")
}

func classifySearchRuntimeFailure(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "access is denied") ||
		strings.Contains(message, "acceso denegado") ||
		strings.Contains(message, "permission denied"):
		return "process_spawn_access_denied"
	case strings.Contains(message, "createprocess") ||
		strings.Contains(message, "fork/exec") ||
		strings.Contains(message, "exec:") ||
		strings.Contains(message, "failed to start") ||
		strings.Contains(message, "invalid image"):
		return "process_spawn_failed"
	default:
		return ""
	}
}

var searchIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?$`)

func isIdentifierLikeQuery(pattern string) bool {
	trimmed := strings.TrimSpace(pattern)
	if len(trimmed) < 3 || strings.ContainsAny(trimmed, " \t\r\n/-") {
		return false
	}
	if !searchIdentifierPattern.MatchString(trimmed) {
		return false
	}
	if intentSymbolPattern.MatchString(trimmed) {
		return true
	}
	return strings.Contains(trimmed, "_") || strings.Contains(trimmed, ".") || hasCamelHump(trimmed)
}

func hasCamelHump(value string) bool {
	prevLower := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			prevLower = true
			continue
		}
		if prevLower && r >= 'A' && r <= 'Z' {
			return true
		}
		prevLower = false
	}
	return false
}

func rankIdentifierSearchItems(pattern string, items []map[string]any) {
	type rankedItem struct {
		item  map[string]any
		score int
		index int
	}
	ranked := make([]rankedItem, len(items))
	for i, item := range items {
		ranked[i] = rankedItem{item: item, score: identifierSearchScore(pattern, item), index: i}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].index < ranked[j].index
		}
		return ranked[i].score > ranked[j].score
	})
	for i, rankedItem := range ranked {
		items[i] = rankedItem.item
	}
}

func identifierSearchScore(pattern string, item map[string]any) int {
	file := strings.ToLower(filepath.ToSlash(stringFromMap(item, "file")))
	text := strings.TrimSpace(stringFromMap(item, "text"))
	textLower := strings.ToLower(text)
	patternLower := strings.ToLower(pattern)
	score := 0

	if isSourceSearchPath(file) {
		score += 200
	}
	if isIdentifierDeclaration(pattern, text) {
		score += 300
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(file)), strings.ToLower(filepath.Ext(file)))
	if base == patternLower {
		score += 240
	}
	if strings.Contains(textLower, patternLower) {
		score += 80
	}
	if strings.Contains(file, patternLower) {
		score += 40
	}
	if isDocumentationSearchPath(file) {
		score -= 180
	}
	if isTestSearchPath(file) {
		score -= 90
	}
	if isBackupOrGeneratedSearchPath(file) {
		score -= 800
	}
	return score
}

func stringFromMap(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return strings.TrimSpace(value)
}

func isIdentifierDeclaration(pattern string, text string) bool {
	quoted := regexp.QuoteMeta(pattern)
	declaration := regexp.MustCompile(`(?i)\b(interface|class|struct|record|enum|type|func|function|const|let|var)\s+` + quoted + `\b`)
	if declaration.MatchString(text) {
		return true
	}
	implementation := regexp.MustCompile(`(?i)\b(class|struct|record|type)\s+[A-Za-z_][A-Za-z0-9_]*\s*[:=]\s*.*\b` + quoted + `\b`)
	return implementation.MatchString(text)
}

func isSourceSearchPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cs", ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".py", ".pyi", ".go":
		return true
	default:
		return false
	}
}

func isDocumentationSearchPath(path string) bool {
	return strings.HasPrefix(path, ".docs/") ||
		strings.HasPrefix(path, "docs/") ||
		strings.HasSuffix(path, ".md") ||
		strings.Contains(path, "/docs/")
}

func isTestSearchPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, ".tests/") ||
		strings.Contains(base, "_test.") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, "test")
}

func isBackupOrGeneratedSearchPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(path, "/bin/") ||
		strings.Contains(path, "/obj/") ||
		strings.Contains(path, "/dist/") ||
		strings.Contains(path, "/build/") ||
		strings.Contains(path, "/generated/") ||
		strings.Contains(path, "/node_modules/") ||
		strings.Contains(path, "/.next/") ||
		strings.Contains(base, ".generated.") ||
		strings.Contains(base, ".g.") ||
		strings.Contains(base, ".bak") ||
		strings.Contains(base, "backup")
}

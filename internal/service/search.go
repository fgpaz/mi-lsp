package service

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func searchPattern(ctx context.Context, root string, project model.ProjectFile, pattern string, useRegex bool, limit int) ([]map[string]any, error) {
	return searchPatternScoped(ctx, root, root, project, pattern, useRegex, limit)
}

func searchPatternScoped(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int) ([]map[string]any, error) {
	rgBin := resolveRgBinary()
	if rgBin != "" {
		return searchPatternRg(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit, rgBin)
	}
	return searchPatternFallback(ctx, workspaceRoot, searchRoot, project, pattern, useRegex, limit)
}

func searchPatternRg(ctx context.Context, workspaceRoot string, searchRoot string, project model.ProjectFile, pattern string, useRegex bool, limit int, rgBin string) ([]map[string]any, error) {
	args := []string{"--line-number", "--no-heading", "--color", "never"}
	if !useRegex {
		args = append(args, "-F")
	}
	args = append(args, pattern, searchRoot)
	command := exec.CommandContext(ctx, rgBin, args...)
	processutil.ConfigureNonInteractiveCommand(command)
	output, err := command.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && strings.TrimSpace(string(output)) == "" {
			return []map[string]any{}, nil
		}
		if len(output) == 0 {
			return nil, err
		}
	}
	resultPattern := regexp.MustCompile(`^(.*):([0-9]+):(.*)$`)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if limit <= 0 {
		limit = DefaultConfig().DefaultSearchLimit
	}
	items := make([]map[string]any, 0, min(limit, len(lines)))
	for _, line := range lines {
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
			break
		}
	}
	return items, nil
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

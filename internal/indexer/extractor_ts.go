package indexer

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	tsClassPattern     = regexp.MustCompile(`^\s*(?:export\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsInterfacePattern = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsTypePattern      = regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsFunctionPattern  = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsConstPattern     = regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`)
	csTypePattern      = regexp.MustCompile(`^\s*(?:public|internal|protected|private|sealed|abstract|partial|static|\s)*(class|interface|record|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	csMethodPattern    = regexp.MustCompile(`^\s*(?:public|internal|protected|private)\s+(?:(?:static|virtual|override|abstract|sealed|async|partial|new)\s+)*(?:[A-Za-z_][A-Za-z0-9_<>\[\],?.]*\s+)+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*(?:\{|=>|where|$)`)
)

func ExtractCatalog(root string, repo model.WorkspaceRepo, absolutePath string, content []byte) ([]model.SymbolRecord, model.FileRecord) {
	relPath, _ := filepath.Rel(root, absolutePath)
	relPath = filepath.ToSlash(relPath)
	hash := digest(content)
	language := languageForPath(absolutePath)
	fileRecord := model.FileRecord{
		FilePath:    relPath,
		RepoID:      repo.ID,
		RepoName:    repo.Name,
		ContentHash: hash,
		IndexedAt:   time.Now().Unix(),
		Language:    language,
	}
	lines := strings.Split(string(content), "\n")
	if language == "csharp" {
		return extractCSharp(repo, relPath, hash, lines), fileRecord
	}
	if language == "python" {
		return extractPython(repo, relPath, hash, content), fileRecord
	}
	return extractTypeScript(repo, relPath, hash, lines), fileRecord
}

func extractTypeScript(repo model.WorkspaceRepo, relPath, hash string, lines []string) []model.SymbolRecord {
	items := make([]model.SymbolRecord, 0)
	addIfMatch := func(kind string, pattern *regexp.Regexp, line string, lineNumber int) {
		match := pattern.FindStringSubmatch(line)
		if len(match) < 2 {
			return
		}
		name := match[1]
		docComment := ExtractDocComment(lines, lineNumber-1)
		searchText := BuildSearchText(name, "", docComment, "", relPath, kind)
		items = append(items, model.SymbolRecord{
			FilePath:      relPath,
			RepoID:        repo.ID,
			RepoName:      repo.Name,
			Name:          name,
			Kind:          kind,
			StartLine:     lineNumber,
			EndLine:       lineNumber,
			QualifiedName: relPath + "::" + name,
			SignatureHash: digest([]byte(relPath + ":" + name + ":" + kind)),
			Language:      "typescript",
			FileHash:      hash,
			SearchText:    searchText,
		})
	}
	for idx, line := range lines {
		lineNumber := idx + 1
		addIfMatch("class", tsClassPattern, line, lineNumber)
		addIfMatch("interface", tsInterfacePattern, line, lineNumber)
		addIfMatch("type", tsTypePattern, line, lineNumber)
		addIfMatch("function", tsFunctionPattern, line, lineNumber)
		addIfMatch("const", tsConstPattern, line, lineNumber)
	}
	if routeName := nextRouteName(relPath); routeName != "" {
		searchText := BuildSearchText(routeName, "", "", "", relPath, "route")
		items = append(items, model.SymbolRecord{
			FilePath:      relPath,
			RepoID:        repo.ID,
			RepoName:      repo.Name,
			Name:          routeName,
			Kind:          "route",
			StartLine:     1,
			EndLine:       1,
			QualifiedName: relPath + "::route",
			SignatureHash: digest([]byte(relPath + ":route")),
			Language:      "typescript",
			FileHash:      hash,
			SearchText:    searchText,
		})
	}
	return items
}

func extractCSharp(repo model.WorkspaceRepo, relPath, hash string, lines []string) []model.SymbolRecord {
	items := make([]model.SymbolRecord, 0)
	currentType := ""
	for idx, line := range lines {
		lineNumber := idx + 1
		if match := csTypePattern.FindStringSubmatch(line); len(match) == 3 {
			currentType = match[2]
			docComment := ExtractDocComment(lines, lineNumber-1)
			searchText := BuildSearchText(match[2], "", docComment, "", relPath, strings.ToLower(match[1]))
			items = append(items, model.SymbolRecord{
				FilePath:      relPath,
				RepoID:        repo.ID,
				RepoName:      repo.Name,
				Name:          match[2],
				Kind:          strings.ToLower(match[1]),
				StartLine:     lineNumber,
				EndLine:       lineNumber,
				QualifiedName: relPath + "::" + match[2],
				SignatureHash: digest([]byte(relPath + ":" + match[2] + ":" + match[1])),
				Scope:         inferScope(line),
				Language:      "csharp",
				FileHash:      hash,
				SearchText:    searchText,
			})
			continue
		}
		if match := csMethodPattern.FindStringSubmatch(line); len(match) == 2 {
			name := match[1]
			qualifiedName := relPath + "::" + name
			if currentType != "" {
				qualifiedName = relPath + "::" + currentType + "." + name
			}
			sig := strings.TrimSpace(line)
			docComment := ExtractDocComment(lines, lineNumber-1)
			searchText := BuildSearchText(name, sig, docComment, currentType, relPath, "method")
			items = append(items, model.SymbolRecord{
				FilePath:      relPath,
				RepoID:        repo.ID,
				RepoName:      repo.Name,
				Name:          name,
				Kind:          "method",
				StartLine:     lineNumber,
				EndLine:       lineNumber,
				Parent:        currentType,
				QualifiedName: qualifiedName,
				Signature:     sig,
				SignatureHash: digest([]byte(relPath + ":" + sig)),
				Scope:         inferScope(line),
				Language:      "csharp",
				FileHash:      hash,
				SearchText:    searchText,
			})
		}
	}
	return items
}

func inferScope(line string) string {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "public "):
		return "public"
	case strings.HasPrefix(line, "private "):
		return "private"
	case strings.HasPrefix(line, "protected "):
		return "protected"
	case strings.HasPrefix(line, "internal "):
		return "internal"
	default:
		return ""
	}
}

func nextRouteName(relPath string) string {
	normalized := filepath.ToSlash(relPath)
	if strings.Contains(normalized, "/app/") || strings.HasPrefix(normalized, "app/") {
		return strings.TrimSuffix(strings.TrimPrefix(normalized, "app/"), filepath.Ext(normalized))
	}
	if strings.Contains(normalized, "/pages/") || strings.HasPrefix(normalized, "pages/") {
		return strings.TrimSuffix(strings.TrimPrefix(normalized, "pages/"), filepath.Ext(normalized))
	}
	return ""
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cs":
		return "csharp"
	case ".py", ".pyi":
		return "python"
	default:
		return "typescript"
	}
}

func digest(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}

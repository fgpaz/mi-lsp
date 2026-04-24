package indexer

import (
	"regexp"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	pythonClassPattern = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	pythonFuncPattern  = regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

type pythonClassScope struct {
	name   string
	indent int
}

// extractPython uses a bounded lexical pass so catalog indexing stays cancelable
// and cannot get trapped inside parser-specific edge cases.
func extractPython(repo model.WorkspaceRepo, relPath, hash string, content []byte) []model.SymbolRecord {
	lines := strings.Split(string(content), "\n")
	items := make([]model.SymbolRecord, 0)
	classStack := make([]pythonClassScope, 0)

	for idx, line := range lines {
		lineNumber := idx + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "@") {
			continue
		}
		indent := pythonIndent(line)
		for len(classStack) > 0 && indent <= classStack[len(classStack)-1].indent {
			classStack = classStack[:len(classStack)-1]
		}

		if match := pythonClassPattern.FindStringSubmatch(line); len(match) == 2 {
			name := match[1]
			items = append(items, pythonSymbol(repo, relPath, hash, lines, name, "class", "", "", lineNumber, strings.TrimSpace(line)))
			classStack = append(classStack, pythonClassScope{name: name, indent: indent})
			continue
		}
		if match := pythonFuncPattern.FindStringSubmatch(line); len(match) == 2 {
			name := match[1]
			kind := "function"
			parent := ""
			scope := "module"
			if len(classStack) > 0 && indent > classStack[len(classStack)-1].indent {
				parent = classStack[len(classStack)-1].name
				scope = parent
				kind = "method"
			}
			items = append(items, pythonSymbol(repo, relPath, hash, lines, name, kind, parent, scope, lineNumber, strings.TrimSpace(line)))
		}
	}
	return items
}

func pythonSymbol(repo model.WorkspaceRepo, relPath, hash string, lines []string, name string, kind string, parent string, scope string, lineNumber int, signature string) model.SymbolRecord {
	if scope == "" {
		scope = "module"
	}
	qualifiedName := relPath + "::" + name
	if parent != "" {
		qualifiedName = relPath + "::" + parent + "." + name
	}
	docComment := ExtractDocComment(lines, lineNumber-1)
	searchText := BuildSearchText(name, signature, docComment, parent, relPath, kind)
	return model.SymbolRecord{
		FilePath:      relPath,
		RepoID:        repo.ID,
		RepoName:      repo.Name,
		Name:          name,
		Kind:          kind,
		StartLine:     lineNumber,
		EndLine:       lineNumber,
		Parent:        parent,
		QualifiedName: qualifiedName,
		Signature:     signature,
		SignatureHash: digest([]byte(relPath + ":" + qualifiedName + ":" + kind)),
		Scope:         scope,
		Language:      "python",
		FileHash:      hash,
		SearchText:    searchText,
	}
}

func pythonIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

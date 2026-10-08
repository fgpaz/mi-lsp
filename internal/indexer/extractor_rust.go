package indexer

import (
	"regexp"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	rustFunctionPattern = regexp.MustCompile(`^\s*(pub(\([^)]*\))?\s+)?((async|unsafe|const|default|extern(\s+"[^"]+")?)\s+)*fn\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	rustTypePattern     = regexp.MustCompile(`^\s*(pub(\([^)]*\))?\s+)?(struct|enum|trait|type)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	rustValuePattern    = regexp.MustCompile(`^\s*(pub(\([^)]*\))?\s+)?(const|static(\s+mut)?)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	rustModulePattern   = regexp.MustCompile(`^\s*(pub(\([^)]*\))?\s+)?mod\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	rustMacroPattern    = regexp.MustCompile(`^\s*macro_rules!\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	rustImplPattern     = regexp.MustCompile(`^\s*impl\b(.*?)\{`)
	rustImplTypePattern = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*(<|$)`)
)

type rustScope struct {
	name  string
	kind  string
	depth int
}

// extractRust uses a bounded lexical pass, like the Python extractor. It indexes
// common Rust declarations without compiling a crate or resolving dependencies;
// rust-analyzer supplies semantic navigation when available.
func extractRust(repo model.WorkspaceRepo, relPath, hash string, content []byte) []model.SymbolRecord {
	lines := strings.Split(string(content), "\n")
	items := make([]model.SymbolRecord, 0)
	scopes := make([]rustScope, 0, 8)
	depth := 0

	for idx, line := range lines {
		lineNumber := idx + 1
		trimmed := strings.TrimSpace(line)
		for len(scopes) > 0 && scopes[len(scopes)-1].depth > depth {
			scopes = scopes[:len(scopes)-1]
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "*") && !strings.HasPrefix(trimmed, "#") {
			if match := rustImplPattern.FindStringSubmatch(line); len(match) == 2 && rustBraceDelta(line) > 0 {
				parent := rustImplName(match[1])
				depth += rustBraceDelta(line)
				scopes = append(scopes, rustScope{name: parent, kind: "impl", depth: depth})
				continue
			}

			name, kind, visibility := "", "", ""
			if match := rustFunctionPattern.FindStringSubmatch(line); len(match) == 7 {
				name, visibility, kind = match[6], rustVisibility(match[1]), "function"
				if parent := rustOwner(scopes); parent != "" {
					kind = "method"
				}
			} else if match := rustTypePattern.FindStringSubmatch(line); len(match) == 5 {
				name, visibility, kind = match[4], rustVisibility(match[1]), match[3]
			} else if match := rustValuePattern.FindStringSubmatch(line); len(match) == 6 {
				name, visibility = match[5], rustVisibility(match[1])
				if strings.HasPrefix(match[3], "static") {
					kind = "static"
				} else {
					kind = "const"
				}
			} else if match := rustModulePattern.FindStringSubmatch(line); len(match) == 4 {
				name, visibility, kind = match[3], rustVisibility(match[1]), "module"
			} else if match := rustMacroPattern.FindStringSubmatch(line); len(match) == 2 {
				name, kind = match[1], "macro"
			}

			if name != "" {
				parent := rustOwner(scopes)
				qualified := relPath + "::" + name
				if parent != "" {
					qualified = relPath + "::" + parent + "." + name
				}
				signature := strings.TrimSpace(line)
				doc := ExtractDocComment(lines, idx)
				items = append(items, model.SymbolRecord{
					FilePath:      relPath,
					RepoID:        repo.ID,
					RepoName:      repo.Name,
					Name:          name,
					Kind:          kind,
					StartLine:     lineNumber,
					EndLine:       lineNumber,
					Parent:        parent,
					QualifiedName: qualified,
					Signature:     signature,
					SignatureHash: digest([]byte(relPath + ":" + qualified + ":" + kind)),
					Scope:         visibility,
					Language:      "rust",
					FileHash:      hash,
					SearchText:    BuildSearchText(name, signature, doc, parent, relPath, kind),
				})
				if (kind == "module" || kind == "trait") && rustBraceDelta(line) > 0 {
					depth += rustBraceDelta(line)
					scopes = append(scopes, rustScope{name: name, kind: kind, depth: depth})
					continue
				}
			}
		}
		depth += rustBraceDelta(line)
	}
	return items
}

func rustImplName(header string) string {
	header = strings.TrimSpace(header)
	if bracket := strings.Index(header, "<"); bracket == 0 {
		if close := strings.Index(header, ">"); close >= 0 {
			header = strings.TrimSpace(header[close+1:])
		}
	}
	if before, after, found := strings.Cut(header, " for "); found {
		_ = before
		header = after
	}
	if bracket := strings.Index(header, "<"); bracket >= 0 {
		header = header[:bracket]
	}
	parts := strings.Split(strings.TrimSpace(header), "::")
	if len(parts) == 0 {
		return ""
	}
	match := rustImplTypePattern.FindStringSubmatch(strings.TrimSpace(parts[len(parts)-1]))
	if len(match) == 2 {
		return match[1]
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func rustVisibility(modifier string) string {
	modifier = strings.TrimSpace(modifier)
	switch {
	case modifier == "":
		return "private"
	case strings.Contains(modifier, "pub(crate)") || strings.Contains(modifier, "pub(super)"):
		return "crate"
	default:
		return "public"
	}
}

func rustOwner(scopes []rustScope) string {
	for i := len(scopes) - 1; i >= 0; i-- {
		if scopes[i].kind == "impl" || scopes[i].kind == "trait" {
			return scopes[i].name
		}
	}
	return ""
}

func rustBraceDelta(line string) int {
	// This deliberately counts only delimiters. It is a catalog hint, not a
	// parser; malformed or macro-heavy syntax remains searchable as file text.
	return strings.Count(line, "{") - strings.Count(line, "}")
}

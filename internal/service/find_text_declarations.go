package service

import (
	"regexp"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/language"
	"github.com/fgpaz/mi-lsp/internal/model"
)

// Text fallback for nav.find: declaration-first, code-only.
//
// Templates use %N% for the declared-name expression. For the single combined
// ripgrep search it is a non-capturing group; for per-language matching it is
// the one capture group, so the declared name can be compared and reported.
const (
	tsModifiers = `(?:(?:public|private|protected|static|async|readonly|override|abstract|get|set|export|default)\s+)`
	csModifiers = `(?:(?:public|private|protected|internal|static|async|override|virtual|abstract|sealed|readonly|partial|extern|new|unsafe)\s+)`
)

type textDeclaration struct {
	template string
	kind     string
	// kindFn refines kind from the matched line when it is cheap to do so.
	kindFn func(text string) string
}

var textDeclarationsByLanguage = map[string][]textDeclaration{
	"go": {
		{template: `^func\s+\([^)]*\)\s*%N%\b`, kind: "method"},
		{template: `^func\s+%N%\b`, kind: "func"},
		{template: `^\s*type\s+%N%\b`, kind: "type", kindFn: func(text string) string {
			if strings.Contains(text, "interface") {
				return "interface"
			}
			return "type"
		}},
		{template: `^\s*(?:var|const)\s+%N%\b`, kind: "const", kindFn: func(text string) string {
			if strings.HasPrefix(strings.TrimSpace(text), "var") {
				return "var"
			}
			return "const"
		}},
	},
	"typescript": tsDeclarations(),
	"javascript": tsDeclarations(),
	"csharp": {
		{template: `\b(?:class|interface|record|struct|enum)\s+%N%\b`, kind: "class", kindFn: func(text string) string {
			for _, kind := range []string{"interface", "record", "struct", "enum"} {
				if regexp.MustCompile(`\b` + kind + `\s`).MatchString(text) {
					return kind
				}
			}
			return "class"
		}},
		{template: `^\s*` + csModifiers + `+[\w<>\[\],.?\s]*?\b%N%\s*(?:\(|\{|=>|=|;)`, kind: "method", kindFn: func(text string) string {
			if strings.Contains(text, "(") {
				return "method"
			}
			return "property"
		}},
	},
	"python": {
		{template: `^\s*(?:async\s+)?def\s+%N%\b`, kind: "func"},
		{template: `^\s*class\s+%N%\b`, kind: "class"},
	},
}

func tsDeclarations() []textDeclaration {
	return []textDeclaration{
		{template: `\b(?:function\*?|class|interface|type|enum|const|let|var)\s+%N%\b`, kind: "func", kindFn: func(text string) string {
			for _, keyword := range []string{"class", "interface", "enum", "type", "const", "let", "var"} {
				if regexp.MustCompile(`\b` + keyword + `\s`).MatchString(text) {
					switch keyword {
					case "let", "var":
						return "var"
					}
					return keyword
				}
			}
			return "func"
		}},
		// Methods: a modifier prefix, or a line opening a body, so plain calls
		// like `Sym(x);` are not mistaken for declarations.
		{template: `^\s*` + tsModifiers + `+%N%\s*(?:<[^>]*>)?\(`, kind: "method"},
		{template: `^\s*%N%\s*(?:<[^>]*>)?\([^;]*\{\s*$`, kind: "method"},
	}
}

// declaredNameExpr is the regex for the declared name: the exact symbol, or any
// identifier containing it.
func declaredNameExpr(symbol string, exact bool) string {
	quoted := regexp.QuoteMeta(symbol)
	if exact {
		return quoted
	}
	return `[A-Za-z0-9_]*` + quoted + `[A-Za-z0-9_]*`
}

type compiledTextDeclaration struct {
	re *regexp.Regexp
	textDeclaration
}

type textDeclarationMatcher struct {
	symbol   string
	exact    bool
	byLang   map[string][]compiledTextDeclaration
	searchRe string
}

func newTextDeclarationMatcher(symbol string, exact bool) *textDeclarationMatcher {
	name := declaredNameExpr(symbol, exact)
	m := &textDeclarationMatcher{symbol: symbol, exact: exact, byLang: map[string][]compiledTextDeclaration{}}
	seen := map[string]bool{}
	var parts []string
	languages := make([]string, 0, len(textDeclarationsByLanguage))
	for lang := range textDeclarationsByLanguage {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	for _, lang := range languages {
		for _, declaration := range textDeclarationsByLanguage[lang] {
			local := strings.ReplaceAll(declaration.template, "%N%", "("+name+")")
			m.byLang[lang] = append(m.byLang[lang], compiledTextDeclaration{re: regexp.MustCompile(local), textDeclaration: declaration})
			search := strings.ReplaceAll(declaration.template, "%N%", "(?:"+name+")")
			if !seen[search] {
				seen[search] = true
				parts = append(parts, "(?:"+search+")")
			}
		}
	}
	m.searchRe = strings.Join(parts, "|")
	return m
}

// match reports the declared name and kind when the line declares the symbol
// in the language of file.
func (m *textDeclarationMatcher) match(file string, text string) (name string, kind string, ok bool) {
	lang, known := language.ForPath(file)
	if !known {
		return "", "", false
	}
	for _, declaration := range m.byLang[lang] {
		span := declaration.re.FindStringSubmatchIndex(text)
		if span == nil {
			continue
		}
		captured := text[span[2]:span[3]]
		if m.exact && captured != m.symbol {
			continue
		}
		name = identifierAt(text, span[2], captured)
		kind = declaration.kind
		if declaration.kindFn != nil {
			kind = declaration.kindFn(text)
		}
		return name, kind, true
	}
	return "", "", false
}

// identifierAt returns the identifier token that starts at the captured span,
// extended to the left over identifier characters. Symbols containing
// metacharacters ("New(config") thus report the declared identifier ("New")
// instead of the raw pattern slice. When no identifier starts there, the
// captured text is returned.
func identifierAt(text string, start int, captured string) string {
	begin := start
	for begin > 0 && isWordRune(rune(text[begin-1])) {
		begin--
	}
	end := start
	for end < len(text) && isWordRune(rune(text[end])) {
		end++
	}
	if end == begin {
		return captured
	}
	return text[begin:end]
}

func isFindableCodeFile(file string) bool {
	return file != "" && !strings.HasPrefix(file, ".docs/") && !strings.Contains(file, "/.docs/") && isSourceSearchPath(file)
}

const maxTextItemSignature = 200

func textItem(name, kind string, hit map[string]any) map[string]any {
	text := strings.TrimSpace(stringFromMap(hit, "text"))
	if len(text) > maxTextItemSignature {
		text = text[:maxTextItemSignature]
	}
	return map[string]any{
		"name":      name,
		"kind":      kind,
		"file":      stringFromMap(hit, "file"),
		"line":      hit["line"],
		"signature": text,
		"text":      text,
		"origin":    model.ItemOriginText,
	}
}

// declarationItems keeps only hits that declare the symbol.
func (m *textDeclarationMatcher) declarationItems(hits []map[string]any) []map[string]any {
	items := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		file := stringFromMap(hit, "file")
		if !isFindableCodeFile(file) {
			continue
		}
		if name, kind, ok := m.match(file, stringFromMap(hit, "text")); ok {
			items = append(items, textItem(name, kind, hit))
		}
	}
	// Exact-name declarations first when matching by substring.
	sort.SliceStable(items, func(i, j int) bool {
		return (items[i]["name"] == m.symbol) && (items[j]["name"] != m.symbol)
	})
	return items
}

// occurrenceItems is the last resort when nothing declares the symbol: word
// occurrences in code files, non-test files first.
func (m *textDeclarationMatcher) occurrenceItems(hits []map[string]any) []map[string]any {
	items := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		if file := stringFromMap(hit, "file"); isFindableCodeFile(file) {
			items = append(items, textItem(m.symbol, "text_match", hit))
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return !isTestSearchPath(items[i]["file"].(string)) && isTestSearchPath(items[j]["file"].(string))
	})
	return items
}

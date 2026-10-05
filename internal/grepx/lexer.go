package grepx

import "strings"

// Line classes of the grep-v1 annotation.
const (
	classDef = "def"
	classRef = "ref"
	classCom = "com"
	classStr = "str"
)

type langSyntax struct {
	lineComment  []string
	blockOpen    string
	blockClose   string
	quotes       string // characters that open a single-line string
	backtick     bool   // backtick strings (Go raw, JS template)
	tripleQuotes bool   // Python docstrings
}

var syntaxByLanguage = map[string]langSyntax{
	"go":         {lineComment: []string{"//"}, blockOpen: "/*", blockClose: "*/", quotes: "\"'", backtick: true},
	"typescript": {lineComment: []string{"//"}, blockOpen: "/*", blockClose: "*/", quotes: "\"'", backtick: true},
	"javascript": {lineComment: []string{"//"}, blockOpen: "/*", blockClose: "*/", quotes: "\"'", backtick: true},
	"csharp":     {lineComment: []string{"//"}, blockOpen: "/*", blockClose: "*/", quotes: "\"'"},
	"python":     {lineComment: []string{"#"}, quotes: "\"'", tripleQuotes: true},
}

// classifyOffset is the minimal per-line lexer. It reports whether byte offset
// `at` of `line` sits inside a comment (com), a string literal (str) or code
// (ref). It only sees one line, so multi-line constructs are approximated:
// lines that begin like a block-comment continuation count as comments and
// Python docstring delimiters count as strings. at < 0 means the match column
// is unknown and only the line prefix is considered.
func classifyOffset(lang, line string, at int) string {
	syn, ok := syntaxByLanguage[lang]
	if !ok {
		return classRef
	}
	trimmed := strings.TrimLeft(line, " \t")
	for _, marker := range syn.lineComment {
		if strings.HasPrefix(trimmed, marker) {
			return classCom
		}
	}
	if syn.blockOpen != "" {
		if strings.HasPrefix(trimmed, syn.blockOpen) ||
			(strings.HasPrefix(trimmed, "*") && !strings.HasPrefix(trimmed, "*/") && !looksLikeCode(trimmed)) {
			return classCom
		}
	}
	if syn.tripleQuotes && (strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`)) {
		return classStr
	}
	if at < 0 {
		return classRef
	}
	if at > len(line) {
		at = len(line)
	}

	i := 0
	for i < at {
		rest := line[i:]
		switch {
		case lineCommentAt(syn, rest):
			return classCom
		case syn.blockOpen != "" && strings.HasPrefix(rest, syn.blockOpen):
			end := strings.Index(rest[len(syn.blockOpen):], syn.blockClose)
			if end < 0 {
				return classCom
			}
			i += len(syn.blockOpen) + end + len(syn.blockClose)
			if at < i {
				return classCom
			}
			continue
		case syn.tripleQuotes && (strings.HasPrefix(rest, `"""`) || strings.HasPrefix(rest, `'''`)):
			delim := rest[:3]
			end := strings.Index(rest[3:], delim)
			if end < 0 {
				return classStr
			}
			i += 3 + end + 3
			if at < i {
				return classStr
			}
			continue
		}
		c := line[i]
		if strings.IndexByte(syn.quotes, c) >= 0 || (syn.backtick && c == '`') {
			closeAt, found := stringEnd(line, i, c, syn.backtick && c == '`')
			if !found {
				// Unterminated on this line: the match is inside the string.
				return classStr
			}
			if at <= closeAt {
				return classStr
			}
			i = closeAt + 1
			continue
		}
		i++
	}
	return classRef
}

func lineCommentAt(syn langSyntax, rest string) bool {
	for _, marker := range syn.lineComment {
		if strings.HasPrefix(rest, marker) {
			return true
		}
	}
	return false
}

// stringEnd returns the offset of the closing quote of the string opened at
// `start`. Backslash escapes are honoured except in raw (backtick) strings.
func stringEnd(line string, start int, quote byte, raw bool) (int, bool) {
	for i := start + 1; i < len(line); i++ {
		c := line[i]
		if !raw && c == '\\' {
			i++
			continue
		}
		if c == quote {
			return i, true
		}
	}
	return 0, false
}

// looksLikeCode avoids calling a pointer dereference or multiplication that
// starts a line (`*p = 1`) a comment: block-comment continuations are
// conventionally written as "* text" (star + space) or a bare "*".
func looksLikeCode(trimmed string) bool {
	if trimmed == "*" {
		return false
	}
	if len(trimmed) > 1 && trimmed[1] == ' ' {
		return false
	}
	return true
}

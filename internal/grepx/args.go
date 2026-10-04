package grepx

import "strings"

// argInfo is what the annotator needs to know about an rg command line. It is
// a conservative reading: anything that could change the output shape in a way
// the annotator cannot reproduce sets skipAnnotate and the run stays a pure
// passthrough.
type argInfo struct {
	skipAnnotate bool
	lineNumber   bool     // user's effective -n state (last of -n / -N wins)
	patterns     []string // -e/--regexp values, or the first positional
	paths        []string // positional paths after the pattern
	fixed        bool     // -F
	ignoreCase   bool     // -i
	smartCase    bool     // -S
	word         bool     // -w
	line         bool     // -x
	pcre         bool     // -P
	patternFile  bool     // -f/--file: patterns unknown
	terminator   int      // index of the `--` argument, -1 when absent
}

// shortValue lists rg short flags that take a value.
const shortValue = "ABCEMTdefgjmrt"

// shortSkip lists rg short flags whose output the annotator does not model.
const shortSkip = "lcoqIUzpba0hVv"

var longValue = map[string]bool{
	"after-context": true, "before-context": true, "context": true, "regexp": true,
	"file": true, "glob": true, "iglob": true, "type": true, "type-not": true,
	"type-add": true, "type-clear": true, "max-count": true, "max-depth": true,
	"maxdepth": true, "max-filesize": true, "max-columns": true, "replace": true,
	"threads": true, "encoding": true, "sort": true, "sortr": true, "colors": true,
	"color": true, "pre": true, "pre-glob": true, "path-separator": true,
	"ignore-file": true, "dfa-size-limit": true, "regex-size-limit": true,
	"context-separator": true, "field-context-separator": true,
	"field-match-separator": true, "engine": true, "hostname-bin": true,
	"hyperlink-format": true, "generate": true,
}

var longSkip = map[string]bool{
	"files-with-matches": true, "files-without-match": true, "count": true,
	"count-matches": true, "json": true, "files": true, "only-matching": true,
	"vimgrep": true, "quiet": true, "heading": true, "pretty": true, "column": true,
	"byte-offset": true, "passthru": true, "passthrough": true, "multiline": true,
	"multiline-dotall": true, "search-zip": true, "null": true, "null-data": true,
	"no-filename": true, "stats": true, "type-list": true, "help": true,
	"version": true, "pcre2-version": true, "generate": true, "pre": true,
	"replace": true, "text": true, "binary": true, "context-separator": true,
	"field-context-separator": true, "field-match-separator": true,
	"hyperlink-format": true, "invert-match": true, "debug": true, "trace": true,
	"include-zero": true, "stop-on-nonmatch": false,
}

// analyzeArgs walks the rg arguments without executing anything.
func analyzeArgs(args []string) argInfo {
	info := argInfo{terminator: -1}
	var positionals []string
	hasExplicitPattern := false
	uCount := 0
	endOfFlags := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if endOfFlags || arg == "-" || !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}
		if arg == "--" {
			info.terminator = i
			endOfFlags = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue := strings.Cut(arg[2:], "=")
			if longSkip[name] {
				info.skipAnnotate = true
			}
			if name == "color" && hasValue && value != "never" && value != "auto" {
				info.skipAnnotate = true
			}
			consumed := ""
			if longValue[name] {
				if hasValue {
					consumed = value
				} else if i+1 < len(args) {
					i++
					consumed = args[i]
					if name == "color" && consumed != "never" && consumed != "auto" {
						info.skipAnnotate = true
					}
				}
			}
			switch name {
			case "regexp":
				info.patterns = append(info.patterns, consumed)
				hasExplicitPattern = true
			case "file":
				info.patternFile = true
				hasExplicitPattern = true
			case "line-number":
				info.lineNumber = true
			case "no-line-number":
				info.lineNumber = false
			case "fixed-strings":
				info.fixed = true
			case "no-fixed-strings":
				info.fixed = false
			case "ignore-case":
				info.ignoreCase = true
			case "smart-case":
				info.smartCase = true
			case "case-sensitive":
				info.ignoreCase, info.smartCase = false, false
			case "word-regexp":
				info.word = true
			case "line-regexp":
				info.line = true
			case "pcre2":
				info.pcre = true
			}
			continue
		}
		// short flag cluster
		cluster := arg[1:]
		for j := 0; j < len(cluster); j++ {
			c := cluster[j]
			if strings.IndexByte(shortValue, c) >= 0 {
				value := cluster[j+1:]
				if value == "" && i+1 < len(args) {
					i++
					value = args[i]
				}
				switch c {
				case 'e':
					info.patterns = append(info.patterns, value)
					hasExplicitPattern = true
				case 'f':
					info.patternFile = true
					hasExplicitPattern = true
				case 'r':
					info.skipAnnotate = true
				}
				break
			}
			if strings.IndexByte(shortSkip, c) >= 0 {
				info.skipAnnotate = true
			}
			switch c {
			case 'n':
				info.lineNumber = true
			case 'N':
				info.lineNumber = false
			case 'F':
				info.fixed = true
			case 'i':
				info.ignoreCase = true
			case 'S':
				info.smartCase = true
			case 's':
				info.ignoreCase, info.smartCase = false, false
			case 'w':
				info.word = true
			case 'x':
				info.line = true
			case 'P':
				info.pcre = true
			case 'u':
				uCount++
			}
		}
	}
	if uCount >= 3 {
		info.skipAnnotate = true
	}
	if hasExplicitPattern {
		info.paths = positionals
	} else if len(positionals) > 0 {
		info.patterns = []string{positionals[0]}
		info.paths = positionals[1:]
	}
	return info
}

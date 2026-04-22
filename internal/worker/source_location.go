package worker

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
)

var identifierTokenPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

var skippedIdentifierKeywords = map[string]struct{}{
	"abstract":  {},
	"async":     {},
	"class":     {},
	"const":     {},
	"def":       {},
	"default":   {},
	"enum":      {},
	"export":    {},
	"final":     {},
	"function":  {},
	"interface": {},
	"internal":  {},
	"let":       {},
	"partial":   {},
	"private":   {},
	"protected": {},
	"public":    {},
	"readonly":  {},
	"record":    {},
	"sealed":    {},
	"static":    {},
	"struct":    {},
	"type":      {},
	"var":       {},
}

func inferLineAndOffset(filePath string, lineNumber int, symbol string) (int, int) {
	file, err := os.Open(filePath)
	if err != nil {
		return normalizeLineNumber(lineNumber, 0), 1
	}
	defer file.Close()

	preferredLine := lineNumber
	if preferredLine <= 0 {
		preferredLine = 1
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	lineNo := 0
	preferredText := ""
	fallbackLine := 0
	fallbackOffset := 0
	lastText := ""
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			line = strings.TrimRight(strings.ReplaceAll(line, "\r\n", "\n"), "\n")
			line = strings.TrimRight(line, "\r")
			lastText = line
			if lineNo == preferredLine {
				preferredText = line
				if offset := exactIdentifierColumn(line, symbol); offset > 0 {
					return lineNo, offset
				}
			}
			if symbol != "" && fallbackOffset == 0 {
				if offset := exactIdentifierColumn(line, symbol); offset > 0 {
					fallbackLine = lineNo
					fallbackOffset = offset
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return normalizeLineNumber(lineNumber, lineNo), 1
		}
	}

	if fallbackOffset > 0 {
		return fallbackLine, fallbackOffset
	}
	if lineNo == 0 {
		return 1, 1
	}
	normalizedLine := normalizeLineNumber(lineNumber, lineNo)
	if preferredText == "" && normalizedLine == lineNo {
		preferredText = lastText
	}
	return normalizedLine, inferOffsetFromLine(preferredText, symbol)
}

func findSymbolLocation(lines []string, preferredLine int, symbol string) (int, int, bool) {
	if preferredLine > 0 && preferredLine <= len(lines) {
		if offset := exactIdentifierColumn(lines[preferredLine-1], symbol); offset > 0 {
			return preferredLine, offset, true
		}
	}
	for index, line := range lines {
		if offset := exactIdentifierColumn(line, symbol); offset > 0 {
			return index + 1, offset, true
		}
	}
	return 0, 0, false
}

func exactIdentifierColumn(line string, symbol string) int {
	trimmed := strings.TrimRight(line, "\r")
	if trimmed == "" || symbol == "" {
		return 0
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(symbol) + `\b`)
	if location := pattern.FindStringIndex(trimmed); location != nil {
		return location[0] + 1
	}
	if index := strings.Index(trimmed, symbol); index >= 0 {
		return index + 1
	}
	return 0
}

func inferOffsetFromLine(line string, symbol string) int {
	if offset := exactIdentifierColumn(line, symbol); offset > 0 {
		return offset
	}

	trimmed := strings.TrimRight(line, "\r")
	tokens := identifierTokenPattern.FindAllStringIndex(trimmed, -1)
	for _, token := range tokens {
		candidate := strings.ToLower(trimmed[token[0]:token[1]])
		if _, skip := skippedIdentifierKeywords[candidate]; skip {
			continue
		}
		return token[0] + 1
	}
	if len(tokens) > 0 {
		return tokens[0][0] + 1
	}

	leftTrimmed := strings.TrimLeft(trimmed, " \t")
	if leftTrimmed == "" {
		return 1
	}
	return len(trimmed) - len(leftTrimmed) + 1
}

func normalizeLineNumber(lineNumber int, lineCount int) int {
	if lineCount <= 0 {
		if lineNumber > 0 {
			return lineNumber
		}
		return 1
	}
	if lineNumber <= 0 {
		return 1
	}
	if lineNumber > lineCount {
		return lineCount
	}
	return lineNumber
}

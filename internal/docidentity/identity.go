// Package docidentity contains the canonical literal grammar shared by
// documentation extraction and identifier search.
package docidentity

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ExtractionVersion changes whenever the persisted reference grammar changes.
// It is stamped only by extractor-backed document publication.
const ExtractionVersion = "doc-identity-v1"

var candidatePattern = regexp.MustCompile(`(?:FL|RS|RF|TP|TECH|CT|DB|AE)-[A-Z0-9]+(?:-[A-Z0-9]+)*(?:\.md)?`)

// Match reports whether identifier occurs as a complete documentation ID
// literal. An optional .md suffix is accepted as part of a Markdown link; a
// terminal sentence dot is accepted, while arbitrary dotted continuations are
// not.
func Match(text, identifier string) bool {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || strings.ContainsAny(identifier, "\r\n") {
		return false
	}
	identifierLow := strings.ToLower(identifier)
	textLow := strings.ToLower(text)
	for from := 0; from <= len(textLow); {
		relative := strings.Index(textLow[from:], identifierLow)
		if relative < 0 {
			return false
		}
		start := from + relative
		end := start + len(identifierLow)
		if boundaryBefore(text, start) {
			if strings.HasSuffix(identifierLow, ".md") {
				if boundaryAfter(text, end) {
					return true
				}
			} else if strings.HasPrefix(textLow[end:], ".md") && boundaryAfter(text, end+3) {
				return true
			} else if boundaryAfter(text, end) {
				return true
			}
		}
		from = start + 1
	}
	return false
}

// Extract returns complete standard ID literals, canonicalized without an
// optional .md suffix and preserving distinct hyphenated IDs.
func Extract(text string) []string {
	matches := candidatePattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	result := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if !Match(text, match) {
			continue
		}
		canonical := match
		if len(canonical) >= 3 && strings.EqualFold(canonical[len(canonical)-3:], ".md") {
			canonical = canonical[:len(canonical)-3]
		}
		key := strings.ToLower(canonical)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, canonical)
	}
	return result
}

func isContinuation(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

func boundaryBefore(text string, offset int) bool {
	if offset <= 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:offset])
	return !isContinuation(r) && r != '.'
}

func boundaryAfter(text string, offset int) bool {
	if offset >= len(text) {
		return true
	}
	r, size := utf8.DecodeRuneInString(text[offset:])
	if isContinuation(r) {
		return false
	}
	if r == '.' {
		if offset+size >= len(text) {
			return true
		}
		next, _ := utf8.DecodeRuneInString(text[offset+size:])
		return unicode.IsSpace(next) || strings.ContainsRune(",;:!?)]}", next)
	}
	return true
}

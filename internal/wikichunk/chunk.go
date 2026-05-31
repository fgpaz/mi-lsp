package wikichunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type Chunk struct {
	ChunkID     string
	Heading     string
	Level       int
	StartLine   int
	EndLine     int
	Text        string
	ContentHash string
}

// ChunkByHeading splits content into chunks by ATX headings (##, ###, etc.),
// respecting fenced code blocks and carrying the H1 title as context.
func ChunkByHeading(content string) []Chunk {
	lines := splitLines(content)
	if len(lines) == 0 {
		return []Chunk{}
	}

	// First pass: find all headings (outside fences)
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	inFence := false
	fenceDelim := ""
	var h1Title string
	type headingInfo struct {
		lineIdx int
		level   int
		text    string
	}
	var headings []headingInfo

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track fenced code blocks
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				fenceDelim = trimmed[:3]
			} else if strings.HasPrefix(trimmed, fenceDelim) {
				inFence = false
				fenceDelim = ""
			}
		}

		// Only parse headings outside fenced blocks
		if !inFence {
			match := headingRegex.FindStringSubmatch(trimmed)
			if len(match) == 3 {
				level := len(match[1])
				text := strings.TrimSpace(match[2])

				if level == 1 && h1Title == "" {
					h1Title = text
				}

				headings = append(headings, headingInfo{
					lineIdx: i,
					level:   level,
					text:    text,
				})
			}
		}
	}

	// Build chunks
	var chunks []Chunk
	ordinal := 0

	// Find first heading with level >= 2
	firstMajorHeadingIdx := -1
	for i, h := range headings {
		if h.level >= 2 {
			firstMajorHeadingIdx = i
			break
		}
	}

	// Intro chunk: lines 0 to (but not including) first level>=2 heading
	if firstMajorHeadingIdx == -1 || headings[firstMajorHeadingIdx].lineIdx > 0 {
		introStartLine := 1
		introEndLine := len(lines)
		if firstMajorHeadingIdx != -1 {
			introEndLine = headings[firstMajorHeadingIdx].lineIdx
		}

		introText := strings.Join(lines[introStartLine-1:introEndLine], "\n")
		if strings.TrimSpace(introText) != "" {
			heading := h1Title
			if heading == "" {
				heading = "(intro)"
			}

			chunk := Chunk{
				ChunkID:     fmt.Sprintf("%s#%d", slug(heading), ordinal),
				Heading:     heading,
				Level:       1,
				StartLine:   introStartLine,
				EndLine:     introEndLine,
				Text:        introText,
				ContentHash: hashText(introText),
			}
			chunks = append(chunks, chunk)
			ordinal++
		}
	}

	// Process heading-based chunks (level >= 2)
	// Only create chunks for headings that aren't nested deeper than a previous section heading
	if firstMajorHeadingIdx >= 0 {
		// Track the minimum level we've seen for top-level sections
		minSectionLevel := headings[firstMajorHeadingIdx].level

		for headingIdx := firstMajorHeadingIdx; headingIdx < len(headings); headingIdx++ {
			h := headings[headingIdx]

			// Skip if this heading is nested deeper than a section heading
			// (i.e., it's a subsection of a section, not a new section)
			if h.level > minSectionLevel {
				continue
			}

			// This is a new section (level <= minSectionLevel)
			// Find the end line: the next heading with level < this heading's level
			// Actually: next heading with level <= this heading's level that was NOT skipped
			endLine := len(lines)
			for nextIdx := headingIdx + 1; nextIdx < len(headings); nextIdx++ {
				// Look for a heading that would end this section
				// A section ends when we find another heading at same or lower level
				if headings[nextIdx].level <= h.level {
					endLine = headings[nextIdx].lineIdx
					break
				}
			}

			chunkText := strings.Join(lines[h.lineIdx:endLine], "\n")
			if strings.TrimSpace(chunkText) != "" {
				chunk := Chunk{
					ChunkID:     fmt.Sprintf("%s#%d", slug(h.text), ordinal),
					Heading:     h.text,
					Level:       h.level,
					StartLine:   h.lineIdx + 1,
					EndLine:     endLine,
					Text:        chunkText,
					ContentHash: hashText(chunkText),
				}
				chunks = append(chunks, chunk)
				ordinal++
			}
		}
	}

	return chunks
}

// slug converts a string to a slug: lowercase, alphanumeric + dash, trim dashes.
func slug(s string) string {
	s = strings.ToLower(s)
	var result []rune
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result = append(result, r)
		} else if unicode.IsSpace(r) || r == '_' || r == '-' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result = append(result, '-')
			}
		}
	}
	s = strings.Trim(string(result), "-")
	if s == "" {
		return "section"
	}
	return s
}

// splitLines splits content by \n and \r\n.
func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(content, "\n")
}

// hashText computes SHA256 hex hash of text.
func hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

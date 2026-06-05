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

const maxChunkChars = 4500

type chunkPart struct {
	Heading   string
	Level     int
	StartLine int
	EndLine   int
	Text      string
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

			appendChunkParts(&chunks, heading, 1, introStartLine, introEndLine, introText, &ordinal)
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
			appendChunkParts(&chunks, h.text, h.level, h.lineIdx+1, endLine, chunkText, &ordinal)
		}
	}

	return chunks
}

func appendChunkParts(chunks *[]Chunk, heading string, level int, startLine int, endLine int, text string, ordinal *int) {
	for _, part := range splitLargeChunk(heading, level, startLine, endLine, text) {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		chunk := Chunk{
			ChunkID:     fmt.Sprintf("%s#%d", slug(part.Heading), *ordinal),
			Heading:     part.Heading,
			Level:       part.Level,
			StartLine:   part.StartLine,
			EndLine:     part.EndLine,
			Text:        part.Text,
			ContentHash: hashText(part.Text),
		}
		*chunks = append(*chunks, chunk)
		(*ordinal)++
	}
}

func splitLargeChunk(heading string, level int, startLine int, endLine int, text string) []chunkPart {
	text = strings.Trim(text, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if len(text) <= maxChunkChars {
		return []chunkPart{{Heading: heading, Level: level, StartLine: startLine, EndLine: endLine, Text: text}}
	}

	lines := splitLines(text)
	if nested := splitByNestedHeadings(lines, heading, level, startLine); len(nested) > 0 {
		var parts []chunkPart
		for _, part := range nested {
			if len(part.Text) <= maxChunkChars {
				parts = append(parts, part)
				continue
			}
			parts = append(parts, splitLargeChunk(part.Heading, part.Level, part.StartLine, part.EndLine, part.Text)...)
		}
		return parts
	}

	return splitByParagraphs(lines, heading, level, startLine)
}

func splitByNestedHeadings(lines []string, heading string, level int, absoluteStartLine int) []chunkPart {
	type nestedHeading struct {
		lineIdx int
		level   int
		text    string
	}
	var headings []nestedHeading
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	inFence := false
	fenceDelim := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				fenceDelim = trimmed[:3]
			} else if strings.HasPrefix(trimmed, fenceDelim) {
				inFence = false
				fenceDelim = ""
			}
		}
		if inFence || i == 0 {
			continue
		}
		match := headingRegex.FindStringSubmatch(trimmed)
		if len(match) != 3 {
			continue
		}
		nestedLevel := len(match[1])
		if nestedLevel <= level {
			continue
		}
		headings = append(headings, nestedHeading{
			lineIdx: i,
			level:   nestedLevel,
			text:    strings.TrimSpace(match[2]),
		})
	}
	if len(headings) == 0 {
		return nil
	}

	var parts []chunkPart
	if headings[0].lineIdx > 0 {
		if part, ok := makePart(heading+" (overview)", level, absoluteStartLine, lines[:headings[0].lineIdx]); ok {
			parts = append(parts, part)
		}
	}
	for idx, h := range headings {
		endIdx := len(lines)
		for nextIdx := idx + 1; nextIdx < len(headings); nextIdx++ {
			if headings[nextIdx].level <= h.level {
				endIdx = headings[nextIdx].lineIdx
				break
			}
		}
		if part, ok := makePart(h.text, h.level, absoluteStartLine+h.lineIdx, lines[h.lineIdx:endIdx]); ok {
			parts = append(parts, part)
		}
	}
	return parts
}

func splitByParagraphs(lines []string, heading string, level int, absoluteStartLine int) []chunkPart {
	var parts []chunkPart
	partStart := 0
	chars := 0
	partIndex := 1
	inFence := false
	fenceDelim := ""

	flush := func(end int) {
		if end <= partStart {
			return
		}
		partHeading := heading
		if partIndex > 1 {
			partHeading = fmt.Sprintf("%s (part %d)", heading, partIndex)
		}
		if part, ok := makePart(partHeading, level, absoluteStartLine+partStart, lines[partStart:end]); ok {
			parts = append(parts, part)
			partIndex++
		}
		partStart = end
		chars = 0
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if chars > 0 && chars+len(line)+1 > maxChunkChars {
			flush(i)
		}
		if len(line)+1 > maxChunkChars {
			partHeading := heading
			if partIndex > 1 {
				partHeading = fmt.Sprintf("%s (part %d)", heading, partIndex)
			}
			for _, segment := range splitLongLine(line, maxChunkChars) {
				if strings.TrimSpace(segment) == "" {
					continue
				}
				parts = append(parts, chunkPart{
					Heading:   partHeading,
					Level:     level,
					StartLine: absoluteStartLine + i,
					EndLine:   absoluteStartLine + i,
					Text:      segment,
				})
				partIndex++
				partHeading = fmt.Sprintf("%s (part %d)", heading, partIndex)
			}
			partStart = i + 1
			chars = 0
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				fenceDelim = trimmed[:3]
			} else if strings.HasPrefix(trimmed, fenceDelim) {
				inFence = false
				fenceDelim = ""
			}
		}
		chars += len(line) + 1
		if chars < maxChunkChars {
			continue
		}
		if inFence {
			continue
		}
		if trimmed == "" {
			flush(i + 1)
		}
	}
	flush(len(lines))
	return parts
}

func splitLongLine(line string, limit int) []string {
	if len(line) <= limit {
		return []string{line}
	}
	var parts []string
	for len(line) > limit {
		cut := strings.LastIndexAny(line[:limit], " \t,;|")
		if cut < limit/2 {
			cut = limit
		}
		parts = append(parts, strings.TrimSpace(line[:cut]))
		line = strings.TrimSpace(line[cut:])
	}
	if line != "" {
		parts = append(parts, line)
	}
	return parts
}

func makePart(heading string, level int, absoluteStartLine int, lines []string) (chunkPart, bool) {
	start := 0
	end := len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return chunkPart{}, false
	}
	text := strings.Join(lines[start:end], "\n")
	return chunkPart{
		Heading:   heading,
		Level:     level,
		StartLine: absoluteStartLine + start,
		EndLine:   absoluteStartLine + end - 1,
		Text:      text,
	}, true
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

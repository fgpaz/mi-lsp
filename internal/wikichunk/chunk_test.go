package wikichunk

import (
	"strings"
	"testing"
)

// Test basic doc with intro, two sections
func TestBasicSectionSplit(t *testing.T) {
	content := `# Title
intro text

## A
body A

## B
body B
`

	chunks := ChunkByHeading(content)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// Intro chunk
	if chunks[0].Heading != "Title" {
		t.Errorf("chunk 0: expected heading 'Title', got '%s'", chunks[0].Heading)
	}
	if chunks[0].Level != 1 {
		t.Errorf("chunk 0: expected level 1, got %d", chunks[0].Level)
	}

	// Section A
	if chunks[1].Heading != "A" {
		t.Errorf("chunk 1: expected heading 'A', got '%s'", chunks[1].Heading)
	}
	if chunks[1].Level != 2 {
		t.Errorf("chunk 1: expected level 2, got %d", chunks[1].Level)
	}

	// Section B
	if chunks[2].Heading != "B" {
		t.Errorf("chunk 2: expected heading 'B', got '%s'", chunks[2].Heading)
	}
	if chunks[2].Level != 2 {
		t.Errorf("chunk 2: expected level 2, got %d", chunks[2].Level)
	}

	// Check hashes are non-empty
	for i, chunk := range chunks {
		if chunk.ContentHash == "" {
			t.Errorf("chunk %d: empty content hash", i)
		}
	}
}

// Test nested headings
func TestNestedHeadings(t *testing.T) {
	content := `## A
text A
### A1
text A1
## B
text B
`

	chunks := ChunkByHeading(content)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	// Chunk A should include A1 (since ### > ##)
	if chunks[0].Heading != "A" {
		t.Errorf("chunk 0: expected heading 'A', got '%s'", chunks[0].Heading)
	}
	if !strings.Contains(chunks[0].Text, "### A1") {
		t.Errorf("chunk 0: expected to contain '### A1', got: %s", chunks[0].Text)
	}

	// Chunk B
	if chunks[1].Heading != "B" {
		t.Errorf("chunk 1: expected heading 'B', got '%s'", chunks[1].Heading)
	}
}

// Test fenced code block
func TestFencedCodeBlock(t *testing.T) {
	content := `## Real
Some text
` + "```" + `
## NotAHeading
inside code block
` + "```" + `
More text
`

	chunks := ChunkByHeading(content)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (fenced heading ignored), got %d", len(chunks))
	}

	if chunks[0].Heading != "Real" {
		t.Errorf("expected heading 'Real', got '%s'", chunks[0].Heading)
	}

	// The chunk should contain the fenced block with the inner heading
	if !strings.Contains(chunks[0].Text, "## NotAHeading") {
		t.Errorf("expected chunk to contain the fenced block with '## NotAHeading'")
	}
}

// Test determinism
func TestDeterminism(t *testing.T) {
	content := `# Title
intro

## Section A
body A

## Section B
body B
`

	chunks1 := ChunkByHeading(content)
	chunks2 := ChunkByHeading(content)

	if len(chunks1) != len(chunks2) {
		t.Fatalf("different chunk counts: %d vs %d", len(chunks1), len(chunks2))
	}

	for i := range chunks1 {
		if chunks1[i].ChunkID != chunks2[i].ChunkID {
			t.Errorf("chunk %d: different ChunkIDs: '%s' vs '%s'", i, chunks1[i].ChunkID, chunks2[i].ChunkID)
		}
		if chunks1[i].ContentHash != chunks2[i].ContentHash {
			t.Errorf("chunk %d: different content hashes: '%s' vs '%s'", i, chunks1[i].ContentHash, chunks2[i].ContentHash)
		}
	}
}

// Test slug function
func TestSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"With_Underscores", "with-underscores"},
		{"Already-Dashed", "already-dashed"},
		{"---Leading---Dashes---", "leading-dashes"},
		{"123Numbers456", "123numbers456"},
		{"", "section"},
		{"---", "section"},
	}

	for _, tc := range tests {
		result := slug(tc.input)
		if result != tc.expected {
			t.Errorf("slug('%s'): expected '%s', got '%s'", tc.input, tc.expected, result)
		}
	}
}

// Test line ranges
func TestLineRanges(t *testing.T) {
	content := `# Title
intro line 2
intro line 3
## A
section A line
## B
section B line
`

	chunks := ChunkByHeading(content)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// Intro should start at line 1
	if chunks[0].StartLine != 1 {
		t.Errorf("chunk 0: expected StartLine 1, got %d", chunks[0].StartLine)
	}
	// Intro should end before the first ## (at line 4)
	if chunks[0].EndLine != 3 {
		t.Errorf("chunk 0: expected EndLine 3, got %d", chunks[0].EndLine)
	}

	// Section A starts at line 4
	if chunks[1].StartLine != 4 {
		t.Errorf("chunk 1: expected StartLine 4, got %d", chunks[1].StartLine)
	}
	// Section A ends before ## B (at line 6)
	if chunks[1].EndLine != 5 {
		t.Errorf("chunk 1: expected EndLine 5, got %d", chunks[1].EndLine)
	}

	// Section B starts at line 6
	if chunks[2].StartLine != 6 {
		t.Errorf("chunk 2: expected StartLine 6, got %d", chunks[2].StartLine)
	}
}

// Test empty intro is skipped
func TestEmptyIntroSkipped(t *testing.T) {
	content := `## First Section
content here
## Second Section
more content
`

	chunks := ChunkByHeading(content)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (no empty intro), got %d", len(chunks))
	}

	if chunks[0].Heading != "First Section" {
		t.Errorf("chunk 0: expected heading 'First Section', got '%s'", chunks[0].Heading)
	}
}

// Test tilde fenced code blocks
func TestTildeFencedCodeBlock(t *testing.T) {
	content := `## Real
Some text
~~~
## AlsoNotAHeading
inside code block
~~~
More text
`

	chunks := ChunkByHeading(content)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Heading != "Real" {
		t.Errorf("expected heading 'Real', got '%s'", chunks[0].Heading)
	}

	if !strings.Contains(chunks[0].Text, "## AlsoNotAHeading") {
		t.Errorf("expected chunk to contain the fenced block")
	}
}

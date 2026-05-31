<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md","RF-IDX-001"]
allowed_paths: ["internal/wikichunk/**"]
forbidden_paths: [".git/**",".mi-lsp/**","worker-dotnet/**",".env",".env.*"]
verify: ["go test ./internal/wikichunk/..."]
stop_if: ["a non-stdlib import would be required"]
secret_scan: "none"
-->

# Task T2: internal/wikichunk — markdown heading chunker

## Shared Context
**Goal:** New pure-Go package that splits a markdown document into section chunks by ATX headings (`##`+), carrying the `#` H1 title as context, never splitting inside fenced code blocks.
**Stack:** Go 1.24, stdlib only (`strings`, `bufio`, `crypto/sha256`, `encoding/hex`, `fmt`). NO new deps.
**Architecture:** Leaf package; `internal/docgraph` will call it later (other task).

## Locked Decisions
- A "chunk" = one section: a heading line of level ≥2 plus all body lines until the next heading of level ≤ that heading's level (or EOF). The preamble before the first `##` (including the H1 `#` title and intro) is its own chunk with heading = the H1 title text (or "(intro)").
- Heading detection ignores `#` sequences INSIDE fenced code blocks (```` ``` ```` or `~~~`).
- `ContentHash` = hex sha256 of the chunk's normalized raw text (trim trailing spaces per line, collapse blank runs is NOT required — just sha256 of the exact chunk text).
- `ChunkID` = stable: `fmt.Sprintf("%s#%d", slug(headingText), ordinal)` where ordinal is 0-based chunk index in the doc. Keep it deterministic.
- Do NOT run git. Do NOT edit files outside `internal/wikichunk/`.

## Task Metadata
```yaml
id: T2
depends_on: []
agent_type: general-purpose
goal_id: G1
expected_outcome: "internal/wikichunk splits markdown into heading sections with stable ids/hashes; unit tests pass."
files:
  - create: internal/wikichunk/chunk.go
  - create: internal/wikichunk/chunk_test.go
complexity: medium
done_when:
  - "go test ./internal/wikichunk/... exits 0"
evidence_expected:
  - "go test ./internal/wikichunk/... output"
stop_if:
  - "a non-stdlib import would be required"
```

## Reference
Design doc: `.docs/raw/plans/2026-05-30-semantic-wiki-nav-design.md` (D4 chunking).

## Prompt
Create package `wikichunk` at `C:/repos/mios/mi-lsp/internal/wikichunk/`. Implement:

```go
type Chunk struct {
    ChunkID     string
    Heading     string // heading text (no leading #), or H1 title for the intro chunk
    Level       int    // heading level (1 for intro/title chunk, 2.. for sections)
    StartLine   int    // 1-based line where the chunk starts
    EndLine     int    // 1-based inclusive line where the chunk ends
    Text        string // full raw text of the chunk including its heading line
    ContentHash string // hex sha256 of Text
}
func ChunkByHeading(content string) []Chunk
```

Rules:
- Split `content` into lines (handle `\n` and `\r\n`). Track a fenced-code-block flag toggled by lines whose trimmed form starts with ```` ``` ```` or `~~~`. While inside a fence, lines are never treated as headings.
- A heading line matches `^(#{1,6})\s+(.*)$` when NOT inside a fence.
- The first chunk covers from line 1 up to (but not including) the first level≥2 heading; its `Heading` = first H1 title text if present else "(intro)", `Level` = 1. If the doc starts with a level≥2 heading immediately, the intro chunk may be empty — skip emitting an empty intro chunk (Text trimmed == "").
- Each subsequent chunk starts at a level≥2 heading and runs until the next heading of level ≤ its own level, or EOF.
- Trim a chunk and skip it if its Text is empty after trimming.
- `slug(s)`: lowercase, replace non-alphanumeric runs with `-`, trim `-`; empty ⇒ "section".

Tests (`chunk_test.go`):
- A doc with `# Title`, intro text, `## A`, body, `## B`, body ⇒ 3 chunks (intro+A+B) with correct headings, levels, line ranges, and non-empty hashes.
- Nested: `## A`, `### A1`, `## B` ⇒ chunk A spans through A1 until B (since `###` > `##`), then B. Verify A.Text contains the A1 heading.
- Fenced code: a `## Real` section containing a fenced block whose body has a line `## NotAHeading` ⇒ that inner line does NOT start a new chunk.
- Determinism: ChunkByHeading called twice on same input ⇒ identical ChunkIDs and hashes.

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp`. Create `internal/wikichunk/`.
2. Write `chunk.go` then `chunk_test.go` per the Prompt.
3. Run `go test ./internal/wikichunk/...`; fix within the package only. Do not run git.
4. Report final test output verbatim.

## Skeleton
```go
package wikichunk

type Chunk struct { ChunkID, Heading string; Level, StartLine, EndLine int; Text, ContentHash string }
func ChunkByHeading(content string) []Chunk { /* line scan, fence-aware, level-based sectioning */ }
func slug(s string) string { /* lowercase + dash */ }
```

## Verify
`go test ./internal/wikichunk/...` → `ok  github.com/fgpaz/mi-lsp/internal/wikichunk`

## Commit
(orchestrator commits) — suggested: `feat(wikichunk): heading-aware markdown chunker`

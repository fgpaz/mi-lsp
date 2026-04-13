# Compound Commands

Use this reference when a task would otherwise require several separate reads or searches.

## `nav multi-read`

```powershell
mi-lsp nav multi-read file1.cs:1-120 file2.cs:260-440 file3.tsx:1-80 --workspace <alias> --format toon
```

- Use for 2+ known files or ranges
- Format supports `file:start-end`, `file:line`, or `file`
- Prefer over sequential `Get-Content`

## `nav search --include-content`

```powershell
mi-lsp nav search "PublishAsync" --include-content --workspace <alias> --format toon
mi-lsp nav search "MapPost" --include-content --context-mode symbol --workspace <alias> --format toon
mi-lsp nav search "pattern" --include-content --context-lines 30 --context-mode lines --workspace <alias> --format toon
```

- `nav search` takes one positional `pattern`; quote it when it contains spaces.
- If the pattern is regex-like, keep it quoted and add `--regex`.
- `hybrid` is the default mode
- Prefer this over `search` plus N file reads

## `nav batch`

```powershell
echo '[
  {"id":"s1","op":"nav.search","params":{"pattern":"MapPost","include_content":true}},
  {"id":"r1","op":"nav.multi-read","params":{"items":["src/Program.cs:1-50","src/Model.cs:1-80"]}},
  {"id":"f1","op":"nav.find","params":{"pattern":"IExpenseRepository","exact":true}},
  {"id":"c1","op":"nav.context","params":{"file":"src/Handler.cs","line":42}}
]' | mi-lsp nav batch --workspace <alias> --format toon
```

- Use when you would otherwise do several heterogeneous `nav` commands in sequence
- Prefer the default parallel behavior unless ordering matters

## `nav related`

```powershell
mi-lsp nav related IExpenseRepository --workspace <alias> --format toon
mi-lsp nav related MyService --depth callers,tests --workspace <alias> --format toon
```

- Best one-call deep-dive for a symbol
- Prefer over `refs` plus several manual reads

## `nav workspace-map`

```powershell
mi-lsp nav workspace-map --workspace <alias> --axi --format toon
```

- Best first command on an unfamiliar parent folder or multi-repo workspace

## `nav diff-context`

```powershell
mi-lsp nav diff-context HEAD~1 --workspace <alias> --format toon
mi-lsp nav diff-context --include-content --workspace <alias> --format toon
mi-lsp nav diff-context main --workspace <alias> --format toon
```

- Use for PR review, impact analysis, or changed-symbol inspection

## `nav trace`

```powershell
mi-lsp nav trace RF-QRY-003 --workspace <alias> --format toon
mi-lsp nav trace --all --summary --workspace <alias> --format toon
mi-lsp nav trace --all --workspace <alias> --format toon
```

- Use to check which code implements a specific RF requirement
- Reads `implements:` and `tests:` from YAML frontmatter in `04_RF/*.md` docs
- Falls back to heuristic keyword matching when no explicit markers exist
- `--all --summary` gives a quick compliance overview across all RFs

## `nav intent`

```powershell
mi-lsp nav intent "where do we handle workspace routing fallback?" --workspace <alias> --format toon
mi-lsp nav intent "error handling daemon" --top 20 --workspace <alias> --format toon
mi-lsp nav intent "forgot password frontend" --workspace <alias> --repo web --format toon
```

- Use when you know what the code does but not the symbol name
- BM25 scoring over enriched index: symbol names, signatures, doc comments, file paths
- Complementary to `nav ask` (which searches docs) and `nav search` (which matches literal text)
- In workspaces `container`, prefer `--repo` when you already know the child repo you want to inspect
- Requires a prior `mi-lsp index` to populate search text

## Cross-workspace search

```powershell
mi-lsp nav search "PublishAsync" --all-workspaces --format toon
mi-lsp nav find IExpenseRepository --all-workspaces --format toon
```

- Use only when the task genuinely spans all registered workspaces
- Mention the workspace for each relevant result in your answer

## Format selection guide

```powershell
# TOON — recommended default, ~20-40% fewer tokens on large arrays
mi-lsp nav search "pattern" --workspace <alias> --format toon

# YAML — readable line-by-line, piping to YAML tools
mi-lsp nav workspace-map --workspace <alias> --format yaml

# compact JSON — backward compat, jq pipelines, strict JSON required
mi-lsp nav search "pattern" --workspace <alias> --format compact
```

**Reading the `hint` field:**
If a response returns `items: []` and includes a `hint`, act on it before retrying:
```
"0 matches for X in workspace Y"         → try different keyword
"pattern looks regex-like, rerun --regex" → add --regex flag
"0 matches: search timed out"            → narrow the search scope
"daemon_unavailable; served from..."     → daemon not running, result is text-only
```

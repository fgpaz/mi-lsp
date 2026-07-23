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

## `nav wiki`

```powershell
mi-lsp nav wiki search "workflow masterformularios" --workspace <alias> --layer RF,FL,CT,TP --format toon
mi-lsp nav wiki pack "workflow con masterformularios" --workspace <alias> --format toon
mi-lsp nav wiki trace RF-QRY-003 --workspace <alias> --format toon
```

- Use for docs-first exploration across RF/FL/TP/CT/TECH/DB.
- Follow returned `next_queries` before broadening to raw search.
- If an old prompt says `nav ask --repo docs`, treat `--repo` as compatibility-only and rerun through `nav wiki`.

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

## Intent planner and bounded graph commands

Use `nav intent` first, and mandatorily for every supported goal-shaped request. Supported intents route automatically without a routing opt-out and return a bounded preview. `explain-change` is an intent/operation of `nav intent`; the user's request need not contain that literal alias. Preserve every available section, especially `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki`, together with graph, evidence, fallbacks, candidates, and omissions. Use the exact `expansions[].command` as the second query and read its `reason`.

```powershell
mi-lsp nav intent "explain the change and its impact" --workspace <alias> --format toon
mi-lsp nav intent "who calls MySymbol?" --workspace <alias> --format toon
mi-lsp nav intent "what does MySymbol call?" --workspace <alias> --format toon
mi-lsp nav intent "path between FromSymbol and ToSymbol" --workspace <alias> --format toon
mi-lsp nav intent "explain edge edge-cross-rid-123" --workspace <alias> --format toon
mi-lsp nav intent "show the neighborhood of MySymbol" --workspace <alias> --format toon
```

The corresponding exposed commands are `nav explain-change`, `nav affected`, `nav callers`, `nav callees`, `nav path`, `nav explain`, and `nav neighbors`. `nav related` is separate and exposes definition/callers/implementors/tests. Graph commands support bounded `--depth`, `--limit`, `--token-budget`, optional `--edge`, and `--generation`; `nav path` takes `<from> <to>`, while `nav explain` takes one edge-cross-RID selector. Redact evidence: never emit prompts, transcripts, secrets, PII, PHI, argv, or raw patterns, and do not invent model/provider metadata.

A timeout, silence, `DONE`, or `PASS` without fresh evidence is not PASS and is not a fallback trigger. Keep any partial envelope, mark it incomplete, and narrow or expand only with an explicit command. External fallback is allowed only for `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete`. Allow at most two same-context recoveries with a smaller packet and no unchanged retry; use 180/300-second soft/hard watchdogs, at most six practical lanes with exclusive `allowed_paths`, fail-closed joins, and fresh verification.

## `nav prepare` versus `nav edit-plan`

```powershell
mi-lsp nav prepare "review the routing change" --affected internal/service/intent.go --workspace <alias> --format toon
mi-lsp nav edit-plan --packet .mi-lsp/plan.json --workspace <alias> --format toon
```

`nav prepare` is read-only semantic preparation and accepts task-specific `--affected` paths or a validating `--plan`. `nav edit-plan` is a guarded patch packet surface, dry-run by default; writes require `--apply --experimental-apply` and its guardrails. Do not conflate the two.

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

## Wiki validators

```powershell
mi-lsp nav wiki validate-harness --workspace <alias> --ids CT-NAV-INTENT --format toon
mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-INTENT --format toon
mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
```

`validate-harness` is scoped to SDD-HARNESS-v1 contracts; `validate-source` is scoped to SDD-WIKI-SOURCE-v1 source blocks. Each accepts comma-separated `--ids` or `--paths`. Use the two named wiki validator subcommands; do not infer a combined validator surface.

## `nav intent`

```powershell
mi-lsp nav intent "where do we handle workspace routing fallback?" --workspace <alias> --format toon
mi-lsp nav intent "error handling daemon" --top 20 --workspace <alias> --format toon
mi-lsp nav intent "forgot password frontend" --workspace <alias> --repo web --format toon
```

- Use first for a goal-shaped request; supported graph/change intents route automatically, while other questions continue in docs|code mode
- For symbol-like questions, BM25 scoring uses enriched index fields: symbol names, signatures, doc comments, and file paths
- Complementary to `nav ask` (docs-first synthesis) and `nav search` (literal text); do not silently substitute either one
- In workspaces `container`, prefer `--repo` when you already know the child repo you want to inspect
- Code-mode intent requires a prior `mi-lsp index` to populate search text; the deterministic planner preserves explicit selectors when catalog resolution is incomplete

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
"0 matches: search timed out"            → report visible incomplete evidence; narrow explicitly
"daemon_unavailable; served from..."     → daemon not running; keep the labeled partial result
```

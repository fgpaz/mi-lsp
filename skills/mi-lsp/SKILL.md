---
name: mi-lsp
description: Use when a folder-based agent should navigate code with the mi-lsp CLI, or when the skill is installed but the mi-lsp binary still needs install/bootstrap on PATH before semantic navigation can begin.
---

# mi-lsp

Use this skill when you want local semantic navigation with `mi-lsp` without introducing an MCP dependency.
If the skill is installed but the binary is missing, bootstrap the CLI first instead of abandoning the flow.

Prefer the AXI-default surfaces for onboarding and discovery: `mi-lsp`, `init`, `workspace status`, `nav route`, `nav search`, and `nav intent`.
Use `nav route` as the cheapest first orientation step — it resolves the canonical anchor doc from governance alone without touching the index.
Use `nav ask` without `--axi` for richer orientation questions when you need evidence synthesis.
Prefer `nav search --include-content` for implementation questions.
Treat `nav intent` as hybrid: natural capability questions should follow `mode=docs`, while symbol-like questions should follow `mode=code`.
When `nav intent` returns `mode=docs`, prefer the returned `doc_path/doc_id/evidence/next_queries` over switching back to broad code search.
Treat `continuation` as the default machine-readable next step when it is present: prefer following `continuation.next` over improvising a broader search.
Treat `memory_pointer` as a tiny repo-local reentry hint: it is there to help a fresh harness resume from recent canonical changes without spending a full query budget.
Use `mi-lsp workspace status <alias> --full` when you need the expanded reentry digest (`recent_canonical_changes`, `handoff`, `best_reentry`, `stale`).
If `workspace status` emits a warning like `"reentry memory snapshot absent; rerun 'mi-lsp index --workspace <alias>'..."`, rerun the suggested `mi-lsp index` before relying on `memory` or `memory_pointer` for reentry.
Use `--classic` when you want the old CLI behavior on an AXI-default surface, and `--axi` only when you need to force AXI on a classic-default surface such as `nav workspace-map`. Use `--axi=false` to suppress the AXI default for a single invocation without affecting `MI_LSP_AXI` or the session.
Prefer an explicit `--workspace <alias>` once the repo is registered.
Prefer compound commands over sequential greps and full-file reads.

## Search syntax rule

`nav search` accepts exactly one positional `pattern` argument.
If the pattern contains spaces, quote the whole pattern: `mi-lsp nav search "forgot password" --workspace <alias> --format toon`.
Do not write several bare words after `nav search`; PowerShell will split them into separate arguments and the CLI will reject the command.
If the pattern is regex-like, keep it quoted and add `--regex`.

## Output formats

| Format | Flag | Typical size | When to use |
|--------|------|-------------|-------------|
| TOON | `--format toon` | ~20-40% smaller | **Recommended default** — best token savings, arrays compress most |
| YAML | `--format yaml` | ~similar to JSON | Readable line-by-line; use when piping to YAML tooling |
| compact JSON | `--format compact` | baseline | Backward compat, `jq` scripting, strict JSON required |
| JSON | `--format json` | largest | Debugging, full fidelity |

### Reading compact JSON

Standard JSON. Extract with `jq` or by parsing the string. Fields use short keys in compact mode:
`f`=file, `l`=line, `k`=kind, `n`=name, `sig`=signature, `impl`=implements, `sc`=scope.

```json
{"ok":true,"workspace":"salud","backend":"text",
 "items":[{"f":"internal/service/app.go","k":"func","l":276,"n":"search"}],
 "stats":{"tokens_est":42}}
```

### Reading TOON

TOON uses `key: value` for scalars and `key[N]{col1,col2,...}:` for arrays.
Each array row is one indented line with comma-separated values in the declared column order.

```
backend: text
items[2]{f,k,l,n}:
  .docs/wiki/02_arquitectura.md,section,19,arquitectura
  internal/service/app.go,func,276,search
ok: true
stats:
  tokens_est: 42
workspace: salud
```

**Parsing rules for TOON:**
- Scalar field: `key: value` — read the value after `: `
- Array header: `key[N]{col1,col2,...}:` — N rows follow, each comma-split in column order
- Empty array: `key[0]:` — zero rows
- Nested object: `key:` followed by indented `child: value` lines
- Quoted strings: `"..."` when value contains spaces, commas, or special chars

**Extracting a value from TOON output:**
```
# To get item file paths from items[N]{f,k,l,n}:
# column index of "f" = 0 → split each row by comma, take index 0
```

### Reading YAML

Standard YAML. Each key on its own line; arrays use `- ` prefix.

```yaml
backend: text
items:
    - f: .docs/wiki/02_arquitectura.md
      k: section
      l: 19
      "n": arquitectura
    - f: internal/service/app.go
      k: func
      l: 276
      "n": search
ok: true
stats:
    tokens_est: 42
workspace: salud
```

Parse with any YAML library, or read field values directly from `key: value` lines.

### Format when items is empty and hint is set

```
# TOON
backend: text
hint: "0 matches for \"chat\": checked 1243 files"
items[0]:
next_hint: rerun with --regex
ok: true
stats:
  tokens_est: 8
workspace: salud

# YAML
backend: text
hint: '0 matches for "chat": checked 1243 files'
items: []
next_hint: rerun with --regex
ok: true
stats:
    tokens_est: 8
workspace: salud
```

### When to switch formats

- Use `--format toon` by default — it is the recommended format for agent use; saves the most tokens on large `items` arrays.
- Use `--format yaml` when you need line-by-line readability or are piping to a YAML-aware tool.
- Use `--format compact` only when strict JSON is required (e.g., `jq` pipelines, backward-compatible scripts).
- Never mix formats in a single session — pick one at the start and stay consistent.

> **AXI auto-format:** When AXI is active (`--axi`, `MI_LSP_AXI=1`, or an AXI-default surface) and you did not pass `--format` explicitly, the CLI selects TOON automatically. You do not need to add `--format toon` in those cases.

## AXI mode

| Precedence | Source |
|---|---|
| 1 (highest) | `--classic` explicit flag |
| 2 | `MI_LSP_AXI=1` env var |
| 3 | `--axi=false` surface override |
| 4 | `--axi` explicit flag |
| 5 (lowest) | per-surface default |

- **`--axi=false`** disables the AXI default for a single invocation. Use it when you want classic output on an AXI-default surface without setting `--classic` for the whole command.
- **`--axi` + `--classic` together are invalid** — the CLI errors immediately before running the operation. Do not combine them expecting a silent fallback.
- **TOON is automatic under active AXI** — if AXI is effective and you did not pass `--format`, the CLI picks TOON. Explicit `--format` always wins.

## Interpreting the `hint` field

All envelopes may include a `hint` field with diagnostic context, and some responses may also include `next_hint` with the recommended rerun:

- `"0 matches for X in workspace Y"` — pattern not found; try a different keyword or `--regex`
- `"0 matches for X: pattern looks regex-like, rerun with --regex"` — literal search on a regex pattern
- `"0 matches for X: search timed out"` — reduce scope or use a more specific pattern
- `"daemon_unavailable; served from local text index"` — daemon not running; result is textual-only
- `"invalid path: contains newline in ..."` — multi-read arg had embedded `\n`; fix the argument

If `hint` is present and `items` is empty, act on the hint before retrying. If `next_hint` is present, prefer that rerun guidance over improvising. Do not retry the same command unchanged.

In cross-workspace `nav find` / `nav search` results, structured formats may include a per-item `workspace` field so agents can preserve provenance without relying on array position alone.

## Tool binding

Run `mi-lsp` through the host shell tool, not through a custom MCP tool:

- Codex: `functions.shell_command`
- Claude Code: shell/Bash tool
- Other skill-based agents: the local terminal/shell tool they already expose

Do not wait for a dedicated `mi-lsp` MCP integration. `mi-lsp` is a CLI-first tool.

## Install bootstrap

If the skill folder exists but `mi-lsp` is not callable, do not stop at "tool unavailable".
Install the CLI first, verify it, and only then continue with repo navigation.

1. Download the release bundle for the user's platform from `https://github.com/fgpaz/mi-lsp/releases`.
2. Choose the right bundle: `win-x64`, `win-arm64`, `linux-x64`, or `linux-arm64`.
   - On this workstation, the preferred global Windows install is `win-arm64`.
   - When refreshing the shared mirror for Windows consumers, keep the mirror binary on `win-x64` / `amd64`.
3. Extract it into a stable tools directory and keep `workers/<rid>/` next to the `mi-lsp` binary.
4. Add that directory to the current session `PATH`, or invoke the binary by absolute path until `PATH` is fixed permanently.
5. Verify the install:

```powershell
where.exe mi-lsp
mi-lsp worker status --format toon
```

6. If the binary was moved after extraction, run:

```powershell
mi-lsp worker install
```

## Updating to a new version

A new release publishes pre-built bundles for all platforms — no Go toolchain needed.

1. Download the new bundle from `https://github.com/fgpaz/mi-lsp/releases` for your platform.
2. Stop the daemon if running:

```powershell
mi-lsp daemon stop
```

3. Replace the `mi-lsp` binary in your install directory with the one from the new bundle.
4. If the new release includes worker changes, replace `workers/<rid>/` too (or run `mi-lsp worker install`).
5. Restart the daemon if you use it:

```powershell
mi-lsp daemon start
```

6. Verify:

```powershell
where.exe mi-lsp
mi-lsp worker status --format toon
```

If the release changes CLI/daemon telemetry or `admin export`, refresh the `mi-lsp` binary and restart the daemon before trusting new fields in `access_events`.
Only replace `workers/<rid>/` when the release notes say the worker changed.
If you update the skill under `C:\\Users\\fgpaz\\.agents\\skills\\mi-lsp`, update the mirrored copy under `C:\\repos\\buho\\assets\\skills\\mi-lsp` in the same task and preserve the Windows architecture split (`global=win-arm64`, `mirror=win-x64`).

### Admin export note

`mi-lsp admin export --summary` aggregates over the full filtered window by default.
Only pass `--limit` when you intentionally want to summarize a partial sample.

Raw export can also filter by:
- `--operation`
- `--session-id`
- `--client-name`
- `--route`
- `--query-format`
- `--truncated`
- `--pattern-mode`
- `--routing-outcome`
- `--failure-stage`
- `--hint-code`

Summary mode can add optional breakdowns with:
- `--by-route`
- `--by-client`
- `--by-hint`
- `--by-failure-stage`

`decision_json` is intentionally sanitized for local debugging.
It may include pattern length, regex suspicion, selector presence, emitted hints, fallback, and result source, but it must not include the raw search pattern, argv, or a full request snapshot.
`result_count` means the number of items actually emitted in the final envelope after truncation or limits.

Telemetry examples:

```powershell
mi-lsp admin export --recent --summary --by-route --by-client --by-failure-stage
mi-lsp admin export --recent --operation nav.search --pattern-mode literal --format compact --limit 50
mi-lsp admin export --recent --routing-outcome router_error --failure-stage selector_validation --format json --limit 20
```

> The worker protocol is versioned. If the CLI and worker versions are incompatible, `worker status` will warn you.

Windows session example:

```powershell
$installDir = Join-Path $HOME "bin\mi-lsp"
$env:PATH = "$installDir;$env:PATH"
where.exe mi-lsp
mi-lsp worker status --format toon
```

Linux session example:

```bash
export PATH="$HOME/.local/opt/mi-lsp:$PATH"
command -v mi-lsp
mi-lsp worker status --format toon
```

## First-use check

1. Confirm `mi-lsp` is callable in the current shell.
2. Prefer the short AXI-default bootstrap path first.
3. If the workspace is already registered, resolve it and continue.

```powershell
mi-lsp workspace list
mi-lsp
mi-lsp init . --name <alias>
mi-lsp workspace status <alias>
```

If `mi-lsp` is not on `PATH`, install it from Releases or repair `PATH` for the current session before falling back to other tools.

## Hot path

Use these commands first:

- Open the discovery home: `mi-lsp`
- Cheapest canonical orientation (no index needed): `mi-lsp nav route "how is this workspace organized?" --workspace <alias> --format toon`
- Canonical reading pack for a task: `mi-lsp nav pack "understand authentication flow" --workspace <alias>`
- Reading pack anchored to an RF spec: `mi-lsp nav pack "how does login work" --rf RF-AUTH-001 --workspace <alias>`
- Richer orientation with evidence: `mi-lsp nav ask "how is this workspace organized?" --workspace <alias>`
- Read 2+ file slices: `mi-lsp nav multi-read file1:1-120 file2:40-160 --workspace <alias> --format toon`
- Search and see code inline: `mi-lsp nav search "billing retry" --include-content --workspace <alias>`
- Search inside one repo of a container workspace: `mi-lsp nav search "forgot password" --workspace <alias> --repo web`
- Understand a symbol in one call: `mi-lsp nav related MySymbol --workspace <alias> --format toon`
- Orient in a new repo or parent folder: `mi-lsp nav workspace-map --workspace <alias> --axi`
- Profile a service: `mi-lsp nav service <path> --workspace <alias> --format toon`
- Inspect recent routing/search telemetry: `mi-lsp admin export --recent --summary --by-route --by-hint --by-failure-stage`
- Expand repo-local reentry memory: `mi-lsp workspace status <alias> --full`
- Batch mixed operations: `mi-lsp nav batch --workspace <alias> --format toon`
- Trace spec-to-code links: `mi-lsp nav trace RF-QRY-003 --workspace <alias> --format toon`
- Search by intent/purpose: `mi-lsp nav intent "where do we handle routing fallback?" --workspace <alias>`

Prefer these over repeated `Get-Content`, plain `rg`, or one-file-at-a-time reads.

## Minimal workflow

1. Bootstrap or verify the workspace.

```powershell
mi-lsp
mi-lsp init . --name <alias>
mi-lsp workspace status <alias>
```

2. Start with intent, not grep.

```powershell
mi-lsp nav route "how is this workspace organized?" --workspace <alias> --format toon
mi-lsp nav ask "how is this workspace organized?" --workspace <alias>
mi-lsp workspace status <alias> --full
mi-lsp nav intent "error handling for daemon connections" --workspace <alias>
```

3. Move to broad discovery when you need structure.

```powershell
mi-lsp nav workspace-map --workspace <alias> --axi
mi-lsp nav find <symbol> --workspace <alias> --format toon
mi-lsp nav search "<text with spaces if needed>" --include-content --workspace <alias>
```

4. Move to deep semantics only when needed.

```powershell
mi-lsp nav refs <symbol> --workspace <alias> --backend roslyn --format toon
mi-lsp nav context <file> <line> --workspace <alias> --backend roslyn --format toon
mi-lsp nav related <symbol> --workspace <alias> --format toon
```

5. Use `nav service` before judging whether a backend service is only scaffolding.

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format toon
```

6. Trace spec-to-code links when reviewing RF compliance.

```powershell
mi-lsp nav trace RF-QRY-003 --workspace <alias> --format toon
mi-lsp nav trace --all --summary --workspace <alias> --format toon
```

## Tool choice ladder

Use `mi-lsp` first for repo navigation, docs-first Q&A, symbol lookup, service audits, and batch reads.

- Start with `mi-lsp`, `workspace status`, `nav route`, or `nav intent` for the first pass on a new repo.
- Use `nav route` as the cheapest orientation step — it resolves the canonical anchor doc from governance without touching the index (Tier 1), then enriches from the index when available (Tier 2). AXI-default preview-first.
- Use `nav ask` for richer orientation when you need full evidence synthesis and next queries.
- Use `nav pack` to build a canonical reading pack docs-first for a task. It uses the same routing core as `nav route` and returns `mode=preview|full`, per-doc `stage` (`anchor|preview|discovery`), and `next_queries`. Anchor optionally with `--rf`, `--fl`, or `--doc`.
- Use `nav search --include-content` before `nav ask` for literal implementation questions like "where is X implemented?".
- Use `nav intent` to find code by purpose when you don't know the symbol name.
- Use `nav trace` to check which code implements a specific RF requirement.
- Use `workspace-map`, `search --include-content`, and `multi-read` before broad raw file reads.
- Use `related`, `context`, `refs`, and `deps` when you need semantic depth.
- Use plain `rg` only when `mi-lsp` is unavailable or the request falls outside the CLI surface.

## Routing model

- Cheap reads stay direct (no daemon): `nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`, `nav.intent`, `nav.trace`, `nav.route`, `nav.pack`, `nav.governance`
- In workspaces `container`, prefer `--repo` for direct `nav.find`, `nav.search`, and `nav.intent` before escalating to semantic selectors.
- Deep semantics may use the daemon: `nav.refs`, `nav.context`, `nav.deps`, `nav.related`, `nav.service`, `nav.workspace-map`, `nav.diff-context`, `nav.batch`, `nav.ask`
- The daemon is optional. If it is unavailable, the CLI must still work in direct mode.

## Container workspaces

If the workspace is a parent folder, start broad on the container and then narrow with the selector that matches the query type:

- Direct catalog reads: `--repo` on `nav.find`, `nav.search`, `nav.intent`
- Semantic queries: `--repo`, `--entrypoint`, `--solution`, or `--project`

If a direct query in a container workspace returns `backend=router`, do not guess. Re-run with `--repo`.
If a semantic query returns `backend=router`, re-run with a narrower semantic selector.

## Shared daemon for multi-agent work

For repeated semantic work across Codex, Claude Code, or subagents, keep the daemon alive:

```powershell
mi-lsp daemon start
mi-lsp workspace warm --workspace <alias>
```

When you want clean governance and telemetry attribution, set:
- `MI_LSP_CLIENT_NAME`
- `MI_LSP_SESSION_ID`

## When to open references

- Read [references/quickstart.md](references/quickstart.md) when you need install help, a slightly longer onboarding, or a command chooser.
- Read [references/compound-commands.md](references/compound-commands.md) when you want `multi-read`, `batch`, `related`, `workspace-map`, `diff-context`, or cross-workspace patterns.
- Read [references/recipes.md](references/recipes.md) when auditing a service, reviewing completeness, or doing PR/impact analysis.
- Read [references/runtime-drift.md](references/runtime-drift.md) when CLI/docs/daemon behavior disagree after rebuilds or reinstalls, especially to confirm `cli_path` and `protocol_version` from `worker status`.

## Noise control

If index or search results are polluted by generated folders, browser profiles, logs, extracted artifacts, or docs templates, suggest exact repo-local entries in `.milspignore`.

Do not suggest `node_modules/`; it is already ignored by default.

## Output discipline

- Summarize the most relevant hits instead of pasting large JSON blobs.
- Mention the selected repo when answering from a container workspace.
- If results are truncated, rerun narrower or explain how to narrow them.
- For `nav ask`, include the primary doc, the strongest code evidence, and one or two follow-up commands.
- For `nav route` and `nav pack`, each doc in the result carries a `stage` field: `anchor` (canonical anchor doc), `preview` (mini pack preview), or `discovery` (advisory, non-authoritative). Use this to distinguish source authority without relying on array position.
- If AXI emits `next_hint` toward `--full`, prefer that rerun before inventing a broader command.
- If `continuation` is present, follow `continuation.next` first; only use `alternate` when the primary path is blocked or clearly insufficient.
- If `memory_pointer.stale=true`, prefer `workspace status --full` or a fresh `index` before leaning on the pointer as ground truth.
- Do not append `--axi` to reruns on AXI-default surfaces unless you are crossing into a classic-default command.

## Fallback

If `mi-lsp` remains unavailable after install and `PATH` repair, fall back to `rg` and targeted file inspection.

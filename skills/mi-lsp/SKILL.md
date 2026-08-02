---
name: mi-lsp
description: Use when a folder-based agent should navigate code with the mi-lsp CLI, or when the skill is installed but the mi-lsp binary still needs install/bootstrap on PATH before semantic navigation can begin.
---

# mi-lsp

Use this skill when you want local semantic navigation with `mi-lsp` without introducing an MCP dependency.
If the skill is installed but the binary is missing, bootstrap the CLI first instead of abandoning the flow.

If `mi-lsp` is missing from `PATH`, use the one-command bootstrap in the `Install bootstrap` section before using `rg`, `Grep`, `Glob`, or broad file reads.
The intended first path is: install or verify CLI -> `mi-lsp init . --name <alias>` -> `nav intent` for every supported goal-shaped request, then the emitted bounded operation; use `nav ask` / `nav pack` / `nav search --include-content` only when the intent lane does not apply.

Prefer the AXI-default surfaces for onboarding and discovery: `mi-lsp`, `init`, `workspace status`, `nav wiki inventory`, `nav wiki search`, `nav route`, `nav search`, and `nav intent`.
Use `nav wiki search` when the task is clearly about project docs, RS/RF/FL/TP/CT/TECH/DB, outcomes, contracts, tests, or traceability.
Use `nav route` as the cheapest first orientation step — it resolves the canonical anchor doc from governance alone without touching the index.
Use `nav ask` without `--axi` for richer orientation questions when you need evidence synthesis.
Prefer `nav search --include-content` for implementation questions.
Use `nav context <file>:<line>` when you already have a line target; the older `nav context <file> <line>` form remains valid.
For Go files, `nav context` / `nav refs` may use optional `gopls`; if `gopls` is missing, treat the catalog/text fallback with install guidance as valid partial evidence.
Treat `nav wiki search|route|pack|trace` as the canonical documentation surface.
Treat `nav search` as a broad text surface: it may return canonical docs, but it may also return prompts, audits, `.docs/raw`, generated files, or other support artifacts.
Do not decide documentation authority from `nav search` alone when a `nav wiki *` surface can answer the question.
Treat `nav intent` as the first hybrid entry point: supported graph/change goals use the automatic deterministic planner; other natural capability questions follow `mode=docs`, while symbol-like questions follow `mode=code`.
When `nav intent` returns `mode=preview`, preserve available sections, candidates, omissions, and exact expansions. When it returns `mode=docs`, prefer the returned `doc_path/doc_id/evidence/next_queries` over switching back to broad code search.
Treat `continuation` as the default machine-readable next step when it is present: prefer following `continuation.next` over improvising a broader search.
Treat `memory_pointer` as a tiny repo-local reentry hint: it is there to help a fresh harness resume from recent canonical changes without spending a full query budget.
Use `mi-lsp workspace status <alias> --full` when you need the expanded reentry digest (`recent_canonical_changes`, `handoff`, `best_reentry`, `stale`).
If `workspace status` emits a warning like `"reentry memory snapshot absent; rerun 'mi-lsp index --workspace <alias>'..."`, rerun the suggested `mi-lsp index` before relying on `memory` or `memory_pointer` for reentry.
Use `--classic` only when you need the legacy response shape on an AXI-default surface, and `--axi` only when you need to force AXI on a classic-default surface such as `nav workspace-map`. `--axi=false` changes preview/output mode for one invocation; it is not a routing opt-out. Do not disable automatic intent routing.
Prefer an explicit `--workspace <alias>` once the repo is registered.
Prefer compound commands over sequential greps and full-file reads.

## Intent-first automatic routing

Use `mi-lsp` first and express the goal as an intent. `nav intent` is mandatory for every supported goal-shaped request. Routing is automatic: do not add a routing opt-out, require a literal operation alias in the user's wording, or silently switch tools. `nav intent <question>` uses the local deterministic planner when the question matches a supported graph/change intent, then keeps the response in bounded preview mode.

The planner may route these goals:

- explain a change or its impact -> `nav explain-change`
- affected code/tests/docs -> `nav affected`
- incoming callers -> `nav callers`
- outgoing callees -> `nav callees`
- a path between two selectors -> `nav path`
- one exact graph edge -> `nav explain`
- a symbol neighborhood -> `nav neighbors`

`explain-change` is an intent/operation of `nav intent`; use `nav intent` as the primary route and use `nav explain-change` only when the runtime emits it as the exact expansion or the user explicitly asks for that operation. The exposed command is `nav neighbors`; “neighborhood” is the natural-language intent, not a command name. `nav related` remains the separate definition/callers/implementors/tests summary surface.

Preview is useful evidence, not PASS. Preserve every available section, especially `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki`, plus graph, evidence, candidates, fallbacks, and omissions. Use the exact `expansions[].command` as the second query and read its `expansions[].reason`; do not invent a broader command. Add `--full` only when the response or `next_hint` requests expansion. If a selector is ambiguous or missing, report that omission and ask for the explicit selector rather than guessing.

A fallback is permitted only with one visible structured `reason_code`: `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete`, plus a separate bounded, sanitized `detail`. Timeout, silence, `DONE`, or `PASS` without fresh evidence is not PASS and never enables fallback. Internal backend degradation that the runtime labels in the envelope is an omission, not an external fallback. Report timeout/incomplete results, keep partial evidence if present, and narrow or rerun only with an explicit command.

For retries/recovery, use the 180-second soft and 300-second hard watchdogs. Allow at most two same-context recoveries, each with a smaller packet and a changed query/packet; never retry unchanged or create a retry storm. Cap parallel work at six practical lanes, require exclusive `allowed_paths` per lane, fail closed on joins, and require fresh verification before PASS. Keep evidence redacted: never emit prompts, transcripts, secrets, PII, PHI, argv, or raw patterns. Do not invent a model or provider; report only runtime-provided metadata.

## Worktree operating rule

Treat every Git worktree as a distinct workspace because each worktree has its own physical `workspace_root`.
Register one explicit alias per worktree, index that exact root, and always query with the alias for the active worktree.

On Windows, when creating or entering a new worktree, prefer a short external worktree path such as `C:\wt\<repo-short>-<task-short>` before `.worktrees\<branch>`. Repos with deep `.docs/auditoria`, `.docs/raw`, generated evidence, browser profile, or fixture paths can fail checkout with `Filename too long` when the worktree is nested inside the repo.

Use this post-worktree bootstrap before any semantic navigation:

```powershell
git worktree add C:\wt\<short-name> -b <branch-name> <base-ref>
Set-Location C:\wt\<short-name>
mi-lsp init . --name <worktree-alias>
mi-lsp index --workspace <worktree-alias>
mi-lsp workspace status <worktree-alias> --format toon
mi-lsp nav governance --workspace <worktree-alias> --format toon
```

Do not use an alias that points to the main checkout while the current task is running inside another worktree.
An explicit `--workspace <alias>` wins over caller CWD; if it points to a different registered root than the CWD, treat the warning as a signal to confirm the intended alias before continuing.
When no `--workspace` is provided, `caller_cwd` resolution may select the registered worktree containing the current directory, but agents should still prefer explicit aliases for repeatability.
If `git worktree add` fails with `Filename too long`, retry from a short external root such as `C:\wt\<short-name>`, then rerun the `mi-lsp init` and `mi-lsp index` steps for that exact root. Do not fall back to the main checkout alias as if it represented the failed worktree.

## Workspace registry hygiene

`workspace list` is alias-preserving by default. Multiple aliases for the same root are valid when they represent roles, smokes, legacy names, or worktrees. Do not dedupe or prune global registry entries unless the user explicitly approves a mutating cleanup.

Use these non-mutating diagnostics before changing registry state:

```powershell
mi-lsp workspace list --group-by-root --format toon
mi-lsp workspace doctor --format toon
```

Interpretation:

- `workspace list`: every alias remains a first-class record.
- `workspace list --group-by-root`: groups aliases by canonical root and reports `alias_count`, `aliases`, `canonical_alias`, `selection_reason`, `kind`, and warnings.
- `workspace doctor`: reports duplicate roots, stale paths, binary provenance/shadowing, governance skips, and suggested cleanup commands only. It must not mutate `~/.mi-lsp/registry.toml`.

Daemon runtime identity is canonical by `workspace_root + backend_type + entrypoint_id`. Alias-specific fields such as `workspace_alias`, `workspace_input`, and human-facing `workspace` are preserved for display and forensics, but duplicate aliases for the same root/backend/entrypoint should share warm runtime state. Different `entrypoint_id` values remain separate runtimes.
Different worktree roots must not share runtime, watcher, or index state even when they share the same Git common directory.

## Canonical wiki-first rule

When the task is asking "what is the canonical doc?", "which RS/RF/TP/CT/TECH/DB applies?", "what does the spec say?", or "how do I trace this requirement?", start from governance-backed wiki surfaces:

1. `nav wiki inventory` when you do not yet know which workspace owns the question — it returns a light per-workspace catalog (alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at). Add `--with-layer-counts` to see RS/FL/RF/TP/TECH/DB/CT counts per workspace before targeting one.
2. `nav route` when you need the cheapest canonical anchor and do not want to depend on the index yet
3. `nav wiki search` when you need canonical doc discovery by topic or ID
4. `nav wiki pack` when you need the small reading set around the canonical anchor
5. `nav wiki trace` when you already have an explicit `RS-*` / `RF-*` / `TP-*` / doc ID

Only drop to `nav search --include-content` when the question becomes implementation-first, or when you need raw disk evidence after the canonical anchor is already known.

Use `--layer RS,RF,FL,TP,CT,TECH,DB` aggressively on `nav wiki search` to narrow the authority lane.

The five `nav wiki *` subcommands (`search`, `route`, `trace`, `pack`, `inventory`) all accept `--all-workspaces` for fan-out across every registered workspace; items in the response carry `workspace:<alias>` and `stats` gains `workspaces_queried` / `workspaces_failed[]`. Use it when the question is "which workspaces talk about X?" before targeting one with `--workspace <alias>`. Cross-machine federation lives outside `mi-lsp` (see Hermes-side wrappers); the CLI stays per-host.
If AXI preview is trimmed or `next_hint` asks for expansion, rerun with `--full` before inventing a broader command.
Follow `next_queries` and `continuation.next` from wiki results before improvising `nav search`.

Canonical wiki location is governed by `00_gobierno_documental.md` and `read-model.toml`, not by assuming the corpus always lives under a fixed path like `.docs/wiki/*`.

## Semantic recall by intent

Use `nav recall --intent` when a knowledge wiki has embeddings configured and you need semantic candidates, not final authority. Embeddings recall discovers candidates; a `route` hit or route-only material is not a final source until you open the canonical doc or evidence it points to.

### Embeddings authentication contract

Authentication is explicit and fail-closed:

- An empty or whitespace-only `api_key_env` means the endpoint is explicitly unauthenticated; send no authentication header.
- When `api_key_env` names an environment variable whose value is missing, empty, or whitespace-only, return `SEM_API_KEY_MISSING` before network I/O.
- Emit `Bearer` only when the resolved key is present; never emit an empty Bearer token.
- Public errors are sanitized and must not expose provider response bodies, credentials, API keys, authorization headers, or other secret material.
- If semantic recall is unavailable because of configuration, authentication, or provider failure, stay in the governed lexical/wiki lane with `nav wiki search` and report only a sanitized reason. Do not use an ungoverned fallback.

```powershell
mi-lsp nav recall "what contract defines recall result fields?" --workspace <alias> --intent formula --format toon
mi-lsp nav recall "collect citations for semantic fallback" --workspace <alias> --intent evidence --format toon
mi-lsp nav recall "where should I start this docs task?" --workspace <alias> --intent route --map --format toon
```

Intent guide:

- `formula`: definitions, rules, contracts, acceptance criteria, and stable technical formulas
- `evidence`: citable support for an answer, audit, or closure packet
- `route`: next canonical anchor or workflow path to inspect; follow with `nav wiki pack|trace`
- `explore`: balanced discovery when vocabulary is still unknown
- `learning`: onboarding, concepts, architecture, and explanatory material

Reference OpenAI-compatible embeddings config:

```toml
[embeddings]
provider = "openai"
base_url = "https://<openai-compatible-embeddings-endpoint>/v1"
model = "<embedding-model>"
dim = 4096
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
profile = "knowledge-wiki"
batch_size = 32
timeout_ms = 30000
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

Secret handling: set the variable named by `api_key_env` through the environment or a wrapper such as `mkey run`. Never print API key values, paste them into prompts, commit them, or read auth/secret stores during normal navigation.

If the provider, key, or embeddings config fails, do not expect a hidden BGE fallback. Stay in the canonical lexical/wiki lane and state that reason explicitly:

```powershell
mi-lsp nav wiki search "<query>" --workspace <alias> --format toon
```

This is an intentional governed `mi-lsp` lane, not an external fallback. External fallback reasons remain limited to `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, and `explicit_incomplete`.

## Preparation versus edit planning

`nav prepare <task>` is exposed and read-only. Use it to collect governance, route/pack, generation, and task-specific allowed-path evidence. Pass `--affected <path>` only for paths supplied by the task; it never infers paths from the git diff. `--plan <file>` validates an edit-plan packet without applying it.

`nav edit-plan` is a separate guarded patch surface, not a synonym for preparation. It builds a deterministic dry-run diff from an edit-plan-v1/v2 JSON packet. Writes require both `--apply` and `--experimental-apply`, a clean Git workspace, safe paths, and matching hashes. Do not describe `edit-plan` as a generic editing command or claim that a preview changed files.

```powershell
mi-lsp nav prepare "review the routing change" --affected internal/service/intent.go --workspace <alias> --format toon
mi-lsp nav edit-plan --packet .mi-lsp/plan.json --workspace <alias> --format toon
mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-INTENT --format toon
```

The validator scopes are real: `validate-harness` checks SDD-HARNESS-v1 contracts; `validate-source` checks SDD-WIKI-SOURCE-v1 source blocks. Both accept comma-separated `--ids` or `--paths`; do not infer a combined validator surface.

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

TOON output is sanitized for unsafe control bytes before serialization: tabs, newlines, and carriage returns remain real whitespace, while other control characters are rendered as visible escapes such as `\u0000`. When this happens the envelope includes a warning. Compact JSON keeps backward-compatible raw string behavior; do not assume a TOON escape means the underlying compact JSON value was changed.

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
- `"0 matches for X: search timed out"` — report visible incomplete evidence; narrow scope only with an explicit rerun
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

Use the combined installer when the user wants both the CLI and this skill installed through the skills CLI:

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.sh | sh
```

Use the CLI-only installer when the skill is already present and only the binary needs install or update:

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.sh | sh
```

Installer rules:

- Supported archive RIDs: `win-x64`, `win-arm64`, `linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`.
- Darwin archives map to worker RIDs `osx-x64` and `osx-arm64`; do not map Darwin to a Linux archive.
- Archives come from GitHub Releases latest and must pass SHA256 verification before extraction.
- The install must keep `workers/<rid>/` next to the `mi-lsp` binary or run `mi-lsp worker install`.
- `install-agent` requires `npx` and uses `npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y`; it has no folder-copy fallback.

Verify the install:

```powershell
where.exe mi-lsp
mi-lsp version --format toon
mi-lsp worker status --format toon
```

If the binary was moved after extraction, run:

```powershell
mi-lsp worker install
```

### Local source install on macOS/Linux

If the release asset is not available yet but the source checkout exists, build
and install the current machine's RID from the repo:

```bash
cd ~/Documents/mi-lsp
sh scripts/release/install-local.sh --install-dir "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"
mi-lsp version --format toon
mi-lsp worker status --format toon
```

## Legacy manual install

Use this only when the one-command installer cannot run in the current shell.

1. Download the release bundle for the user's platform from `https://github.com/fgpaz/mi-lsp/releases`.
2. Choose the right bundle: `win-x64`, `win-arm64`, `linux-x64`, `linux-arm64`, `darwin-x64`, or `darwin-arm64`.
3. Extract it into a stable tools directory and keep `workers/<rid>/` next to the `mi-lsp` binary.
4. Add that directory to the current session `PATH`, or invoke the binary by absolute path until `PATH` is fixed permanently.
5. Verify the install:

```powershell
where.exe mi-lsp
mi-lsp version --format toon
mi-lsp worker status --format toon
```

6. If the binary was moved after extraction, run:

```powershell
mi-lsp worker install
```

## Updating to a new version

The CLI-only installer is also the normal update command. It downloads the latest release, replaces the binary and worker bundle for the host RID, and reruns the install probes.

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.sh | sh
```

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
mi-lsp version --format toon
mi-lsp worker status --format toon
mi-lsp workspace list --group-by-root --format toon
mi-lsp workspace doctor --format toon
```

If the release changes CLI/daemon telemetry or `admin export`, refresh the `mi-lsp` binary and restart the daemon before trusting new fields in `access_events`.
If `daemon status` reports missing executable metadata, an `executable_sha256` mismatch, or stale-daemon guidance, rebuild/install the CLI and run `mi-lsp daemon restart` before trusting daemon-backed results.
Only replace `workers/<rid>/` when the release notes say the worker changed.
## Weekly release check

At most once every 7 days, an agent may check whether a newer GitHub Release exists.
Cache the check timestamp and last seen tag under `~/.mi-lsp/release-check.json` or an equivalent local cache.

- Compare local `mi-lsp version --format toon` with `https://api.github.com/repos/fgpaz/mi-lsp/releases/latest`.
- If a newer release exists, notify the user and show the CLI-only update command.
- Do not run the installer automatically unless the user explicitly asks to update.
- To update this skill itself, use `npx skills update mi-lsp -g -y`.

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
- Wiki-first doc search: `mi-lsp nav wiki search "workflow masterformularios" --workspace <alias> --layer RS,RF,FL,CT,TP --format toon`
- Wiki reading pack: `mi-lsp nav wiki pack "workflow con masterformularios" --workspace <alias> --format toon`
- Cheapest canonical orientation (no index needed): `mi-lsp nav route "how is this workspace organized?" --workspace <alias> --format toon`
- Canonical reading pack for a task: `mi-lsp nav pack "understand authentication flow" --workspace <alias>`
- Reading pack anchored to an RF spec: `mi-lsp nav pack "how does login work" --rf RF-AUTH-001 --workspace <alias>`
- Richer orientation with evidence: `mi-lsp nav ask "how is this workspace organized?" --workspace <alias>`
- Read 2+ file slices: `mi-lsp nav multi-read file1:1-120 file2:40-160 --workspace <alias> --format toon`
- Search and see code inline: `mi-lsp nav search "billing retry" --include-content --workspace <alias>`
- Search inside one repo of a container workspace: `mi-lsp nav search "forgot password" --workspace <alias> --repo web`
- Semantic wiki recall by intent: `mi-lsp nav recall "what contract defines fallback?" --workspace <alias> --intent formula --format toon`
- Understand a symbol in one call: `mi-lsp nav related MySymbol --workspace <alias> --format toon`
- Orient in a new repo or parent folder: `mi-lsp nav workspace-map --workspace <alias> --axi`
- Profile a service: `mi-lsp nav service <path> --workspace <alias> --format toon` (`go-package` is language-aware Go evidence, not .NET evidence)
- Inspect recent routing/search telemetry: `mi-lsp admin export --recent --summary --by-route --by-hint --by-failure-stage`
- Expand repo-local reentry memory: `mi-lsp workspace status <alias> --full`
- Batch mixed operations: `mi-lsp nav batch --workspace <alias> --format toon`
- Trace spec-to-code/doc links: `mi-lsp nav trace RF-QRY-003 --workspace <alias> --format toon`
- Trace from the wiki surface: `mi-lsp nav wiki trace RS-EXAMPLE-001 --workspace <alias> --format toon`
- Search and route by intent: `mi-lsp nav intent "where do we handle routing fallback?" --workspace <alias>`
- Explain a change with bounded preview: `mi-lsp nav explain-change --path internal/service/intent.go --workspace <alias> --format toon`
- Inspect affected paths: `mi-lsp nav affected internal/service/intent.go --include-tests --include-docs --workspace <alias> --format toon`
- Inspect callers/callees: `mi-lsp nav callers MySymbol --workspace <alias> --format toon` / `mi-lsp nav callees MySymbol --workspace <alias> --format toon`
- Inspect graph path/edge/neighbors: `mi-lsp nav path FromSymbol ToSymbol --workspace <alias> --format toon`, `mi-lsp nav explain <edge-cross-rid> --workspace <alias> --format toon`, `mi-lsp nav neighbors MySymbol --workspace <alias> --format toon`
- Prepare read-only task evidence: `mi-lsp nav prepare "review the routing change" --affected internal/service/intent.go --workspace <alias> --format toon`
- Validate real wiki scopes: `mi-lsp nav wiki validate-harness --workspace <alias> --ids CT-NAV-INTENT --format toon` and `mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-INTENT --format toon`

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
mi-lsp nav wiki search "RF IDX" --workspace <alias> --layer RS,RF,TP,CT --format toon
mi-lsp nav ask "how is this workspace organized?" --workspace <alias>
mi-lsp workspace status <alias> --full
mi-lsp nav intent "error handling for daemon connections" --workspace <alias>
```

If the task is document-first, stay in the wiki lane longer:

```powershell
mi-lsp nav route "how does login work?" --workspace <alias> --format toon
mi-lsp nav wiki search "RF-AUTH login" --workspace <alias> --layer RS,RF,TP,CT --format toon
mi-lsp nav wiki pack "how does login work?" --workspace <alias> --format toon
mi-lsp nav wiki trace RF-AUTH-001 --workspace <alias> --format toon
```

Use `nav search` only after the canonical anchor is known, or when the task is code-first.

3. Move to broad discovery when you need structure.

```powershell
mi-lsp nav workspace-map --workspace <alias> --axi
mi-lsp nav find <symbol> --workspace <alias> --format toon
mi-lsp nav search "<text with spaces if needed>" --include-content --workspace <alias>
```

4. Move to deep semantics only when needed.

```powershell
mi-lsp nav refs <symbol> --workspace <alias> --backend roslyn --format toon
mi-lsp nav context <file>:<line> --workspace <alias> --format toon
mi-lsp nav context <file> <line> --workspace <alias> --backend roslyn --format toon
mi-lsp nav related <symbol> --workspace <alias> --format toon
```

5. Use `nav service` before judging whether a backend service is only scaffolding.

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format toon
```

6. Trace spec-to-code/doc links when reviewing RS/RF/TP compliance.

```powershell
mi-lsp nav trace RF-QRY-003 --workspace <alias> --format toon
mi-lsp nav trace --all --summary --workspace <alias> --format toon
```

## Tool choice ladder

Use `mi-lsp` first for repo navigation, docs-first Q&A, symbol lookup, service audits, and batch reads.

- Start with `mi-lsp`, `workspace status`, `nav wiki search`, `nav route`, or `nav intent` for the first pass on a new repo.
- Use `nav wiki search` for documentation exploration. Filter with `--layer RS,RF,FL,TP,CT,TECH,DB`; follow returned `next_queries` toward `nav wiki pack`, `nav wiki trace`, `nav multi-read`, or `nav ask`.
- Use `nav wiki trace` when you already know the requirement or test ID and need the canonical doc/evidence lane instead of a broad text match list.
- Use `nav route` as the cheapest orientation step — it resolves the canonical anchor doc from governance without touching the index (Tier 1), then enriches from the index when available (Tier 2). AXI-default preview-first.
- Use `nav ask` for richer orientation when you need full evidence synthesis and next queries.
- Use `nav pack` to build a canonical reading pack docs-first for a task. It uses the same routing core as `nav route` and returns `mode=preview|full`, per-doc `stage` (`anchor|preview|discovery`), and `next_queries`. Anchor optionally with `--rf`, `--fl`, or `--doc`.
- Use `nav search --include-content` before `nav ask` for literal implementation questions like "where is X implemented?".
- If `nav search` returns prompts, audits, `.docs/raw`, or other support artifacts while you are answering a documentation/traceability question, treat those hits as non-authoritative and reroute to `nav wiki search|route|pack|trace`.
- Use `nav intent` to find code by purpose when you don't know the symbol name.
- Use `nav trace` to inspect RS/RF/TP evidence; RF remains the implementation-link path, while RS returns the outcome document identity.
- Use `workspace-map`, `search --include-content`, and `multi-read` before broad raw file reads.
- Use `related`, `context`, `refs`, and `deps` when you need semantic depth.
- Use plain `rg`, `Grep`, `Glob`, or broad raw reads only when the runtime visibly reports `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete`; state which condition applies.

## Routing model

- Cheap reads stay direct (no daemon): `nav.find`, `nav.search`, `nav.wiki.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`, `nav.intent`, `nav.trace`, `nav.route`, `nav.pack`, `nav.governance`, `nav.prepare`
- In workspaces `container`, prefer `--repo` for direct `nav.find`, `nav.search`, and `nav.intent` before escalating to semantic selectors.
- Deep semantics may use the daemon: `nav.refs`, `nav.context`, `nav.deps`, `nav.related`, `nav.service`, `nav.workspace-map`, `nav.diff-context`, `nav.batch`, `nav.ask`, `nav.affected`, `nav.callers`, `nav.callees`, `nav.path`, `nav.explain`, `nav.neighbors`, `nav.explain-change`
- `nav.edit-plan` is a separate guarded patch packet surface; it is not semantic preparation or a fallback lane.
- The daemon is optional. If it is unavailable, preserve the runtime’s labeled partial/direct result; do not silently switch tools.

## Container workspaces

If the workspace is a parent folder, start broad on the container and then narrow with the selector that matches the query type:

- Direct catalog reads: `--repo` on `nav.find`, `nav.search`, `nav.intent`
- Semantic queries: `--repo`, `--entrypoint`, `--solution`, or `--project`
- Wiki/docs queries: do not use `--repo` as a docs selector. If an old prompt says `nav ask --repo docs`, rerun through `nav wiki search|route|pack`; the CLI accepts the flag only as compatibility guidance.

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

`.mi-lsp/` is operational state and is hard-ignored recursively by index/search/evidence collection. Do not ask users to add `.mi-lsp/` to `.milspignore`, and do not treat nested `.mi-lsp/index.db` hits as valid code evidence.

`nav ask` code evidence skips operational paths and binary sidecars such as `.db`, `.sqlite`, `.exe`, and `.dll` before snippets are added. If you need to diagnose those files, do it as an explicit operational/debug step, not as normal answer evidence.

If index or search results are polluted by generated folders, browser profiles, logs, extracted artifacts, or docs templates, suggest exact repo-local entries in `.milspignore`.

Do not suggest `node_modules/`; it is already ignored by default.

## Output discipline

For graph/change explanations, organize the answer with the seven available sections in this order when present: `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, `wiki`. Explain graph relationships together with wiki anchors, evidence, labeled fallbacks, and omissions; preserve absent sections as unavailable rather than inventing them. A preview or join is not completion without fresh verification.

- Summarize the most relevant hits instead of pasting large JSON blobs.
- Mention the selected repo when answering from a container workspace.
- If results are truncated, rerun narrower or explain how to narrow them.
- For `nav ask`, include the primary doc, the strongest code evidence, and one or two follow-up commands.
- For `nav route` and `nav pack`, each doc in the result carries a `stage` field: `anchor` (canonical anchor doc), `preview` (mini pack preview), or `discovery` (advisory, non-authoritative). Use this to distinguish source authority without relying on array position.
- For `nav wiki search`, treat returned docs and `next_queries` as the canonical path; do not let a later broad `nav search` override source authority unless you explicitly state that you are now showing non-canonical/supporting evidence.
- If AXI emits `next_hint` toward `--full`, prefer that rerun before inventing a broader command.
- If `continuation` is present, follow `continuation.next` first; only use `alternate` when the primary path is blocked or clearly insufficient.
- If `memory_pointer.stale=true`, prefer `workspace status --full` or a fresh `index` before leaning on the pointer as ground truth.
- Do not append `--axi` to reruns on AXI-default surfaces unless you are crossing into a classic-default command.

## Fallback

Keep `mi-lsp` first. An external fallback is allowed only when the result carries one of these visible reasons: `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete`. A timeout is never a silent fallback trigger. Preserve preview sections, partial evidence, candidates, omissions, and heuristic labels; report the limitation and use the exact expansion command when one is emitted. If none of the four reasons is present, do not leave the `mi-lsp` lane.

## Portable preparation

Use `mi-lsp prepare create`, `mi-lsp prepare verify`, and `mi-lsp prepare refresh` with an explicit `--workspace` and validated `--output`/evidence root. Seed receipts contain metadata and digests only; typed drift results identify repairable conditions without widening authority. Evidence is observational and never authorizes writes, protected mutations, or promotions.

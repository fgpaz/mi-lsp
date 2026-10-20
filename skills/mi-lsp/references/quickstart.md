# mi-lsp Quickstart

Use this when `SKILL.md` is not enough and you need a slightly longer decision guide without loading the full recipe set.

## If `mi-lsp` is missing

If the skill is installed but the command is missing, report `unavailable_binary`. For a separately authorized installation:

1. Download the matching release bundle from `https://github.com/fgpaz/mi-lsp/releases`.
2. Extract it into a stable directory and keep `workers/<rid>/` next to `mi-lsp(.exe)`.
3. Add that directory to the current session `PATH`.
4. Verify the binary before trying to initialize any workspace:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

If the bundle was moved after extraction, run `mi-lsp worker install`.

Windows bundles for `win-x64` and `win-arm64` contain `mi-lsp.exe`. A `.cmd` shim is not the product. If `MI_LSP_BIN` is set, it must point at that real executable. Confirm the executable with `mi-lsp workspace which` before blaming the daemon. `mi-lsp nav suggest` only names an existing `nav` command; it does not replace `nav intent`.

## First-use bootstrap

Only for explicitly authorized workspace setup; ordinary queries reuse existing setup.

```powershell
mi-lsp workspace list
mi-lsp
mi-lsp init . --name <alias>
mi-lsp workspace status <alias>
```

If the repo already exists in the registry, reuse that alias instead of creating a new one.
If `mi-lsp workspace list` fails because the command is missing, return to the install steps above. Leave the `mi-lsp` lane only for a visible `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete` reason; never fall back silently.

## Ordinary queries: one entry, then exact next steps

Setup above is for explicitly authorized first use, not a per-query preflight.

1. For a goal, run `mi-lsp nav intent "<goal>"`. A literal requested operation stays direct.
2. Follow the emitted `continuation.next`, `expansions[].command`, or executable `next_queries` exactly.
3. When ranges are known, read them together with `nav multi-read`; do not reload the whole file.
4. Report partial/degraded results and missing evidence honestly. A README fallback cannot establish callers, impact, or a semantic graph.

Use only the workspace selector and output format the caller actually needs:

```powershell
# Code Mode parses JSON; replace the example workspace with the resolved alias.
mi-lsp nav intent "how does the daemon start" --workspace myapp --format json
# Read the exact ranges returned by that query, not invented paths.
```

For model-readable output, choose `--format toon` instead. Do not add `--profile`, `--classic`, `--axi`, output budgets, or backend overrides by habit. Use additional controls only for a specific requirement or an emitted continuation.

In mi-mcp Code Mode, use `herramientas.buscar` -> `herramientas.describe` -> `herramientas.llamar`; discover operation names and argument schemas rather than copying a static helper catalog. The provider owns CLI mapping and JSON output. Do not append arbitrary flags or remove its fixed restrictions. Direct Code Mode execution follows the same navigation and failure contract.

`--no-auto-register`, `--no-auto-daemon`, and `--no-daemon` are explicit diagnostics, not caller-added defaults. A provider may impose a restriction: the current manifest fixes `--no-auto-register`. Missing setup through that door must be diagnosed separately, not bypassed by the caller. The CLI may manage its authorized internal lifecycle; explicit installation, indexing, and daemon control still need their own authorization.

Process the response inside Code Mode and return only needed evidence plus workspace/authority, warnings/omissions, incomplete state, truncation and next commands. Preserve each call's failure when batching independent reads; do not parallelize dependent continuations.

## Canonical wiki-first loop

Use this loop when the question is document-first instead of code-first:

1. `nav route` to get the canonical anchor from governance
2. `nav wiki search` with `--layer` to discover the right RF/FL/TP/CT/TECH/DB docs
3. `nav wiki pack` to read the minimal canonical set
4. `nav wiki trace` when you already have an explicit ID
5. `nav search --include-content` only after the canonical anchor is known, or when you need raw implementation evidence

Example:

```powershell
mi-lsp nav route "how does login work?" --workspace <alias> --format toon
mi-lsp nav wiki search "RF-AUTH login" --workspace <alias> --layer RF,TP,CT --format toon
mi-lsp nav wiki pack "how does login work?" --workspace <alias> --format toon
mi-lsp nav wiki trace RF-AUTH-001 --workspace <alias> --format toon
```

If AXI preview is trimmed or `next_hint` asks for expansion, rerun the same wiki command with `--full` before switching to a broader surface.
If a broad `nav search` returns `.docs/raw`, audits, prompts, or generated/support files, treat them as non-canonical evidence and go back to the wiki lane for source authority.
Canonical doc location follows governance and `read-model`, not a fixed path assumption.

## Choose the right command

| Need | Prefer |
|---|---|
| Find wiki RF/FL/TP/CT/TECH/DB docs | `nav wiki search "workflow masterformularios" --layer RF,FL,CT,TP` |
| Build a pack from wiki anchors | `nav wiki pack "workflow con masterformularios"` |
| Trace one requirement/test ID through the canonical wiki surface | `nav wiki trace RF-QRY-003` |
| Cheapest canonical orientation (no index needed) | `nav route "how is this workspace organized?"` |
| Find semantic wiki candidates by job | `nav recall "what contract defines fallback?" --intent formula` |
| Understand the repo with full evidence | `nav ask "how is this workspace organized?"` |
| Find the right repo/entrypoint in a parent folder | `nav workspace-map --axi` |
| Understand one symbol fully | `nav related MySymbol` |
| Find code by purpose | `nav intent "password reset frontend"` |
| Read code around a known line | `nav context path/to/file.cs:42` or `nav context path/to/file.cs 42` |
| Search text and see the matching code | `nav search "pattern" --include-content` |
| Read several files/ranges together | `nav multi-read ...` |
| Inspect additive wiki↔code context | Existing `nav.trace`, `nav.wiki.trace`, `nav.pack`, `nav.wiki.pack`, `nav.related`, `nav.neighbors`, `nav.prepare`, and `nav.change-pack` surfaces with `wiki_code_context` |
| Do mixed search + read + context in one shot | `nav batch` |

## Search syntax reminder

- `nav search` takes exactly one positional `pattern` argument.
- Quote the full pattern when it contains spaces: `mi-lsp nav search "forgot password" --workspace <alias> --format compact`.
- If the pattern is regex-like, keep it quoted and add `--regex`.
- Avoid `mi-lsp nav search forgot password ...`; those bare words are parsed as extra arguments.

## Routing reminder

Start with the user’s goal in `nav intent`; supported graph/change intents are routed automatically without a routing opt-out. `explain-change` is an intent/operation of `nav intent`, so the request need not contain that literal alias. The planner returns a bounded `mode=preview` envelope with graph, wiki, evidence, fallbacks, available information, candidates, omissions, and exact `expansions[].command` plus its reason. Preserve the seven sections `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki`. Follow the exact expansion command verbatim and add `--full` only when requested by `next_hint` or the expansion.

Supported intent lanes map to exposed commands: explain-change -> `explain-change`, affected-change -> `affected`, callers -> `callers`, callees -> `callees`, path-between -> `path`, explain-edge -> `explain`, and neighborhood -> `neighbors`. Use `related` only for its separate definition/callers/implementors/tests summary. Preview, timeout, silence, `DONE`, or `PASS` without fresh evidence is not PASS and never enables fallback.

Direct and daemon-insensitive: `find`, `search`, `wiki search`, `wiki map`, `intent`, `symbols`, `outline`, `overview`, `multi-read`, `route`, `pack`, `trace`, `governance`, `prepare`
Potentially daemon-backed: `refs`, `context`, `deps`, `related`, `service`, `workspace-map`, `diff-context`, `batch`, `callers`, `callees`, `path`, `explain`, `neighbors`, `affected`, `explain-change`

When a listed surface returns `wiki_code_context`, preserve its separate `direct_code`, `tests`, `supporting_code`, and `candidates` lanes together with every typed omission; do not collapse omissions into absence.

If a cheap read is slow, suspect stale binary, stale index, or wrong PATH before suspecting daemon health. A timeout is visible incomplete evidence, never a silent fallback.
In container workspaces, prefer `--repo` for direct `find`, `search`, or `intent` before reaching for semantic selectors.
Do not use `--repo docs` as a wiki selector. Use `nav wiki search|route|pack`; `nav ask|route|pack --repo` is compatibility-only and will be ignored for docs.
Do not use `nav search` to decide which documentation source is canonical when `nav wiki search|route|pack|trace` can answer that question.
If `nav recall` cannot use configured embeddings because config, key, or provider fails, remain in the governed lexical lane with `nav wiki search` and state only a sanitized reason; there is no hidden BGE runtime fallback. Authentication is explicit: empty or whitespace-only `api_key_env` means no auth header; a configured environment variable that is missing, empty, or whitespace-only returns `SEM_API_KEY_MISSING` before network I/O; `Bearer` is emitted only for a present value. Public errors never include provider bodies, credentials, keys, or authorization headers.
For source validation, `nav wiki validate-source` returns `BLOCKED` with `scope=no_match` for non-source-only or unmatched scopes. Canonical source IDs, canonical source paths, and mixed valid source ID/path scopes return `ready`; pass `--ids` and/or `--paths` explicitly.
For Go files, `nav context` / `nav refs` may use `gopls` when it is installed. If `gopls` is unavailable, accept only the runtime’s visible catalog/text partial evidence; do not silently switch tools.

Allowed external fallback reasons are only `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, and `explicit_incomplete`, each with a separate sanitized `detail`. Global `--max-chars` caps output; `0` means unset and truncation must keep `continuation.next`. `nav edit-plan` is a guarded patch preview/apply surface and is not a fallback for `nav prepare`. Do not use `rg`, `Grep`, `Glob`, or broad reads before `mi-lsp`; leave the lane only for one of those four visible reasons. Use 180/300-second soft/hard watchdogs, at most two smaller same-context recoveries without unchanged retry, six practical lanes with exclusive `allowed_paths`, fail-closed joins, fresh verification, and redacted evidence without prompts, transcripts, secrets, PII, PHI, argv, or raw patterns. Do not invent model/provider metadata.

## Portable preparation

For portable readiness, run `mi-lsp prepare create|verify|refresh --workspace <alias>` with an explicit output/evidence root. Seed receipts are metadata/digest inputs only; typed drift can be repaired, but evidence never authorizes writes or promotions.

# mi-lsp Quickstart

Use this when `SKILL.md` is not enough and you need a slightly longer decision guide without loading the full recipe set.

## If `mi-lsp` is missing

If the skill is installed but the command is missing, install the CLI before doing anything else:

1. Download the matching release bundle from `https://github.com/fgpaz/mi-lsp/releases`.
2. Extract it into a stable directory and keep `workers/<rid>/` next to `mi-lsp(.exe)`.
3. Add that directory to the current session `PATH`.
4. Verify the binary before trying to initialize any workspace:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

If the bundle was moved after extraction, run `mi-lsp worker install`.

## First-use bootstrap

```powershell
mi-lsp workspace list
mi-lsp
mi-lsp init . --name <alias>
mi-lsp workspace status <alias>
```

If the repo already exists in the registry, reuse that alias instead of creating a new one.
If `mi-lsp workspace list` fails because the command is missing, return to the install steps above. Leave the `mi-lsp` lane only for a visible `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete` reason; never fall back silently.

## Preferred command order

For every supported goal-shaped request, start the query with `nav intent`; routing is automatic and has no opt-out. Bootstrap or `workspace status` may precede it when the CLI/workspace needs onboarding.

1. `mi-lsp` or `workspace status` when you need the first onboarding/discovery pass
2. `nav wiki search` when you need RF/FL/TP/CT/TECH/DB docs or traceability anchors
3. `nav route` when you need the cheapest canonical orientation (no index needed, governance-first)
4. `nav ask` when you need richer orientation, ownership, or docs-first evidence synthesis
5. `nav recall --intent formula|evidence|route|explore|learning` when embeddings are configured and you need semantic wiki candidates
6. `nav search --include-content` when you need literal implementation evidence
7. `nav workspace-map --axi` when you need structure across repos or services
8. `nav related` when you need one symbol's neighborhood in one call
9. `nav service` when you need evidence-first understanding of a backend area
10. `nav intent` when you know what the code does but not the symbol name
11. `nav multi-read` or `nav batch` when you already know the targets

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
| Do mixed search + read + context in one shot | `nav batch` |

## Search syntax reminder

- `nav search` takes exactly one positional `pattern` argument.
- Quote the full pattern when it contains spaces: `mi-lsp nav search "forgot password" --workspace <alias> --format compact`.
- If the pattern is regex-like, keep it quoted and add `--regex`.
- Avoid `mi-lsp nav search forgot password ...`; those bare words are parsed as extra arguments.

## Routing reminder

Start with the user’s goal in `nav intent`; supported graph/change intents are routed automatically without a routing opt-out. `explain-change` is an intent/operation of `nav intent`, so the request need not contain that literal alias. The planner returns a bounded `mode=preview` envelope with graph, wiki, evidence, fallbacks, available information, candidates, omissions, and exact `expansions[].command` plus its reason. Preserve the seven sections `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki`. Follow the exact expansion command verbatim and add `--full` only when requested by `next_hint` or the expansion.

Supported intent lanes map to exposed commands: explain-change -> `explain-change`, affected-change -> `affected`, callers -> `callers`, callees -> `callees`, path-between -> `path`, explain-edge -> `explain`, and neighborhood -> `neighbors`. Use `related` only for its separate definition/callers/implementors/tests summary. Preview, timeout, silence, `DONE`, or `PASS` without fresh evidence is not PASS and never enables fallback.

Direct and daemon-insensitive: `find`, `search`, `wiki search`, `intent`, `symbols`, `outline`, `overview`, `multi-read`, `route`, `pack`, `trace`, `governance`, `prepare`
Potentially daemon-backed: `refs`, `context`, `deps`, `related`, `service`, `workspace-map`, `diff-context`, `batch`, `callers`, `callees`, `path`, `explain`, `neighbors`, `affected`, `explain-change`

If a cheap read is slow, suspect stale binary, stale index, or wrong PATH before suspecting daemon health. A timeout is visible incomplete evidence, never a silent fallback.
In container workspaces, prefer `--repo` for direct `find`, `search`, or `intent` before reaching for semantic selectors.
Do not use `--repo docs` as a wiki selector. Use `nav wiki search|route|pack`; `nav ask|route|pack --repo` is compatibility-only and will be ignored for docs.
Do not use `nav search` to decide which documentation source is canonical when `nav wiki search|route|pack|trace` can answer that question.
If `nav recall` cannot use configured embeddings because config, key, or provider fails, remain in the governed lexical lane with `nav wiki search` and state only a sanitized reason; there is no hidden BGE runtime fallback. Authentication is explicit: empty or whitespace-only `api_key_env` means no auth header; a configured environment variable that is missing, empty, or whitespace-only returns `SEM_API_KEY_MISSING` before network I/O; `Bearer` is emitted only for a present value. Public errors never include provider bodies, credentials, keys, or authorization headers.
For source validation, `nav wiki validate-source` returns `BLOCKED` with `scope=no_match` for non-source-only or unmatched scopes. Canonical source IDs, canonical source paths, and mixed valid source ID/path scopes return `ready`; pass `--ids` and/or `--paths` explicitly.
For Go files, `nav context` / `nav refs` may use `gopls` when it is installed. If `gopls` is unavailable, accept only the runtime’s visible catalog/text partial evidence; do not silently switch tools.

Allowed external fallback reasons are only `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, and `explicit_incomplete`. `nav edit-plan` is a guarded patch preview/apply surface and is not a fallback for `nav prepare`. Do not use `rg`, `Grep`, `Glob`, or broad reads before `mi-lsp`; leave the lane only for one of those four visible reasons. Use 180/300-second soft/hard watchdogs, at most two smaller same-context recoveries without unchanged retry, six practical lanes with exclusive `allowed_paths`, fail-closed joins, fresh verification, and redacted evidence without prompts, transcripts, secrets, PII, PHI, argv, or raw patterns. Do not invent model/provider metadata.

## Portable preparation

For portable readiness, run `mi-lsp prepare create|verify|refresh --workspace <alias>` with an explicit output/evidence root. Seed receipts are metadata/digest inputs only; typed drift can be repaired, but evidence never authorizes writes or promotions.

---
name: mi-lsp
description: Navigate code/docs, find files and check diagnostics after editing. Same CLI/provider for pi, Claude Code, Codex and PiG.
---

# mi-lsp

One binary, one provider manifest, one skill. No harness-specific hooks required.
Use `mi-lsp models` for the small executable surface; `mi-lsp nav --help` and
`mi-lsp provider-manifest` retain the full advanced catalog and legacy APIs.

| Need | Code Mode operation | Direct CLI |
|---|---|---|
| Goal / code or canonical docs entry | `lsp.intent` | `mi-lsp nav intent "<goal>"` |
| Read known ranges | `lsp.read` | `mi-lsp nav multi-read path:20-80` |
| Text matches | `lsp.search` | `mi-lsp nav search "<pattern>"` |
| Symbol references | `lsp.refs` | `mi-lsp nav refs "<symbol>"` |
| Errors/warnings after editing | `lsp.diagnostics` | `mi-lsp nav diagnostics path` or `--changed` |
| File names / glob | `fs.find` | `mi-lsp find-files "<pattern>"` |

Set `MI_LSP_CLIENT_NAME` to the harness and `MI_LSP_SESSION_ID` to the session
for direct CLI use. Reuse the resolved workspace; specify `--workspace` when
ambiguous. Code Mode uses `workspace` in the input object.

## Small, composable calls

These are **mi-mcp execute sandbox globals** projected from the provider by a
published H3 consumer supporting `model_alias`, not arbitrary host tool names.
Metadata alone or an older gateway (CLI129) does not make them executable:

```js
const files = await fs.find({pattern: "*.go", workspace: "project", max_items: 10});
const context = await lsp.intent({question: "where is request routing?", workspace: "project"});
return {files, context};
```

```js
const read = await lsp.read({ranges: ["src/router.go:20-80"], workspace: "project"});
const diagnostics = await lsp.diagnostics({file: "src/router.go", workspace: "project"});
return {read, diagnostics};
```

### `execute` body: the returned value is the result

- The body of a mi-mcp `execute` call is an async function body and **what it
  returns is the result**. One result → `return await lsp.intent({...});`.
  Several results → one single `return` carrying an object or array, so no
  result is dropped.
- The mi-mcp sandbox does not define `console`: a `console.log(...)` inside that
  body ends the call with `script_error` ("console is not defined") and prints
  nothing. Observed on mi-mcp 0.3.139; the durable rule comes from the sandbox
  source in fgpaz/mi-mcp (`core/sandbox/child.go`, `core/sandbox/scriptapi.go`):
  the exported value of the body is the delivered result, and `text()`/`json()`
  stay available as the published payload channel. A body that only declares
  variables and never returns delivers an empty result.
- Independent calls may run concurrently (`Promise.all`, or `Promise.allSettled`
  when one failure must not discard the others); the results still travel in that
  one structured `return`.
- **This is not the harness `codemode` tool.** That other surface has its own
  contract, where `console.log`, `text(value)` and a top-level `return` are
  valid. `console.log` is also valid in Node, for example in the `.mjs` scripts
  of this repo. Read every example against the backend that runs it; nothing
  here replaces `console` globally.
- **Compatibility is per backend and version, not a universal cut.** The `return`
  contract above is credited for the mi-mcp `execute` sandbox as observed on
  0.3.139. For another version or another backend, check the current contract
  before assuming it.
- **Legacy examples.** An `execute` body that prints with `console.log` (for
  example `console.log(await lsp.read({...}))`) is outdated for that backend
  only: label it legacy instead of deleting it, and never generalize it to
  another surface.

After each edit, invoke diagnostics for edited files (or `changed: true` for a
batch), inspect errors and repair relevant ones before requesting parent tests.
A missing backend or partial result is not a clean diagnostic run. This skill
requests the call; automatic after-edit integration is owned and verified by the
harness adapter, never inferred from a manual call.

Default navigation budget: 20 items / approximately 2000 tokens. Narrow paths,
ranges and patterns first. Raise `max_items` / `token_budget` only for a concrete
omission. Preserve warnings, authority, workspace, truncation and typed failures.
Follow exact emitted `continuation.next` / `expansions[].command` before widening;
advanced CLI operations remain executable directly. A literal requested operation
stays direct. Lexical candidates are not semantic evidence.

## Honest recovery

If an alias is absent, inspect `herramientas.describe({cli:"lsp", comando:"diagnostics"})`
inside execute, or the installed `mi-lsp provider-manifest`. Do not invent argv.
Legacy `milsp.*` remains supported. Provider restrictions such as fixed
`--no-auto-register` remain binding. Installation/init/index/restarts are separate
operations, never implicit prerequisites to a read.

External fallback requires visible `reason_code` plus bounded sanitized `detail`:
`unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or
`explicit_incomplete`. Retain partial evidence; timeout/silence is not success.
Use bounded local file/search fallback only with that reason. Never publish secrets,
raw prompts or transcripts. Plugin installation / tool listing is not runtime QA.

Distribution and per-user configuration: [all harnesses](references/all-harnesses.md).
Advanced recipes remain in `references/`; load only the needed recipe.

## Canonical wiki-first rule

`WIKI-CONTRACT-v1` is the canonical contract; its typed `links` feed the unified wiki+code graph.

When the task asks for the canonical document, applicable RS/RF/TP/CT/TECH/DB, specification meaning, or requirement traceability, start from governance-backed wiki surfaces:

1. Use `nav wiki inventory` when the owning workspace is unknown. It returns a bounded per-workspace catalog; add `--with-layer-counts` when layer counts are needed.
2. Use `nav wiki-root` (alias `nav wiki root`) when the portable wiki root is needed. Pass `--role producto|ecosistema|gobierno_local` when the workspace declares `[[canon]]`; do not hardcode `.docs/wiki`.
3. Use `nav route` for the least expensive canonical anchor, then `nav wiki search`, `nav wiki pack`, or `nav wiki trace` for the bounded reading set.
4. Use `nav wiki map` only for a compact knowledge-wiki catalog; it does not return full bodies.

Only use `nav search --include-content` after the canonical anchor is known, or when the question is explicitly implementation-first. Treat broad search hits from prompts, audits, generated paths, and support artifacts as non-authoritative when a `nav wiki *` surface can answer the question. Follow exact `continuation.next` and `next_queries` before widening a query. Use `--layer RS,RF,FL,TP,CT,TECH,DB` to narrow canonical discovery. The five fan-out wiki operations (`search`, `route`, `trace`, `pack`, `inventory`) accept `--all-workspaces`; `nav wiki map` remains single-workspace.

## Intent and graph routing

Use `nav intent "<goal>"` as the first hybrid entry point for a goal-shaped request. Supported graph/change goals route to the bounded planner; documentation questions remain in `mode=docs`, and symbol-like questions remain in `mode=code`. Preserve the returned sections, candidates, omissions, authority, and exact expansions. Follow the emitted `expansions[].command` or `continuation.next`; do not invent a broader command or infer a selector.

The planner may expand to these graph operations:

- change explanation and impact: `nav explain-change`
- affected code, tests, and docs: `nav affected`
- incoming callers or outgoing callees: `nav callers` / `nav callees`
- a path between selectors: `nav path`
- one exact edge: `nav explain`
- a symbol neighborhood: `nav neighbors`
- a bounded combined packet: `nav change-pack`

`nav explain-change` is an expansion of `nav intent`, not a routing opt-out. `nav neighbors` is the exposed command for a neighborhood query; `nav related` remains the separate definition/callers/implementors/tests summary. For wiki-centered graph navigation, use `nav wiki graph --mode neighbors|path|explain|affected|change-pack|matrix`; `nav wiki trace` and `nav trace` retain their canonical document-to-code trace surfaces.

Preview is evidence, not completion. Keep `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki` sections when present, along with graph provenance, freshness, candidates, fallbacks, and omissions. A timeout, silence, or a `DONE`/`PASS` label without fresh evidence is not success.

## Unified wiki-code context

The wiki is authoritative; catalog, bindings, graph, SQLite, and caches are derived. The existing `nav.trace`, `nav.wiki.trace`, `nav.pack`, `nav.wiki.pack`, `nav.related`, `nav.neighbors`, `nav.prepare`, and `nav.change-pack` surfaces may add the read-only `wiki_code_context` envelope. This is additive and does not introduce a second public router.

Keep the evidence lanes distinct: `direct_code` for exact active bindings, `tests` for exact test bindings, `supporting_code` only when the graph is current, and bounded lexical `candidates` that never become direct claims. In code-to-wiki results, `wiki_context` is the canonical reverse projection. Report independent freshness for `docs_manifest`, `bindings`, `catalog`, `graph`, and `authority`; preserve typed omissions such as `graph_stale`. Planned bindings are nonnavigable, and retired bindings are excluded by default except for an explicit old ID/path that yields a bounded `superseded_by` redirect. Raw and audit artifacts are not authority.

Use [references/compound-commands.md](references/compound-commands.md) for schemas and fields, and [references/recipes.md](references/recipes.md) for bounded graph, trace, and change-pack recipes. Keep the workspace explicit in every direct CLI example.

## Hot path

- Resolve the workspace and executable: `mi-lsp workspace which --format toon`
- Resolve the canonical wiki root: `mi-lsp nav wiki-root --workspace <alias> --format toon`
- Search canonical documentation: `mi-lsp nav wiki search "<topic>" --workspace <alias> --layer RS,RF,FL,TP,CT,TECH,DB --format toon`
- Trace a requirement or document: `mi-lsp nav wiki trace <ID> --workspace <alias> --format toon` or `mi-lsp nav trace <ID> --workspace <alias> --format toon`
- Explain change impact: `mi-lsp nav intent "explain the impact of this change" --workspace <alias> --format toon`
- Inspect affected paths: `mi-lsp nav affected <path> --include-tests --include-docs --workspace <alias> --format toon`
- Inspect callers, callees, paths, and neighbors: `mi-lsp nav callers <symbol> --workspace <alias> --format toon`, `mi-lsp nav callees <symbol> --workspace <alias> --format toon`, `mi-lsp nav path <from> <to> --workspace <alias> --format toon`, and `mi-lsp nav neighbors <symbol> --workspace <alias> --format toon`
- Build a bounded change packet: `mi-lsp nav change-pack <ref> --workspace <alias> --format toon`
- Query the unified wiki graph: `mi-lsp nav wiki graph --mode neighbors --workspace <alias> --format toon`

Prefer these bounded surfaces over repeated raw searches or full-file reads. If a result is truncated, use its exact continuation or narrowing hint.

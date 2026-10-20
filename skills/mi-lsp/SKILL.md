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
console.log({files, context});
```

```js
console.log(await lsp.read({ranges: ["src/router.go:20-80"], workspace: "project"}));
console.log(await lsp.diagnostics({file: "src/router.go", workspace: "project"}));
```

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

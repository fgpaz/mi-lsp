---
name: mi-lsp
description: Agent-first semantic code navigation with the mi-lsp CLI, without requiring an MCP server. Use for Codex, Claude Code, and any coding agent that supports folder-based skills when you need fast workspace orientation, docs-first repo Q&A, symbol search, semantic refs/context, service profiling, batched file reads, or repo-local noise control with .milspignore.
---

# mi-lsp

Use this skill when `mi-lsp` is available and you want local semantic navigation without introducing an MCP dependency.

Prefer `--format compact` and an explicit `--workspace <alias>`.
Prefer compound commands over sequential greps and full-file reads.

## Tool binding

Run `mi-lsp` through the host shell tool, not through a custom MCP tool:

- Codex: `functions.shell_command`
- Claude Code: shell/Bash tool
- Other skill-based agents: the local terminal/shell tool they already expose

Do not wait for a dedicated `mi-lsp` MCP integration. `mi-lsp` is a CLI-first tool.

## First-use check

1. Confirm `mi-lsp` is callable in the current shell.
2. Prefer the short bootstrap path first.
3. If the workspace is already registered, resolve it and continue.

```powershell
mi-lsp workspace list
mi-lsp init . --name <alias>
mi-lsp workspace status <alias> --format compact
```

If `mi-lsp` is not on `PATH`, repair `PATH` for the current session before falling back to other tools.

## Hot path

Use these commands first:

- Ask the repo how it is organized: `mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format compact`
- Read 2+ file slices: `mi-lsp nav multi-read file1:1-120 file2:40-160 --workspace <alias> --format compact`
- Search and see code inline: `mi-lsp nav search billing retry --include-content --workspace <alias> --format compact`
- Understand a symbol in one call: `mi-lsp nav related MySymbol --workspace <alias> --format compact`
- Orient in a new repo or parent folder: `mi-lsp nav workspace-map --workspace <alias> --format compact`
- Profile a service: `mi-lsp nav service <path> --workspace <alias> --format compact`
- Batch mixed operations: `mi-lsp nav batch --workspace <alias> --format compact`

Prefer these over repeated `Get-Content`, plain `rg`, or one-file-at-a-time reads.

## Minimal workflow

1. Bootstrap or verify the workspace.

```powershell
mi-lsp init . --name <alias>
mi-lsp workspace status <alias> --format compact
```

2. Start with intent, not grep.

```powershell
mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format compact
mi-lsp nav ask "where is daemon routing implemented?" --workspace <alias> --format compact
```

3. Move to broad discovery when you need structure.

```powershell
mi-lsp nav workspace-map --workspace <alias> --format compact
mi-lsp nav find <symbol> --workspace <alias> --format compact
mi-lsp nav search "<text>" --include-content --workspace <alias> --format compact
```

4. Move to deep semantics only when needed.

```powershell
mi-lsp nav refs <symbol> --workspace <alias> --backend roslyn --format compact
mi-lsp nav context <file> <line> --workspace <alias> --backend roslyn --format compact
mi-lsp nav related <symbol> --workspace <alias> --format compact
```

5. Use `nav service` before judging whether a backend service is only scaffolding.

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format compact
```

## Tool choice ladder

Use `mi-lsp` first for repo navigation, docs-first Q&A, symbol lookup, service audits, and batch reads.

- Start with `nav ask` for orientation or "where is X decided?" questions.
- Use `workspace-map`, `search --include-content`, and `multi-read` before broad raw file reads.
- Use `related`, `context`, `refs`, and `deps` when you need semantic depth.
- Use plain `rg` only when `mi-lsp` is unavailable or the request falls outside the CLI surface.

## Routing model

- Cheap reads stay direct: `nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`
- Deep semantics may use the daemon: `nav.refs`, `nav.context`, `nav.deps`, `nav.related`, `nav.service`, `nav.workspace-map`, `nav.diff-context`, `nav.batch`, `nav.ask`
- The daemon is optional. If it is unavailable, the CLI must still work in direct mode.

## Container workspaces

If the workspace is a parent folder, use broad discovery on the container and rerun deep semantic queries with one selector:
- `--repo`
- `--entrypoint`
- `--solution`
- `--project`

If a semantic query returns `backend=router`, do not guess. Re-run with a narrower selector.

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

- Read [references/quickstart.md](references/quickstart.md) when you need a slightly longer onboarding or command chooser.
- Read [references/compound-commands.md](references/compound-commands.md) when you want `multi-read`, `batch`, `related`, `workspace-map`, `diff-context`, or cross-workspace patterns.
- Read [references/recipes.md](references/recipes.md) when auditing a service, reviewing completeness, or doing PR/impact analysis.
- Read [references/runtime-drift.md](references/runtime-drift.md) when CLI/docs/daemon behavior disagree after rebuilds or reinstalls.

## Noise control

If index or search results are polluted by generated folders, browser profiles, logs, extracted artifacts, or docs templates, suggest exact repo-local entries in `.milspignore`.

Do not suggest `node_modules/`; it is already ignored by default.

## Output discipline

- Summarize the most relevant hits instead of pasting large JSON blobs.
- Mention the selected repo when answering from a container workspace.
- If results are truncated, rerun narrower or explain how to narrow them.
- For `nav ask`, include the primary doc, the strongest code evidence, and one or two follow-up commands.

## Fallback

If `mi-lsp` remains unavailable after repairing `PATH`, fall back to `rg` and targeted file inspection.

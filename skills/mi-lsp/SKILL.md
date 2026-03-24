---
name: mi-lsp
description: Local semantic code navigation with the mi-lsp CLI for Codex, Claude Code, and any coding agent that supports folder-based skills. Use when you need fast workspace orientation, symbol search, semantic refs/context, service profiling, batched file reads, or repo-local noise control with .milspignore.
---

# mi-lsp

Prefer `--format compact` and an explicit `--workspace <alias>`.
Prefer compound commands over sequential greps and full-file reads.

## First-use check

1. Confirm `mi-lsp` is callable in the current shell.
2. Confirm the target workspace already exists.
3. If it does not exist, register it and let `mi-lsp` index it.

```powershell
mi-lsp workspace list
mi-lsp workspace status <alias> --format compact
mi-lsp workspace add <repo-or-parent-path> --name <alias>
```

If `mi-lsp` is not on `PATH`, repair `PATH` for the current session before falling back to other tools.

## Hot path

Use these commands first:

- Read 2+ file slices: `mi-lsp nav multi-read file1:1-120 file2:40-160 --workspace <alias> --format compact`
- Search and see code inline: `mi-lsp nav search "pattern" --include-content --workspace <alias> --format compact`
- Understand a symbol in one call: `mi-lsp nav related MySymbol --workspace <alias> --format compact`
- Orient in a new repo or parent folder: `mi-lsp nav workspace-map --workspace <alias> --format compact`
- Profile a service: `mi-lsp nav service <path> --workspace <alias> --format compact`
- Batch mixed operations: `mi-lsp nav batch --workspace <alias> --format compact`

Prefer these over repeated `Get-Content`, plain `rg`, or one-file-at-a-time reads.

## Minimal workflow

1. Inspect workspace shape.

```powershell
mi-lsp workspace status <alias> --format compact
```

2. Start broad with the narrowest useful command.

```powershell
mi-lsp nav workspace-map --workspace <alias> --format compact
mi-lsp nav find <symbol> --workspace <alias> --format compact
mi-lsp nav search "<text>" --include-content --workspace <alias> --format compact
```

3. Move to deep semantics only when needed.

```powershell
mi-lsp nav refs <symbol> --workspace <alias> --backend roslyn --format compact
mi-lsp nav context <file> <line> --workspace <alias> --backend roslyn --format compact
mi-lsp nav related <symbol> --workspace <alias> --format compact
```

4. Use `nav service` before judging whether a backend service is only scaffolding.

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format compact
```

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

The daemon is optional. If it is unavailable, the CLI must still work in direct mode.

## Noise control

If index or search results are polluted by generated folders, browser profiles, logs, or extracted artifacts, suggest exact repo-local entries in `.milspignore`.

Do not suggest `node_modules/`; it is already ignored by default.

## Output discipline

- Summarize the most relevant hits instead of pasting large JSON blobs.
- Mention the selected repo when answering from a container workspace.
- If results are truncated, rerun narrower or explain how to narrow them.

## Fallback

If `mi-lsp` remains unavailable after repairing `PATH`, fall back to `rg` and targeted file inspection.


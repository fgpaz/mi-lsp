---
name: mi-lsp
description: Use when a folder-based agent should navigate code with the mi-lsp CLI, or when the skill is installed but the mi-lsp binary still needs install/bootstrap on PATH before semantic navigation can begin.
---

# mi-lsp

Use this skill when you want local semantic navigation with `mi-lsp` without introducing an MCP dependency.
If the skill is installed but the binary is missing, bootstrap the CLI first instead of abandoning the flow.

Prefer `--format compact` and an explicit `--workspace <alias>`.
Prefer compound commands over sequential greps and full-file reads.

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
3. Extract it into a stable tools directory and keep `workers/<rid>/` next to the `mi-lsp` binary.
4. Add that directory to the current session `PATH`, or invoke the binary by absolute path until `PATH` is fixed permanently.
5. Verify the install:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

6. If the binary was moved after extraction, run:

```powershell
mi-lsp worker install
```

Windows session example:

```powershell
$installDir = Join-Path $HOME "bin\mi-lsp"
$env:PATH = "$installDir;$env:PATH"
where.exe mi-lsp
mi-lsp info
```

Linux session example:

```bash
export PATH="$HOME/.local/opt/mi-lsp:$PATH"
command -v mi-lsp
mi-lsp info
```

## First-use check

1. Confirm `mi-lsp` is callable in the current shell.
2. Prefer the short bootstrap path first.
3. If the workspace is already registered, resolve it and continue.

```powershell
mi-lsp workspace list
mi-lsp init . --name <alias>
mi-lsp workspace status <alias> --format compact
```

If `mi-lsp` is not on `PATH`, install it from Releases or repair `PATH` for the current session before falling back to other tools.

## Hot path

Use these commands first:

- Ask the repo how it is organized: `mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format compact`
- Read 2+ file slices: `mi-lsp nav multi-read file1:1-120 file2:40-160 --workspace <alias> --format compact`
- Search and see code inline: `mi-lsp nav search billing retry --include-content --workspace <alias> --format compact`
- Search inside one repo of a container workspace: `mi-lsp nav search "forgot password" --workspace <alias> --repo web --format compact`
- Understand a symbol in one call: `mi-lsp nav related MySymbol --workspace <alias> --format compact`
- Orient in a new repo or parent folder: `mi-lsp nav workspace-map --workspace <alias> --format compact`
- Profile a service: `mi-lsp nav service <path> --workspace <alias> --format compact`
- Batch mixed operations: `mi-lsp nav batch --workspace <alias> --format compact`
- Trace spec-to-code links: `mi-lsp nav trace RF-QRY-003 --workspace <alias> --format compact`
- Search by intent/purpose: `mi-lsp nav intent "where do we handle routing fallback?" --workspace <alias> --format compact`

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
mi-lsp nav intent "error handling for daemon connections" --workspace <alias> --format compact
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

6. Trace spec-to-code links when reviewing RF compliance.

```powershell
mi-lsp nav trace RF-QRY-003 --workspace <alias> --format compact
mi-lsp nav trace --all --summary --workspace <alias> --format compact
```

## Tool choice ladder

Use `mi-lsp` first for repo navigation, docs-first Q&A, symbol lookup, service audits, and batch reads.

- Start with `nav ask` for orientation or "where is X decided?" questions.
- Use `nav intent` to find code by purpose when you don't know the symbol name.
- Use `nav trace` to check which code implements a specific RF requirement.
- Use `workspace-map`, `search --include-content`, and `multi-read` before broad raw file reads.
- Use `related`, `context`, `refs`, and `deps` when you need semantic depth.
- Use plain `rg` only when `mi-lsp` is unavailable or the request falls outside the CLI surface.

## Routing model

- Cheap reads stay direct: `nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`, `nav.intent`, `nav.trace`
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

## Fallback

If `mi-lsp` remains unavailable after install and `PATH` repair, fall back to `rg` and targeted file inspection.

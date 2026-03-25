# mi-lsp

![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)
![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)
![CI](https://github.com/fgpaz/mi-lsp/actions/workflows/test.yml/badge.svg)

> Semantic code navigation for coding agents, without requiring MCP.

`mi-lsp` exists because large local codebases are painful to explore from plain grep alone, and many agent workflows add too much setup by depending on an MCP bridge or a long-lived server before the first useful answer appears.

`mi-lsp` takes a different path: a direct local CLI, an optional shared daemon, deep C# semantics through Roslyn, compact output built for agents, and a ready-to-install `$mi-lsp` skill shipped inside the repository.

## Why We Built It

We wanted a tool that:

- works directly from the terminal, even when no MCP server is configured
- gives coding agents compact, high-signal answers instead of grep-heavy loops
- handles large `.NET/C# + TypeScript` workspaces without assuming a monorepo
- lets multiple agents share warm local state without turning the CLI into a remote dependency
- profiles services from observable evidence instead of overconfident completeness scores

In short: `mi-lsp` was created to make local semantic navigation reliable, fast, and easy to adopt for both developers and agent-driven workflows.

## Why It Feels Different

| Workflow question | MCP-dependent workflow | `mi-lsp` |
|---|---|---|
| How do I get started? | Configure a bridge or server, then wire a client | Download the binary and run it |
| What happens if warm infrastructure is unavailable? | The integration path may depend on that bridge | The CLI still works directly; the daemon is optional |
| How do I use it with agents? | Tool-specific integration is usually required | The repo already ships a public skill in [`skills/mi-lsp`](skills/mi-lsp) |
| How do I get the first useful answer fast? | Often search plus several manual reads | Use `workspace-map`, `related`, `multi-read`, `search --include-content`, or `service` |
| Can several agents share warm state? | Sometimes, but often through extra infrastructure | Yes, through one optional local daemon per OS user |
| Is the output agent-friendly? | Depends on the bridge or transport | Compact, deterministic CLI output designed for token budgets |

This is a workflow-level comparison, not a claim about any specific named tool.

## 60-Second Quickstart

The recommended install path is a bundled release from GitHub Releases.

1. Download the asset for your platform (`win-x64`, `win-arm64`, `linux-x64`, or `linux-arm64`) from the [Releases page](https://github.com/fgpaz/mi-lsp/releases).
2. Extract the archive and keep the `workers/<rid>/` directory next to the `mi-lsp` binary.
3. Run the binary directly or add it to your `PATH`.
4. Verify the install:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

If you move the binary after extraction, run `mi-lsp worker install` once to copy the bundled worker into `~/.mi-lsp/workers/<rid>/`.

Register a workspace and get the first useful answers:

```powershell
mi-lsp workspace add C:\code\my-dotnet-app --name myapp
mi-lsp nav workspace-map --workspace myapp --format compact
mi-lsp nav related IOrderRepository --workspace myapp --format compact
mi-lsp nav service src/backend/orders --workspace myapp --format compact
```

## Use With Claude Code, Codex, and Skill-Based Agents

The repository ships a ready-to-install skill in [`skills/mi-lsp`](skills/mi-lsp).
If your coding tool supports folder-based skills, copy or symlink that folder into the skill directory your tool scans.

Typical installs:

```powershell
# Codex
New-Item -ItemType Directory -Force $HOME\.codex\skills | Out-Null
Copy-Item -Recurse .\skills\mi-lsp $HOME\.codex\skills\

# Claude Code or any runner using a folder-based skills setup
New-Item -ItemType Directory -Force $HOME\.agents\skills | Out-Null
Copy-Item -Recurse .\skills\mi-lsp $HOME\.agents\skills\
```

If you prefer live updates while iterating on the skill, use a symlink instead of copying the folder.

Once the skill is installed, an agent can start with prompts such as:

```text
Use $mi-lsp to orient in this repo and summarize the main services.
Use $mi-lsp to find IOrderRepository and tell me which repo owns it.
Use $mi-lsp to audit src/backend/orders and summarize endpoints, consumers, publishers, and entities.
Use $mi-lsp to read the relevant files for OrderHandler and show only the important slices.
```

For shared daemon attribution across several agents, set:

```powershell
$env:MI_LSP_CLIENT_NAME = "codex"
$env:MI_LSP_SESSION_ID = "demo-session"
```

The public connection guide lives in the wiki:

- [Agent Integration](https://github.com/fgpaz/mi-lsp/wiki/Agent-Integration)

## Why Agents Get Useful Answers Faster

The shipped skill steers agents toward the commands that reduce round-trips the most:

- `nav workspace-map` to orient in a repo or parent folder quickly
- `nav related` to understand a symbol in one call
- `nav multi-read` to read several relevant slices at once
- `nav search --include-content` to search and see inline code immediately
- `nav service` to audit a backend from observable evidence instead of guesswork

That makes `mi-lsp` feel less like "grep, then read, then grep again" and more like a local semantic toolchain that can answer useful questions immediately.

## What `mi-lsp` Is Good At

- large local `.NET/C# + TypeScript` codebases
- parent folders containing several independent repos
- agent-driven code exploration with compact outputs
- service-level audits and onboarding
- environments where you want optional warm state but do not want a mandatory MCP dependency

## What `mi-lsp` Is Not

- not an MCP server
- not a remote multi-host daemon platform
- not a semantic editing or refactoring tool
- not a strong service-completeness scoring engine

## Workspace Model

`mi-lsp` supports two canonical workspace shapes:

- `single`: one repo with one obvious semantic root
- `container`: one parent folder that contains many independent repos without requiring a parent `.sln`

Recommended operating pattern:

- use the parent folder for broad discovery: `find`, `search`, `overview`, `symbols`
- use the child repo or explicit selectors for deep semantics: `refs`, `context`, `deps`
- use `service` for evidence-first exploration of an implementation area

## Runtime Model

- One global daemon per OS user, shared across terminals, Claude Code, Codex, and local subagents
- One live runtime per `(workspace_root, backend_type, entrypoint_id)` inside the daemon
- Repo-local semantic state:
  - `.mi-lsp/project.toml`
  - `.mi-lsp/index.db`
- Global local-machine state:
  - `~/.mi-lsp/registry.toml`
  - `~/.mi-lsp/daemon/state.json`
  - `~/.mi-lsp/daemon/daemon.db`

The daemon is a performance optimization, not a prerequisite for the CLI.

## Core Capabilities

```text
mi-lsp workspace add|remove|scan|list|warm|status
mi-lsp nav symbols|find|refs|overview|outline|service|search|context|deps|multi-read|batch|related|workspace-map|diff-context
mi-lsp index [path] [--clean]
mi-lsp info
mi-lsp daemon start|stop|restart|status|logs
mi-lsp worker install|status
mi-lsp admin open|status|export
```

Useful global flags:

```text
--workspace
--format compact|json|text
--client-name
--session-id
--backend roslyn|tsserver|catalog|text
--no-auto-daemon
--compress
```

Environment fallbacks:

- `MI_LSP_CLIENT_NAME`
- `MI_LSP_SESSION_ID`

## Build From Source

Source builds are intended for contributors and maintainers.

Prerequisites:

- Go 1.24+
- .NET 10 SDK

Build and test:

```bash
make build
make test
make lint
```

`make test` uses `-race` when the local Go toolchain supports it and falls back to regular `go test -v ./...` on platforms where the race detector is not available.

For release-like local validation on a specific RID:

```powershell
pwsh ./scripts/release/build-dist.ps1 -Rids @('win-x64') -Clean
pwsh ./scripts/release/install-local.ps1 -Rid win-x64 -InstallDir $HOME\bin
```

This materializes `dist/<rid>/mi-lsp(.exe)` + `dist/<rid>/workers/<rid>/`.

## Troubleshooting

Common first checks:

```powershell
mi-lsp info
mi-lsp worker status --format compact
mi-lsp daemon status
mi-lsp workspace status myapp --format compact
```

If a command fails before `mi-lsp` itself starts, treat it as a host incident first.
See the public runbook in [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Current v0.1.0 Scope

- Global daemon with governance UI and local telemetry
- Repo-local lightweight catalog in SQLite with repo ownership
- Semantic C# queries via Roslyn worker
- Container workspaces with explicit or inferred repo/entrypoint routing
- TS/JS discovery index for symbols, routes, and overview
- Optional TS semantic bridge through `tsserver`
- Optional Python semantic bridge through `pyright-langserver`
- Service exploration summaries via `nav service`

Out of scope for `v0.1.0`:

- MCP transport
- Semantic editing/refactors
- Automatic semantic fanout across all child repos
- Remote or multi-host daemon sharing
- Authenticated governance UI
- Additional languages beyond the current C#/TS/Python focus
- Strong completeness scoring for services

## Documentation

Start here:

- [Agent Integration](https://github.com/fgpaz/mi-lsp/wiki/Agent-Integration)
- [Architecture](https://github.com/fgpaz/mi-lsp/wiki/Architecture)
- [Workspace Model](https://github.com/fgpaz/mi-lsp/wiki/Workspace-Model)
- [Command Reference](https://github.com/fgpaz/mi-lsp/wiki/Command-Reference)
- [Worker Installation](https://github.com/fgpaz/mi-lsp/wiki/Worker-Installation)
- [Troubleshooting](https://github.com/fgpaz/mi-lsp/wiki/Troubleshooting)
- [Developer Guide](https://github.com/fgpaz/mi-lsp/wiki/Developer-Guide)

## License

MIT

# mi-lsp

![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)
![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)
![CI](https://github.com/fgpaz/mi-lsp/actions/workflows/test.yml/badge.svg)

`mi-lsp` is a non-MCP semantic CLI for large `.NET/C# + TypeScript` codebases, especially when agents need reliable local execution and compact outputs.

## Quick start

The recommended install path is a bundled release from GitHub Releases.

1. Download the asset for your platform (`win-x64`, `win-arm64`, `linux-x64`, or `linux-arm64`) from the [Releases page](https://github.com/fgpaz/mi-lsp/releases).
2. Extract the archive and keep the `workers/<rid>/` directory next to the `mi-lsp` binary.
3. Run the binary directly or add it to your `PATH`.
4. Verify the install:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

If you want to move the binary somewhere else after extraction, run `mi-lsp worker install` once to copy the bundled worker into `~/.mi-lsp/workers/<rid>/`.

## Design goals

- Reliable local execution without an MCP server dependency
- Compact JSON output for skills, Codex, and Claude Code
- Real C# semantics through a Roslyn worker
- Lightweight repo-local indexing for discovery and fast navigation
- Service-level exploration that surfaces evidence before conclusions
- Windows and Linux support, with `arm64` as a first-class target
- No remote telemetry; operational data stays on the local machine

## Workspace model

`mi-lsp` supports two canonical workspace shapes.

- `single`: one repo with one obvious semantic root
- `container`: one parent folder that contains many independent repos without requiring a parent `.sln`

The recommended operating pattern is:

- `workspace list` preserves the registered alias names from `registry.toml`
- parent folder for global discovery: `find`, `search`, `overview`, `symbols`
- child repo or explicit selector for deep semantics: `refs`, `context`, `deps`
- service path for evidence-first exploration: `service`

## Runtime model

- One global daemon per OS user, shared across terminals, Claude Code, Codex, and local subagents
- One live runtime per `(workspace_root, backend_type, entrypoint_id)` inside the daemon
- Repo-local semantic state:
  - `.mi-lsp/project.toml`
  - `.mi-lsp/index.db`
- Global local-machine state:
  - `~/.mi-lsp/registry.toml`
  - `~/.mi-lsp/daemon/state.json`
  - `~/.mi-lsp/daemon/daemon.db`

## Build from source

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

`make test` uses `-race` when the local Go toolchain supports it and falls back
to a regular `go test -v ./...` on platforms where the race detector is not
available.

For release-like local validation on a specific RID:

```powershell
pwsh ./scripts/release/build-dist.ps1 -Rids @('win-x64') -Clean
pwsh ./scripts/release/install-local.ps1 -Rid win-x64 -InstallDir $HOME\bin
```

This materializes `dist/<rid>/mi-lsp(.exe)` + `dist/<rid>/workers/<rid>/`.

## Command surface

```text
mi-lsp workspace add|remove|scan|list|warm|status
mi-lsp nav symbols|find|refs|overview|outline|service|search|context|deps|multi-read|batch|related|workspace-map|diff-context
mi-lsp index [path] [--clean]
mi-lsp info
mi-lsp daemon start|stop|restart|status|logs
mi-lsp worker install|status
mi-lsp admin open|status|export
```

## Semantic selectors

For container workspaces, semantic commands support explicit routing selectors:

```text
--repo
--entrypoint
--solution
--project
```

Routing order is:

1. `--entrypoint`
2. `--solution` / `--project`
3. `--repo`
4. file ownership
5. unique catalog match
6. workspace default if it is `single`

If a query is ambiguous in a container workspace, `mi-lsp` fails with `backend=router`, candidate repos, and a `next_hint` instead of guessing.

## Global flags

```text
--workspace
--format compact|json|text
--token-budget
--max-items
--max-chars
--client-name
--session-id
--backend roslyn|tsserver|catalog|text
--no-auto-daemon
--compress
--verbose
```

Environment fallbacks:

- `MI_LSP_CLIENT_NAME`
- `MI_LSP_SESSION_ID`

## Ignore rules

The repo-local index respects layered ignore rules.

Sources, in order:

- built-in defaults: `.git/`, `.idea/`, `.mi-lsp/`, `.next/`, `.worktrees/`, `bin/`, `dist/`, `node_modules/`, `obj/`
- `.gitignore`
- `.milspignore`
- `.mi-lsp/project.toml` under `[ignore].extra_patterns`

Use `.milspignore` when you want `mi-lsp` to ignore repo-local noise that should not affect Git itself.

## Governance UI

Start the daemon and inspect the shared runtime state:

```powershell
mi-lsp daemon start
mi-lsp admin status
mi-lsp admin open --workspace myapp
```

The governance UI is local-only on loopback and exposes:

- workspace `kind`
- repo and entrypoint of each warm runtime
- recent accesses with repo/entrypoint metadata
- safe actions only: refresh, warm, open logs, copy CLI

## Troubleshooting

If a command fails before `mi-lsp` itself starts, treat it as a host incident first.

Common examples:

- PowerShell/CoreCLR startup failure
- stale binary on `PATH`
- missing or incompatible worker install
- `backend=router` because a container workspace needs `--repo` or `--entrypoint`

See the public runbook in [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Service exploration

Use `nav service` before any audit, gap analysis, or onboarding where you need to answer "what does this service actually do?"

```powershell
mi-lsp nav service src/backend/orders --workspace myapp --format compact
mi-lsp nav service src/backend/orders --workspace myapp --include-archetype --format json
```

The command returns evidence, not a score:

- symbol counts by kind from the repo-local catalog
- HTTP endpoints observed via minimal API wiring
- event consumers and publishers observed in code
- entities found under `Domain/Entities` or `Domain/Models`
- infrastructure signals such as EventBus, Redis, or database wiring
- `archetype_matches` when known placeholders are detected

## Real-world usage

### Single repo

```powershell
mi-lsp workspace add C:\code\my-dotnet-app --name myapp
mi-lsp index --workspace myapp --clean
mi-lsp nav refs IOrderRepository --workspace myapp --backend roslyn --format json
```

### Parent folder with many repos

```powershell
mi-lsp workspace add C:\code\customer-systems --name customer-systems
mi-lsp index --workspace customer-systems --clean
mi-lsp nav find IOrderRepository --workspace customer-systems --format json
mi-lsp nav refs IOrderRepository --workspace customer-systems --repo MyApp.Api --backend roslyn --format json
```

### Audit a backend service before estimating completeness

```powershell
mi-lsp nav service src/backend/orders --workspace myapp --format compact
mi-lsp nav context src/backend/orders/Program.cs 42 --workspace myapp --format compact
mi-lsp nav search "IConsumer<|PublishAsync<" --workspace myapp --format compact
```

## Current v0.1.0 scope

- Global daemon with governance UI and local telemetry
- Repo-local lightweight catalog in SQLite with repo ownership
- Semantic C# queries via Roslyn worker
- Container workspaces with explicit or inferred repo/entrypoint routing
- TS/JS discovery index for symbols, routes, and overview
- Optional TS semantic bridge through `tsserver`
- Optional Python semantic bridge through `pyright-langserver`
- Service exploration summaries via `nav service`

Out of scope for v0.1.0:

- MCP transport
- Semantic editing/refactors
- Automatic semantic fanout across all child repos
- Remote or multi-host daemon sharing
- Authenticated governance UI
- Additional languages beyond the current C#/TS/Python focus
- Strong completeness scoring for services

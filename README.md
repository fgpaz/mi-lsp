# mi-lsp

![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)
![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)
![CI](https://github.com/fgpaz/mi-lsp/actions/workflows/test.yml/badge.svg)

> Semantic code navigation for coding agents, without requiring MCP.

`mi-lsp` is a local CLI for exploring large `.NET/C# + TypeScript` codebases from the terminal.
It keeps a lightweight repo-local index, supports optional warm state through a daemon, and now includes a docs-first entrypoint for onboarding a repo fast.
For onboarding and discovery, AXI is now selective by default on the surfaces where it saves the most tokens; use `--classic` when you want the old CLI behavior and `--axi` when you want to force AXI on a classic surface.

## Quick Start

### 1. Install

The recommended install path is a bundled release from GitHub Releases.

1. Download the asset for your platform (`win-x64`, `win-arm64`, `linux-x64`, or `linux-arm64`) from the [Releases page](https://github.com/fgpaz/mi-lsp/releases).
2. Extract it and keep `workers/<rid>/` next to the `mi-lsp` binary.
3. Run the binary directly or add it to your `PATH`.

Sanity check:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

If you move the binary after extraction, run `mi-lsp worker install` once to copy the bundled worker into `~/.mi-lsp/workers/<rid>/`.
Regular C# queries resolve the Roslyn worker by layout presence in `bundle -> installed -> dev-local` order, while `mi-lsp worker status` is the explicit compatibility probe.
`worker status` keeps the same visible diagnostic payload whether it is served directly or through the daemon; only `active_workers` changes with live state. It now also surfaces `cli_path` and `protocol_version`, which makes stale or unexpected binaries on `PATH` easier to diagnose.
On Windows, non-interactive child processes are started hidden so normal queries should not open extra console windows.

### 2. Initialize a workspace

The shortest first-run path is:

```powershell
mi-lsp init . --name myapp
```

That command:
- detects the workspace shape
- registers it in `~/.mi-lsp/registry.toml`
- writes `.mi-lsp/project.toml`
- indexes code and docs by default
- leaves you with a ready `--workspace myapp`

If you prefer the explicit workflow, `workspace add` still exists:

```powershell
mi-lsp workspace add C:\code\my-dotnet-app --name myapp
mi-lsp workspace status myapp --format compact
```

AXI discovery starts from the root command by default:

```powershell
mi-lsp
mi-lsp workspace status myapp
```

### 3. Ask one useful question first

```powershell
mi-lsp nav ask "how is this workspace organized?" --workspace myapp
```

`nav ask` is docs-first:
- it prioritizes `.docs/wiki` when the repo has one
- it uses explicit traceability links before text heuristics
- it adds code evidence so you can jump into the implementation immediately

When you want the reading order instead of a prose answer:

```powershell
mi-lsp nav pack "understand how this login flow works" --workspace myapp
mi-lsp nav pack "understand how this login flow works" --workspace myapp --full
```

### 4. Use the right command for the job

| You want to... | Run this |
|---|---|
| Understand the repo quickly | `mi-lsp nav ask "how is this workspace organized?" --workspace myapp` |
| Get the canonical docs reading order for a task | `mi-lsp nav pack "understand how billing retry works" --workspace myapp` |
| See the high-level map of services | `mi-lsp nav workspace-map --workspace myapp --axi` |
| Understand one symbol deeply | `mi-lsp nav related MySymbol --workspace myapp --format compact` |
| Read the code around one line | `mi-lsp nav context path/to/file.cs 42 --workspace myapp --format compact` |
| Search text and see the matching code | `mi-lsp nav search "billing retry" --include-content --workspace myapp` |
| Search symbols by intent | `mi-lsp nav intent "password reset frontend" --workspace myapp --repo web` |
| Audit one backend/service path | `mi-lsp nav service src/backend/orders --workspace myapp --format compact` |
| Read several files in one call | `mi-lsp nav multi-read file1.cs:1-80 file2.ts:20-80 --workspace myapp --format compact` |

Use `--full` when an AXI preview asks you to expand detail:

```powershell
mi-lsp nav search "billing retry" --include-content --workspace myapp --full
mi-lsp nav workspace-map --workspace myapp --axi --full
```

### 5. Parent folder with several repos

Start broad, then narrow semantic queries:

```powershell
mi-lsp nav workspace-map --workspace myapp --format compact
mi-lsp nav search OrderHandler --workspace myapp --format compact
mi-lsp nav search "forgot password" --workspace myapp --repo web --format compact
mi-lsp nav refs IOrderRepository --workspace myapp --repo Orders.Api --format compact
```

## Docs-First Search

If the repo has `.docs/wiki`, `mi-lsp nav ask` uses it as the primary source of truth.
The project can optionally add `.docs/wiki/_mi-lsp/read-model.toml` to teach `mi-lsp` how to rank:
- functional docs (`01-06`)
- technical docs (`07-09`)
- UX/UI docs (`10-16`)
- generic fallback docs (`README*`, `docs/`, `.docs/`)

That gives you a local, explainable answer instead of a black-box summary.

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

Once the skill is installed, an agent can start with prompts such as:

```text
Use $mi-lsp to initialize this repo and explain how it is organized.
Use $mi-lsp to answer where daemon routing is documented and which code backs it.
Use $mi-lsp to audit src/backend/orders and summarize endpoints, consumers, publishers, and entities.
Use $mi-lsp to read the relevant files for OrderHandler and show only the important slices.
```

For session-wide AXI discovery defaults:

```powershell
$env:MI_LSP_AXI = "1"
```

To opt out on an AXI-default surface:

```powershell
mi-lsp --classic
mi-lsp nav search "billing retry" --workspace myapp --classic --format compact
```

For shared daemon attribution across several agents, set:

```powershell
$env:MI_LSP_CLIENT_NAME = "codex"
$env:MI_LSP_SESSION_ID = "demo-session"
```

## Workspace Model

`mi-lsp` supports two canonical workspace shapes:
- `single`: one repo with one obvious semantic root
- `container`: one parent folder that contains many independent repos without requiring a parent `.sln`

Recommended operating pattern:
- use the parent folder for broad discovery: `ask`, `find`, `search`, `overview`, `workspace-map`
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
mi-lsp init [path] [--name <alias>] [--no-index]
mi-lsp workspace add|remove|scan|list|warm|status
mi-lsp nav ask|pack|symbols|find|refs|overview|outline|service|search|context|deps|multi-read|batch|related|workspace-map|diff-context
mi-lsp index [path] [--clean]
mi-lsp info
mi-lsp daemon start|stop|restart|status|logs
mi-lsp worker install|status
mi-lsp admin open|status|export
```

Useful global flags:

```text
--workspace
--axi
--classic
--full
--format compact|json|text
--client-name
--session-id
--backend roslyn|tsserver|catalog|text
--no-auto-daemon
--compress
```

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

For release-like local validation on a specific RID:

```powershell
pwsh ./scripts/release/build-dist.ps1 -Rids @('win-x64') -Clean
pwsh ./scripts/release/install-local.ps1 -Rid win-x64 -InstallDir $HOME\bin
```

## Troubleshooting

Common first checks:

```powershell
mi-lsp info
mi-lsp worker status --format compact
mi-lsp workspace status myapp --format compact
mi-lsp nav ask "how is this workspace organized?" --workspace myapp
```

If a repo changed heavily under `.docs/wiki`, rerun:

```powershell
mi-lsp index --workspace myapp --clean
```

If a command fails before `mi-lsp` itself starts, treat it as a host incident first.
See the public runbook in [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Current v0.1.0 Scope

- Global daemon with governance UI and local telemetry
- Repo-local lightweight catalog in SQLite with repo ownership and docs graph
- Semantic C# queries via Roslyn worker
- Container workspaces with explicit or inferred repo/entrypoint routing
- TS/JS discovery index for symbols, routes, and overview
- Optional TS semantic bridge through `tsserver`
- Optional Python semantic bridge through `pyright-langserver`
- Service exploration summaries via `nav service`
- Docs-first repo questions via `nav ask`
- Canonical reading packs via `nav pack`

Out of scope for `v0.1.0`:
- MCP transport
- Semantic editing/refactors
- Automatic semantic fanout across all child repos
- Remote or multi-host daemon sharing
- Authenticated governance UI
- Additional languages beyond the current C#/TS/Python focus
- Strong completeness scoring for services
- Embeddings or remote semantic search services

## Documentation

The versioned documentation canon lives in `.docs/wiki/`.
`README.md` is the public entrypoint; the repo wiki is the source of truth.

Start here:
- [Documentation Governance](.docs/wiki/00_gobierno_documental.md)
- [Functional Scope](.docs/wiki/01_alcance_funcional.md)
- [Architecture](.docs/wiki/02_arquitectura.md)
- [Flow Index](.docs/wiki/03_FL.md)
- [Requirements Index](.docs/wiki/04_RF.md)
- [Data Model](.docs/wiki/05_modelo_datos.md)
- [Test Matrix](.docs/wiki/06_matriz_pruebas_RF.md)
- [Technical Baseline](.docs/wiki/07_baseline_tecnica.md)
- [Physical Data Model](.docs/wiki/08_modelo_fisico_datos.md)
- [Technical Contracts](.docs/wiki/09_contratos_tecnicos.md)
- [Troubleshooting](TROUBLESHOOTING.md)

## License

MIT

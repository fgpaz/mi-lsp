![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)
![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)
![CI](https://github.com/fgpaz/mi-lsp/actions/workflows/test.yml/badge.svg)

> Stop burning agent context on repo discovery.

`mi-lsp` is a local semantic navigation CLI for coding agents and developers who work in large `.NET/C#`, TypeScript, Python, and Go codebases.
It gives Codex, Claude Code, and other terminal-based agents a compact way to understand a repo before they start reading whole files.

Instead of asking an agent to grep, open ten files, summarize them, and then try again, you can ask `mi-lsp` for the repo map, the canonical docs, the exact file slices, or the symbol neighborhood in one command.
The result is fewer round-trips, less pasted code, and output formats built for token budgets.

## Why mi-lsp exists

Agents are good at reasoning once they have the right context. They are expensive when they have to discover that context by trial and error.

`mi-lsp` turns daily repo discovery into small, repeatable shell commands:

- docs-first answers when a repo has `.docs/wiki`
- canonical reading packs for a task before the agent opens files
- `multi-read`, `batch`, and `related` commands to replace repeated full-file reads
- TOON output for large result arrays, typically smaller than JSON
- semantic C# queries through a bundled Roslyn worker, with text/catalog fallbacks elsewhere
- an optional local daemon for warm state, never a required MCP server

## One-command install

Install the CLI and the `mi-lsp` skill for Codex/Claude-style agents:

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.sh | sh
```

Install or update only the CLI:

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.sh | sh
```

The installers download the latest GitHub Release, pick the host RID (`win-x64`, `win-arm64`, `linux-x64`, or `linux-arm64`), verify the SHA256 checksum, install the bundled `workers/<rid>/` layout, and run `mi-lsp version` plus `mi-lsp worker status`.
macOS assets are not published yet, so the shell installer exits with a clear unsupported-OS message on Darwin.

`install-agent` intentionally requires `npx` and installs the skill through `npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y`.
There is no direct folder-copy fallback in that path.

Manual release downloads are still available on the [Releases page](https://github.com/fgpaz/mi-lsp/releases).
If you move only the binary after extracting a release, run `mi-lsp worker install` once so C# semantic queries can find the bundled worker.

## First minute

From any repo:

```powershell
mi-lsp init . --name myapp
mi-lsp nav ask "how is this workspace organized?" --workspace myapp
mi-lsp nav pack "understand how authentication works" --workspace myapp
```

`mi-lsp init` detects the workspace shape, registers an alias, writes `.mi-lsp/project.toml`, and indexes code plus docs by default.
`nav ask` answers from canonical docs first when the repo has them; `nav pack` gives the reading order when you want to inspect the evidence yourself.

## Daily workflows

| You need to... | Run this |
|---|---|
| Orient in a new repo | `mi-lsp nav ask "how is this workspace organized?" --workspace myapp` |
| Get the docs reading order for a task | `mi-lsp nav pack "understand billing retry" --workspace myapp` |
| Find canonical RF/FL/TP/CT/TECH docs | `mi-lsp nav wiki search "billing retry" --workspace myapp --format toon` |
| Search text and see matching code | `mi-lsp nav search "billing retry" --include-content --workspace myapp` |
| Read only useful slices | `mi-lsp nav multi-read file1.cs:1-80 file2.ts:20-80 --workspace myapp --format toon` |
| Understand a symbol neighborhood | `mi-lsp nav related MySymbol --workspace myapp --format toon` |
| Read code around one line | `mi-lsp nav context path/to/file.cs 42 --workspace myapp --format toon` |
| Audit one service path | `mi-lsp nav service src/backend/orders --workspace myapp --format toon` |
| Resume from evidence without opening logs | `mi-lsp nav evidence inventory "release evidence" --workspace myapp --format toon` |
| Map a parent folder with many repos | `mi-lsp nav workspace-map --workspace myapp --axi` |

Use `--full` when a preview asks you to expand detail:

```powershell
mi-lsp nav search "billing retry" --include-content --workspace myapp --full
mi-lsp nav workspace-map --workspace myapp --axi --full
```

For container workspaces, start broad and then narrow with `--repo`, `--entrypoint`, `--solution`, or `--project`:

```powershell
mi-lsp nav workspace-map --workspace myapp --format toon
mi-lsp nav search "forgot password" --workspace myapp --repo web --format toon
mi-lsp nav refs IOrderRepository --workspace myapp --repo Orders.Api --format toon
```

## Docs-First Search

If the repo has `.docs/wiki`, `mi-lsp nav ask` uses it as the primary source of truth.
The project can optionally add `.docs/wiki/_mi-lsp/read-model.toml` to teach `mi-lsp` how to rank:
- functional docs (`01-06`)
- technical docs (`07-09`)
- UX/UI docs (`10-16`)
- generic fallback docs (`README*`, `docs/`, `.docs/`)

That gives you a local, explainable answer instead of a black-box summary.

## Semantic Recall Over Knowledge Wikis

For repositories that have a markdown knowledge wiki but no formal `00_gobierno_documental.md`, use `mi-lsp nav recall` to embed a freeform query and rank wiki sections by semantic similarity.
It works multilingually: a Spanish query will find matching English notes by meaning, not just text.
Offline ⇒ lexical fallback: when embeddings service is unavailable, `recall` degrades gracefully to keyword search.

The feature is gated by optional `[embeddings]` configuration in `.mi-lsp/project.toml`.
A block with both `base_url` and `model` is active by default; set `enabled = false` only when you need an explicit local kill switch:

```toml
[embeddings]
# enabled = false  # optional kill switch; omit for normal active config
provider = "openai"
base_url = "http://localhost:8000/v1"
model = "bge-m3"
dim = 1024
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
profile = "knowledge-wiki"
batch_size = 100
timeout_ms = 30000
```

The API key is populated via `mkey run` and injected as an environment variable (`MI_LSP_EMBEDDINGS_API_KEY`), never committed to the repo.
`tesla bge-m3` is the documented reference endpoint (1024-dim multilingual embeddings).
The `knowledge-wiki` profile auto-detects when no formal governance exists, bypassing the spec-driven gate.
Chunks are stored in repo-local `wiki_chunk_embeddings` table with incremental re-embedding by content hash.
Rerunning `mi-lsp index` can backfill missing vectors even when the document catalog reports no source changes.

## Evidence Inventory For Agent Reentry

Use `mi-lsp nav evidence inventory "<query>" --workspace myapp --format toon` before opening large audit folders or historical prompts.
The preview returns canonical wiki anchors first, then metadata-only summaries for `.docs/auditoria`, `.docs/raw/prompts`, and `.docs/raw/plans`.
It prefers `manifest.yaml`, `verdict.md`, `issues.yaml`, summaries, assertions, and hashes before raw turns, logs, screenshots, or prompt bodies.
Heavy raw evidence is counted with file/byte/token estimates and omitted from content by default.

## Use With Claude Code, Codex, and Skill-Based Agents

The repository ships a ready-to-install skill in [`skills/mi-lsp`](skills/mi-lsp).
The recommended path installs the CLI and registers the skill through the skills CLI:

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.sh | sh
```

That path uses:

```text
npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y
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

To update only the installed skill later:

```powershell
npx skills update mi-lsp -g -y
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
mi-lsp nav ask|pack|symbols|find|refs|overview|outline|service|search|context|deps|multi-read|batch|related|workspace-map|diff-context|recall
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
--format compact|json|text|toon|yaml
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

For AE-managed release distribution across Windows/Linux and ARM64/x64:

```powershell
pwsh ./scripts/release/ae-release-binaries.ps1 -Clean
pwsh ./scripts/release/ae-release-binaries.ps1 -Clean -MirrorRoot C:\repos\buho\assets\skills\mi-lsp
pwsh ./scripts/release/ae-release-binaries.ps1 -Clean -Publish -Tag vX.Y.Z
```

The `-Publish` mode requires a clean worktree and a tag that points at `HEAD`; pushing the tag triggers the GitHub release workflow that uploads all platform assets.

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

## Current Scope

- Global daemon with governance UI and local telemetry
- Repo-local lightweight catalog in SQLite with repo ownership and docs graph
- Semantic C# queries via Roslyn worker
- Container workspaces with explicit or inferred repo/entrypoint routing
- TS/JS discovery index for symbols, routes, and overview
- Optional TS semantic bridge through `tsserver`
- Optional Python semantic bridge through `pyright-langserver`
- Optional Go semantic enrichment through `gopls`
- Service exploration summaries via `nav service`
- Docs-first repo questions via `nav ask`
- Canonical reading packs via `nav pack`
- Semantic recall over knowledge wikis via optional embeddings
- Evidence inventory for low-token agent reentry

Out of scope:
- MCP transport
- Semantic editing/refactors
- Automatic semantic fanout across all child repos
- Remote or multi-host daemon sharing
- Authenticated governance UI
- Additional languages beyond the current C#/TS/Python/Go catalog focus
- Strong completeness scoring for services

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

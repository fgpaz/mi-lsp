# mi-lsp

[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)
[![CI](https://github.com/fgpaz/mi-lsp/actions/workflows/test.yml/badge.svg)](https://github.com/fgpaz/mi-lsp/actions/workflows/test.yml)

**A local CLI that answers "where is X" and "what does the spec say" without an agent burning its context window to find out.**

Coding agents (and developers) waste tokens the same way over and over: grep, open a whole file, skim it, grep again, still unsure what's canonical. `mi-lsp` replaces that loop with a repo-local index and an optional shared daemon. You ask a goal in plain language, and it returns exact file ranges, symbol relationships, or the canonical wiki doc — never a raw file dump. When a repository defines governed documentation (`.docs/wiki` or similar), `mi-lsp` treats it as authority before falling back to code search. It works from a terminal, from Claude Code or Codex through a skill, or through the optional `mi-lsp mcp` door — the CLI itself is always the authority, with or without any of those doors.

**Who it's for:** developers navigating an unfamiliar or large repository, and coding agents that need reliable, bounded, token-cheap navigation instead of a heavyweight always-on MCP server.

[Quickstart](#quickstart-in-5-minutes) · [Core concepts](#core-concepts) · [Which command do I use?](#which-command-do-i-use) · [Releases](https://github.com/fgpaz/mi-lsp/releases)

## Quickstart in 5 minutes

**1. Install.** The recommended installer adds the CLI and the `mi-lsp` skill for Claude Code and Codex. It requires `npx`.

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install-agent.sh | sh
```

Want only the CLI (no skill)? Use `install.ps1` / `install.sh` instead:

```powershell
irm https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.ps1 | iex
```

```bash
curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.sh | sh
```

The installer picks the right bundle for `win-x64`, `win-arm64`, `linux-x64`, `linux-arm64`, `darwin-x64`, or `darwin-arm64`, verifies its SHA256 checksum, and keeps the bundled worker beside the CLI.

**2. Initialize your repo.** This detects the workspace, registers an alias, and starts indexing.

```powershell
mi-lsp init . --name myapp
```

For very large workspaces, indexing can smart-sync degrade to the background on its own (or you can force it with `--background`); poll it with `mi-lsp index status <job-id>` before your first query. Use `--wait` to force a full inline sync instead.

**3. Ask for what you actually want, in plain language.** `nav intent` is the goal-shaped entry point — it classifies your question and routes to code or to canonical docs automatically:

```powershell
mi-lsp nav intent "how does the mcp door work" --workspace myapp --format toon
```

A real run against this repository returns a bounded preview with the matching docs, a score, and — critically — a `next_queries` list telling you exactly what to run next instead of leaving you to guess:

```
backend: intent
items[5]:
  - doc_id: RF-GPH-010
    doc_path: .docs/wiki/04_RF/RF-GPH-010.md
    next_queries[2]: mi-lsp nav search RF-GPH-010 --include-content --workspace myapp,"mi-lsp nav multi-read .docs/wiki/04_RF/RF-GPH-010.md:1-120 --workspace myapp"
    score: 108
    title: RF-GPH-010 - ...
mode: docs
next_hint: rerun with --full for expanded detail
ok: true
workspace: myapp
```

Follow the emitted `next_queries` / `continuation.next` verbatim rather than improvising a broader search.

**4. Read only the exact ranges you need**, never a whole file:

```powershell
mi-lsp nav search "billing retry" --include-content --workspace myapp --format toon
mi-lsp nav multi-read src/billing/retry.go:20-80 tests/billing/retry_test.go:10-55 --workspace myapp --format toon
```

![Illustrative terminal demo: initialize a repository, search for billing retry, and read two exact file ranges](docs/assets/readme/daily-flow-demo.gif)

The query and paths above are illustrative; use whatever your own search returns. `multi-read` batches several file ranges into one call — no separate "open file" round-trip per result.

## Core concepts

| Concept | What it means |
|---|---|
| **Workspace** | A repo (or a parent folder holding several independent repos — a "container" workspace) registered under an alias. `mi-lsp workspace which` reports the resolved alias, root, and the executable in use, without mutating anything. |
| **Index** | A repo-local SQLite catalog (`.mi-lsp/index.db`) of symbols, files, and the wiki graph. Built by `init`/`index`; queried directly without a daemon. |
| **Daemon (optional)** | A per-user background process that keeps semantic workers (Roslyn, `tsserver`, `gopls`, `pyright`) warm across terminals and agents for lower-latency queries. If it is down or stale, cheap reads (`search`, `multi-read`, `wiki search`) still work directly against the index; deeper semantic surfaces fall back visibly instead of failing silently. |
| **Wiki / canon navigation** | `nav wiki *` treats a repo's canonical documentation (RS/RF/FL/TP/CT/TECH/DB) as authority. `nav ask`/`nav pack`/`nav route` rank canonical docs first and use code only as supporting evidence. |
| **TOON format and budgets** | `--format toon` is the recommended default output format — smaller than JSON, easy to parse (`key: value` scalars, `key[N]{cols}:` arrays). `--token-budget` and `--max-chars` (global; `0` = unset) bound how much comes back; truncation always leaves `continuation.next` and a truncation marker instead of silently dropping content. |
| **Continuation** | Most envelopes carry a `continuation` block. Prefer `continuation.next` over improvising a wider query. |
| **The four fallback reason codes** | A terminal external failure carries exactly one `reason_code` — `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, or `explicit_incomplete` — plus a separate, sanitized `detail`. A timeout, silence, or an unlabeled "done" is **not** one of these and is not a license to switch to `rg`/`grep` silently. |

## Which command do I use?

| I want to... | Run |
|---|---|
| Understand a goal without knowing the exact command | `mi-lsp nav intent "<goal>" --workspace <alias> --format toon` |
| Find a symbol by name | `mi-lsp nav find <symbol> --workspace <alias> --format toon` |
| Search text and see matching code inline | `mi-lsp nav search "<pattern>" --include-content --workspace <alias> --format toon` |
| Read one or more exact file ranges | `mi-lsp nav multi-read file1:10-60 file2:1-40 --workspace <alias> --format toon` |
| See what a change affects (callers, tests, docs) | `mi-lsp nav affected <path> --include-tests --include-docs --workspace <alias> --format toon` |
| Understand one symbol fully (definition, callers, implementors, tests) | `mi-lsp nav related <symbol> --workspace <alias> --format toon` |
| Navigate the wiki / governance docs (RS/RF/FL/TP/CT/TECH/DB) | `mi-lsp nav wiki search "<topic>" --layer RF,FL,TP,CT --workspace <alias> --format toon` |
| Get the cheapest canonical anchor doc (no index needed) | `mi-lsp nav route "<task>" --workspace <alias> --format toon` |

## Using it from agents and hosts

The CLI is the authority — every door below is a thin, optional layer over the same `nav` commands, and none of them add their own cache, index, or background worker.

- **Agent skill.** Installing the skill (via `install-agent`) gives Claude Code and Codex a `mi-lsp` skill describing the command surface; see [`skills/mi-lsp/SKILL.md`](skills/mi-lsp/SKILL.md) and the longer [quickstart reference](skills/mi-lsp/references/quickstart.md).
- **`mi-lsp mcp` (optional).** A stdio JSON-RPC 2.0 door (`mi-lsp mcp --help`) that execs the same binary and exposes 13 `nav_*` tools (`nav_intent`, `nav_route`, `nav_pack`, `nav_wiki`, `nav_search`, `nav_find`, `nav_refs`, `nav_related`, `nav_flow_slice`, `nav_change_pack`, `nav_affected`, `nav_multi_read`, `nav_overview`). It holds no cache and opens no index of its own; if it fails, run `mi-lsp` directly and get the same result. See the [MCP door decision](.docs/wiki/07_tech/TECH-MCP-ADAPTER.md).
- **Per-host integrations under [`integrations/`](integrations/)** (each optional, none required):
  - **Claude Code** ([`integrations/claude-code`](integrations/claude-code)) — a plugin with the `mi-lsp mcp` server plus advisory `UserPromptSubmit`/`PostToolUse` hooks that call `mi-lsp nav suggest`. Hooks fail open: a missing binary, timeout, or bad JSON just means no suggestion, never a blocked turn.
  - **Pi** ([`integrations/pi`](integrations/pi)), **Codex** ([`integrations/codex`](integrations/codex)), **Cursor** ([`integrations/cursor`](integrations/cursor)) — MCP server registration only (`command: mi-lsp`, `args: ["mcp"]`).
  - **Grok** ([`integrations/grok`](integrations/grok)) — a validated plugin with an MCP entry; install with `grok plugin install integrations/grok --trust`.
- **`MI_LSP_BIN`.** When set, it must point at the real executable — `mi-lsp.exe` on Windows, `mi-lsp` elsewhere. A `.cmd`/`.bat` shim is not the product and is not a valid override; hosts that `spawn` without a shell (including the Claude Code hooks) cannot execute a shim at all.

## Performance and footprint

Measured on 2026-09-24 against this repository; see [`.docs/wiki/07_tech/TECH-MCP-ADAPTER.md`](.docs/wiki/07_tech/TECH-MCP-ADAPTER.md) for the full note and caveats.

| Surface | Measured | Caveat |
|---|---|---|
| `mi-lsp mcp` working set | ~13.7 MiB RSS after `initialize` + `tools/list` (no `tools/call`, no index opened) | Budget target is ≤20 MB/session; a `tools/call` can still buffer up to 4 MB stdout / 4 MB stderr in the child, which was not measured here. |
| Warm `nav overview` (daemon already warm) | p50 ≈ 232 ms, p95 ≈ 374 ms, over 20 timed calls | **Not strictly comparable** to an older baseline (p50 ≈ 4.7 s) from a different client binary against the same long-running daemon; the payload size of that baseline was never archived, so the comparison is directional at best. |

These are workflow numbers on one sample, not a universal benchmark — results depend on your repository, query, and daemon warm-state.

## Troubleshooting

- **`daemon status` warns the executable is stale** (its hash doesn't match this CLI build). Run `mi-lsp daemon restart` before trusting daemon-backed results; direct/cheap reads keep working regardless.
- **A large first index returns a `job_id`.** Use `mi-lsp init --background` to return immediately, then poll with `mi-lsp index status <job-id>` before querying that workspace.
- **"workspace not found" / ambiguous workspace.** Run `mi-lsp workspace which --format toon` to see the resolved alias, root, and executable without mutating anything; `mi-lsp workspace list --group-by-root --format toon` and `mi-lsp workspace doctor --format toon` diagnose duplicate roots and stale paths.
- **Windows `.cmd` shim.** If `mi-lsp` on `PATH` resolves to a `.cmd`/`.bat` shim instead of the real `mi-lsp.exe`, host doors that `spawn` without a shell (MCP integrations, Claude Code hooks) will fail to launch it. Point `MI_LSP_BIN`, or the host's `command` field, at the real executable.

More detail: [`TROUBLESHOOTING.md`](TROUBLESHOOTING.md).

## Learn more

- [Agent skill and command guide](skills/mi-lsp/SKILL.md)
- [Quickstart reference](skills/mi-lsp/references/quickstart.md)
- [Compound command recipes](skills/mi-lsp/references/compound-commands.md)
- [Host integrations](integrations/)
- [MCP door decision](.docs/wiki/07_tech/TECH-MCP-ADAPTER.md)
- [Functional scope](.docs/wiki/01_alcance_funcional.md)
- [Technical baseline](.docs/wiki/07_baseline_tecnica.md)
- [CLI and protocol contracts](.docs/wiki/09_contratos_tecnicos.md)
- [Release distribution](.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## Compatibility and limits

Public release bundles cover Windows (`win-x64`, `win-arm64`), Linux (`linux-x64`, `linux-arm64`), and macOS (`darwin-x64`, `darwin-arm64`). Source builds are documented in [CONTRIBUTING.md](CONTRIBUTING.md).

- C# gets the deepest semantics, through a bundled Roslyn worker. TypeScript/JavaScript and Python use local catalogs with optional `tsserver`/`pyright-langserver` enrichment. Go uses a native AST catalog with optional `gopls` enrichment — not Roslyn-level semantics.
- No semantic editing or automated refactoring, no remote or multi-host daemon sharing, no authenticated remote governance UI.
- `mi-lsp mcp` and every door under `integrations/` are optional; the CLI is never a second product waiting behind them.

## License

[MIT](LICENSE)

<!--
harness_protocol: SDD-HARNESS-v1
id: README
kind: public-entrypoint
audience: public-human
imports:
  - .docs/wiki/01_alcance_funcional.md
  - .docs/wiki/07_baseline_tecnica.md
  - .docs/wiki/09_contratos_tecnicos.md
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - .docs/wiki/07_tech/TECH-MCP-ADAPTER.md
exports:
  - public onboarding narrative
  - public install commands
  - agent workflow examples
agent_must_read:
  - README.md
agent_may_edit:
  - README.md
  - docs/assets/readme/**
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - .mi-lsp/**
verify:
  - mi-lsp workspace status . --format toon
  - mi-lsp nav governance --workspace . --format toon
  - mi-lsp nav wiki validate-harness --workspace . --format toon
  - mi-lsp nav wiki validate-source --workspace . --format toon
  - validate README local links and asset paths
stop_if:
  - README leads with implementation details before user benefit
  - README makes unsupported benchmark or platform claims
  - public install commands drift from scripts/install paths
  - linked README assets are missing
  - README links to a path excluded by .gitignore
evidence:
  - .docs/auditoria/2026-07-16-readme-redesign/
  - .docs/raw/reportes/2026-09-24-native-plugin-surface.md
-->

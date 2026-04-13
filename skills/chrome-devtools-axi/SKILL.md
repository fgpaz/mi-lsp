---
name: chrome-devtools-axi
description: Use when browser work should prefer chrome-devtools-axi for navigation, screenshots, form interaction, console or network inspection, DevTools debugging, or shell-native browser automation with TOON output and contextual hints.
allowed-tools: Bash(npx -y chrome-devtools-axi:*), Bash(chrome-devtools-axi:*)
---

# Browser Automation with chrome-devtools-axi

`chrome-devtools-axi` wraps `chrome-devtools-mcp` with an AXI-style CLI.
Its strengths are TOON-friendly output, contextual `help[N]` hints, a no-args home view, and DevTools-oriented debugging flows that stay shell-native.

Use this skill first for general browser work when shell execution is acceptable and DevTools visibility matters.
Prefer it over `agent-browser` or `browser-use` when the task benefits from accessibility snapshots, console or network inspection, Lighthouse, or performance tracing from the same CLI.

## Bootstrap

Start with the `npx` entrypoint unless the CLI is already installed:

```bash
npx -y chrome-devtools-axi --help
npx -y chrome-devtools-axi
npx -y chrome-devtools-axi open https://example.com
```

- Once the package is installed or cached locally, `chrome-devtools-axi ...` is fine.
- On Windows PowerShell, quote ref arguments such as `"@1"` or `"@1_3"` so the shell does not mangle them.
- If `npx` is blocked by network policy, request permission or install the package once before continuing.

## First-Run Side Effects

On supported agents, the upstream CLI may auto-install session hooks unless `CHROME_DEVTOOLS_AXI_DISABLE_HOOKS=1` is set before the first run.
Expect possible updates to:

- `~/.claude/settings.json`
- `~/.codex/hooks.json`
- `~/.codex/config.toml`

If the user wants zero global config changes, set the environment variable before the first run:

```powershell
$env:CHROME_DEVTOOLS_AXI_DISABLE_HOOKS='1'
```

```bash
export CHROME_DEVTOOLS_AXI_DISABLE_HOOKS=1
```

## Core Workflow

1. Open a page with `open <url>`.
2. Read `page`, `snapshot`, and any emitted `help[N]` hints before improvising.
3. Reuse the emitted refs with `click`, `fill`, `type`, `press`, `hover`, `drag`, `dialog`, or `upload`.
4. Re-run `snapshot` after navigation or major DOM changes.
5. Switch to DevTools diagnostics with `console`, `network`, `lighthouse`, `perf-start`, `perf-stop`, `perf-insight`, or `heap` when the problem is broader than a single click path.

## Essential Commands

```bash
# Home view / current browser state
npx -y chrome-devtools-axi

# Navigation and page state
npx -y chrome-devtools-axi open https://example.com
npx -y chrome-devtools-axi snapshot
npx -y chrome-devtools-axi screenshot page.png --full-page
npx -y chrome-devtools-axi back
npx -y chrome-devtools-axi wait 2000
npx -y chrome-devtools-axi wait "Example Domain"

# Interaction
npx -y chrome-devtools-axi click "@1"
npx -y chrome-devtools-axi fill "@2" "user@example.com"
npx -y chrome-devtools-axi type "hello"
npx -y chrome-devtools-axi press Enter
npx -y chrome-devtools-axi hover "@3"
npx -y chrome-devtools-axi drag "@4" "@5"

# Page and tab management
npx -y chrome-devtools-axi pages
npx -y chrome-devtools-axi newpage https://example.com
npx -y chrome-devtools-axi selectpage 2
npx -y chrome-devtools-axi closepage 2
npx -y chrome-devtools-axi resize 1440 900

# DevTools diagnostics
npx -y chrome-devtools-axi console
npx -y chrome-devtools-axi network
npx -y chrome-devtools-axi lighthouse
npx -y chrome-devtools-axi perf-start
npx -y chrome-devtools-axi perf-stop
npx -y chrome-devtools-axi heap heap.heapsnapshot

# Scripted / advanced
npx -y chrome-devtools-axi eval "document.title"
npx -y chrome-devtools-axi run < script.txt
```

Use `--full` when the default output truncates useful details.

## Preferred Usage Patterns

- Use the no-args command to inspect current browser state before guessing the next step.
- Follow emitted `help[N]` suggestions when the tool already tells you the likely next action.
- Use `open` plus a follow-up `snapshot` when you need a fresh, explicit page state after navigation.
- Use `console` and `network` for debugging instead of trying to infer everything from DOM state alone.
- Use `lighthouse` and the `perf-*` commands when the task is about UX regressions, rendering, or runtime performance.

## When To Prefer This Skill

- The user wants browser automation from the shell rather than an MCP browser tool.
- The task mixes interaction and diagnostics.
- Compact output and next-step hints are more valuable than a large generic command surface.
- A persistent bridge across commands is useful so Chrome and the MCP session stay warm.

## When Not To Prefer It

- The environment already exposes direct Playwright browser tools and those are faster for the exact task.
- The user explicitly asked for `agent-browser`, `browser-use`, or another browser stack.
- The task is pure DOM automation with no need for DevTools data, AXI-style hints, or the home view.

## Operational Notes

- The bridge state lives under `~/.chrome-devtools-axi/`.
- The default bridge port is `9224`; override with `CHROME_DEVTOOLS_AXI_PORT`.
- Running with no command shows the home view instead of generic help.
- The tool is designed to keep stdout structured and agent-readable; treat stderr as diagnostics only.

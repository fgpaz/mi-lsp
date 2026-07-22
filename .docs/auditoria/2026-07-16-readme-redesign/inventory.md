# README redesign inventory

## Scope

Read-only classification of the public README before rewriting it. The worker audit was independently checked against the main workspace because its governance diagnosis was incorrect: `.docs/wiki/00_gobierno_documental.md` exists and the main workspace reports `governance_blocked=false`.

## Keep

- Project name, MIT/Go/CI badges.
- Public PowerShell and shell installers.
- The claim that no MCP server is required.
- The optional-daemon model.
- Docs-first navigation as the main differentiator.
- Links to CONTRIBUTING, SECURITY, TROUBLESHOOTING, wiki canon, and license.
- The README harness contract.

## Rewrite

- Hero: energetic problem statement followed by a literal product description.
- Installation: CLI + skill first; CLI-only second; mention the `npx` prerequisite only where relevant.
- Demo: use `init -> nav search --include-content -> nav multi-read`.
- Benefits: replace broad comparison tables with three concrete outcomes.
- How it works: summarize local index, optional daemon, semantic backends, and visible fallbacks.
- Compatibility: state the six published Windows, Linux, and Darwin release RIDs.
- Documentation: reduce to a small progressive-disclosure link set.

## Move

- Full embeddings configuration -> `.docs/wiki/09_contratos/CT-NAV-RECALL.md`.
- Evidence inventory details -> `.docs/wiki/09_contratos/CT-NAV-EVIDENCE.md`.
- Runtime and daemon internals -> `.docs/wiki/07_baseline_tecnica.md`.
- Full CLI and envelope reference -> `.docs/wiki/09_contratos_tecnicos.md`.
- Build and contribution procedures -> `CONTRIBUTING.md`.
- Release distribution details -> `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`.
- Operational diagnosis -> `TROUBLESHOOTING.md`.
- Agent recipes -> `skills/mi-lsp/SKILL.md` and `skills/mi-lsp/references/*.md`.

## Delete from the public narrative

- Repeated install commands.
- Exhaustive command and flag inventories.
- The unverified `30-second` label.
- The `20-40% fewer tokens` marketing comparison from the main flow.
- Detailed AXI, attribution, telemetry, and release instructions.
- Unsupported platform claims. Live release evidence confirms `win-*`, `linux-*`, and `darwin-*` x64/arm64 assets.

## Acceptance oracle

- README has 150-220 physical lines, including its harness comment.
- Section order is Hero -> Install -> Demo -> Why -> Docs-first -> How it works -> Compatibility/limits -> Learn more -> License.
- Demo and visual asset use `init`, `nav search --include-content`, and `nav multi-read`.
- All local links and assets exist.
- Install commands match `scripts/install/install-agent.*` and `scripts/install/install.*`.
- Platform claims match the six assets published by the current release distribution.
- No unsupported benchmark claims remain.
- No changes touch `internal/**` or other forbidden paths.

# Branch Hygiene Evidence

```yaml
harness_protocol: SDD-HARNESS-v1
id: "BRANCH-HYGIENE-2026-06-27"
kind: "historical-branch-evidence"
audience: "human"
imports:
  - '[[AE-EVIDENCE-POLICY]]'
  - '[[AE-PHASES]]'
exports:
  - 'BRANCH-HYGIENE-2026-06-27'
agent_must_read:
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/branch-hygiene.md
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/session-contract.yaml
agent_may_edit:
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/branch-hygiene-annotations.md
agent_must_not_edit:
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/branch-hygiene.md
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki validate-harness --workspace <alias> --format toon
  - compare the historical branch inventory with its session and closure evidence
stop_if:
  - historical branch state is treated as current repository state
  - the historical branch evidence would be rewritten instead of annotated separately
  - supporting session or closure evidence is missing
evidence:
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/branch-hygiene.md
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/audit-manifest.yaml
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/evidence-index.yaml
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/session-contract.yaml
  - .docs/auditoria/2026-06-27-mi-lsp-rerank-extension/traceability-closure.yaml
```

## Preflight

- Started on `v055/macos-binaries` at `5b5793ba48d72e04ed4d02642d17e2a8983a5e72`, clean worktree, ahead of `origin/v055/macos-binaries` by one commit.
- Ran `git fetch --prune origin`; `origin/main` advanced from `4df94bd` to `46fec761aae550afaf3981bc5a3c4e9a4d874655`.
- Switched to `main` and ran `git pull --ff-only origin main`; local `main` now equals `origin/main` (`0 0` ahead/behind).
- Created `codex/mi-lsp-rerank-extension` from `46fec761aae550afaf3981bc5a3c4e9a4d874655`.

## Cleanup Decisions

- Deleted local `codex/wsl-execution-deep-audit` after `git merge-base --is-ancestor codex/wsl-execution-deep-audit main` returned success.
- Deleted local `codex/mi-cowork-v1-w25-mi-lsp-mjs-full-index-repair` after `git cherry v055/macos-binaries codex/mi-cowork-v1-w25-mi-lsp-mjs-full-index-repair` and the inverse both reported patch-equivalent commits with `-`.
- Preserved `v055/macos-binaries`; it remains local and ahead of `origin/v055/macos-binaries` by one commit.
- No remote branches were deleted.

## Post-cleanup Inventory

- Local branches: `main`, `v055/macos-binaries`, `codex/mi-lsp-rerank-extension`.
- Worktrees: only `C:/repos/mios/mi-lsp`, attached to `codex/mi-lsp-rerank-extension`.

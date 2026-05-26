---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: writing-plans
anchors:
  fl: [FL-QRY-01]
  rf: [RF-QRY-018]
  ct: [CT-NAV-EDIT-PLAN]
allowed_paths:
  - .docs/raw/plans/2026-05-26-mi-lsp-nav-edit-plan.md
  - .docs/raw/plans/2026-05-26-mi-lsp-nav-edit-plan/**
  - .docs/auditoria/2026-05-26-mi-lsp-nav-edit-plan/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - git status --short --branch
stop_if:
  - governance_blocked=true
secret_scan:
  required: true
---

# T0 - Plan, Session Contract, Decision Lock

Create the main plan, task packet files, and `.docs/auditoria/2026-05-26-mi-lsp-nav-edit-plan/session-contract.yaml`.

Decision lock:
- selected mode: `orquestado_deterministico`
- branch: `codex/mi-lsp-nav-edit-plan`
- worktree: `C:\repos\mios\mi-lsp-nav-edit-plan`
- branch disposition: `integrate-main`
- cleanup: `auto-after-successful-integration`
- no external issue/card linked
- release gate required because public CLI and installable binary behavior change.

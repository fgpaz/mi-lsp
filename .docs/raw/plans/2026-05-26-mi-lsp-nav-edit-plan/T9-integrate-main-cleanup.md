---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: ae-orquestador
anchors:
  rf: [RF-QRY-018]
allowed_paths:
  - .docs/auditoria/2026-05-26-mi-lsp-nav-edit-plan/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - git status --short --branch
  - mi-lsp workspace hygiene --format toon
secret_scan:
  required: true
---

# T9 - Integration And Cleanup

Open PR, wait for checks, merge through PR flow, update local `main`, refresh local binary if needed, reindex if docs changed, run workspace hygiene, remove task worktree, delete local and remote task branches, and confirm clean `main`.

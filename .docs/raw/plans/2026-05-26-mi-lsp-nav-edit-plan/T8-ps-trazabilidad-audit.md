---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: ps-trazabilidad
anchors:
  rf: [RF-QRY-018]
  ct: [CT-NAV-EDIT-PLAN]
allowed_paths:
  - .docs/auditoria/2026-05-26-mi-lsp-nav-edit-plan/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
secret_scan:
  required: true
---

# T8 - Traceability And Audit

Run `ps-trazabilidad`, then `ps-auditar-trazabilidad`. If either detects drift, fix the drift and rerun both before closing.

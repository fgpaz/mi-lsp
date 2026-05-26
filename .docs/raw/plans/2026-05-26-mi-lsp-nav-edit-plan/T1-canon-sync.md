---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: mi-lsp
anchors:
  fl: [FL-QRY-01]
  rf: [RF-QRY-018]
  ct: [CT-NAV-EDIT-PLAN]
allowed_paths:
  - .docs/wiki/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
secret_scan:
  required: true
---

# T1 - Canon Sync

Create `RF-QRY-018` and `CT-NAV-EDIT-PLAN`. Update `04_RF.md`, `TP-QRY`, `06_matriz_pruebas_RF.md`, `07_baseline_tecnica.md`, and `09_contratos_tecnicos.md`.

Required canon decisions:
- `nav edit-plan` is dry-run by default.
- apply requires `--apply --experimental-apply`.
- no AST in this wave.
- no writes to `.docs/wiki/_mi-lsp/read-model.toml`, `.git/**`, `.mi-lsp/**`, binaries, env or secret files.

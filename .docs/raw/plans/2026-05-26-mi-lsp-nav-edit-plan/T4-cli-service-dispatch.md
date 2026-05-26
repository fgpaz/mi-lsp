---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: mi-lsp
anchors:
  rf: [RF-QRY-018]
  ct: [CT-NAV-EDIT-PLAN]
allowed_paths:
  - internal/cli/**
  - internal/service/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test ./internal/cli ./internal/service
stop_if:
  - command depends on daemon health
secret_scan:
  required: true
---

# T4 - CLI And Service Dispatch

Add `nav edit-plan` and internal operation `nav.edit-plan`. Support `--stdin`, `--packet`, `--strict`, `--include-content`, `--apply`, and `--experimental-apply`. The operation must resolve the workspace directly and must not auto-route through daemon.

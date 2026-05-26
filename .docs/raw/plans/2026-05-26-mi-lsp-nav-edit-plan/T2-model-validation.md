---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: mi-lsp
anchors:
  rf: [RF-QRY-018]
allowed_paths:
  - internal/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test ./internal/model ./internal/service
stop_if:
  - packet schema requires AST edits
secret_scan:
  required: true
---

# T2 - Model And Validation

Add `edit-plan-v1` request/result structs and validation. The validator must reject invalid versions, duplicate IDs, missing targets, invalid ranges, missing hashes for apply, path traversal, symlinks outside the workspace, binary files, denied paths, overlapping operations, regex without max replacements, and multiline regex.

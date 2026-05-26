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
  - go test ./internal/service ./internal/cli ./internal/model
secret_scan:
  required: true
---

# T6 - Tests And Fixtures

Add service and CLI tests for packet parsing, dry-run no-write, apply success in a temp git repo, dirty git apply rejection, rollback, path safety, read-model block, binary block, hash mismatch, max replacements, regex limits, overlapping ranges, and invalid packets.

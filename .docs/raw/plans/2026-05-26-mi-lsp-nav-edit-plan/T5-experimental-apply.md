---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: mi-lsp
anchors:
  rf: [RF-QRY-018]
allowed_paths:
  - internal/service/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test ./internal/service -run EditPlan
stop_if:
  - apply cannot rollback touched files
secret_scan:
  required: true
---

# T5 - Experimental Apply Gate

Implement apply only for `--apply --experimental-apply`, clean git tree, expected hashes, safe paths, no overlapping operations, and revalidation before writes. Use temp-file replacement per file. On failure, restore previous bytes for files already touched and return rollback evidence.

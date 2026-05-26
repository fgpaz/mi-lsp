---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: mi-lsp
anchors:
  rf: [RF-QRY-018]
allowed_paths:
  - internal/service/**
  - internal/model/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - go test ./internal/service -run EditPlan
stop_if:
  - dry-run writes any file
secret_scan:
  required: true
---

# T3 - Dry-Run Diff Engine

Apply operations in memory only, preserve per-file line endings, generate deterministic unified diff, emit evidence and guardrails, and never write files in dry-run.

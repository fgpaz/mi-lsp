---
doc_id: TECH-SEMANTIC-PREPARATION
title: Semantic preparation evidence
layer: TECH
family: navigation
status: implemented
---

# TECH-SEMANTIC-PREPARATION

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-SEMANTIC-PREPARATION"
kind: "technical-detail"
audience: "llm-first"
imports:
  - '[[CT-NAV-EDIT-PLAN]]'
exports:
  - 'TECH-SEMANTIC-PREPARATION'
agent_may_edit:
  - .docs/wiki/07_tech/TECH-SEMANTIC-PREPARATION.md
verify:
  - go test ./...
stop_if:
  - preparation grants mutation authority
  - allowed_paths are inferred from an implicit git diff
```

`nav.prepare` is a single read-only service call for semantic readiness. The
packet is bound to the canonical workspace root, task digest, governance digest,
index generation, and optional plan digest. It calls governance inspection with
auto-sync disabled and reuses the route, pack, and edit-plan validation cores in
process. A supplied plan contributes only validated target paths; explicit
affected paths are normalized and root-bounded. Missing task-specific paths fail
closed with an empty `allowed_paths` list and a warning.

The daemon cache uses all packet identity fields, so changes to governance
sources, the index database generation, task, or plan input cannot reuse an old
packet. Direct fallback uses the same `service.App.prepare` implementation and
therefore preserves the canonical evidence item schema.

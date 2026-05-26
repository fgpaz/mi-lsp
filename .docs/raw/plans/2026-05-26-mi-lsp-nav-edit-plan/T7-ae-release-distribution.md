---
linear_parent: null
linear_child: null
goal_id: G1
agent_type: ae-orquestador
anchors:
  ct: [CT-NAV-EDIT-PLAN]
allowed_paths:
  - .docs/auditoria/2026-05-26-mi-lsp-nav-edit-plan/**
  - dist/**
forbidden_paths:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - .\scripts\release\ae-release-binaries.ps1 -SkipWslInstall -SkipMirror
secret_scan:
  required: true
---

# T7 - AE Release Distribution

Run the release-distribution gate because CLI behavior changes. Record checksums, installed binary path, skipped WSL/mirror/publish status, and explicit publish waiver.

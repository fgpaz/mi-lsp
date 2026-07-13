# CT-GOVERNANCE-AE-KERNEL-V2

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-GOVERNANCE-AE-KERNEL-V2
id: CT-GOVERNANCE-AE-KERNEL-V2
kind: contract
audience: dual
imports:
  - '[[00_gobierno_documental]]'
  - '.docs/ae/repo-policy.yaml'
  - '<kernel_home>/canon/AE-KERNEL-V2.md'
  - '<kernel_home>/canon/AE-HARNESS-ORCHESTRATION.md'
exports:
  - CT-GOVERNANCE-AE-KERNEL-V2
  - ae_canon
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/ae/repo-policy.yaml
  - .docs/wiki/09_contratos/CT-GOVERNANCE-AE-KERNEL-V2.md
agent_may_edit:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/ae/repo-policy.yaml
  - .docs/wiki/09_contratos/CT-GOVERNANCE-AE-KERNEL-V2.md
agent_must_not_edit:
  - <kernel_home>/canon/**
  - .docs/raw/**
  - .env*
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - ae_canon.status in [missing, mismatch]
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/_mi-lsp/read-model.toml
  - .docs/ae/repo-policy.yaml
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/
```

## Canonical binding

```toon
doc_id: CT-GOVERNANCE-AE-KERNEL-V2
block_id: CT-GOVERNANCE-AE-KERNEL-V2.canonical-binding
kind: normative
source_of_truth: this
normative:
  ae_canon.mode: kernel_v2
  ae_canon.source: <kernel_home>/canon
  repo_policy: .docs/ae/repo-policy.yaml
  required_modules:
    - AE-KERNEL-V2.md
    - AE-PHASES.md
    - AE-HARNESS-ORCHESTRATION.md
    - AE-EVIDENCE-POLICY.md
    - AE-POLICY-PROJECTION.md
  states: [valid, missing, mismatch]
verify:
  - inspect governance status and projection round-trip
stop_if:
  - ae_canon.mode != kernel_v2
  - ae_canon.source != <kernel_home>/canon
  - ae_canon.repo_policy != .docs/ae/repo-policy.yaml
evidence:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/_mi-lsp/read-model.toml
```

## Repository policy slots

```toon
doc_id: CT-GOVERNANCE-AE-KERNEL-V2
block_id: CT-GOVERNANCE-AE-KERNEL-V2.repo-policy-slots
kind: normative
source_of_truth: .docs/ae/repo-policy.yaml
required_slots:
  - repo.name
  - repo.description
  - language.policy_lang
  - language.docs_lang
  - tracker.provider
  - tracker.key_env
  - tracker.conf_file
  - tracker.linear.base_url
  - tracker.linear.workspace
  - secrets.vault
  - secrets.tool
  - wiki.layers_map.02
  - wiki.workspace_alias
  - wiki.paths_file
  - subagents.roster_file
  - tracker.linear.projects[0].key
  - wrappers[]
  - qa.canon_paths[]
  - last_updated
policy:
  no_secrets: true
  no_fixture_config: true
  unset_tracker_values_must_be_explicit: true
verify:
  - parse YAML and validate every required slot is non-empty
stop_if:
  - missing_required_slot
  - secret_material_present
  - fixture_config_used_as_productive_policy
evidence:
  - .docs/ae/repo-policy.yaml
```

## Path and symlink safety

```toon
doc_id: CT-GOVERNANCE-AE-KERNEL-V2
block_id: CT-GOVERNANCE-AE-KERNEL-V2.path-safety
kind: normative
source_of_truth: internal/docgraph/governance.go
rules:
  path_traversal: reject absolute paths, dot paths, and parent escapes
  governed_boundary: source_doc must remain below .docs/wiki/
  symlink: reject symlinked root, directory components, and files
  regular_file: require regular files for canon modules and repo policy
  external_kernel: resolve <kernel_home> from AE_KERNEL_HOME or local AE config without copying canon into repo
states:
  valid: all modules and slots pass checks
  missing: a required module or slot is absent
  mismatch: configuration, path, YAML, or symlink safety is invalid
verify:
  - go test ./internal/docgraph ./internal/service
  - inspect governance status
stop_if:
  - path_traversal_detected
  - symlink_detected
  - invalid_yaml
  - unsafe_external_source
evidence:
  - internal/docgraph/governance.go
  - internal/service/governance_test.go
```

## Projection and closure

```toon
doc_id: CT-GOVERNANCE-AE-KERNEL-V2
block_id: CT-GOVERNANCE-AE-KERNEL-V2.projection-closure
kind: normative
source_of_truth: .docs/wiki/00_gobierno_documental.md
projection: .docs/wiki/_mi-lsp/read-model.toml
required_slots:
  - imports
  - exports
  - verify
  - stop_if
  - evidence
  - doc_id
  - block_id
closure:
  completion_handoff: BLOCKED
  handoff_ready: forbidden
  audit_required: true
verify:
  - projection matches the fenced YAML source
  - audit manifest and traceability closure exist
stop_if:
  - projection_out_of_sync
  - audit_missing
  - completion_handoff != BLOCKED
evidence:
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/session-contract.yaml
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/audit-manifest.yaml
  - .docs/auditoria/2026-07-13-ae-kernel-v2-integration/traceability-closure.yaml
```

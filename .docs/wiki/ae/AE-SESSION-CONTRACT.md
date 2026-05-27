# AE-SESSION-CONTRACT

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-SESSION-CONTRACT"
id: "AE-SESSION-CONTRACT"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-PHASES]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - 'AE-SESSION-CONTRACT'
agent_must_read:
  - .docs/wiki/ae/AE-SESSION-CONTRACT.md
agent_may_edit:
  - .docs/wiki/ae/AE-SESSION-CONTRACT.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/AE-SESSION-CONTRACT.md
```

## Contract Schema

```toon
doc_id: AE-SESSION-CONTRACT
block_id: AE-SESSION-CONTRACT.schema
kind: policy
source_of_truth: this
path: .docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml
required_for:
  - mutating_work
  - non_trivial_work
  - policy_work
  - shared_skill_work
  - harness_work
  - multi_step_work
required_fields:
  - schema
  - task_slug
  - issue_or_waiver
  - base_ref
  - base_sha
  - branch
  - worktree_path
  - ae_contract
  - anchors
  - expected_scope
  - allowed_paths
  - forbidden_paths
  - required_evidence
  - touched_linear_issues
  - ticket_frontier
  - waivers
  - branch_disposition
  - cleanup_policy
ae_contract_required_fields:
  - gateway_skill
  - selected_mode
  - decision_lock
  - harness_adapter
  - orchestration_depth
  - ledgers_required
  - runtime_target
  - stop_before_functional_work
forbidden_path_defaults:
  - .git/**
  - .mi-lsp/**
  - .docs/wiki/_mi-lsp/read-model.toml
  - .env
  - .env.*
  - "**/*.pem"
  - "**/*.key"
  - "**/*.pfx"
  - dist/**
closure_consumers:
  - ps-trazabilidad
  - ps-auditar-trazabilidad
  - scripts/ae/pre-push-guard.ps1
stop_if:
  - contract_missing_for_required_work
  - real_diff_exceeds_allowed_paths
  - forbidden_path_touched_without_explicit_waiver
verify:
  - ./scripts/ae/pre-push-guard.ps1 -SessionContract <path> -AllowDirty
evidence:
  - .docs/wiki/ae/AE-SESSION-CONTRACT.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml
```

## Session Rule

The session contract is the operational scope boundary. Chat memory, branch names, and plan prose do not override it.

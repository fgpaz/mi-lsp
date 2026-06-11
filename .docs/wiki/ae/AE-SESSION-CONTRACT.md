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
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --format toon
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
  - runtime_or_deployable_work
  - independent_axis_work
required_fields:
  - schema
  - task_slug
  - issue_or_waiver
  - base_ref
  - base_sha
  - branch
  - worktree_path
  - mode
  - decision_lock
  - harness_adapter
  - orchestration_depth
  - closure_profile
  - ae_contract
  - worker_decision
  - worker_adapter_available
  - worker_authorized_by_user
  - independent_axes
  - selected_adapter
  - adapter_evidence_or_blocker
  - worker_verdicts
  - mi_lsp_preflight
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
  - worker_decision
  - independent_axes
  - adapter_evidence_or_blocker
  - stop_before_functional_work
mi_lsp_preflight_required_fields:
  - alias
  - root
  - cli_path
  - governance_blocked
  - docs_ready
  - doc_count
  - ae_canon.status
  - ae_canon.roots
  - ae_canon.source
  - ae_canon.blocking
  - client_name
  - session_id
worker_audit_required_fields:
  - worker_session_attribution_matrix
  - admin_export_summary
  - manual_cli_exception_review
  - wsl_read_only_evidence_handling
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
  - mi_lsp_preflight_missing_for_governed_work
  - mi_lsp_preflight.client_name=manual-cli
  - mi_lsp_preflight.session_id matches cli-<pid>
  - mi_lsp_preflight.governance_blocked=true
  - mi_lsp_preflight.docs_ready=false
  - mi_lsp_preflight.doc_count=0
  - mi_lsp_preflight.ae_canon.status in [missing, mismatch, projection_only]
  - real_diff_exceeds_allowed_paths
  - forbidden_path_touched_without_explicit_waiver
  - required_worker_scope_with_worker_decision_none
  - why_no_worker_used_as_authorization
verify:
  - ./scripts/ae/pre-push-guard.ps1 -SessionContract <path> -AllowDirty
evidence:
  - .docs/wiki/ae/AE-SESSION-CONTRACT.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml
```

## Session Rule

The session contract is the operational scope boundary. Chat memory, branch names, and plan prose do not override it.

## Evidence Reentry Rule

```toon
doc_id: AE-SESSION-CONTRACT
block_id: AE-SESSION-CONTRACT.evidence_reentry
kind: policy
source_of_truth: this
optional_pre_closure_check: mi-lsp nav evidence inventory <task-or-evidence-query> --workspace <alias> --format toon
use_when:
  - required_evidence points at .docs/auditoria
  - evidence roots may contain turns logs screenshots or raw prompts
  - context budget is tight and the agent needs manifest-first guidance
record_in_session_contract:
  - required_evidence
  - evidence_loading_profile
  - context_loading_profile
  - why_not_cheaper
stop_if:
  - inventory suggests full_raw without why_not_cheaper
  - raw evidence would be loaded before manifest/verdict/summary paths
verify:
  - mi-lsp nav evidence inventory "<task>" --workspace <alias> --format toon
evidence:
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/evidence-index.yaml
```

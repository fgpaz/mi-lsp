# AE-EVIDENCE-POLICY

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-EVIDENCE-POLICY"
id: "AE-EVIDENCE-POLICY"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-HARNESS-MANIFEST]]'
exports:
  - 'AE-EVIDENCE-POLICY'
agent_must_read:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
agent_may_edit:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
```

## Evidence Contract

```toon
doc_id: AE-EVIDENCE-POLICY
block_id: AE-EVIDENCE-POLICY.contract
kind: policy
source_of_truth: this
durable_evidence_root: .docs/auditoria/<YYYY-MM-DD>-<task-slug>/
scratch_roots:
  - .docs/raw/
  - artifacts/
  - C:/tmp/
release_evidence_required:
  - command_invoked
  - git_revision
  - rids_built
  - cli_sha256_by_rid
  - local_install_paths
  - wsl_install_paths_or_waiver
  - worker_status_result_or_waiver
  - version_output_by_install_path_or_waiver
  - admin_export_summary_for_telemetry_changes_or_waiver
  - safe_degrade_planner_evidence_or_waiver
  - attributed_mi_lsp_preflight
  - publish_tag_or_waiver
  - mirror_sync_or_waiver
  - governance_result
  - tests_result
dirty_binary_policy:
  dirty_source_build: "allowed only for local smoke"
  publish_ready: "requires vcs_modified=false or explicit human waiver"
  release_claim: "requires clean tag and GitHub release upload path"
stop_if:
  - final answer claims published binaries without tag/workflow evidence
  - final answer claims installed parity without version path/hash evidence
  - final answer claims token-savings precision or planner safety without telemetry/guardrail evidence
  - evidence lives only in chat or .docs/raw
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/
```

## Audit Hygiene

```toon
doc_id: AE-EVIDENCE-POLICY
block_id: AE-EVIDENCE-POLICY.audit_hygiene
kind: policy
source_of_truth: this
schema: ae-audit-hygiene/v1
manifest: .docs/auditoria/<YYYY-MM-DD>-<task-slug>/audit-manifest.yaml
retention_ttl_days: 14
hash_algorithm: sha256
required_when:
  - non_trivial_ae_work
  - worker_or_qa_or_runtime_evidence
  - raw_logs_screenshots_transcripts_prompts_or_plans
artifact_classes:
  durable_keep: sanitized durable evidence
  promote_summary: raw artifact must become sanitized-summary.md or evidence-index.yaml
  raw_prune_after_14d: temporary raw evidence pruned after TTL when not promoted
  quarantine_redact: sensitive raw evidence held until redacted
  delete_now: duplicate, accidental, or unsafe artifact with no durable value
  blocked_hold: cleanup blocked with owner, reason, next action, and recheck date
stop_if:
  - raw audit evidence is treated as durable without manifest, summary, sha256, retention decision
  - durable evidence would be deleted without replacement summary, hash, path, date, owner, and reason
verify:
  - mi-lsp nav evidence inventory "AE evidence" --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
```

## Closure Rule

For binary-affecting work, `ps-trazabilidad` must include the AE release evidence or an explicit waiver. `ps-auditar-trazabilidad` is required for cross-OS, release, worker bootstrap, or policy changes.

## Evidence Inventory

```toon
doc_id: AE-EVIDENCE-POLICY
block_id: AE-EVIDENCE-POLICY.inventory
kind: policy
source_of_truth: this
surface: mi-lsp nav evidence inventory <query> --workspace <alias> --format toon
purpose:
  - choose cheapest safe reentry path before reading raw evidence
  - distinguish canonical wiki from operational evidence
inventoriable_evidence:
  manifest: manifest.yaml|manifest.yml
  verdict: verdict.md|verdict.yaml|verdict.yml
  issues: issues.yaml|issues.yml
  assertions: assertions.yaml|assertions.yml|assertions.json|assertions.md
  turns: metadata_only
  logs: metadata_only
  screenshots: metadata_only
  raw_prompts: historical_non_authoritative
  raw_plans: historical_non_authoritative
authority:
  canonical_wiki: source_of_truth_for_decisions
  evidence_not_canon: closure_or_handoff_evidence
recommended_order:
  - route
  - manifest_verdict
  - summary_assertions
  - targeted_raw
  - full_raw
stop_if:
  - evidence lives only in chat
  - raw prompt/log/turn/screenshot content would be emitted by inventory
  - .docs/raw is treated as canonical source
verify:
  - mi-lsp nav evidence inventory "AE evidence" --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/
```

## Pre-Push Guard

```toon
doc_id: AE-EVIDENCE-POLICY
block_id: AE-EVIDENCE-POLICY.pre_push_guard
kind: policy
source_of_truth: this
guard_script: scripts/ae/pre-push-guard.ps1
required_before:
  - push_ready_claim
  - direct_main_push
  - merge_or_cleanup_after_policy_work
requires:
  - session_contract
  - ae_contract
  - allowed_paths
  - forbidden_paths
  - required_evidence
  - cleanup_policy
checks:
  - session_contract_exists
  - branch_not_main_unless_waived
  - dirty_paths_are_committed_or_explicitly_waived
  - mi_lsp_preflight_present
  - mi_lsp_preflight_client_name_not_manual_cli
  - mi_lsp_preflight_session_id_not_default_cli_pid
  - mi_lsp_preflight_governance_blocked_false
  - mi_lsp_preflight_docs_ready_true
  - mi_lsp_preflight_doc_count_greater_than_zero
  - mi_lsp_preflight_ae_canon_status_not_missing_mismatch_or_projection_only
  - no_forbidden_path_drift
  - no_ungoverned_raw_artifacts
stop_if:
  - session_contract_missing
  - uncommitted_dirty_path_not_listed_in_session_contract_waivers
  - mi_lsp_preflight_missing
  - mi_lsp_preflight.client_name=manual-cli
  - mi_lsp_preflight.session_id matches cli-<pid>
  - mi_lsp_preflight.governance_blocked=true
  - mi_lsp_preflight.docs_ready=false
  - mi_lsp_preflight.doc_count=0
  - mi_lsp_preflight.ae_canon.status in [missing, mismatch, projection_only]
  - dirty_worktree_without_explicit_allow_dirty_precommit_mode
  - forbidden_path_changed
  - raw_artifact_changed_without_governed_allowlist
verify:
  - ./scripts/ae/pre-push-guard.ps1 -SessionContract .docs/auditoria/<task>/session-contract.yaml -AllowDirty
evidence:
  - scripts/ae/pre-push-guard.ps1
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/traceability-closure.yaml
```

## WSL And Worker Audit Evidence

```toon
doc_id: AE-EVIDENCE-POLICY
block_id: AE-EVIDENCE-POLICY.wsl_worker_audit
kind: policy
source_of_truth: this
applies_when:
  - WSL execution audit
  - subagent execution audit
  - worker execution audit
required_artifacts:
  - wsl-execution-inventory.yaml
  - telemetry-export-summary.toon
  - manual-cli-attribution-findings.yaml
  - worker-session-attribution-matrix.yaml
required_sources_first:
  - mi-lsp admin export --since <window> --summary --by-client --by-route --by-failure-stage --format toon
  - mi-lsp admin export --since <window> --client-name manual-cli --format toon --limit <n>
  - mi-lsp admin export --since <window> --operation nav.governance --format toon --limit <n>
  - mi-lsp admin export --since <window> --operation workspace.status --format toon --limit <n>
evidence_handling:
  - WSL filesystems are read-only during audit
  - shell histories and worker transcripts are summarized, not dumped
  - secrets are redacted or the evidence source is blocked
  - telemetry rows and transcripts are closure evidence, not canon
stop_if:
  - audit requires mutating WSL state
  - raw history or transcript content would expose secrets
  - worker/session attribution matrix is missing
  - manual-cli governed worker session has no exception or waiver
  - missing governance, doc_count=0, missing attribution, or ae_canon blocking state is treated as warning-only
verify:
  - ./scripts/ae/pre-push-guard.ps1 -SessionContract .docs/auditoria/<task>/session-contract.yaml -AllowDirty
evidence:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/worker-session-attribution-matrix.yaml
```

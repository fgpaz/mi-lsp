# AE-PHASES

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-PHASES"
id: "AE-PHASES"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-HARNESS-MANIFEST]]'
  - '[[AE-SESSION-CONTRACT]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - 'AE-PHASES'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/ae/AE-PHASES.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
agent_may_edit:
  - .docs/wiki/ae/AE-PHASES.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --format toon
  - mi-lsp nav wiki validate-source --workspace <alias> --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/AE-PHASES.md
```

## Phase Contract

```toon
doc_id: AE-PHASES
block_id: AE-PHASES.phase_contract
kind: policy
source_of_truth: this
entrypoint: ae-programa
phases:
  - id: gateway
    owner: ae-programa
    required_before:
      - harness_adapter
      - manual_non_trivial_work
      - mutating_work
      - policy_work
    outputs:
      - selected_mode
      - decision_lock_status
      - adapter_selection
      - session_contract_path_or_waiver
  - id: context_and_governance
    owner: ps-contexto
    requires:
      - workspace_status
      - nav_governance
      - attributed_mi_lsp_preflight
    stop_if:
      - governance_blocked=true
      - docs_ready=false
      - doc_count=0
      - ae_canon.status in [missing, mismatch, projection_only]
      - client_name=manual-cli
      - session_id matches cli-<pid>
  - id: decision_lock
    owner: ae-decision-lock
    requires:
      - execution_changing_decisions_named
      - allowed_paths
      - forbidden_paths
      - evidence
  - id: execution
    owner: selected_adapter
    requires:
      - session_contract
      - adapter_profile
      - stop_conditions
  - id: verification
    owner: selected_adapter
    requires:
      - local_tests_or_waiver
      - governance_or_harness_checks
      - runtime_profile_when_deployable
  - id: closure
    owner: ps-trazabilidad
    requires:
      - traceability_packet
      - audit_when_policy_or_large
      - cleanup_disposition
stop_if:
  - ae_programa_gateway_missing
  - session_contract_missing_for_non_trivial_work
  - adapter_selected_without_profile
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --format toon
evidence:
  - .docs/wiki/ae/AE-PHASES.md
```

## Phase Rule

No functional work starts until `gateway`, `context_and_governance`, and `decision_lock` are satisfied or an explicit read-only/trivial waiver is recorded in the session contract.

## Integration Rule

```toon
doc_id: AE-PHASES
block_id: AE-PHASES.integration_rule
kind: policy
source_of_truth: this
rule: auto_integrate_on_green_closure
applies_when:
  - cycle_produced_a_real_diff
green_closure_gates:
  - mi_lsp_preflight_present_and_clean
  - ps_trazabilidad_closure_packet_present
  - ps_auditar_trazabilidad_verdict=APPROVED
  - all_detected_drift_repaired
  - pre_push_guard_green
  - pr_ci_status_checks_green
on_green:
  - guarded_integration_into_origin_main_through_pr_flow
  - no_separate_human_approval_required
  - independent_review_satisfied_by: ps-auditar-trazabilidad
  - if_branch_protection_requires_review_and_enforce_admins_false: admin_merge_allowed
  - merge_command_example: "gh pr merge <n> --merge --admin --delete-branch"
  - after_merge: delete_local_and_remote_task_branch_per_cleanup_policy
hold_for_human_only_when:
  - ps_auditar_trazabilidad_verdict in [BLOCKED]
  - approved_with_follow_ups_that_need_a_human_decision
  - a_waiver_is_required
  - user_explicitly_requests_review
stop_if:
  - missing governance, docs_ready=false, doc_count=0, missing attribution, or ae_canon blocking state is downgraded to warning-only
  - integration_attempted_while_any_green_closure_gate_is_red
  - admin_merge_used_to_bypass_a_failing_ci_check
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --format toon
evidence:
  - .docs/wiki/ae/AE-PHASES.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/audit-verdict.yaml
```

The PR flow is the integration mechanism, not a human approval gate. When the green closure gates pass, the agent completes the merge itself (guarded, admin merge when branch protection requires the review that `ps-auditar-trazabilidad` already provides). A `BLOCKED` audit, a follow-up that needs a human decision, a required waiver, or an explicit user request to review are the only reasons to leave the PR open instead of integrating.

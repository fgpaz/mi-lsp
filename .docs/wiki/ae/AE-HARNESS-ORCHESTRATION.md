# AE-HARNESS-ORCHESTRATION

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-HARNESS-ORCHESTRATION"
id: "AE-HARNESS-ORCHESTRATION"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-HARNESS-MANIFEST]]'
  - '[[AE-PHASES]]'
  - '[[AE-SESSION-CONTRACT]]'
exports:
  - 'AE-HARNESS-ORCHESTRATION'
agent_must_read:
  - .docs/wiki/ae/AE-HARNESS-ORCHESTRATION.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
agent_may_edit:
  - .docs/wiki/ae/AE-HARNESS-ORCHESTRATION.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/AE-HARNESS-ORCHESTRATION.md
```

## Orchestration Contract

```toon
doc_id: AE-HARNESS-ORCHESTRATION
block_id: AE-HARNESS-ORCHESTRATION.contract
kind: policy
source_of_truth: this
gateway: ae-programa
mode_router: ae-orquestador
decision_lock: ae-decision-lock
default_adapter: codex
compatible_adapters:
  - codex
  - claude-code
  - opencode
  - hermes-programa
  - manual_acotado
subagent_rule:
  mandatory_for:
    - mutating_work
    - non_trivial_work
    - policy_work
    - large_or_multi_step_work
  minimum:
    trivial_read: 1
    medium_task: 3
    complex_task: 5
  first_wave: read_only_exploration
  writing_wave: specialized_implementation_or_worker_lane
  zero_subagents: non_compliant
recursion_depth:
  default: v0_shadow
  active_levels: 2
  third_level: shadow_only
  escalation_requires:
    - child_contract_complete
    - evidence_dir_unique
    - stop_if_present
    - allowed_paths_do_not_overlap_sibling_exclusive_paths
operational_ledgers_required_when:
  - multiple_orchestrators
  - recursion
  - policy_or_harness_repair
ledger_files:
  - orchestrator-registry.yaml
  - decision-ledger.yaml
  - recursion-learning-log.yaml
  - evidence-index.yaml
stop_if:
  - no_session_contract_for_mutating_work
  - subagent_or_worker_scope_is_ambiguous
  - two_orchestrators_own_same_exclusive_path
  - evidence_would_live_only_in_chat
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-HARNESS-ORCHESTRATION.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/orchestrator-registry.yaml
```

## Adapter Rule

Harness adapters execute; they do not become conceptual authority. If Codex, Claude Code, OpenCode, Hermes, or any future runner contradicts the AE canon, stop and repair the canon/projection drift before continuing.

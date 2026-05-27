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
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
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
    stop_if:
      - governance_blocked=true
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
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-PHASES.md
```

## Phase Rule

No functional work starts until `gateway`, `context_and_governance`, and `decision_lock` are satisfied or an explicit read-only/trivial waiver is recorded in the session contract.

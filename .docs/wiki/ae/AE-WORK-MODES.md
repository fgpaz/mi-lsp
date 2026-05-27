# AE-WORK-MODES

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-WORK-MODES"
id: "AE-WORK-MODES"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-HARNESS-MANIFEST]]'
exports:
  - 'AE-WORK-MODES'
agent_must_read:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
  - .docs/wiki/ae/AE-WORK-MODES.md
agent_may_edit:
  - .docs/wiki/ae/AE-WORK-MODES.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - wiki_source_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/AE-WORK-MODES.md
```

## Modes

```toon
doc_id: AE-WORK-MODES
block_id: AE-WORK-MODES.modes
kind: policy
source_of_truth: this
modes:
  - id: manual_exploratorio
    use_when:
      - execution_changing_decisions_are_open
      - task_is_discovery_or_design
    required_controls:
      - ps-contexto
      - governance_gate
      - brainstorming
      - learning_classification
  - id: manual_acotado
    use_when:
      - bounded repo edit
      - decisions are known
      - no independent worker has a clean write slice
    required_controls:
      - ae-programa_gateway
      - session_contract
      - targeted_tests
      - traceability
  - id: manifest_repair
    use_when:
      - governance or AE layer is missing, stale, or ambiguous
      - policy files need projection from wiki
    required_controls:
      - ae-programa_gateway
      - crear-gobierno-documental_when_00_changes
      - ae-crear-politicas_when_policy_changes
      - reindex
  - id: canon_repair
    use_when:
      - governed wiki source is missing or contradicted
      - projection must be regenerated from canon
    required_controls:
      - ps-asistente-wiki_or_owner_skill
      - session_contract
      - governance_revalidation
  - id: orquestado_deterministico
    use_when:
      - task can be split into disjoint write sets
      - worker prompt can be atomic and evidence-bounded
    required_controls:
      - ae-programa_gateway
      - ae-decision-lock
      - explicit_write_scope
      - no_reverting_foreign_changes
      - integration_review
  - id: validation_audit
    use_when:
      - implementation exists and needs independent closure review
      - traceability or runtime evidence may be stale
    required_controls:
      - ps-trazabilidad
      - ps-auditar-trazabilidad
      - no_drift_snapshot
  - id: release_distribution
    use_when:
      - CLI or worker binaries can drift
      - installed paths must be refreshed
      - GitHub release assets or skill mirrors must receive new binaries
    required_controls:
      - AE-RELEASE-DISTRIBUTION
      - scripts/release/ae-release-binaries.ps1
      - provenance_evidence
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-WORK-MODES.md
```

## Decision Lock

```toon
doc_id: AE-WORK-MODES
block_id: AE-WORK-MODES.decision_lock
kind: policy
source_of_truth: this
lock_fields:
  - selected_mode
  - why
  - non_goals
  - allowed_paths
  - forbidden_paths
  - release_distribution_required
  - required_evidence
stop_if:
  - selected_mode is absent
  - allowed_paths conflict with real diff
  - release_distribution_required is absent for binary-affecting work
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-WORK-MODES.md
```

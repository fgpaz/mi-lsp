# AE-PROJECTION-POLICY

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-PROJECTION-POLICY"
id: "AE-PROJECTION-POLICY"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-HARNESS-MANIFEST]]'
  - '[[AE-HARNESS-ORCHESTRATION]]'
exports:
  - 'AE-PROJECTION-POLICY'
agent_must_read:
  - .docs/wiki/ae/AE-PROJECTION-POLICY.md
  - AGENTS.md
  - CLAUDE.md
  - PATHS.md
agent_may_edit:
  - .docs/wiki/ae/AE-PROJECTION-POLICY.md
  - AGENTS.md
  - CLAUDE.md
  - PATHS.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - policy_projection_drift=true
evidence:
  - .docs/wiki/ae/AE-PROJECTION-POLICY.md
  - AGENTS.md
  - CLAUDE.md
  - PATHS.md
```

## Projection Contract

```toon
doc_id: AE-PROJECTION-POLICY
block_id: AE-PROJECTION-POLICY.contract
kind: policy
source_of_truth: this
canon_wins_over:
  - AGENTS.md
  - CLAUDE.md
  - PATHS.md
  - runner_prompts
  - worker_contracts
projection_targets:
  - path: AGENTS.md
    audience: codex
    must_include:
      - ae_programa_gateway
      - governance_gate
      - session_contract
      - mandatory_subagents
      - pre_push_guard
  - path: CLAUDE.md
    audience: claude-code
    must_include:
      - ae_programa_gateway
      - governance_gate
      - session_contract
      - mandatory_subagents
      - pre_push_guard
  - path: PATHS.md
    audience: all_agents
    must_include:
      - canonical_wiki_paths
      - ae_paths
      - audit_paths
      - script_paths
projection_update_required_when:
  - ae_canon_changes
  - workflow_policy_changes
  - subagent_or_worker_rules_change
  - pre_push_or_traceability_gates_change
shared_skill_mirror_rule:
  source_root: C:/Users/fgpaz/.agents/skills
  mirror_root: C:/repos/buho/assets/skills
  byte_identical_required: true
stop_if:
  - AGENTS_or_CLAUDE_changes_without_AE_source_update
  - PATHS_missing_after_AE_policy_change
  - shared_skill_source_and_mirror_hashes_differ
verify:
  - rg -n "ae-programa|AE-PROJECTION-POLICY|Subagent Orchestration|pre-push" AGENTS.md CLAUDE.md PATHS.md
evidence:
  - .docs/wiki/ae/AE-PROJECTION-POLICY.md
  - AGENTS.md
  - CLAUDE.md
  - PATHS.md
```

## Drift Rule

If a projection and `.docs/wiki/ae/**` disagree, the projection is stale. Repair the AE canon or projection before continuing.

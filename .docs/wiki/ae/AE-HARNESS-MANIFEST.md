# AE-HARNESS-MANIFEST

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-HARNESS-MANIFEST"
id: "AE-HARNESS-MANIFEST"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-PHASES]]'
  - '[[AE-HARNESS-ORCHESTRATION]]'
  - '[[AE-WORK-MODES]]'
  - '[[AE-SESSION-CONTRACT]]'
  - '[[AE-PROJECTION-POLICY]]'
  - '[[AE-RELEASE-DISTRIBUTION]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - 'AE-HARNESS-MANIFEST'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
agent_may_edit:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
```

## Manifest

```toon
doc_id: AE-HARNESS-MANIFEST
block_id: AE-HARNESS-MANIFEST.manifest
kind: policy
source_of_truth: this
protocols:
  - SDD-HARNESS-v1
  - SDD-WIKI-SOURCE-v1
entry_skill: ae-programa
mode_router: ae-orquestador
required_sequence:
  - ae-programa_gateway
  - ps-contexto
  - governance_gate
  - brainstorming
  - session_contract
  - ae_harness_manifest
  - ae_decision_lock
  - work_mode
  - implementation_or_repair
  - evidence_policy
  - ps-trazabilidad
  - ps-auditar-trazabilidad_when_risky
session_contract_path: .docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml
required_session_fields:
  - ae_contract
  - ae_mode
  - ae_docs
  - release_distribution_required
  - binary_targets
  - publish_strategy
  - local_install_targets
  - mirror_targets
  - waivers
verify:
  - every AE doc has harness contract
  - every normative section has doc_id and block_id
  - release-visible work declares binary targets
  - closure evidence is durable under .docs/auditoria
evidence:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml
stop_if:
  - ae-programa gateway was skipped for non-trivial work
  - session contract is missing for mutating or non-trivial work
  - release_distribution_required=true and AE-RELEASE-DISTRIBUTION was skipped
  - policy files drift from AE layer
```

## Routing

```toon
doc_id: AE-HARNESS-MANIFEST
block_id: AE-HARNESS-MANIFEST.routing
kind: routing
source_of_truth: this
routes:
  - when: "agent workflow, policy, or harness shape changes"
    next: AE-HARNESS-ORCHESTRATION
  - when: "work mode, phase, or recursion depth must be selected"
    next: AE-PHASES
  - when: "session contract schema or scope guard changes"
    next: AE-SESSION-CONTRACT
  - when: "AGENTS.md, CLAUDE.md, PATHS.md, or runner prompt projection changes"
    next: AE-PROJECTION-POLICY
  - when: "binaries, release assets, worker bootstrap, version provenance, or install flow can drift"
    next: AE-RELEASE-DISTRIBUTION
  - when: "closure evidence, traceability, or audit surface changes"
    next: AE-EVIDENCE-POLICY
  - when: "a runner must choose between canon, manifest/verdict, summaries, or raw evidence"
    next: "mi-lsp nav evidence inventory <query> --workspace <alias> --format toon"
  - when: "AGENTS.md or CLAUDE.md changes"
    next: ps-crear-agentsclaudemd
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
```

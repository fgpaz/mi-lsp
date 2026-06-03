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
  - mi-lsp nav governance --workspace <alias> --format toon
  - mi-lsp nav wiki validate-harness --workspace <alias> --format toon
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
  - attributed_mi_lsp_preflight
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
  - mi_lsp_preflight
  - ae_mode
  - ae_docs
  - release_distribution_required
  - binary_targets
  - publish_strategy
  - local_install_targets
  - mirror_targets
  - worker_session_attribution_matrix_when_worker_or_wsl_audit
  - admin_export_summary_when_worker_or_wsl_audit
  - manual_cli_exception_review_when_worker_or_wsl_audit
  - waivers
verify:
  - every AE doc has harness contract
  - every normative section has doc_id and block_id
  - mi_lsp_preflight records alias, root, client_name, session_id, docs_ready, doc_count, and ae_canon.status
  - release-visible work declares binary targets
  - closure evidence is durable under .docs/auditoria
evidence:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml
stop_if:
  - ae-programa gateway was skipped for non-trivial work
  - session contract is missing for mutating or non-trivial work
  - mi_lsp_preflight is missing for governed AE work
  - mi_lsp_preflight.client_name=manual-cli
  - mi_lsp_preflight.session_id matches cli-<pid>
  - mi_lsp_preflight.docs_ready=false
  - mi_lsp_preflight.doc_count=0
  - mi_lsp_preflight.ae_canon.status in [missing, mismatch, projection_only]
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
  - when: "WSL, subagent, worker, or historical execution audit is requested"
    next: AE-HARNESS-ORCHESTRATION
  - when: "a runner must choose between canon, manifest/verdict, summaries, or raw evidence"
    next: "mi-lsp nav evidence inventory <query> --workspace <alias> --format toon"
  - when: "AGENTS.md or CLAUDE.md changes"
    next: ps-crear-agentsclaudemd
verify:
  - mi-lsp nav wiki validate-source --workspace <alias> --format toon
evidence:
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
```

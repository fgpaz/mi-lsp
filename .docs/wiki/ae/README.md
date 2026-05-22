# AE Layer

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "AE-README"
id: "AE-README"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-HARNESS-MANIFEST]]'
  - '[[AE-WORK-MODES]]'
  - '[[AE-RELEASE-DISTRIBUTION]]'
  - '[[AE-EVIDENCE-POLICY]]'
exports:
  - 'AE-README'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/ae/README.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
agent_may_edit:
  - .docs/wiki/ae/README.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
  - .docs/wiki/ae/AE-WORK-MODES.md
  - .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/ae/README.md
```

## Purpose

The AE layer is the agent-engineering operating layer for this repo. It does not replace `00..09`; it binds the existing SDD wiki, agent policies, release scripts, and closure evidence into a repeatable execution contract.

```toon
doc_id: AE-README
block_id: AE-README.overview
kind: policy
source_of_truth: this
authority: governed_annex
scope:
  - agent orchestration
  - harness manifest
  - work-mode selection
  - release distribution closure
non_goals:
  - replacing functional RF/FL/TP authority
  - changing CLI behavior without 09/CT docs
  - treating .docs/raw as durable evidence
must_read:
  - AE-HARNESS-MANIFEST
  - AE-WORK-MODES
  - AE-RELEASE-DISTRIBUTION
  - AE-EVIDENCE-POLICY
verify:
  - governance passes
  - harness passes
  - release-distribution gate runs when binaries can drift
evidence:
  - .docs/wiki/ae/README.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
stop_if:
  - AE docs are not indexed by governance/read-model
  - release-visible work closes without binary provenance evidence
```

## Operating Rule

Use `ae-orquestador` for AE-scoped work. It selects a work mode, then routes to `ae-harness-manifest`, `ae-decision-lock`, release distribution, and evidence policy as needed.

Any change that can alter the installed CLI, worker bootstrap, release assets, version provenance, or cross-OS behavior must close through [[AE-RELEASE-DISTRIBUTION]] before it is considered done.

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
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
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
  - evidence lives only in chat or .docs/raw
verify:
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
evidence:
  - .docs/wiki/ae/AE-EVIDENCE-POLICY.md
  - .docs/auditoria/<YYYY-MM-DD>-<task-slug>/
```

## Closure Rule

For binary-affecting work, `ps-trazabilidad` must include the AE release evidence or an explicit waiver. `ps-auditar-trazabilidad` is required for cross-OS, release, worker bootstrap, or policy changes.

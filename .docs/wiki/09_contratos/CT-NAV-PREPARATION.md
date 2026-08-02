---
doc_id: CT-NAV-PREPARATION
title: Contrato de preparación semántica portable
layer: CT
status: implemented_and_verified
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: CT-NAV-PREPARATION
kind: canonical-artifact
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[CT-NAV-PREPARATION]]'
  - '[[RF-QRY-019]]'
  - '[[TP-QRY-PREPARATION]]'
exports:
  - CT-NAV-PREPARATION

agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - .docs/auditoria/
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids CT-NAV-PREPARATION --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - authorization_claim=true
  - evidence_escape=true
evidence:
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

---
# CT-NAV-PREPARATION
## [ct-nav-preparation-harness]

```toon
block_id: ct-nav-preparation-harness
source_protocol: SDD-WIKI-SOURCE-v1
harness_protocol: SDD-HARNESS-v1
id: CT-NAV-PREPARATION
kind: technical-contract
audience: llm-first
imports: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/04_RF/RF-QRY-019.md]
exports: [CT-NAV-PREPARATION]
agent_must_read: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/09_contratos/CT-NAV-PREPARATION.md, .docs/wiki/04_RF/RF-QRY-019.md, .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md]
agent_may_edit: [.docs/wiki/09_contratos/CT-NAV-PREPARATION.md]
agent_must_not_edit: [.docs/wiki/_mi-lsp/read-model.toml, .docs/auditoria/]
verify: [mi-lsp nav governance --workspace mi-lsp --format toon, mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon, mi-lsp nav wiki validate-source --workspace mi-lsp --ids CT-NAV-PREPARATION --format toon]
stop_if: [governance_blocked=true, harness_verdict=BLOCKED, authorization_claim=true, evidence_escape=true]
evidence: [.docs/wiki/04_RF/RF-QRY-019.md, .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md, .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
source_of_truth: this
```
## [ct-nav-preparation-schema]

```toon
block_id: ct-nav-preparation-schema
source_protocol: SDD-WIKI-SOURCE-v1
kind: normative-schema
schema: mi-lsp-preparation/v1
commands: [prepare create, prepare verify, prepare refresh]
intents: [read_only, source_mutation, artifact_create, artifact_promote, evidence_write, temp_internal]
seed_receipt: optional_metadata_and_digests_only
canonical_digest: excludes_own_digest
fields: [schema, workspace_identity, task_digest, governance_digest, index_digest, plan_digest, intent, targets, output_scope, created_at, expires_at, evidence, digest]
source_of_truth: this
verify: [validate-harness, validate-source]
evidence: [.docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
``` 
## [ct-nav-preparation-security]

```toon
block_id: ct-nav-preparation-security
source_protocol: SDD-WIKI-SOURCE-v1
kind: normative-security
workspace: explicit_workspace_makes_cwd_independent
output: explicit_validated_output_under_workspace_or_declared_evidence_root
path_rules: [targets_relative, reject_absolute, reject_traversal, reject_symlink, reject_junction, reject_evidence_escape]
authority: mi-lsp_emits_serializes_verifies_refreshes_evidence_only
forbidden: [authorize, promote, protected_write, broker]
ttl: default_15m; configurable_max_24h
source_of_truth: this
verify: [validate-harness, validate-source]
evidence: [.docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
``` 
## [ct-nav-preparation-results]

```toon
block_id: ct-nav-preparation-results
source_protocol: SDD-WIKI-SOURCE-v1
kind: normative-results
codes: [PREPARATION_READY, PREPARATION_MISSING, TRANSFER_REQUIRED, PACKET_EXPIRED, WORKSPACE_MISMATCH, TASK_DIGEST_MISMATCH, GOVERNANCE_DRIFT, INDEX_DRIFT, PLAN_DRIFT, PATH_SCOPE_MISMATCH, PACKET_TAMPERED]
result_fields: [code, repairable, recommended_action]
legacy_packets: accepted_as_legacy
source_of_truth: this
verify: [validate-harness, validate-source]
evidence: [.docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
``` 
```
La versión `mi-lsp-preparation/v1` es un sobre portable y determinista. `create` emite, `verify` valida y `refresh` vuelve a emitir evidencia; ninguna operación concede autoridad de escritura o promoción. Los fallos deben declarar si son reparables y una acción recomendada. La compatibilidad legacy conserva lectura, pero no amplía permisos.


## Cierre v0.7.0 — implementación y evidencia

```toon
block_id: closure-v070-evidence
status: implemented_and_verified
automation: automated
result: 17/17 PASS
tp11_windows_junction: PASS
implementation: [internal/cli/prepare.go, internal/service/prepare.go, internal/service/preparation_packet.go, internal/model/preparation.go, internal/skills/preparation_receipt.go]
tests: [internal/service/preparation_packet_matrix_test.go, internal/service/preparation_packet_test.go, internal/service/prepare_test.go, internal/skills/preparation_receipt_test.go]
evidence_root: .docs/auditoria/mi-lsp-portable-preparation-v1/
review: 541bd2bf
head: a449abf
authority: evidence_never_authorizes_writes_or_promotions
```

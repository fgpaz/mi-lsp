---
doc_id: TECH-SEMANTIC-PREPARATION
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
source_kind: canonical-technical-detail
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: TECH-SEMANTIC-PREPARATION
kind: technical-detail
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-NAV-PREPARATION]]'
  - '[[RF-QRY-019]]'
  - '[[TP-QRY-PREPARATION]]'
exports:
  - TECH-SEMANTIC-PREPARATION
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-SEMANTIC-PREPARATION.md
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-SEMANTIC-PREPARATION.md
agent_must_not_edit:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids TECH-SEMANTIC-PREPARATION --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - preparation_grants_mutation_authority
  - allowed_paths_are_inferred
evidence:
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
---

# TECH-SEMANTIC-PREPARATION

```toon
source_protocol: SDD-WIKI-SOURCE-v1
block_id: tech-semantic-preparation-mapping
kind: normative-migration-mapping
source_of_truth: CT-NAV-PREPARATION
rf: RF-QRY-019
tp: TP-QRY-PREPARATION
cli: [prepare create, prepare verify, prepare refresh]
implementation_boundary: evidence_only
security_boundary: explicit_output_workspace_or_evidence_root
compatibility: legacy_packets_read_only
verify: [validate-harness, validate-source]
evidence: [.docs/auditoria/mi-lsp-portable-preparation-v1/]
```

La preparación portable migra el paquete local de readiness a `mi-lsp-preparation/v1`; CT-NAV-PREPARATION define serialización, seguridad y códigos, RF-QRY-019 define el ciclo y TP-QRY-PREPARATION define sus 17 casos.


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

---
doc_id: RF-QRY-019
title: Preparar y verificar evidencia semántica portable
layer: RF
status: implemented_and_verified
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-019
kind: canonical-artifact
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[CT-NAV-PREPARATION]]'
  - '[[TP-QRY-PREPARATION]]'
exports:
  - RF-QRY-019

agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-019.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - .docs/auditoria/
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-019 --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - preparation_grants_mutation_authority
  - packet_writes_outside_validated_scope
evidence:
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

---

# RF-QRY-019 — Ciclo de preparación portable

## [rf-qry-019-harness]

```toon
block_id: rf-qry-019-harness
source_protocol: SDD-WIKI-SOURCE-v1
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-019
kind: functional-requirement
audience: llm-first
imports: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/03_FL/FL-QRY-01.md, .docs/wiki/09_contratos/CT-NAV-PREPARATION.md]
exports: [RF-QRY-019]
agent_must_read: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/04_RF/RF-QRY-019.md, .docs/wiki/09_contratos/CT-NAV-PREPARATION.md, .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md]
agent_may_edit: [.docs/wiki/04_RF/RF-QRY-019.md]
agent_must_not_edit: [.docs/wiki/_mi-lsp/read-model.toml, .docs/auditoria/]
verify: [mi-lsp nav governance --workspace mi-lsp --format toon, mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon, mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-019 --format toon]
stop_if: [governance_blocked=true, harness_verdict=BLOCKED, preparation_grants_mutation_authority, packet_writes_outside_validated_scope]
evidence: [.docs/wiki/09_contratos/CT-NAV-PREPARATION.md, .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md, .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
source_of_truth: this
```

## [rf-qry-019-behavior]

```toon
block_id: rf-qry-019-behavior
source_protocol: SDD-WIKI-SOURCE-v1
kind: normative-requirement
source_of_truth: RF-QRY-019
actor: Skill|Agente|CLI
origin: FL-QRY-01
status: ready
intentos: [read_only, source_mutation, artifact_create, artifact_promote, evidence_write, temp_internal]
commands: [mi-lsp prepare create, mi-lsp prepare verify, mi-lsp prepare refresh]
default_ttl: 15m
max_ttl: 24h
result_codes: [PREPARATION_READY, PREPARATION_MISSING, TRANSFER_REQUIRED, PACKET_EXPIRED, WORKSPACE_MISMATCH, TASK_DIGEST_MISMATCH, GOVERNANCE_DRIFT, INDEX_DRIFT, PLAN_DRIFT, PATH_SCOPE_MISMATCH, PACKET_TAMPERED]
ownership: mi-lsp emits|serializes|verifies|refreshes evidence only
forbidden: authorize|promote|protected_write|broker
verify: [validate-harness, validate-source]
evidence: [.docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
```

## Contrato funcional

`prepare create` produce un paquete portable ligado al workspace explícito, tarea, gobernanza, índice y plan; `verify` comprueba esa identidad y `refresh` renueva evidencia sin cambiar autoridad. Un recibo seed opcional aporta únicamente metadatos y digests. La preparación nunca autoriza una mutación, promoción, escritura protegida ni broker.

El `--workspace` explícito hace la operación independiente del CWD. Solo se escriben paquetes en un `--output` explícito y validado bajo el workspace o una raíz de evidencia declarada; los targets son relativos y se rechazan absolutos, traversal, symlink/junction y escapes de la raíz de evidencia. El digest propio se excluye de la serialización canónica.

## Trazabilidad

Los 17 casos de `TP-QRY-PREPARATION` cubren aceptación positiva, rechazo fail-closed y reparación sin elevar autoridad.


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

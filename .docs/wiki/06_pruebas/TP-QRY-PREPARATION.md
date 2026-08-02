---
doc_id: TP-QRY-PREPARATION
title: Pruebas de preparación semántica portable
layer: TP
status: ready
source_schema: SDD-WIKI-SOURCE-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
normative_format: toon
harness_protocol: SDD-HARNESS-v1
id: TP-QRY-PREPARATION
kind: canonical-artifact
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[CT-NAV-PREPARATION]]'
  - '[[RF-QRY-019]]'
  - '[[TP-QRY-PREPARATION]]'
exports:
  - TP-QRY-PREPARATION

agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - .docs/auditoria/
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids TP-QRY-PREPARATION --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - oracle_missing=true
  - evidence_scope_unknown=true
evidence:
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/09_contratos/CT-NAV-PREPARATION.md
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

---
# TP-QRY-PREPARATION
## [tp-qry-preparation-harness]

```toon
block_id: tp-qry-preparation-harness
source_protocol: SDD-WIKI-SOURCE-v1
harness_protocol: SDD-HARNESS-v1
id: TP-QRY-PREPARATION
kind: test-plan
audience: llm-first
imports: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/04_RF/RF-QRY-019.md, .docs/wiki/09_contratos/CT-NAV-PREPARATION.md]
exports: [TP-QRY-PREPARATION]
agent_must_read: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/04_RF/RF-QRY-019.md, .docs/wiki/09_contratos/CT-NAV-PREPARATION.md, .docs/wiki/06_pruebas/TP-QRY-PREPARATION.md]
agent_may_edit: [.docs/wiki/06_pruebas/TP-QRY-PREPARATION.md]
agent_must_not_edit: [.docs/wiki/_mi-lsp/read-model.toml, .docs/auditoria/]
verify: [mi-lsp nav governance --workspace mi-lsp --format toon, mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon, mi-lsp nav wiki validate-source --workspace mi-lsp --ids TP-QRY-PREPARATION --format toon]
stop_if: [governance_blocked=true, harness_verdict=BLOCKED, oracle_missing=true, evidence_scope_unknown=true]
evidence: [.docs/wiki/04_RF/RF-QRY-019.md, .docs/wiki/09_contratos/CT-NAV-PREPARATION.md, .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
source_of_truth: this
```
## [tp-qry-preparation-cases]

```toon
block_id: tp-qry-preparation-cases
source_protocol: SDD-WIKI-SOURCE-v1
kind: normative-test-catalog
count: 17
cases:
  - id: TC-QRY-PREPARATION-01
    name: "parent-child neutral"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-02
    name: "cwd identity"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-03
    name: "workspace mismatch"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-04
    name: "task mismatch"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-05
    name: "expiry"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-06
    name: "governance drift"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-07
    name: "index drift"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-08
    name: "plan drift"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-09
    name: "allowlist expansion"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-10
    name: "traversal"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-11
    name: "symlink/junction"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-12
    name: "evidence root"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-13
    name: "tamper"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-14
    name: "legacy"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-15
    name: "seed failure reparable no auth"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-16
    name: "isolated --skills-root/--catalog"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
  - id: TC-QRY-PREPARATION-17
    name: "index/search same catalog"
    oracle: resultado tipado y alcance seguro; reparación solo cuando corresponda
    stop_if: evidencia ausente, autorización implícita o escape de raíz
    evidence: packet + verify envelope + sanitized audit manifest
source_of_truth: this
verify: [validate-harness, validate-source]
evidence: [.docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml, .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md]
```

Cada caso conserva exactamente el nombre recibido en el handoff. El oráculo común exige `code`, `repairable` y `recommended_action`; el stop condition es fail-closed ante autoridad implícita, path inseguro, digest no verificable o evidencia ausente. La aceptación se traza a RF-QRY-019 y CT-NAV-PREPARATION.

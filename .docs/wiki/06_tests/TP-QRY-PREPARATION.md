---
wiki_contract: WIKI-CONTRACT-v1
id: TP-QRY-PREPARATION
kind: test
status: active
title: TP-QRY-PREPARATION
links:
  verifies:
  - RF-QRY-019
---

## Alcance

RF-QRY-019. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

### [tp-qry-preparation-harness]

- **Marcador estable**: tp-qry-preparation-harness
- **id**: TP-QRY-PREPARATION
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Comprobación prevista**:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids TP-QRY-PREPARATION --format toon
- **Condiciones de rechazo**:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - oracle_missing=true
  - evidence_scope_unknown=true
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-QRY-019.md
  - .docs/wiki/07_technical/contracts/CT-NAV-PREPARATION.md
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md
### [tp-qry-preparation-cases]

- **Marcador estable**: tp-qry-preparation-cases
- **count**: 17
- **Casos**:
  - **id**: TC-QRY-PREPARATION-01
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: parent-child neutral
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-02
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: cwd identity
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-03
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: workspace mismatch
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-04
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: task mismatch
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-05
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: expiry
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-06
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: governance drift
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-07
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: index drift
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-08
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: plan drift
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-09
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: allowlist expansion
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-10
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: traversal
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-11
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: symlink/junction
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-12
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: evidence root
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-13
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: tamper
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-14
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: legacy
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-15
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: seed failure reparable no auth
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-16
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: isolated --skills-root/--catalog
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
  - **id**: TC-QRY-PREPARATION-17
  - RF-QRY-019 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **name**: index/search same catalog
  - **Oráculo**: resultado tipado y alcance seguro; reparación solo cuando corresponda
  - **Condiciones de rechazo**: evidencia ausente, autorización implícita o escape de raíz
  - **evidence_requirement**: packet + verify envelope + sanitized audit manifest
- **Comprobación prevista**:
  - validate-harness
  - validate-source
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

Cada caso conserva exactamente el nombre recibido en el handoff. El oráculo común exige `code`, `repairable` y `recommended_action`; el stop condition es fail-closed ante autoridad implícita, path inseguro, digest no verificable o evidencia ausente. La aceptación se traza a RF-QRY-019 y CT-NAV-PREPARATION.


### Cierre v0.7.0 — implementación y evidencia

- **Marcador estable**: closure-v070-evidence
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace <alias> --ids TP-QRY-PREPARATION --format toon
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/traceability-closure.yaml
- **Estado del comportamiento**: implemented_and_verified
- **automation**: automated
- **result**: 17/17 PASS
- **tp11_windows_junction**: PASS
- **Implementación**:
  - internal/cli/prepare.go
  - internal/service/prepare.go
  - internal/service/preparation_packet.go
  - internal/model/preparation.go
  - internal/skills/preparation_receipt.go
- **tests**:
  - internal/service/preparation_packet_matrix_test.go
  - internal/service/preparation_packet_test.go
  - internal/service/prepare_test.go
  - internal/skills/preparation_receipt_test.go
- **evidence_root**: .docs/auditoria/mi-lsp-portable-preparation-v1/
- **review**: 541bd2bf
- **head**: a449abf
- **authority**: evidence_never_authorizes_writes_or_promotions

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-QRY-PREPARATION
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-QRY-019.md
exports:
  - TP-QRY-PREPARATION
agent_must_read:
  - .docs/wiki/06_tests/TP-QRY-PREPARATION.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-QRY-PREPARATION.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-QRY-019.md
  - internal/
verify:
  - .docs/wiki/06_tests/TP-QRY-PREPARATION.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - preparation_evidence_is_treated_as_authorization
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-QRY-PREPARATION.md
  - .docs/wiki/05_requirements/RF-QRY-019.md
```

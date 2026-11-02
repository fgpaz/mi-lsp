---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-019
kind: requirement
status: active
title: RF-QRY-019 — Ciclo de preparación portable
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/prepare.go
    role: behavior
  - path: internal/model/preparation.go
    role: behavior
  - path: internal/service/preparation_packet.go
    role: behavior
  - path: internal/service/prepare.go
    role: behavior
  - path: internal/skills/preparation_receipt.go
    role: behavior
---

## Propósito

RF-QRY-019 — Ciclo de preparación portable

## Comportamiento ordenado

### [rf-qry-019-harness]

- **Marcador estable**: rf-qry-019-harness
- **id**: RF-QRY-019
- **Comprobación prevista**:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --ids RF-QRY-019 --format toon
- **Condiciones de rechazo**:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - preparation_grants_mutation_authority
  - packet_writes_outside_validated_scope
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-NAV-PREPARATION.md
  - .docs/wiki/06_tests/TP-QRY-PREPARATION.md
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

### [rf-qry-019-behavior]

- **Marcador estable**: rf-qry-019-behavior
- **actor**: Skill|Agente|CLI
- **origin**: FL-QRY-01
- **Estado del comportamiento**: ready
- **intentos**:
  - read_only
  - source_mutation
  - artifact_create
  - artifact_promote
  - evidence_write
  - temp_internal
- **commands**:
  - mi-lsp prepare create
  - mi-lsp prepare verify
  - mi-lsp prepare refresh
- **default_ttl**: 15m
- **max_ttl**: 24h
- **result_codes**:
  - PREPARATION_READY
  - PREPARATION_MISSING
  - TRANSFER_REQUIRED
  - PACKET_EXPIRED
  - WORKSPACE_MISMATCH
  - TASK_DIGEST_MISMATCH
  - GOVERNANCE_DRIFT
  - INDEX_DRIFT
  - PLAN_DRIFT
  - PATH_SCOPE_MISMATCH
  - PACKET_TAMPERED
- **ownership**: mi-lsp emits|serializes|verifies|refreshes evidence only
- **forbidden**: authorize|promote|protected_write|broker
- **Comprobación prevista**:
  - validate-harness
  - validate-source
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml
  - .docs/auditoria/mi-lsp-portable-preparation-v1/current-handoff.md

### Contrato funcional

`prepare create` produce un paquete portable ligado al workspace explícito, tarea, gobernanza, índice y plan; `verify` comprueba esa identidad y `refresh` renueva evidencia sin cambiar autoridad. Un recibo seed opcional aporta únicamente metadatos y digests. La preparación nunca autoriza una mutación, promoción, escritura protegida ni broker.

El `--workspace` explícito hace la operación independiente del CWD. Solo se escriben paquetes en un `--output` explícito y validado bajo el workspace o una raíz de evidencia declarada; los targets son relativos y se rechazan absolutos, traversal, symlink/junction y escapes de la raíz de evidencia. El digest propio se excluye de la serialización canónica.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. `prepare create` produce un paquete portable ligado al workspace explícito, tarea, gobernanza, índice y plan; `verify` comprueba esa identidad y `refresh` renueva evidencia sin cambiar autoridad. Un recibo seed opcional aporta únicamente metadatos y digests. La preparación nunca autoriza una mutación, promoción, escritura protegida ni broker.
2. El `--workspace` explícito hace la operación independiente del CWD. Solo se escriben paquetes en un `--output` explícito y validado bajo el workspace o una raíz de evidencia declarada; los targets son relativos y se rechazan absolutos, traversal, symlink/junction y escapes de la raíz de evidencia. El digest propio se excluye de la serialización canónica.
3. Los 17 casos de `TP-QRY-PREPARATION` cubren aceptación positiva, rechazo fail-closed y reparación sin elevar autoridad.
4. ## Cierre v0.7.0 — implementación y evidencia

### Trazabilidad

Los 17 casos de `TP-QRY-PREPARATION` cubren aceptación positiva, rechazo fail-closed y reparación sin elevar autoridad.

### Cierre v0.7.0 — implementación y evidencia

- **Marcador estable**: closure-v070-evidence
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace <alias> --ids RF-QRY-019 --format toon
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
id: RF-QRY-019
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-019
- RF-QRY-019 — Ciclo de preparación portable
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-019.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-019.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/prepare.go
- internal/model/preparation.go
- internal/service/preparation_packet.go
- internal/service/prepare.go
- internal/skills/preparation_receipt.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-QRY-PREPARATION.md sin
  inferir que fueron ejecutados.
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-019.
evidence:
- .docs/wiki/05_requirements/RF-QRY-019.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
```

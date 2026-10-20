---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-SEMANTIC-PREPARATION
kind: technical
status: active
title: TECH-SEMANTIC-PREPARATION
links:
  supports:
  - RF-QRY-019
  code:
  - path: internal/cli/prepare.go
    role: implementation
  - path: internal/model/preparation.go
    role: implementation
  - path: internal/service/preparation_packet.go
    role: implementation
  - path: internal/service/prepare.go
    role: implementation
  - path: internal/skills/preparation_receipt.go
    role: implementation
---

## Resumen

TECH-SEMANTIC-PREPARATION

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

- **Marcador estable**: tech-semantic-preparation-mapping
- **RF**: RF-QRY-019
- **tp**: TP-QRY-PREPARATION
- **cli**:
  - prepare create
  - prepare verify
  - prepare refresh
- **implementation_boundary**: evidence_only
- **security_boundary**: explicit_output_workspace_or_evidence_root
- **compatibility**: legacy_packets_read_only
- **Comprobación prevista**:
  - validate-harness
  - validate-source
- **Evidencia referenciada**:
  - .docs/auditoria/mi-lsp-portable-preparation-v1/session-contract.yaml

La preparación portable migra el paquete local de readiness a `mi-lsp-preparation/v1`; CT-NAV-PREPARATION define serialización, seguridad y códigos, RF-QRY-019 define el ciclo y TP-QRY-PREPARATION define sus 17 casos.

### Cierre v0.7.0 — implementación y evidencia

- **Marcador estable**: closure-v070-evidence
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace <alias> --ids TECH-SEMANTIC-PREPARATION --format toon
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

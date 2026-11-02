---
wiki_contract: WIKI-CONTRACT-v1
id: TP-CS
kind: test
status: active
title: TP-CS
links:
  verifies:
  - RF-CS-001
---

## Alcance

RF-CS-001. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

### Cobertura objetivo

- RF-CS-001

### Escenarios
- **Caso**: TC-CS-001; **Tipo**: positivo; **RF**: RF-CS-001; **Descripción**: resuelve consulta C# con `backend=roslyn`
  - RF-CS-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: resuelve consulta C# con `backend=roslyn`. No acredita una ejecución de esta migración.
- **Caso**: TC-CS-002; **Tipo**: positivo; **RF**: RF-CS-001; **Descripción**: reutiliza runtime caliente o worker ya disponible
  - RF-CS-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: reutiliza runtime caliente o worker ya disponible. No acredita una ejecución de esta migración.
- **Caso**: TC-CS-003; **Tipo**: negativo; **RF**: RF-CS-001; **Descripción**: falla con error accionable cuando el worker no esta disponible
  - RF-CS-001 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: falla con error accionable cuando el worker no esta disponible. No acredita una ejecución de esta migración.
- **Caso**: TC-CS-004; **Tipo**: positivo; **RF**: RF-CS-001; **Descripción**: `nav context` devuelve `slice_text` y metadatos semánticos en la misma respuesta
  - RF-CS-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: `nav context` devuelve `slice_text` y metadatos semánticos en la misma respuesta. No acredita una ejecución de esta migración.
- **Caso**: TC-CS-005; **Tipo**: positivo; **RF**: RF-CS-001; **Descripción**: si Roslyn no puede enriquecer pero el archivo existe, el core devuelve el slice con warning accionable
  - RF-CS-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: si Roslyn no puede enriquecer pero el archivo existe, el core devuelve el slice con warning accionable. No acredita una ejecución de esta migración.
- **Caso**: TC-CS-006; **Tipo**: positivo; **RF**: RF-CS-001; **Descripción**: si el primer candidato Roslyn falla por bootstrap, el core reintenta una sola vez con el siguiente sin reprobe en cascada
  - RF-CS-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: si el primer candidato Roslyn falla por bootstrap, el core reintenta una sola vez con el siguiente sin reprobe en cascada. No acredita una ejecución de esta migración.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-CS
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-CS-001.md
exports:
  - TP-CS
agent_must_read:
  - .docs/wiki/06_tests/TP-CS.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-CS.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-CS-001.md
  - internal/
verify:
  - .docs/wiki/06_tests/TP-CS.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - manual_evidence_pending_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-CS.md
  - .docs/wiki/05_requirements/RF-CS-001.md
```

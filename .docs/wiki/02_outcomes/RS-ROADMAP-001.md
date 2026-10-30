---
wiki_contract: WIKI-CONTRACT-v1
id: RS-ROADMAP-001
kind: outcome
status: active
title: "Consultar prioridades locales por horizonte sin alterar el canon"
---

## Resultado observable
La persona o agente consulta el respaldo local de prioridades como nodos en tres horizontes, con estado y dependencias, sin convertirlo en requisitos ni en gates de cierre.

## Beneficiario
Personas y agentes que navegan una wiki con mi-lsp.

## Criterios de éxito
- La respuesta distingue explícitamente el roadmap no canónico.
- La vista solicitada es efímera y muestra horizontes, estados y dependencias.
- La ausencia del respaldo produce una proyección vacía; los errores documentales se informan como avisos no bloqueantes. Un fallo operativo de workspace/base no se convierte en éxito vacío.

## Fuera de alcance del RS
Sincronizar un tracker remoto, crear o activar RS desde el roadmap, persistir Mermaid y planificar fechas.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RS-ROADMAP-001
kind: outcome
audience: dual
imports:
  - .docs/wiki/00_governance.md
exports:
  - RS-ROADMAP-001
agent_must_read:
  - .docs/wiki/02_outcomes/RS-ROADMAP-001.md
agent_may_edit:
  - .docs/wiki/02_outcomes/RS-ROADMAP-001.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-ROADMAP-001.md
verify:
  - "Lectura/cotejo: contrastar no-canonicidad, horizontes, estados, dependencias y avisos no bloqueantes con RF-ROADMAP-001 y FL-ROADMAP-01; cotejar criterios y disposiciones con TP-ROADMAP-001 sin afirmar que sus casos fueron ejecutados."
stop_if:
  - harness_verdict=BLOCKED
  - outcome_status_or_acceptance_would_change
evidence:
  - .docs/wiki/05_requirements/RF-ROADMAP-001.md
  - .docs/wiki/04_flows/FL-ROADMAP-01.md
  - .docs/wiki/06_tests/TP-ROADMAP-001.md
  - internal/model/wiki_roadmap.go
  - internal/service/wiki_roadmap.go
```

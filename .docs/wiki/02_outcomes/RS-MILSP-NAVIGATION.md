---
wiki_contract: WIKI-CONTRACT-v1
id: RS-MILSP-NAVIGATION
kind: outcome
status: active
title: Navegación documental y de código compacta y confiable
---

## Resultado observable

Un desarrollador o agente encuentra el propietario documental y la evidencia de código de una pregunta sin confundir menciones, índices derivados ni documentos de apoyo con autoridad. Search, inventory, route, trace, pack y map conservan sus contratos y sus límites.

## Beneficiario

Desarrolladores, skills, agentes y mantenedores de wikis locales o vinculadas por raíces portables.

## Criterios de éxito

- La selección explícita falla de forma cerrada y la salida identifica el workspace.
- Las consultas conservan orden, presupuesto, warnings, omisiones y backend visibles.
- Route/pack conservan el ancla gobernada aun sin indexar, con read-model relativo al canon.
- El fan-out intra-máquina conserva cada workspace; no crea un super-pack ni promete federación entre máquinas.
- Recall opcional descubre candidatos; no reemplaza el documento canónico ni introduce un proveedor oculto.

## Fuera de alcance del RS

Edición semántica automática, autoridad de negocio, MCP persistente propio y orquestación cross-máquina.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RS-MILSP-NAVIGATION
kind: outcome
audience: dual
imports:
  - .docs/wiki/00_governance.md
exports:
  - RS-MILSP-NAVIGATION
agent_must_read:
  - .docs/wiki/02_outcomes/RS-MILSP-NAVIGATION.md
agent_may_edit:
  - .docs/wiki/02_outcomes/RS-MILSP-NAVIGATION.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/
  - .docs/wiki/07_technical/contracts/
verify:
  - "Lectura/cotejo: cotejar cada resultado de búsqueda, route/trace/pack/map/root, límites y fallback con RF-WIKI-001/003/004/005/006/007, RF-QRY-023, RF-WKS-008, FL-WIKI-01 y FL-QRY-01; tratar TP-WIKI y los planes citados como planes, no como ejecución."
stop_if:
  - harness_verdict=BLOCKED
  - outcome_status_or_acceptance_would_change
evidence:
  - .docs/wiki/05_requirements/RF-WIKI-001.md
  - .docs/wiki/05_requirements/RF-WIKI-003.md
  - .docs/wiki/05_requirements/RF-WIKI-004.md
  - .docs/wiki/05_requirements/RF-WIKI-005.md
  - .docs/wiki/05_requirements/RF-WIKI-006.md
  - .docs/wiki/05_requirements/RF-WIKI-007.md
  - .docs/wiki/05_requirements/RF-QRY-023.md
  - .docs/wiki/05_requirements/RF-WKS-008.md
  - .docs/wiki/04_flows/FL-WIKI-01.md
  - .docs/wiki/04_flows/FL-QRY-01.md
  - .docs/wiki/06_tests/TP-WIKI.md
```

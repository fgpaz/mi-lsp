---
wiki_contract: WIKI-CONTRACT-v1
id: RS-MILSP-GRAPH
kind: outcome
status: active
title: Catálogo y grafo derivados explicables y reproducibles
---

## Resultado observable

El catálogo documental y de símbolos, las generaciones de grafo y sus consultas permiten explicar identidad, procedencia e impacto sin invertir la autoridad de la wiki ni mezclar publicaciones parciales.

## Beneficiario

Agentes, desarrolladores y responsables de los backends semánticos.

## Criterios de éxito

- NodeKey y GraphGeneration conservan los contratos de identidad y publicación.
- La publicación respeta ownership, fencing, cancelación y snapshot previo.
- Query, explain e impact mantienen presupuestos y omisiones explícitas.
- El modelo semántico permanece separado del almacenamiento físico y los caches son derivados.
- Las extensiones MILX-v1 siguen aisladas; Kotlin experimental no obtiene promoción por una decisión documental.

## Fuera de alcance del RS

Certificar oráculos pendientes, modificar almacenes durante una consulta o conceder admisión runtime desde un pin SOURCE.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RS-MILSP-GRAPH
kind: outcome
audience: dual
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/04_flows/FL-GPH-01.md
exports:
  - RS-MILSP-GRAPH
agent_must_read:
  - .docs/wiki/02_outcomes/RS-MILSP-GRAPH.md
agent_may_edit:
  - .docs/wiki/02_outcomes/RS-MILSP-GRAPH.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-GPH-008.md
  - .docs/wiki/06_tests/TP-GPH.md
verify:
  - "Lectura/cotejo: contrastar identidad/publicación, ownership, cancelación, snapshots y omisiones con RF-GPH-001, RF-GPH-008 y FL-GPH-01/02; revisar TP-GPH como plan y mantener Kotlin experimental y sus oráculos pendientes sin afirmar ejecución o promoción."
stop_if:
  - harness_verdict=BLOCKED
  - outcome_status_or_acceptance_would_change
  - pending_oracles_are_presented_as_passed
evidence:
  - .docs/wiki/05_requirements/RF-GPH-001.md
  - .docs/wiki/05_requirements/RF-GPH-008.md
  - .docs/wiki/04_flows/FL-GPH-01.md
  - .docs/wiki/04_flows/FL-GPH-02.md
  - .docs/wiki/06_tests/TP-GPH.md
  - internal/model/graph.go
  - internal/model/graph_observation.go
  - internal/indexer/graph_pipeline.go
```

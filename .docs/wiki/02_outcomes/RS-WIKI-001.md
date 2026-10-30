---
wiki_contract: WIKI-CONTRACT-v1
id: RS-WIKI-001
kind: outcome
status: active
title: "Recorrer la wiki y el código en una generación, respetando su idioma"
---

## Resultado observable
El consumidor recorre promesas, requisitos, pruebas y código sin abrir documentos para reconstruir relaciones. Los títulos y criterios se interpretan según el idioma declarado; los diagramas ausentes se avisan sin bloquear.

## Beneficiario
Agentes y personas que usan la CLI o el proveedor de mi-lsp desde codemode.

## Criterios de éxito
- Vecinos, caminos, impacto y trace consumen el grafo generacional wiki+código.
- La API devuelve JSON tipado, paginación por cursor y Mermaid efímero.
- validate-contract y matrix reconocen es/en; una declaración inválida no se traduce silenciosamente.

## Fuera de alcance del RS
Otro almacén de relaciones, lógica de dominio en mi-mcp, modelos de IA, gates de diagramas y sincronización de trackers. El respaldo no canónico del roadmap se rige por RS-ROADMAP-001.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RS-WIKI-001
kind: outcome
audience: dual
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/02_outcomes/RS-ROADMAP-001.md
exports:
  - RS-WIKI-001
agent_must_read:
  - .docs/wiki/02_outcomes/RS-WIKI-001.md
agent_may_edit:
  - .docs/wiki/02_outcomes/RS-WIKI-001.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-WIKI-001.md
verify:
  - "Lectura/cotejo: contrastar navegación, ownership, paginación/Mermaid e idioma declarado con RF-WIKI-001/002/003/004/005/006/007 y RF-WGRAPH-002; cotejar disposiciones con TP-WIKI-001/002 sin inferir ejecución."
stop_if:
  - harness_verdict=BLOCKED
  - outcome_status_or_acceptance_would_change
evidence:
  - .docs/wiki/05_requirements/RF-WIKI-001.md
  - .docs/wiki/05_requirements/RF-WIKI-002.md
  - .docs/wiki/05_requirements/RF-WIKI-003.md
  - .docs/wiki/05_requirements/RF-WIKI-004.md
  - .docs/wiki/05_requirements/RF-WIKI-005.md
  - .docs/wiki/05_requirements/RF-WIKI-006.md
  - .docs/wiki/05_requirements/RF-WIKI-007.md
  - .docs/wiki/05_requirements/RF-WGRAPH-002.md
  - .docs/wiki/06_tests/TP-WIKI-001.md
  - .docs/wiki/06_tests/TP-WIKI-002.md
  - internal/service/wiki_graph.go
  - internal/model/wiki_graph.go
```

---
wiki_contract: WIKI-CONTRACT-v1
id: TP-ROADMAP-001
kind: test
status: active
title: "Comprobación acotada de roadmap local"
links:
  verifies: [RF-ROADMAP-001]
---

## Alcance
Comportamiento retrospectivo publicado en 9372f01 y disponible en v0.14.4. Casos manuales reproducibles con CLI real, fixture aislado fuera del repo/wiki de producto y output JSON. No se añaden unit tests ni se reejecuta la suite completa. Evidencia de ejecución en trace.json del cierre; este plan describe la receta, no inventa PASS históricos.

## Casos (cada caso cita el número del criterio RF, con disposición y evidencia esperada)
- Criterio 1 — disposición: manual; evidencia: ayuda de nav wiki graph, manifiesto de proveedor y respuesta JSON con roadmap.canonical=false.
- Criterio 2 — disposición: manual; evidencia: fixture con tabla id/rs/horizon/status/depends_on/title y respuesta JSON que conserva texto libre y entradas.
- Criterio 3 — disposición: manual; evidencia: nav wiki graph --mode roadmap --render mermaid sobre fixture aislado; source contiene tres subgraph, cuatro classDef y flechas; no se crea archivo Mermaid.
- Criterio 4 — disposición: manual; evidencia: fixture con horizonte/estado inválidos, dependencia desconocida y ciclo; roadmap_warnings muestra códigos tipados y blocking=false.
- Criterio 5 — disposición: manual; evidencia: fixture indexado con un RS existente y referencia RS inexistente; enlace derived roadmap_for solo al existente y aviso roadmap_rs_missing con grafo current.
- Criterio 6 — disposición: manual; evidencia: misma consulta en fixture sin respaldo; entries vacío y ninguna escritura documental.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-ROADMAP-001
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/02_outcomes/RS-ROADMAP-001.md
exports:
  - TP-ROADMAP-001
agent_must_read:
  - .docs/wiki/06_tests/TP-ROADMAP-001.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-ROADMAP-001.md
agent_must_not_edit:
  - .docs/wiki/02_outcomes/RS-ROADMAP-001.md
  - .docs/wiki/05_requirements/RF-ROADMAP-001.md
verify:
  - .docs/wiki/06_tests/TP-ROADMAP-001.md#casos-cada-caso-cita-el-numero-del-criterio-rf-con-disposicion-y-evidencia-esperada
stop_if:
  - harness_verdict=BLOCKED
  - manual_evidence_pending_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-ROADMAP-001.md
  - .docs/wiki/05_requirements/RF-ROADMAP-001.md
```

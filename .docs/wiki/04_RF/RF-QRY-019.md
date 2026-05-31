---
id: RF-QRY-019
title: Inventariar evidencia operacional preview-first con nav evidence inventory
implements:
  - internal/cli/nav.go
  - internal/cli/root.go
  - internal/service/app.go
  - internal/service/evidence_inventory.go
tests:
  - internal/cli/nav_test.go
  - internal/cli/root_test.go
  - internal/service/evidence_inventory_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-019"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-QRY-01]]'
  - '[[CT-NAV-EVIDENCE]]'
exports:
  - 'RF-QRY-019'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-QRY-01.md
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-019.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-019.md
  - .docs/wiki/09_contratos/CT-NAV-EVIDENCE.md
```

# RF-QRY-019 - Inventariar evidencia operacional preview-first con nav evidence inventory

## Descripcion

Exponer `mi-lsp nav evidence inventory <query>` como superficie agent-first para decidir el camino de lectura mas barato y seguro antes de abrir evidencia pesada. El comando debe resolver primero el anchor wiki canonico y luego inventariar raices conocidas bajo `.docs/auditoria` y `.docs/raw` usando solo metadata.

## Actor principal

Skill / Agente / CLI

## FL origen

FL-QRY-01

## Estado

implemented

## TP asociado

TP-QRY

## Entradas

- `<query>`: descripcion de tarea, evidencia, decision o reentry que el agente intenta resolver.
- `--workspace <alias>`: workspace a inspeccionar.
- `--full`: expande presupuesto de inventario sin cambiar autoridad.
- `--format compact|toon|json|yaml|text`: formato de salida.

## Salida

El envelope debe usar `backend=evidence.inventory`, `mode=preview|full`, y `items[0]` con:

- `canonical`: anchor y mini pack canonico wiki-first.
- `recommended_read_path`: `route`, `wiki_search`, `wiki_pack`, `multi_read`, `manifest_verdict`, `targeted_raw` o `full_raw`.
- `context_loading_profile`: `CL0_NONE|CL1_EXACT|CL2_OWNER_PACK|CL3_SUBSYSTEM|CL4_FULL_RUNTIME`.
- `evidence_loading_profile`: `EL0_NONE|EL1_MANIFEST_VERDICT|EL2_SUMMARY_ASSERTIONS|EL3_TARGETED_RAW|EL4_FULL_RAW`.
- `evidence_roots`: raices de evidencia con `summary_first`, `artifacts`, `heavy_artifacts`, `authority`, `next_queries` y conteos de archivos/bytes.

## Reglas

- El comando no depende del daemon y debe funcionar en camino directo.
- La wiki canonica siempre precede a `.docs/raw` y `.docs/auditoria` como autoridad de tarea.
- `manifest.yaml`, `verdict.md`, `issues.yaml`, assertions, summaries y hashes se recomiendan antes que `turns`, `logs`, `screenshots`, prompts o planes raw.
- Prompts, planes historicos, turns, logs y screenshots se clasifican como `evidence_not_canon`; no son plantillas ni fuente de verdad funcional.
- El output no puede incluir cuerpos de prompts, transcripciones, logs, OCR de screenshots, secretos, emails ni PHI.
- Las rutas se reportan relativas al workspace y no se sigue evidencia fuera del root.
- Si el scan excede presupuesto, devuelve `truncated=true` y `continuation` hacia la misma query con `--full`.

## Data model

- `QueryEnvelope`
- `RouteCanonicalLane`
- `EvidenceInventoryResult`
- `EvidenceInventoryRoot`
- `EvidenceArtifactStats`

## Trazabilidad de tests

- Positivo: `TP-QRY / TC-QRY-135`
- Positivo: `TP-QRY / TC-QRY-136`
- Positivo: `TP-QRY / TC-QRY-137`
- Negativo: `TP-QRY / TC-QRY-138`
- Positivo: `TP-QRY / TC-QRY-139`

## Fuera de alcance

- Borrar, mover, compactar o reescribir evidencia.
- Leer o resumir contenido raw por defecto.
- Hacer `workspace status --full` como camino default de reentry.
- Probar cumplimiento funcional; el inventario orienta lectura, no cierra validacion.

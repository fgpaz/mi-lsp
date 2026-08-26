---
id: RF-DEMO-LEGACY
title: Demo legacy retirado
lifecycle: retired
superseded_by: RF-DEMO-001
---

```toon
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-DEMO-LEGACY"
block_id: "RF-DEMO-LEGACY.requirement_core"
kind: "req"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
exports:
  - 'RF-DEMO-LEGACY'
artifact_bindings:
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/service.mjs
    target_symbol: legacyRun
    doc_lifecycle: retired
    superseded_by: RF-DEMO-001
```

# RF-DEMO-LEGACY — Demo legacy retirado

## Descripcion

Documento de version antigua, retirado y reemplazado por `RF-DEMO-001`.

## Estado

retired

## Superseded by

- RF-DEMO-001

## Comportamiento esperado

- No aparece en busquedas activas por defecto.
- Busqueda explicita por ID antiguo (`RF-DEMO-LEGACY`) devuelve la ficha con `lifecycle=retired` y `superseded_by=RF-DEMO-001` (redirect historico).
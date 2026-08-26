---
id: RF-DEMO-PLANNED
title: Binding planificado futuro
---

```toon
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-DEMO-PLANNED"
block_id: "RF-DEMO-PLANNED.requirement_core"
kind: "req"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
exports:
  - 'RF-DEMO-PLANNED'
artifact_bindings:
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/future.mjs
    target_symbol: futureFunction
    binding_status: planned
```

# RF-DEMO-PLANNED — Binding planificado

## Descripcion

Documento que declara un binding futuro que todavia no es navegable. Los bindings con `binding_status: planned` no crean aristas navegables ni se promueven a `direct_code`.

## Estado

planned

## Comportamiento esperado

- El binding se almacena pero no se presenta como resultado de navegacion.
- `artifact_bindings` con `binding_status: planned` no se promueven a `direct_code`; no se siguen aristas desde aqui.
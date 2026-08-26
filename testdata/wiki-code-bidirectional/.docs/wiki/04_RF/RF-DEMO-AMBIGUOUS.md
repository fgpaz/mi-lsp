---
id: RF-DEMO-AMBIGUOUS
title: Caso de simbolos ambiguo y path faltante
---

```toon
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-DEMO-AMBIGUOUS"
block_id: "RF-DEMO-AMBIGUOUS.requirement_core"
kind: "req"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
exports:
  - 'RF-DEMO-AMBIGUOUS'
artifact_bindings:
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/ambiguous-a.mjs
    target_symbol: resolveConflict
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/ambiguous-b.mjs
    target_symbol: resolveConflict
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/does-not-exist.mjs
    target_symbol: missingSymbol
```

# RF-DEMO-AMBIGUOUS — Ambiguedad y path faltante

## Descripcion

Documento que declara bindings ambiguos (dos modulos con el mismo simbolo) y un path inexistente para validar la deteccion de casos negativos.

## Comportamiento esperado

1. `src/demo/ambiguous-a.mjs` y `src/demo/ambiguous-b.mjs` exportan `resolveConflict`; el resolver retorna candidatos bounded, no un unico resultado.
2. `src/demo/does-not-exist.mjs` no existe; el resolver retorna `missing_path`, no un fallback textual.
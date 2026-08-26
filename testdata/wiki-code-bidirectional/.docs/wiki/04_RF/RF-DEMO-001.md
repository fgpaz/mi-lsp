---
id: RF-DEMO-001
title: Demostración de navegación bidireccional wiki ↔ código
---

```toon
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-DEMO-001"
block_id: "RF-DEMO-001.requirement_core"
kind: "req"
audience: "engine"
imports:
  - '[[00_gobierno_documental]]'
exports:
  - 'RF-DEMO-001'
artifact_bindings:
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/service.mjs
    target_symbol: runDemo
  - target_kind: file
    relation: operates
    role: compiler
    target_path: src/demo/service.mjs
  - target_kind: test
    relation: tests
    role: test
    target_path: test/demo/service.test.mjs
```

# RF-DEMO-001 — Demostración de navegación bidireccional wiki ↔ código

## Descripcion

Ejemplo minimo que declara bindings wiki-codigo para que el parser, store, freshness y reverse lookup verifiquen el flujo bidireccional completo.

## Actor principal

Agente interno / motor de navegacion

## Estado

active

## FL origen

FL-DEMO-001

## Comportamiento esperado

1. El parser lee `RF-DEMO-001` y extrae `artifact_bindings` con target_path y target_symbol.
2. El store indexa las aristas wiki → codigo y codigo → wiki.
3. La navegacion wiki → codigo retorna `direct_code` con `resolved_symbol` para `service.mjs::runDemo`.
4. La navegacion codigo → wiki retorna `RF-DEMO-001` como documento de proveniencia.

## Invariantes

- `artifact_bindings` es la unica forma canonica para bindings de este documento.
- El binding de `target_kind: symbol` con `target_symbol` produce `resolved_symbol`, no `resolved_file`.
- El binding de `target_kind: test` se clasifica como `tests` y no como `direct_code`.
- El binding de `target_kind: file` con `relation: operates` se clasifica como supporting_code.
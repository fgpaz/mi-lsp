---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-015
kind: requirement
status: active
title: RF-QRY-015 - Reutilizar el motor de routing canonico internamente desde nav ask y nav pack
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/ask.go
    role: behavior
  - path: internal/service/pack.go
    role: behavior
  - path: internal/service/route.go
    role: behavior
---

## Propósito

RF-QRY-015 - Reutilizar el motor de routing canónico internamente desde nav ask y nav pack

### Descripción

`nav ask` y `nav pack` deben reutilizar el mismo motor de routing canónico que `nav route` para la selección docs-first del documento primario, en vez de duplicar la logica de ranking y seleccion. Ese motor comparte también el scorer owner-aware y el override diagnóstico `MI_LSP_DOC_RANKING=legacy`.

### Actor principal

Core/Service (interno)

### FL origen

FL-QRY-01

## Comportamiento ordenado

### Estado

implemented

### TP asociado

TP-QRY

### Comportamiento esperado

1. `nav ask` llama al route core para obtener el `anchor_doc + canonical lane` y luego agrega evidencia de código sobre el resultado
2. `nav pack` llama al route core para obtener el `primary_doc + canonical lane` y luego construye el reading pack canónico completo sobre ese anchor
3. La semántica de `nav ask` y `nav pack` queda preservada externamente; el cambio es interno

### Invariantes

- El route core es la única fuente de verdad para selección docs-first
- El scorer owner-aware compartido es la única fuente de verdad para priorizar docs canónicos positivos sobre `README`/generic
- Si el índice de docs esta vacio pero existe wiki canónica, Tier 1 produce un governed anchor en vez de README.md
- La governance gate se ejecuta una vez antes de cualquier routing
- Discovery del route core nunca sobreescribe la canonical lane usada por ask o pack

### Impacto en tests

- Los tests de ask y pack siguen pasando con la nueva semántica
- `TestNavAskFallsBackWhenDocsIndexIsEmpty` debe actualizarse: cuando el índice esta vacio pero existe gobernanza valida, el anchor es el primer doc canónico del perfil activo, no README.md
- `TestNavPackWarnsWhenCanonicalWikiExistsButDocsAreNotIndexed` puede necesitar revision similar

### Notas de implementacion

- Route core compartido: `internal/service/route.go` (`resolveCanonicalRoute`)
- Fallback Tier 1 en ask: `internal/service/ask.go` — cuando el índice de docs esta vacio pero existe wiki canónica, usa Tier 1 canonical en vez de README.md
- Fallback Tier 1 en pack: `internal/service/pack.go` — misma semántica que ask para índice vacio/stale
- El test `TestNavAskFallsBackWhenDocsIndexIsEmpty` fue actualizado: el anchor es el primer doc canónico del perfil activo, no README.md

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### Modelo de datos
`RouteResult`, `RouteCanonicalLane`, `DocsReadProfile`, `AskResult`, `PackResult`

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. `nav ask` llama al route core para obtener el `anchor_doc + canonical lane` y luego agrega evidencia de código sobre el resultado
2. `nav pack` llama al route core para obtener el `primary_doc + canonical lane` y luego construye el reading pack canónico completo sobre ese anchor
3. La semántica de `nav ask` y `nav pack` queda preservada externamente; el cambio es interno

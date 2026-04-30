# RF-QRY-015 - Reutilizar el motor de routing canonico internamente desde nav ask y nav pack

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-015"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-015]]'
exports:
  - 'RF-QRY-015'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-015.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-015.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-015.md
```

## Descripcion

`nav ask` y `nav pack` deben reutilizar el mismo motor de routing canonico que `nav route` para la seleccion docs-first del documento primario, en vez de duplicar la logica de ranking y seleccion. Ese motor comparte tambien el scorer owner-aware y el override diagnostico `MI_LSP_DOC_RANKING=legacy`.

## Actor principal

Core/Service (interno)

## FL origen

FL-QRY-01

## Estado

implemented

## TP asociado

TP-QRY

## Comportamiento esperado

1. `nav ask` llama al route core para obtener el `anchor_doc + canonical lane` y luego agrega evidencia de codigo sobre el resultado
2. `nav pack` llama al route core para obtener el `primary_doc + canonical lane` y luego construye el reading pack canonico completo sobre ese anchor
3. La semantica de `nav ask` y `nav pack` queda preservada externamente; el cambio es interno

## Invariantes

- El route core es la unica fuente de verdad para seleccion docs-first
- El scorer owner-aware compartido es la unica fuente de verdad para priorizar docs canonicos positivos sobre `README`/generic
- Si el indice de docs esta vacio pero existe wiki canonica, Tier 1 produce un governed anchor en vez de README.md
- La governance gate se ejecuta una vez antes de cualquier routing
- Discovery del route core nunca sobreescribe la canonical lane usada por ask o pack

## Impacto en tests

- Los tests de ask y pack siguen pasando con la nueva semantica
- `TestNavAskFallsBackWhenDocsIndexIsEmpty` debe actualizarse: cuando el indice esta vacio pero existe gobernanza valida, el anchor es el primer doc canonico del perfil activo, no README.md
- `TestNavPackWarnsWhenCanonicalWikiExistsButDocsAreNotIndexed` puede necesitar revision similar

## Data model

`RouteResult`, `RouteCanonicalLane`, `DocsReadProfile`, `AskResult`, `PackResult`

## Notas de implementacion

- Route core compartido: `internal/service/route.go` (`resolveCanonicalRoute`)
- Fallback Tier 1 en ask: `internal/service/ask.go` — cuando el indice de docs esta vacio pero existe wiki canonica, usa Tier 1 canonical en vez de README.md
- Fallback Tier 1 en pack: `internal/service/pack.go` — misma semantica que ask para indice vacio/stale
- El test `TestNavAskFallsBackWhenDocsIndexIsEmpty` fue actualizado: el anchor es el primer doc canonico del perfil activo, no README.md

---
id: RF-QRY-025
doc_id: RF-QRY-025
kind: requirement
audience: llm-first
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
wiki_graph_protocol: SDD-WIKI-GRAPH-v1
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-Q-V1]]'
  - '[[CT-NAV-INTENT]]'
  - '[[RF-QRY-004]]'
  - '[[TP-QRY]]'
exports:
  - 'RF-QRY-025'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/04_RF/RF-QRY-004.md
  - .docs/wiki/04_RF/RF-QRY-025.md
  - .docs/wiki/06_pruebas/TP-QRY.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-025.md
  - .docs/wiki/06_pruebas/TP-QRY.md
  - .docs/wiki/06_matriz_pruebas_RF.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-025.md
  - .docs/wiki/09_contratos/CT-Q-V1.md
  - .docs/wiki/09_contratos/CT-NAV-INTENT.md
  - .docs/wiki/06_pruebas/TP-QRY.md
---

# Generación publicada, mapa de decisiones y marca de sensibilidad

```toon
block_id: rf-qry-025-published-snapshot
kind: normative
source_of_truth: this
status: implemented
contract: q-v1
purpose: cachear contra la generación publicada y exponer la marca de visibilidad sin filtrar
surfaces:
  - mi-lsp q
  - mi-lsp nav intent
  - mi-lsp nav multi-read
generation_id:
  source: snapshot publicado del workspace
  keys: [last_index, active_catalog, active_docs, active_memory]
  same_value: las tres superficies publican el mismo snapshot cuando leen la misma generación
  cache: el consumidor puede usar generation_id como clave de caché
  preserve: si el envelope ya trae generation_id, no se reescribe
decision_map:
  surface: mi-lsp nav intent
  trigger: la pregunta es exactamente un id D-<dígitos>
  map: wiki/90-mapa-ids.md del workspace
  before_search: true
  origin: decision-map
  range: segunda celda de la fila del id, archivo:inicio-fin
  miss: sin mapa, mapa ilegible o id ausente, sigue el intent ordinario
sensibilidad:
  field: sensibilidad
  source: frontmatter YAML del archivo del resultado
  empty: cadena vacía si no hay marca, el archivo no se lee o el rango no tiene archivo
  filter: mi-lsp no descarta ni reordena por esta marca; el filtro es de mi-mcp
cases: [TC-QRY-205, TC-QRY-206, TC-QRY-207, TC-QRY-208]
```

`q`, `nav intent` y `nav multi-read` publican `generation_id` con el snapshot de la generación ya publicada (`last_index`, `active_catalog`, `active_docs`, `active_memory`). Sirve para cachear: dos lecturas de la misma generación devuelven el mismo valor. Si otra ruta ya puso `generation_id`, se conserva.

Cuando la pregunta de `nav intent` es un id de decisión (`D-056` y el resto de la forma `D-` más dígitos), mi-lsp lee `wiki/90-mapa-ids.md` del workspace y devuelve el rango de esa fila antes de buscar. El ítem lleva `origin=decision-map`, `result_kind=decision` y el texto del rango. Si no hay mapa, está vacío o el id no figura, el intent sigue su camino normal.

Cada resultado con archivo trae `sensibilidad` copiada del frontmatter. Sin marca, el campo va vacío. mi-lsp no filtra por esa marca.

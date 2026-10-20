---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-025
kind: requirement
status: active
title: Generación publicada, mapa de decisiones y marca de sensibilidad
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/ask.go
    role: behavior
---

## Propósito

Generación publicada, mapa de decisiones y marca de sensibilidad

## Comportamiento ordenado

- **Marcador estable**: rf-qry-025-published-snapshot
- **Estado del comportamiento**: implemented
- **contract**: q-v1
- **Propósito**: cachear contra la generación publicada y exponer la marca de visibilidad sin filtrar
- **surfaces**:
  - mi-lsp q
  - mi-lsp nav intent
  - mi-lsp nav multi-read
- **generation_id**:
  - **source**: snapshot publicado del workspace
  - **keys**:
    - last_index
    - active_catalog
    - active_docs
    - active_memory
  - **same_value**: las tres superficies publican el mismo snapshot cuando leen la misma generación
  - **cache**: el consumidor puede usar generation_id como clave de caché
  - **preserve**: si el envelope ya trae generation_id, no se reescribe
- **decision_map**:
  - **surface**: mi-lsp nav intent
  - **trigger**: la pregunta es exactamente un id D-<dígitos>
  - **map**: wiki/90-mapa-ids.md del workspace
  - **before_search**: true
  - **origin**: decision-map
  - **range**: segunda celda de la fila del id, archivo:inicio-fin
  - **miss**: sin mapa, mapa ilegible o id ausente, sigue el intent ordinario
- **sensibilidad**:
  - **field**: sensibilidad
  - **source**: frontmatter YAML del archivo del resultado
  - **empty**: cadena vacía si no hay marca, el archivo no se lee o el rango no tiene archivo
  - **filter**: mi-lsp no descarta ni reordena por esta marca; el filtro es de mi-mcp
- **Casos**:
  - TC-QRY-205
  - TC-QRY-206
  - TC-QRY-207
  - TC-QRY-208
- **Comprobación prevista**:
  - mi-lsp nav wiki validate-source --workspace <alias> --ids RF-QRY-025 --format toon
  - go build ./cmd/mi-lsp ./internal/service
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-QRY-025.md
  - .docs/wiki/07_technical/contracts/CT-Q-V1.md
  - .docs/wiki/07_technical/contracts/CT-NAV-INTENT.md
  - internal/service/ask.go

`q`, `nav intent` y `nav multi-read` publican `generation_id` con el snapshot de la generación ya publicada (`last_index`, `active_catalog`, `active_docs`, `active_memory`). Sirve para cachear: dos lecturas de la misma generación devuelven el mismo valor. Si otra ruta ya puso `generation_id`, se conserva.

Cuando la pregunta de `nav intent` es un id de decisión (`D-056` y el resto de la forma `D-` más dígitos), mi-lsp lee `wiki/90-mapa-ids.md` del workspace y devuelve el rango de esa fila antes de buscar. El ítem lleva `origin=decision-map`, `result_kind=decision` y el texto del rango. Si no hay mapa, está vacío o el id no figura, el intent sigue su camino normal.

Cada resultado con archivo trae `sensibilidad` copiada del frontmatter. Sin marca, el campo va vacío. mi-lsp no filtra por esa marca.

## Superficies

- mi-lsp nav intent

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. `q`, `nav intent` y `nav multi-read` publican `generation_id` con el snapshot de la generación ya publicada (`last_index`, `active_catalog`, `active_docs`, `active_memory`). Sirve para cachear: dos lecturas de la misma generación devuelven el mismo valor. Si otra ruta ya puso `generation_id`, se conserva.
2. Cuando la pregunta de `nav intent` es un id de decisión (`D-056` y el resto de la forma `D-` más dígitos), mi-lsp lee `wiki/90-mapa-ids.md` del workspace y devuelve el rango de esa fila antes de buscar. El ítem lleva `origin=decision-map`, `result_kind=decision` y el texto del rango. Si no hay mapa, está vacío o el id no figura, el intent sigue su camino normal.
3. Cada resultado con archivo trae `sensibilidad` copiada del frontmatter. Sin marca, el campo va vacío. mi-lsp no filtra por esa marca.

---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-016
kind: requirement
status: active
title: RF-QRY-016 - Explorar la wiki con una superficie dedicada para agentes
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/axi_mode.go
    role: behavior
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/cli/root.go
    role: behavior
  - path: internal/docgraph/docgraph.go
    role: behavior
  - path: internal/model/types.go
    role: behavior
  - path: internal/model/wiki_code_context.go
    role: behavior
  - path: internal/service/app.go
    role: behavior
  - path: internal/service/ask.go
    role: behavior
  - path: internal/service/change_pack.go
    role: behavior
  - path: internal/service/coach.go
    role: behavior
  - path: internal/service/harness_validate.go
    role: behavior
  - path: internal/service/pack.go
    role: behavior
  - path: internal/service/prepare.go
    role: behavior
  - path: internal/service/related.go
    role: behavior
  - path: internal/service/route.go
    role: behavior
  - path: internal/service/search.go
    role: behavior
  - path: internal/service/source_validate.go
    role: behavior
  - path: internal/service/trace.go
    role: behavior
  - path: internal/service/wiki_compat.go
    role: behavior
  - path: internal/service/wiki_search.go
    role: behavior
  - path: internal/store/queries_docs.go
    role: behavior
  - path: internal/store/schema.go
    role: behavior
---

## Propósito

RF-QRY-016 - Explorar la wiki con una superficie dedicada para agentes

### Descripción

Exponer `mi-lsp nav wiki` como superficie publica orientada a exploracion documental guiada. La meta es que un agente pueda encontrar RS/RF/FL/TP/CT/TECH/DB, pasar de un candidato a pack/trace/multi-read/ask, validar readiness de contratos `SDD-HARNESS-v1`, y no romperse si todavía usa patrones antiguos como `nav ask --repo docs`.

### Actor principal

Usuario / Skill / Agente

### FL origen

FL-QRY-01

## Comportamiento ordenado

### Estado

implemented

### TP asociado

TP-QRY

### Comportamiento esperado

1. Ejecutar `mi-lsp nav wiki search <query> --workspace <alias> [--layer RS,RF,FL,TP,CT,TECH,DB] [--top N] [--offset N] [--include-content]`
2. Aplicar gate de gobernanza antes de buscar en la wiki
3. Si el docgraph esta vacio, devolver `backend=wiki.search`, `items=[]`, warning/hint de reindexado documental y no sugerir resultados inventados
4. Si hay docgraph, devolver candidatos con `doc_id`, `path`, `title`, `layer`, `family`, `stage`, `score`, `why`, `snippet/content` y `next_queries`
4.a. Cuando exista evidencia de ubicacion, incluir `line_start`/`line_end` o rangos equivalentes dentro de `snippet/content`, apuntando al `path` canónico del item
5. Ejecutar `nav wiki route`, `nav wiki pack` y `nav wiki trace` reutilizando la semántica de `nav route`, `nav pack` y `nav trace`
6. Aceptar `--repo` en `nav ask`, `nav route` y `nav pack` como compatibilidad guiada: se ignora para la lane documental y emite warning/hint hacia `nav wiki`
7. Clasificar `RS-*`, `.docs/wiki/02_resultados_soluciones_usuario.md` y `.docs/wiki/02_resultados/*.md` como `layer=RS`, `stage=outcome`, sin tratarlos como RF
8. Ejecutar `mi-lsp nav wiki validate-harness --workspace <alias>` como lectura directa que aplica gate de gobernanza, reutiliza `DocRecord` y emite `harness_protocol`, `harness_readiness`, `harness_verdict`, blockers/warnings, conteos de contratos/links y evidencia requerida/encontrada
9. Devolver `BLOCKED` si faltan contratos `SDD-HARNESS-v1`, hay imports/Obsidian links rotos, conflictos de edicion o docs `llm-first`/unknown sin `verify`, `stop_if` o `evidence`
10. Ejecutar `mi-lsp nav wiki validate-source --workspace <alias>` como lectura directa que valida solo artefactos que declaran `wiki_source_protocol: SDD-WIKI-SOURCE-v1`
11. Indexar bloques `toon` normativos y records referenciables en tablas typed (`doc_source_blocks`, `doc_source_records`) y emitir menciones compatibles para `source_protocol`, `doc_id`, `block_id`, `record_id`, imports y exports
12. Resolver busquedas exactas por `doc_id`, `block_id` y `record_id` desde las tablas typed antes del ranking textual normal
13. Para harnesses que consultan identificadores C#/TS/Python/Go, la guia agent-first de busqueda debe recomendar `nav find --exact`, luego `nav related`, y recien después `nav context` o `nav search` textual según la evidencia disponible.
14. Si `nav search` recibe un literal symbol-like, debe emitir `coach.trigger=symbol_query_detected` y priorizar declaraciones/implementaciones fuente antes que docs, tests, backups o generados, sin ocultar esos matches secundarios.

### Invariantes

- `nav wiki search` no reimplementa la semántica de `route`, `pack` ni `trace`.
- Los filtros `--layer` son documentales, no selectores de repositorio.
- `RS/outcome` debe resolverse desde `governance.hierarchy[*].pack_stage` antes de caer a heuristicas por número de archivo.
- `--repo docs` no crea un repositorio documental virtual.
- La salida debe ser util para agentes: cada candidato expone proximos comandos concretos.
- La evidencia de línea es parte del contrato de navegacion: no debe inventarse y debe omitirse antes que apuntar a un rango incorrecto.
- La guia para agentes no debe convertir `nav search` en único siguiente paso cuando la query parece un identificador de codigo.
- `governance_blocked=true` corta la busqueda normal.
- `validate-harness` no crea un parser Markdown paralelo: reusa docgraph para inventario y abre los markdown gobernados solo para compilar contratos YAML.
- `human` y `dual` pueden declarar `verify`, `stop_if` o `evidence` vacios como warning no bloqueante; `llm-first` y `unknown` no pueden pasar sin esos campos.
- `validate-source` no bloquea documentos no migrados; falla cerrado solo cuando el documento declara `SDD-WIKI-SOURCE-v1`.
- Los bloques fuente normativos deben vivir en fences `toon` con `block_id`; las tablas Markdown normativas requieren excepcion explícita o audiencia `human`/`dual`.

### Notas de implementacion

- CLI: `internal/cli/nav.go` (`newNavWikiCommand`)
- Handler: `internal/service/wiki_search.go` (`wikiSearch`)
- Harness compiler: `internal/service/harness_validate.go` (`validateHarness`)
- Wiki Source compiler: `internal/service/source_validate.go` (`validateSource`)
- Source parser/storage: `internal/docgraph/docgraph.go`, `internal/store/queries_docs.go`
- Trace: `internal/service/trace.go`
- Compatibilidad `--repo`: `internal/service/wiki_compat.go`
- Contrato: `.docs/wiki/07_technical/contracts/CT-NAV-WIKI.md`

### Puente wiki ↔ código en superficies existentes

- **Marcador estable**: RF-QRY-016.live-wiki-code-bridge
- **Estado del comportamiento**: implemented_slice
- **surfaces**:
  - **nav.trace**: additive_wiki_code_context
  - **nav.wiki.trace**: additive_wiki_code_context
  - **nav.pack**: additive_wiki_code_context
  - **nav.wiki.pack**: additive_wiki_code_context
  - **nav.related**: additive_wiki_context_on_existing_neighborhood
  - **nav.neighbors**: additive_bounded_mixed_context
  - **nav.prepare**: reuses_existing_pack_anchor_and_bridge_context
  - **nav.change-pack**: additive_classification_and_next_queries
- **path_bearing_records**:
  - **bindings**:
    - doc_path
    - target_path
    - target_symbol
    - block_id
    - source_content_hash
  - **Evidencia referenciada**:
    - path
    - doc_path
    - source_doc
    - source_block
    - source_line
  - **reverse_wiki**:
    - doc_id
    - path
    - block_id
    - binding_ref
    - parents
  - **control**:
    - direction
    - freshness
    - omissions
    - continuation
    - next_cursor
    - truncated
- **semantics**:
  - **direction**:
    - wiki_to_code
    - code_to_wiki
  - **freshness**: bounded_fresh_per_domain
  - **read_your_writes**: request_scoped_overlay
  - **graph_support**: optional_and_current_only
- **compatibility**:
  - **existing_command_names**: preserved
  - **existing_fields**: preserved
  - **additive_nested_field**: wiki_code_context
  - **no_new_public_command**: true
  - **direct_daemon**: canonical_items_order_omissions_and_digest_match
  - **query_writes**: forbidden
- **authority**:
  - **canonical_wiki**: unchanged
  - **catalog_graph_sqlite**: derived_only
  - **raw_and_audit**: never_primary
- **Comprobación prevista**:
  - FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes)
  - wiki-code-bridge-runner/v1: PASS; inventory=37
- **Evidencia referenciada**:
  - internal/service/app.go
  - internal/service/trace.go
  - internal/service/pack.go
  - internal/service/related.go
  - internal/service/prepare.go
  - internal/service/change_pack.go
  - internal/model/wiki_code_context.go
  - .docs/wiki/06_tests/TP-QRY.md

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### Modelo de datos
`WikiSearchResult`, `HarnessValidationResult`, `WikiSourceValidationResult`, `DocRecord`, `DocSourceBlock`, `DocSourceRecord`, `DocsReadProfile`, `GovernanceStatus`, `TraceResult`, `QueryEnvelope`

## Errores

### Códigos de error

- `QRY_WIKI_QUERY_REQUIRED`
- `QRY_WIKI_WORKSPACE_NOT_FOUND`
- `QRY_WIKI_GOVERNANCE_BLOCKED`
- `QRY_WIKI_SOURCE_BLOCKED`

## Criterios de aceptación (numerados)

1. Ejecutar `mi-lsp nav wiki search <query> --workspace <alias> [--layer RS,RF,FL,TP,CT,TECH,DB] [--top N] [--offset N] [--include-content]`
2. Aplicar gate de gobernanza antes de buscar en la wiki
3. Si el docgraph esta vacio, devolver `backend=wiki.search`, `items=[]`, warning/hint de reindexado documental y no sugerir resultados inventados
4. Si hay docgraph, devolver candidatos con `doc_id`, `path`, `title`, `layer`, `family`, `stage`, `score`, `why`, `snippet/content` y `next_queries`
5. Ejecutar `nav wiki route`, `nav wiki pack` y `nav wiki trace` reutilizando la semántica de `nav route`, `nav pack` y `nav trace`
6. Aceptar `--repo` en `nav ask`, `nav route` y `nav pack` como compatibilidad guiada: se ignora para la lane documental y emite warning/hint hacia `nav wiki`
7. Clasificar `RS-*`, `.docs/wiki/02_resultados_soluciones_usuario.md` y `.docs/wiki/02_resultados/*.md` como `layer=RS`, `stage=outcome`, sin tratarlos como RF
8. Ejecutar `mi-lsp nav wiki validate-harness --workspace <alias>` como lectura directa que aplica gate de gobernanza, reutiliza `DocRecord` y emite `harness_protocol`, `harness_readiness`, `harness_verdict`, blockers/warnings, conteos de contratos/links y evidencia requerida/encontrada
9. Devolver `BLOCKED` si faltan contratos `SDD-HARNESS-v1`, hay imports/Obsidian links rotos, conflictos de edicion o docs `llm-first`/unknown sin `verify`, `stop_if` o `evidence`
10. Ejecutar `mi-lsp nav wiki validate-source --workspace <alias>` como lectura directa que valida solo artefactos que declaran `wiki_source_protocol: SDD-WIKI-SOURCE-v1`
11. Indexar bloques `toon` normativos y records referenciables en tablas typed (`doc_source_blocks`, `doc_source_records`) y emitir menciones compatibles para `source_protocol`, `doc_id`, `block_id`, `record_id`, imports y exports
12. Resolver busquedas exactas por `doc_id`, `block_id` y `record_id` desde las tablas typed antes del ranking textual normal
13. Para harnesses que consultan identificadores C#/TS/Python/Go, la guia agent-first de busqueda debe recomendar `nav find --exact`, luego `nav related`, y recien después `nav context` o `nav search` textual según la evidencia disponible.
14. Si `nav search` recibe un literal symbol-like, debe emitir `coach.trigger=symbol_query_detected` y priorizar declaraciones/implementaciones fuente antes que docs, tests, backups o generados, sin ocultar esos matches secundarios.

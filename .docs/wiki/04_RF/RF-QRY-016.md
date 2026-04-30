---
id: RF-QRY-016
title: Explorar la wiki con una superficie dedicada para agentes
implements:
  - internal/cli/nav.go
  - internal/cli/root.go
  - internal/cli/axi_mode.go
  - internal/model/types.go
  - internal/service/app.go
  - internal/service/harness_validate.go
  - internal/service/source_validate.go
  - internal/service/wiki_search.go
  - internal/service/wiki_compat.go
  - internal/service/ask.go
  - internal/service/route.go
  - internal/service/pack.go
  - internal/docgraph/docgraph.go
  - internal/store/schema.go
  - internal/store/queries_docs.go
tests:
  - internal/cli/nav_test.go
  - internal/cli/root_test.go
  - internal/output/formatter_test.go
  - internal/service/harness_validate_test.go
  - internal/service/source_validate_test.go
  - internal/service/wiki_search_test.go
  - internal/docgraph/docgraph_test.go
  - internal/store/docs_test.go
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-016"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-016]]'
exports:
  - 'RF-QRY-016'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-016.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-016.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-016.md
```

# RF-QRY-016 - Explorar la wiki con una superficie dedicada para agentes

## Descripcion

Exponer `mi-lsp nav wiki` como superficie publica orientada a exploracion documental guiada. La meta es que un agente pueda encontrar RS/RF/FL/TP/CT/TECH/DB, pasar de un candidato a pack/trace/multi-read/ask, validar readiness de contratos `SDD-HARNESS-v1`, y no romperse si todavia usa patrones antiguos como `nav ask --repo docs`.

## Actor principal

Usuario / Skill / Agente

## FL origen

FL-QRY-01

## Estado

implemented

## TP asociado

TP-QRY

## Comportamiento esperado

1. Ejecutar `mi-lsp nav wiki search <query> --workspace <alias> [--layer RS,RF,FL,TP,CT,TECH,DB] [--top N] [--offset N] [--include-content]`
2. Aplicar gate de gobernanza antes de buscar en la wiki
3. Si el docgraph esta vacio, devolver `backend=wiki.search`, `items=[]`, warning/hint de reindexado documental y no sugerir resultados inventados
4. Si hay docgraph, devolver candidatos con `doc_id`, `path`, `title`, `layer`, `family`, `stage`, `score`, `why`, `snippet/content` y `next_queries`
5. Ejecutar `nav wiki route`, `nav wiki pack` y `nav wiki trace` reutilizando la semantica de `nav route`, `nav pack` y `nav trace`
6. Aceptar `--repo` en `nav ask`, `nav route` y `nav pack` como compatibilidad guiada: se ignora para la lane documental y emite warning/hint hacia `nav wiki`
7. Clasificar `RS-*`, `.docs/wiki/02_resultados_soluciones_usuario.md` y `.docs/wiki/02_resultados/*.md` como `layer=RS`, `stage=outcome`, sin tratarlos como RF
8. Ejecutar `mi-lsp nav wiki validate-harness --workspace <alias>` como lectura directa que aplica gate de gobernanza, reutiliza `DocRecord` y emite `harness_protocol`, `harness_readiness`, `harness_verdict`, blockers/warnings, conteos de contratos/links y evidencia requerida/encontrada
9. Devolver `BLOCKED` si faltan contratos `SDD-HARNESS-v1`, hay imports/Obsidian links rotos, conflictos de edicion o docs `llm-first`/unknown sin `verify`, `stop_if` o `evidence`
10. Ejecutar `mi-lsp nav wiki validate-source --workspace <alias>` como lectura directa que valida solo artefactos que declaran `wiki_source_protocol: SDD-WIKI-SOURCE-v1`
11. Indexar bloques `toon` normativos y records referenciables en tablas typed (`doc_source_blocks`, `doc_source_records`) y emitir menciones compatibles para `source_protocol`, `doc_id`, `block_id`, `record_id`, imports y exports
12. Resolver busquedas exactas por `doc_id`, `block_id` y `record_id` desde las tablas typed antes del ranking textual normal

## Invariantes

- `nav wiki search` no reimplementa la semantica de `route`, `pack` ni `trace`.
- Los filtros `--layer` son documentales, no selectores de repositorio.
- `RS/outcome` debe resolverse desde `governance.hierarchy[*].pack_stage` antes de caer a heuristicas por numero de archivo.
- `--repo docs` no crea un repositorio documental virtual.
- La salida debe ser util para agentes: cada candidato expone proximos comandos concretos.
- `governance_blocked=true` corta la busqueda normal.
- `validate-harness` no crea un parser Markdown paralelo: reusa docgraph para inventario y abre los markdown gobernados solo para compilar contratos YAML.
- `human` y `dual` pueden declarar `verify`, `stop_if` o `evidence` vacios como warning no bloqueante; `llm-first` y `unknown` no pueden pasar sin esos campos.
- `validate-source` no bloquea documentos no migrados; falla cerrado solo cuando el documento declara `SDD-WIKI-SOURCE-v1`.
- Los bloques fuente normativos deben vivir en fences `toon` con `block_id`; las tablas Markdown normativas requieren excepcion explicita o audiencia `human`/`dual`.

## Data model

`WikiSearchResult`, `HarnessValidationResult`, `WikiSourceValidationResult`, `DocRecord`, `DocSourceBlock`, `DocSourceRecord`, `DocsReadProfile`, `GovernanceStatus`, `TraceResult`, `QueryEnvelope`

## Codigos de error

- `QRY_WIKI_QUERY_REQUIRED`
- `QRY_WIKI_WORKSPACE_NOT_FOUND`
- `QRY_WIKI_GOVERNANCE_BLOCKED`
- `QRY_WIKI_SOURCE_BLOCKED`

## Notas de implementacion

- CLI: `internal/cli/nav.go` (`newNavWikiCommand`)
- Handler: `internal/service/wiki_search.go` (`wikiSearch`)
- Harness compiler: `internal/service/harness_validate.go` (`validateHarness`)
- Wiki Source compiler: `internal/service/source_validate.go` (`validateSource`)
- Source parser/storage: `internal/docgraph/docgraph.go`, `internal/store/queries_docs.go`
- Trace: `internal/service/trace.go`
- Compatibilidad `--repo`: `internal/service/wiki_compat.go`
- Contrato: `.docs/wiki/09_contratos/CT-NAV-WIKI.md`

---
id: RF-QRY-016
title: Explorar la wiki con una superficie dedicada para agentes
implements:
  - internal/cli/nav.go
  - internal/cli/root.go
  - internal/cli/axi_mode.go
  - internal/model/types.go
  - internal/service/app.go
  - internal/service/wiki_search.go
  - internal/service/wiki_compat.go
  - internal/service/ask.go
  - internal/service/route.go
  - internal/service/pack.go
tests:
  - internal/cli/nav_test.go
  - internal/cli/root_test.go
  - internal/service/wiki_search_test.go
---

# RF-QRY-016 - Explorar la wiki con una superficie dedicada para agentes

## Descripcion

Exponer `mi-lsp nav wiki` como superficie publica orientada a exploracion documental guiada. La meta es que un agente pueda encontrar RS/RF/FL/TP/CT/TECH/DB, pasar de un candidato a pack/trace/multi-read/ask, y no romperse si todavia usa patrones antiguos como `nav ask --repo docs`.

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

## Invariantes

- `nav wiki search` no reimplementa la semantica de `route`, `pack` ni `trace`.
- Los filtros `--layer` son documentales, no selectores de repositorio.
- `RS/outcome` debe resolverse desde `governance.hierarchy[*].pack_stage` antes de caer a heuristicas por numero de archivo.
- `--repo docs` no crea un repositorio documental virtual.
- La salida debe ser util para agentes: cada candidato expone proximos comandos concretos.
- `governance_blocked=true` corta la busqueda normal.

## Data model

`WikiSearchResult`, `DocRecord`, `DocsReadProfile`, `GovernanceStatus`, `TraceResult`, `QueryEnvelope`

## Codigos de error

- `QRY_WIKI_QUERY_REQUIRED`
- `QRY_WIKI_WORKSPACE_NOT_FOUND`
- `QRY_WIKI_GOVERNANCE_BLOCKED`

## Notas de implementacion

- CLI: `internal/cli/nav.go` (`newNavWikiCommand`)
- Handler: `internal/service/wiki_search.go` (`wikiSearch`)
- Trace: `internal/service/trace.go`
- Compatibilidad `--repo`: `internal/service/wiki_compat.go`
- Contrato: `.docs/wiki/09_contratos/CT-NAV-WIKI.md`

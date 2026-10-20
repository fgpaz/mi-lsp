---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WGRAPH-001
kind: requirement
status: active
title: "Consultar el grafo único wiki+código"
links:
  implements: [FL-WGRAPH-001]
  code:
    - path: internal/wikisource/wiki_contract.go
    - path: internal/wikisource/parser.go
    - path: internal/docgraph/docgraph.go
    - path: internal/indexer/graph_staging.go
    - path: internal/model/graph.go
    - path: internal/model/graph_impact.go
    - path: internal/model/wiki_graph.go
    - path: internal/model/types.go
    - path: internal/service/wiki_graph.go
    - path: internal/service/trace.go
    - path: internal/service/app.go
    - path: internal/cli/nav.go
    - path: internal/cli/provider_manifest.go
    - path: integrations/mi-mcp/provider-manifest.json
---

## Propósito
Cruzar la cadena documental y el código real mediante una sola generación persistida.

## Comportamiento ordenado
El parser extrae links source-only; el indexador resuelve documentos, archivos y símbolos. El kernel conserva aristas orientadas; la dirección inversa se deriva al recorrer, no se exige ni escribe en el documento destino. Un code con símbolo puede resolver archivo y símbolo: el conteo de aristas no es necesariamente el de declaraciones.

## Superficies
nav neighbors/path/explain, nav wiki graph, nav trace y nav wiki trace para documentos v1. El trace anterior sigue disponible para documentos no migrados.

## Entradas/salidas tipadas
wiki-graph/v1: query.mode neighbors/path/explain/affected/change-pack/matrix, selectores, relations, direction, depth, limit, cursor y render=mermaid opcional. Salida: generación, frescura, nodes, edges, provenance, omissions, continuation y Mermaid opcional. El cursor queda vinculado a la consulta y generación.

## Errores
Destinos ausentes o ambiguos son candidatos/omisiones, nunca aristas positivas inventadas. Los presupuestos, selectores y cursores inválidos conservan los errores del kernel. Trace v1 usa vecinos bidireccionales de profundidad dos, acotados por el kernel.

## Criterios de aceptación
1. fulfills, implements, verifies, affects, supports, materializes, supersedes, experiences, stages, maps, grounds_in y code son relaciones tipadas; las consultas inversas y el código pertenecen a la misma generación.
2. Una consulta de vecinos RF alcanza FL, TP, ADR, TECH y código; un camino RS→símbolo cruza la cadena, explain explica una arista y trace v1 consulta el mismo grafo.
3. affected y change-pack ante un diff controlado incluyen RF, TP y TECH; una publicación incremental refleja el cambio sin indexar por consulta.
4. Mermaid se deriva de la página sin escribir wiki; JSON y cursor permiten la siguiente página, y provider-manifest expone la operación para codemode sin lógica nueva en mi-mcp.

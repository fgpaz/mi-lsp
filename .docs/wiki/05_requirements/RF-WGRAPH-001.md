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

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WGRAPH-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-WGRAPH-001.md
exports:
- RF-WGRAPH-001
- '"Consultar el grafo único wiki+código"'
agent_must_read:
- .docs/wiki/05_requirements/RF-WGRAPH-001.md
- .docs/wiki/04_flows/FL-WGRAPH-001.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WGRAPH-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/wikisource/wiki_contract.go
- internal/wikisource/parser.go
- internal/docgraph/docgraph.go
- internal/indexer/graph_staging.go
- internal/model/graph.go
- internal/model/graph_impact.go
- internal/model/wiki_graph.go
- internal/model/types.go
- internal/service/wiki_graph.go
- internal/service/trace.go
- internal/service/app.go
- internal/cli/nav.go
- internal/cli/provider_manifest.go
- integrations/mi-mcp/provider-manifest.json
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-GPH.md, ejecutar el
  comando publicado: go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  ./internal/daemon/... (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-WGRAPH-001.
evidence:
- .docs/wiki/05_requirements/RF-WGRAPH-001.md
- .docs/wiki/06_tests/TP-GPH.md
```

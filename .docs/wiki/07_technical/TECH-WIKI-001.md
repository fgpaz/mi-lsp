---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-WIKI-001
kind: technical
status: active
title: "Ensamblaje generacional y proyecciones wiki v1.1"
links:
  supports: [RF-WGRAPH-001, RF-WGRAPH-002]
  code:
    - path: internal/indexer/graph_staging.go
    - path: internal/service/wiki_graph.go
    - path: internal/service/trace.go
    - path: internal/model/wiki_contract.go
    - path: internal/service/wiki_contract_validate.go
    - path: internal/docgraph/wiki_views.go
    - path: internal/service/wiki_views.go
---

## Resumen
Las relaciones v1 entran en el kernel existente durante indexación. No existían todas como relaciones tipadas antes de v0.14.0: había navegación documental y puente wiki/código parcial. La generación publica docs y observaciones de código; doc_generation_id y code_generation_id identifican sus fuentes, no prometen cadenas de ID iguales entre subsistemas. La coherencia se determina por la generación activa del grafo.

## Mapa de componentes
wikisource extrae links; docgraph resuelve destinos; graph_staging ensambla nodos/aristas; wiki_graph proyecta JSON y Mermaid; trace v1 es alias, mientras trace legacy conserva su resultado previo. model/wiki_contract define títulos y parsers es/en; wiki_views calcula RF×TP con ese idioma. La CLI y provider-manifest son las únicas superficies de integración: no se añade dominio a mi-mcp.

Cuando un backend solo observa símbolos, graph_staging deriva el nodo de archivo únicamente para targets code con owner_path observado, conservando identidad de módulo/backend. Un path declarado sin observaciones sigue sin resolver. Un link con símbolo resuelto genera aristas al archivo y al símbolo; los links path-only alcanzan el archivo.

Las aristas se almacenan una sola vez por declaración resuelta. El recorrido direction=in deriva el inverso sin copiar links. La paginación reutiliza el cursor del kernel. Mermaid no se persiste. Roadmap se consume como metadato no canónico bajo CT-ROADMAP-001, sin duplicar su implementación ni su verificación histórica.

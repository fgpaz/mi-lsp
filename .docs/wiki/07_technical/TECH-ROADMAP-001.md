---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-ROADMAP-001
kind: technical
status: active
title: "Proyección local no canónica del roadmap"
links:
  supports: [RF-ROADMAP-001]
  code:
    - path: internal/model/wiki_roadmap.go
      role: behavior
    - path: internal/service/wiki_roadmap.go
      role: behavior
    - path: internal/service/wiki_graph.go
      role: behavior
---

## Resumen
El servicio lee `.docs/roadmap/roadmap.md` fuera de la wiki y deriva nodos, enlaces `roadmap_for` hacia RS existentes y dependencias. El documento no es canónico (`canonical=false`) y no participa en readiness ni gates. Un RS ausente se avisa como `roadmap_rs_missing` solo si la generación del grafo está current.

## Mapa de componentes
`internal/model/wiki_roadmap.go` interpreta la tabla. `internal/service/wiki_roadmap.go` la carga desde el workspace. `internal/service/wiki_graph.go` proyecta entradas, avisos y el Mermaid efímero por horizonte. El enlace `supports` hacia RF-ROADMAP-001 está declarado en este header; la prosa de TECH-GRAPH-NATIVE no lo sustituye.

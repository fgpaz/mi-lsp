---
wiki_contract: WIKI-CONTRACT-v1
id: RF-ROADMAP-001
kind: requirement
status: active
title: "Proyectar el respaldo local de roadmap por horizonte"
links:
  implements: [FL-ROADMAP-01]
  code:
    - path: internal/cli/nav.go
      role: behavior
    - path: internal/cli/provider_manifest.go
      role: behavior
    - path: integrations/mi-mcp/provider-manifest.json
      role: behavior
    - path: internal/model/types.go
      role: behavior
    - path: internal/model/wiki_graph.go
      role: behavior
    - path: internal/model/wiki_roadmap.go
      role: behavior
    - path: internal/service/wiki_graph.go
      role: behavior
    - path: internal/service/wiki_roadmap.go
      role: behavior
---

## Propósito
Exponer el comportamiento de roadmap publicado en v0.14.1. Este documento se añade retrospectivamente; no acredita una fase ni aceptación histórica.

## Comportamiento ordenado
CLI y proveedor admiten mode=roadmap. El servicio lee la tabla opcional, interpreta entradas y avisos, resuelve referencias con la generación disponible y deriva la respuesta; render=mermaid añade una vista efímera.

## Superficies
CLI nav wiki graph y operación nav.wiki.graph del proveedor mi-mcp; modelos de envelope y servicio local. No hay UI web.

## Entradas/salidas tipadas
Entrada: workspace, mode=roadmap y render opcional mermaid. Tabla: id, rs, horizon, status; title y depends_on opcionales. Horizontes short/medium/long; estados todo/doing/done/blocked. Salida: roadmap.canonical=false, entries, nodes, edges con provenance=derived, roadmap_warnings con code/message/blocking=false y mermaid opcional.

## Errores
Archivo ausente: proyección vacía. Lectura inválida: roadmap_unreadable no bloqueante. Tabla, campos, IDs, duplicados, dependencias y ciclos generan códigos roadmap_* no bloqueantes. Un RS ausente se avisa como roadmap_rs_missing solo con grafo current; falta de generación produce roadmap_graph_unavailable. Un workspace/base inaccesible o render inválido puede fallar la operación: no se promete que todo error operativo sea fail-open.

## Criterios de aceptación
1. CLI y proveedor admiten roadmap y la respuesta identifica las entradas como no canónicas.
2. Lee columnas inglesas, conserva title libre y proyecta estados, horizontes y depends_on.
3. Render Mermaid agrupa short, medium y long, colorea los cuatro estados y deriva flechas de dependencia; sin Gantt ni archivo persistido.
4. Campos inválidos, dependencia desconocida y ciclo emiten avisos con blocking=false sin impedir las entradas válidas.
5. Resuelve enlaces roadmap_for solo hacia RS existentes; con generación current avisa RS ausentes sin crear ni activar documentos.
6. Ausencia del respaldo devuelve proyección vacía y no cambia readiness ni genera un gate.

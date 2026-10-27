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

El inventario `00_retirements.md` acepta YAML con `entries` o el formato previo `retirements`, ambos listas de mappings. Cada fila exige `id`, `old_path`, `archived_path` y `reason` no vacíos, `terminal` booleano y, salvo retiro terminal, `superseded_by` no vacío que exista. El path viejo debe estar ausente y fuera de `_retired`; el archivado debe existir bajo `_retired/<kind>/` sin escapar del workspace. Se acepta explícitamente una lista vacía `entries: []` o `retirements: []`, o la frase completa «No hay retiros registrados.» (también seguida de «Este registro no modifica ni retira identificadores existentes.»). La frase solo declara inventario vacío como prosa fuera de fences y bloques YAML, no como contenido de una razón. Una declaración vacía contradictoria con filas, YAML inválido, documento vacío/ausente, prosa ambigua o fila inválida conserva ADVISORY. Cada bloque YAML debe contener un único documento de inventario reconocido; un bloque válido no oculta otros bloques desconocidos, documentos YAML adicionales ni estructura de inventario que quedó sin analizar. No se añade soporte de tablas.

La autoridad de governance se resuelve con las guardas de raíz y symlinks antes de leerla: Setup pendiente, `source_doc` inválido o ruta insegura no habilitan fallback a otra autoridad. La ausencia legítima del default conserva español no declarado con advertencia; el nombre legacy válido sigue siendo compatible.

Cuando un backend solo observa símbolos, graph_staging deriva el nodo de archivo únicamente para targets code con owner_path observado, conservando identidad de módulo/backend. Un path declarado sin observaciones sigue sin resolver. Un link con símbolo resuelto genera aristas al archivo y al símbolo; los links path-only alcanzan el archivo.

Las aristas se almacenan una sola vez por declaración resuelta. El recorrido direction=in deriva el inverso sin copiar links. La paginación reutiliza el cursor del kernel. Mermaid no se persiste. Roadmap se consume como metadato no canónico bajo CT-ROADMAP-001, sin duplicar su implementación ni su verificación histórica.

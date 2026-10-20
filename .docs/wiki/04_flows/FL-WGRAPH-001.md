---
wiki_contract: WIKI-CONTRACT-v1
id: FL-WGRAPH-001
kind: flow
status: active
title: "Indexar links tipados y consultar la generación wiki+código"
links:
  fulfills: [RS-WIKI-001]
---

## Objetivo
Ofrecer navegación, impacto y comprobaciones de contrato sin reconstrucción manual de links.

## Alcance
Indexación nativa, consultas generacionales y proyecciones de lectura.

## Actores
Autor de la wiki, indexador, consumidor CLI/proveedor.

## Pre/postcondiciones
La wiki declara WIKI-CONTRACT-v1. La indexación nativa publica hechos documentales y observaciones de código. Las consultas conservan generación, frescura y omisiones.

## Secuencia
```mermaid
sequenceDiagram
  participant A as Autor
  participant I as Indexador
  participant G as Grafo
  participant C as Consumidor
  A->>I: Wiki con links y código real
  I->>G: Publicar generación
  C->>G: Vecinos, camino o impacto
  G-->>C: Nodos, aristas, cursor y frescura
  C->>G: Solicitar Mermaid derivado
  G-->>C: Proyección efímera de la página
```
Las consultas no vuelven a indexar cada vez. Un cambio de fuente necesita publicación nativa nueva; no se persiste un overlay de consultas.

## Restricciones ADR
Se reutiliza el kernel de grafo existente; se mantienen los checks previos de contrato. El texto prevalece sobre los diagramas.

---
wiki_contract: WIKI-CONTRACT-v1
id: CT-ROADMAP-001
kind: contract
status: active
title: "Contrato de nav wiki graph en modo roadmap"
links:
  supports: [RF-ROADMAP-001]
  code:
    - path: internal/cli/nav.go
      role: behavior
    - path: internal/cli/provider_manifest.go
      role: behavior
    - path: integrations/mi-mcp/provider-manifest.json
      role: behavior
    - path: internal/model/types.go
      role: behavior
---

## Consumidor
Personas y agentes que invocan `mi-lsp nav wiki graph --mode roadmap` o la operación equivalente del proveedor mi-mcp. No hay UI web.

## Operaciones
`mode=roadmap` admite `render=mermaid`. La respuesta incluye entradas no canónicas, nodos, aristas derivadas y avisos. El Mermaid se calcula para esa respuesta y no se escribe a disco.

## Errores
Archivo ausente: proyección vacía. Lectura o campos inválidos: códigos `roadmap_*` con `blocking=false`. `roadmap_rs_missing` solo con grafo current. `roadmap_graph_unavailable` si no hay generación. Un workspace inaccesible o un render distinto de mermaid puede fallar la operación.

## Compatibilidad
Publicado en v0.14.1 y presente en el binario v0.14.4. CT-NAV-WIKI describe la superficie unificada y remite a este contrato complementario. Las listas Markdown legacy son compatibilidad y no sustituyen `links.supports`; el enlace a RF-ROADMAP-001 se declara en el header de cada contrato.

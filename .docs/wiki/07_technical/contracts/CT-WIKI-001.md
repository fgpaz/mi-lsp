---
wiki_contract: WIKI-CONTRACT-v1
id: CT-WIKI-001
kind: contract
status: active
title: "Contrato componible wiki-graph/v1 e idioma"
links:
  supports: [RF-WGRAPH-001, RF-WGRAPH-002]
  code:
    - path: internal/model/wiki_graph.go
    - path: internal/model/wiki_contract.go
    - path: internal/cli/nav.go
    - path: internal/cli/provider_manifest.go
    - path: integrations/mi-mcp/provider-manifest.json
---

## Consumidor
CLI y gateway codemode que descubre las operaciones wiki_graph y wiki_validate_contract del manifiesto del proveedor mi-lsp.

## Operaciones
nav wiki graph con mode=neighbors/path/explain/affected/change-pack/matrix; cursor opaco en continuation.next.query (op=nav.neighbors para vecinos), generación, nodos/aristas y render=mermaid opcional. La salida Mermaid es una proyección de esa página, no toda la wiki. nav trace y nav wiki trace con un ID v1 indexado son alias del vecindario bidireccional de profundidad dos. Las opciones legacy --all, --summary y binding_sources mantienen su superficie anterior.

validate-contract y matrix exponen language y checks con claves estables; matrix añade RF×TP por criterio usando el parser del idioma. El roadmap opcional continúa siendo no canónico y su contrato está en CT-ROADMAP-001.

## Errores
Se conservan errores de kernel, omisiones de destinos no resueltos y advertencias no bloqueantes. Ausencia de language usa es con warning; valor inválido no selecciona idioma ni cobertura por traducción implícita.

## Compatibilidad
No cambia q-v1 ni se crea una herramienta de dominio en mi-mcp. La operación graph ya publicada se mantiene; este cierre corrige trace y matrix y documenta la cadena source-typed. Los links en prosa no son bindings.

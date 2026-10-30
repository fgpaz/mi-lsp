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

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-WIKI-001
kind: wiki-graph-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-WIKI-001.md, .docs/wiki/07_technical/contracts/CT-ROADMAP-001.md]
exports: [CT-WIKI-001]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-WIKI-001.md, internal/model/wiki_graph.go, internal/model/wiki_contract.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-WIKI-001.md]
agent_must_not_edit: [internal/model/, internal/cli/provider_manifest.go, integrations/mi-mcp/provider-manifest.json]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-WIKI-001.md y .docs/wiki/07_technical/TECH-WIKI-001.md; cotejar las afirmaciones con la fuente publicada internal/model/wiki_graph.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Un cambio trate prosa como binding, infiera traducción/idioma o expanda Mermaid a grafo completo sin evidencia.]
evidence: [.docs/wiki/07_technical/contracts/CT-WIKI-001.md, .docs/wiki/07_technical/TECH-WIKI-001.md, internal/model/wiki_graph.go]
```

## Consumidor
CLI y gateway codemode que descubre las operaciones wiki_graph y wiki_validate_contract del manifiesto del proveedor mi-lsp.

## Operaciones
nav wiki graph con mode=neighbors/path/explain/affected/change-pack/matrix; cursor opaco en continuation.next.query (op=nav.neighbors para vecinos), generación, nodos/aristas y render=mermaid opcional. La salida Mermaid es una proyección de esa página, no toda la wiki. nav trace y nav wiki trace con un ID v1 indexado son alias del vecindario bidireccional de profundidad dos. Las opciones legacy --all, --summary y binding_sources mantienen su superficie anterior.

validate-contract y matrix exponen language y checks con claves estables; matrix añade RF×TP por criterio usando el parser del idioma. El roadmap opcional continúa siendo no canónico y su contrato está en CT-ROADMAP-001.

## Errores
Se conservan errores de kernel, omisiones de destinos no resueltos y advertencias no bloqueantes. Ausencia de language usa es con warning; valor inválido no selecciona idioma ni cobertura por traducción implícita.

## Compatibilidad
No cambia q-v1 ni se crea una herramienta de dominio en mi-mcp. La operación graph ya publicada se mantiene; este cierre corrige trace y matrix y documenta la cadena source-typed. Los links en prosa no son bindings.

---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-014
kind: requirement
status: active
title: RF-QRY-014 - Resolver el documento canonico minimo para una tarea spec-driven
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/axi_mode.go
    role: behavior
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/docgraph/route.go
    role: behavior
  - path: internal/service/route.go
    role: behavior
---

## Propósito

RF-QRY-014 - Resolver el documento canónico mínimo para una tarea spec-driven

### Descripción

Exponer el comando publico `nav route <task>` que resuelve el documento canónico de anclaje y un mini reading pack previo usando el menor número de tokens posible.

### Actor principal

Usuario / Skill / Agente

### FL origen

FL-QRY-01

## Comportamiento ordenado

### Estado

implemented

### TP asociado

TP-QRY

### Comportamiento esperado

1. Ejecutar `mi-lsp nav route <task> --workspace <alias> --format toon`
2. Aplicar el gate de gobernanza antes de cualquier selección documental
3. Resolver la `canonical lane` con `anchor_doc + mini_pack_preview`
4. Adjuntar una `discovery summary` no autoritativa (docs-only por default)
5. Devolver el envelope con `backend=route`

### Contrato de lanes

- **canonical lane**: autoritativa, derivada de governance/read-model y raiz canónica del workspace. Nunca puede ser sobreescrita por discovery. Peso al menos 2x el de discovery.
- **discovery lane**: no autoritativa, advisory-only. Solo docs por default; código solo bajo `--full` o `--include-code-discovery`.
- El scorer Tier 2 es owner-aware: combina FTS, overlap lexico, `doc_id`, stem/path, `owner_hints` opcionales desde `00` y penalizacion a `generic/README` cuando ya existe un candidato canónico positivo.
- La recencia documental solo puede actuar como `weak tie-break` entre candidatos ya positivos; nunca rescata un doc irrelevante.

### Tier 1 - resolución canónica sin índice

Cuando el índice de docs esta vacio o incompleto, Tier 1 puede producir `anchor_doc + mini_pack_preview` derivando el skeleton de routing desde `governance/read-model` y los docs raiz que existan en el filesystem.

### Tier 2 - enriquecimiento con índice

Si el índice esta disponible, Tier 2 enriquece la canonical lane con FTS + scorer owner-aware compartido y construye una discovery summary basada en los docs indexados.

### Flags

- `--rf`, `--fl`, `--doc`: anchors opcionales para hardening de selección
- `--include-code-discovery`: incluye discovery de código (default: false, docs-only)
- `--full`: expande discovery y slices canónicos

### Notas AXI

`nav route` es una superficie preview-first por default (AXI-default). `--full` expande la canonical lane y agrega discovery completa. No repetir `--axi` en next_steps a menos que el comando sea classic-default.

### Notas de implementacion

- Comando CLI: `internal/cli/nav.go` (`routeCommand`)
- Service core: `internal/service/route.go` (`resolveCanonicalRoute`, handler `route()`)
- Docgraph Tier 1: `internal/docgraph/route.go` (`Tier1CanonicalRoute`)
- AXI surface: `internal/cli/axi_mode.go` — `nav.route` en `supportsAXISurface` y `defaultAXIForOperation`; `buildWorkspaceAXINextSteps` incluye `nav route` antes de `nav ask`
- `shouldUseDaemon` excluye `nav.route` (liviano, no requiere daemon)
- Tier 1 omite patrones glob; solo verifica paths concretos en el filesystem

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### Datos de entrada/salida

Entrada: `task` (string), workspace, flags opcionales
Salida: `RouteResult` con `canonical` y `discovery` opcionalmente

### Modelo de datos
`RouteResult`, `RouteCanonicalLane`, `RouteDiscoveryLane`, `RouteDoc`, `DocsReadProfile`, `GovernanceStatus`, `QueryEnvelope`

## Errores

### Semántica fail-closed

Si routing canónico no puede ser confiado, el comando devuelve discovery como advisory-only y marca la canonical lane como `authoritative=false`. No cae silenciosamente a README.md cuando governance y wiki existen.

### Códigos de error

- `QRY_ROUTE_TASK_REQUIRED`
- `QRY_ROUTE_WORKSPACE_NOT_FOUND`
- `QRY_ROUTE_GOVERNANCE_BLOCKED`

## Criterios de aceptación (numerados)

1. Ejecutar `mi-lsp nav route <task> --workspace <alias> --format toon`
2. Aplicar el gate de gobernanza antes de cualquier selección documental
3. Resolver la `canonical lane` con `anchor_doc + mini_pack_preview`
4. Adjuntar una `discovery summary` no autoritativa (docs-only por default)
5. Devolver el envelope con `backend=route`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-014
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-014
- RF-QRY-014 - Resolver el documento canonico minimo para una tarea spec-driven
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-014.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-014.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/axi_mode.go
- internal/cli/nav.go
- internal/docgraph/route.go
- internal/service/route.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-014.
evidence:
- .docs/wiki/05_requirements/RF-QRY-014.md
- .docs/wiki/06_tests/TP-QRY.md
```

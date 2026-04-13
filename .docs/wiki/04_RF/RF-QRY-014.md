# RF-QRY-014 - Resolver el documento canonico minimo para una tarea spec-driven

## Descripcion

Exponer el comando publico `nav route <task>` que resuelve el documento canonico de anclaje y un mini reading pack previo usando el menor numero de tokens posible.

## Actor principal

Usuario / Skill / Agente

## FL origen

FL-QRY-01

## Estado

implemented

## TP asociado

TP-QRY

## Comportamiento esperado

1. Ejecutar `mi-lsp nav route <task> --workspace <alias> --format toon`
2. Aplicar el gate de gobernanza antes de cualquier seleccion documental
3. Resolver la `canonical lane` con `anchor_doc + mini_pack_preview`
4. Adjuntar una `discovery summary` no autoritativa (docs-only por default)
5. Devolver el envelope con `backend=route`

## Contrato de lanes

- **canonical lane**: autoritativa, derivada de governance/read-model y raiz canonica del workspace. Nunca puede ser sobreescrita por discovery. Peso al menos 2x el de discovery.
- **discovery lane**: no autoritativa, advisory-only. Solo docs por default; codigo solo bajo `--full` o `--include-code-discovery`.

## Tier 1 - resolucion canonica sin indice

Cuando el indice de docs esta vacio o incompleto, Tier 1 puede producir `anchor_doc + mini_pack_preview` derivando el skeleton de routing desde `governance/read-model` y los docs raiz que existan en el filesystem.

## Tier 2 - enriquecimiento con indice

Si el indice esta disponible, Tier 2 enriquece la canonical lane con FTS+ranking y construye una discovery summary basada en los docs indexados.

## Semantica fail-closed

Si routing canonico no puede ser confiado, el comando devuelve discovery como advisory-only y marca la canonical lane como `authoritative=false`. No cae silenciosamente a README.md cuando governance y wiki existen.

## Flags

- `--rf`, `--fl`, `--doc`: anchors opcionales para hardening de seleccion
- `--include-code-discovery`: incluye discovery de codigo (default: false, docs-only)
- `--full`: expande discovery y slices canonicos

## Datos de entrada/salida

Entrada: `task` (string), workspace, flags opcionales
Salida: `RouteResult` con `canonical` y `discovery` opcionalmente

## Data model

`RouteResult`, `RouteCanonicalLane`, `RouteDiscoveryLane`, `RouteDoc`, `DocsReadProfile`, `GovernanceStatus`, `QueryEnvelope`

## Codigos de error

- `QRY_ROUTE_TASK_REQUIRED`
- `QRY_ROUTE_WORKSPACE_NOT_FOUND`
- `QRY_ROUTE_GOVERNANCE_BLOCKED`

## Notas AXI

`nav route` es una superficie preview-first por default (AXI-default). `--full` expande la canonical lane y agrega discovery completa. No repetir `--axi` en next_steps a menos que el comando sea classic-default.

## Notas de implementacion

- Comando CLI: `internal/cli/nav.go` (`routeCommand`)
- Service core: `internal/service/route.go` (`resolveCanonicalRoute`, handler `route()`)
- Docgraph Tier 1: `internal/docgraph/route.go` (`Tier1CanonicalRoute`)
- AXI surface: `internal/cli/axi_mode.go` — `nav.route` en `supportsAXISurface` y `defaultAXIForOperation`; `buildWorkspaceAXINextSteps` incluye `nav route` antes de `nav ask`
- `shouldUseDaemon` excluye `nav.route` (liviano, no requiere daemon)
- Tier 1 omite patrones glob; solo verifica paths concretos en el filesystem

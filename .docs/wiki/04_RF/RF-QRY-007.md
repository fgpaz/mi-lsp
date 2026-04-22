---
id: RF-QRY-007
title: Generar mapa de workspace con servicios, endpoints, eventos y dependencias
implements:
  - internal/service/workspace_map.go
  - internal/cli/root.go
tests:
  - internal/service/workspace_map_test.go
  - internal/cli/root_test.go
---

# RF-QRY-007 - Generar mapa de workspace con servicios, endpoints, eventos y dependencias

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-007 |
| Titulo | Generar mapa de workspace con servicios, endpoints, eventos y dependencias |
| Actores | Skill, Agente, CLI |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace indexado | funcional | obligatorio |
| Catalogo de servicios disponible | funcional | preferido |
| Manifest/proyecto resoluble | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `workspace` | string | si | CLI | nombre o path del workspace | RF-QRY-007 |
| `--include-internals` | booleano | no | CLI | si true, incluye simbolos privados | RF-QRY-007 |

## 4. Process Steps (Happy Path)

1. La CLI recibe workspace name o path.
2. El core resuelve el workspace en el catalogo.
3. Escanea proyectos, servicios y endpoints.
4. Identifica eventos/publishers/consumers.
5. Detecta dependencias entre servicios.
6. Devuelve WorkspaceMap structurado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `map` | objeto | usuario/skill | WorkspaceMapEntry con servicios, endpoints, eventos, dependencias |
| `stats` | objeto | usuario/skill | conteos, profundidad detectada |
| `warnings` | lista | usuario/skill | catalogo incompleto o parse failures |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_WORKSPACE_UNRESOLVED` | workspace no encontrado | nombre invalido | error explicito |
| `QRY_CATALOG_UNAVAILABLE` | catalogo no disponible | corrupted o missing | error con sugerencia de re-index |
| `QRY_PARSE_ERROR` | fallo al parsear manifest | manifest corrupted | warning, continuar con parseo parcial |

## 7. Special Cases and Variants

- Si catalogo no esta disponible, devolver error con instruccion de indexar.
- `--include-internals` expande la lista de simbolos pero no cambia la estructura.
- Dependencias se detectan por imports/require statements en el dialecto.

## 8. Data Model Impact

- `WorkspaceMapEntry` (services, endpoints, events, dependencies)
- `ServiceRecord`, `EndpointRecord`, `EventRecord`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Generar mapa de workspace con servicios y endpoints
  Given un workspace indexado y catalogo disponible
  When ejecuto "mi-lsp nav workspace-map --workspace myapp"
  Then la respuesta incluye servicios, endpoints HTTP/gRPC, eventos
  And estadisticas de dependencias y profundidad

Scenario: Degrade si catalogo incompleto
  Given catalogo parcial o parse failures en manifest
  When ejecuto la consulta
  Then devuelvo mapa parcial con warning
  And indico cuales servicios no pudieron parsearse

Scenario: Rechazar workspace invalido
  Given nombre de workspace no registrado
  When ejecuto la consulta
  Then fallo con "QRY_WORKSPACE_UNRESOLVED"
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-023`
- Positivo: `TP-QRY / TC-QRY-024`
- Negativo: `TP-QRY / TC-QRY-025`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir estructura de servicios homogenea
  - no exigir que catalogo este perfecto
- Decisiones cerradas:
  - mapa estructurado por workspace
  - graceful degrade con warnings
- TODO explicit = 0
- Fuera de alcance:
  - rastreo dinamico de flujo en runtime
  - analisis de seguridad
- Dependencias externas explicitas:
  - catalogo sintactico local

---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-007
kind: requirement
status: active
title: RF-QRY-007 - Generar mapa de workspace con servicios, endpoints, eventos y dependencias
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/root.go
    role: behavior
  - path: internal/service/workspace_map.go
    role: behavior
---

## Propósito

RF-QRY-007 - Generar mapa de workspace con servicios, endpoints, eventos y dependencias

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-007
- **Campo**: Título; **Valor**: Generar mapa de workspace con servicios, endpoints, eventos y dependencias
- **Campo**: Actores; **Valor**: Skill, Agente, CLI
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace indexado; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Catálogo de servicios disponible; **Tipo**: funcional; **Estado requerido**: preferido
- **Condición**: Manifest/proyecto resoluble; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe workspace name o path.
2. El core resuelve el workspace en el catalogo.
3. Escanea proyectos, servicios y endpoints.
4. Identifica eventos/publishers/consumers.
5. Detecta dependencias entre servicios.
6. Devuelve WorkspaceMap structurado.

### 7. Casos especiales y variantes
- Si catálogo no esta disponible, devolver error con instruccion de indexar.
- `--include-internals` expande la lista de símbolos pero no cambia la estructura.
- Dependencias se detectan por imports/require statements en el dialecto.
- En workspaces Go, el mapa incluye paquetes `cmd/*`, `internal/*` y `pkg/*` como servicios `go-package` con `symbol_count` y `entity_count` derivados del catalogo.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir estructura de servicios homogenea
  - no exigir que catálogo este perfecto
- Decisiones cerradas:
  - mapa estructurado por workspace
  - graceful degrade con warnings
- TODO explicit = 0
- Fuera de alcance:
  - rastreo dinamico de flujo en runtime
  - analisis de seguridad
- Dependencias externas explícitas:
  - catálogo sintactico local

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `workspace`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: nombre o path del workspace; **RN**: RF-QRY-007
- **Campo**: `--include-internals`; **Tipo**: booleano; **Req.**: no; **Origen**: CLI; **Validacion**: si true, incluye símbolos privados; **RN**: RF-QRY-007

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `map`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: WorkspaceMapEntry con servicios, endpoints, eventos, dependencias
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: conteos, profundidad detectada
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: catálogo incompleto o parse failures

### 8. Impacto en el modelo de datos
- `WorkspaceMapEntry` (services, endpoints, events, dependencies)
- `ServiceRecord`, `EndpointRecord`, `EventRecord`

## Errores

### 6. Errores tipados
- **Código**: `QRY_WORKSPACE_UNRESOLVED`; **Causa**: workspace no encontrado; **Trigger**: nombre inválido; **Respuesta esperada**: error explícito
- **Código**: `QRY_CATALOG_UNAVAILABLE`; **Causa**: catálogo no disponible; **Trigger**: corrupted o missing; **Respuesta esperada**: error con sugerencia de re-index
- **Código**: `QRY_PARSE_ERROR`; **Causa**: fallo al parsear manifest; **Trigger**: manifest corrupted; **Respuesta esperada**: warning, continuar con parseo parcial

## Criterios de aceptación (numerados)

1. Generar mapa de workspace con servicios y endpoints: Dado un workspace indexado y catálogo disponible Cuando ejecuto "mi-lsp nav workspace-map --workspace myapp" Entonces la respuesta incluye servicios, endpoints HTTP/gRPC, eventos Y estadisticas de dependencias y profundidad
2. Degrade si catálogo incompleto: Dado catálogo parcial o parse failures en manifest Cuando ejecuto la consulta Entonces devuelvo mapa parcial con warning Y indico cuales servicios no pudieron parsearse
3. Rechazar workspace inválido: Dado nombre de workspace no registrado Cuando ejecuto la consulta Entonces fallo con "QRY_WORKSPACE_UNRESOLVED"

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-023`
- Positivo: `TP-QRY / TC-QRY-024`
- Positivo: `TP-QRY / TC-QRY-047B`
- Negativo: `TP-QRY / TC-QRY-025`

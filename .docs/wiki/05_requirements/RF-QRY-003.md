---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-003
kind: requirement
status: active
title: RF-QRY-003 - Resumir la superficie de implementacion de un servicio sin sobreconcluir completitud
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-003 - Resumir la superficie de implementacion de un servicio sin sobreconcluir completitud

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-003
- **Campo**: Título; **Valor**: Resumir la superficie de implementacion de un servicio sin sobreconcluir completitud
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Path del servicio existente; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Catálogo disponible o busqueda textual operativa; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `nav service <path>`.
2. El core resuelve el path relativo dentro del workspace.
3. El agregador consulta catálogo repo-local y busqueda textual scoped al servicio.
4. El agregador produce evidencia estructurada: símbolos, endpoints HTTP, consumidores, publishers, entidades e infraestructura observada.
5. El resultado se devuelve en el envelope comun, sin score de completitud ni inferencias fuertes.

### 7. Casos especiales y variantes
- Si el catálogo no tiene símbolos bajo el path, la operación degrada a evidencia textual y emite warning accionable.
- Si `include_archetype=false`, placeholders conocidos no se incluyen en `entities` ni `event_consumers`, pero siguen apareciendo en `archetype_matches`.
- Si el catálogo bajo el path es mayoritariamente Go, `nav service` usa perfil `go-package` y no ejecuta scans textuales especificos de .NET para evitar falsos positivos en fixtures o strings.
- El comando no devuelve porcentaje de completitud, ni `low|medium|high`, ni conclusion final de auditoria.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no declarar completitud solo por ausencia de clases CQRS
  - no convertir el summary en score fuerte
- Decisiones cerradas:
  - el output es evidence-first
  - placeholders van a `archetype_matches`
- TODO explicit = 0
- Fuera de alcance:
  - score de completitud
  - generación automática de documentacion de arquitectura

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `path`; **Tipo**: path relativo/absoluto; **Req.**: si; **Origen**: CLI; **Validacion**: debe caer dentro del workspace; **RN**: RF-QRY-003
- **Campo**: `include_archetype`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: default `false`; **RN**: RF-QRY-003
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json` o `text`; **RN**: RF-QRY-001

### 5. Salidas
- **Campo**: `items[0].service`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: nombre derivado del path
- **Campo**: `items[0].profile`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: perfil heuristico no autoritativo (`dotnet-microservice`, `go-package`, `generic`)
- **Campo**: `items[0].http_endpoints`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: endpoints observados por evidencia textual
- **Campo**: `items[0].event_consumers`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: consumidores observados
- **Campo**: `items[0].event_publishers`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: publishers observados
- **Campo**: `items[0].entities`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: entidades observadas por catálogo
- **Campo**: `items[0].archetype_matches`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: placeholders detectados
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: degradaciones o falta de índice

### 8. Impacto en el modelo de datos
- `QueryEnvelope`
- `ServiceSurfaceSummary`

## Errores

### 6. Errores tipados
- **Código**: `QRY_SERVICE_PATH_REQUIRED`; **Causa**: falta path; **Trigger**: `nav service` sin argumento; **Respuesta esperada**: abortar con error explícito
- **Código**: `QRY_SERVICE_PATH_INVALID`; **Causa**: path fuera del workspace; **Trigger**: path no resoluble; **Respuesta esperada**: abortar con error tipado
- **Código**: `QRY_SERVICE_EVIDENCE_UNAVAILABLE`; **Causa**: ni catálogo ni texto aportan evidencia; **Trigger**: path vacio o índice roto; **Respuesta esperada**: responder `ok=true` con warning si la consulta pudo ejecutarse

## Criterios de aceptación (numerados)

1. Responder con evidencia estructurada para un microservicio .NET: Dado un workspace indexado y un path de servicio valido Cuando ejecuto "mi-lsp nav service src/backend/conversation-fabric --workspace salud --format compact" Entonces la respuesta incluye endpoints, entidades, consumers o publishers observados Y no incluye un score fuerte de completitud
2. Ocultar placeholders de arquetipo por default: Dado un servicio con entidad archetype `Usuario` Cuando ejecuto "mi-lsp nav service <path> --workspace salud" Entonces `Usuario` no aparece dentro de `entities` Y aparece dentro de `archetype_matches`
3. Incluir placeholders al pedirlo explicitamente: Dado un servicio con placeholders conocidos Cuando ejecuto "mi-lsp nav service <path> --workspace salud --include-archetype" Entonces los placeholders aparecen dentro de la evidencia retornada
4. Resumir paquete Go sin falsos positivos .NET: Dado un workspace Go indexado y un path `internal/service` Cuando ejecuto "mi-lsp nav service internal/service --workspace mi-lsp" Entonces `profile` es `go-package` Y no aparecen endpoints .NET derivados de strings o fixtures

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Responder con evidencia estructurada para un microservicio .NET
  Given un workspace indexado y un path de servicio valido
  When ejecuto "mi-lsp nav service src/backend/conversation-fabric --workspace salud --format compact"
  Then la respuesta incluye endpoints, entidades, consumers o publishers observados
  And no incluye un score fuerte de completitud

Scenario: Ocultar placeholders de arquetipo por default
  Given un servicio con entidad archetype `Usuario`
  When ejecuto "mi-lsp nav service <path> --workspace salud"
  Then `Usuario` no aparece dentro de `entities`
  And aparece dentro de `archetype_matches`

Scenario: Incluir placeholders al pedirlo explicitamente
  Given un servicio con placeholders conocidos
  When ejecuto "mi-lsp nav service <path> --workspace salud --include-archetype"
  Then los placeholders aparecen dentro de la evidencia retornada

Scenario: Resumir paquete Go sin falsos positivos .NET
  Given un workspace Go indexado y un path `internal/service`
  When ejecuto "mi-lsp nav service internal/service --workspace mi-lsp"
  Then `profile` es `go-package`
  And no aparecen endpoints .NET derivados de strings o fixtures
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-QRY / TC-QRY-009`
- Positivo: `TP-QRY / TC-QRY-010`
- Positivo: `TP-QRY / TC-QRY-009A`
- Negativo: `TP-QRY / TC-QRY-011`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-003
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-003
- RF-QRY-003 - Resumir la superficie de implementacion de un servicio sin sobreconcluir
  completitud
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-003.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-003.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-QRY-003.md
- .docs/wiki/06_tests/TP-QRY.md
```

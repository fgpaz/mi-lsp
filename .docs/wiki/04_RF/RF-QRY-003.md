# RF-QRY-003 - Resumir la superficie de implementacion de un servicio sin sobreconcluir completitud

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-003 |
| Titulo | Resumir la superficie de implementacion de un servicio sin sobreconcluir completitud |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Path del servicio existente | funcional | obligatorio |
| Catalogo disponible o busqueda textual operativa | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `path` | path relativo/absoluto | si | CLI | debe caer dentro del workspace | RF-QRY-003 |
| `include_archetype` | bool | no | CLI | default `false` | RF-QRY-003 |
| `format` | enum | no | CLI | `compact`, `json` o `text` | RF-QRY-001 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `nav service <path>`.
2. El core resuelve el path relativo dentro del workspace.
3. El agregador consulta catalogo repo-local y busqueda textual scoped al servicio.
4. El agregador produce evidencia estructurada: simbolos, endpoints HTTP, consumidores, publishers, entidades e infraestructura observada.
5. El resultado se devuelve en el envelope comun, sin score de completitud ni inferencias fuertes.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `items[0].service` | string | usuario/skill | nombre derivado del path |
| `items[0].profile` | string | usuario/skill | perfil heuristico no autoritativo (`dotnet-microservice`, `generic`) |
| `items[0].http_endpoints` | lista | usuario/skill | endpoints observados por evidencia textual |
| `items[0].event_consumers` | lista | usuario/skill | consumidores observados |
| `items[0].event_publishers` | lista | usuario/skill | publishers observados |
| `items[0].entities` | lista | usuario/skill | entidades observadas por catalogo |
| `items[0].archetype_matches` | lista | usuario/skill | placeholders detectados |
| `warnings` | lista | usuario/skill | degradaciones o falta de indice |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_SERVICE_PATH_REQUIRED` | falta path | `nav service` sin argumento | abortar con error explicito |
| `QRY_SERVICE_PATH_INVALID` | path fuera del workspace | path no resoluble | abortar con error tipado |
| `QRY_SERVICE_EVIDENCE_UNAVAILABLE` | ni catalogo ni texto aportan evidencia | path vacio o indice roto | responder `ok=true` con warning si la consulta pudo ejecutarse |

## 7. Special Cases and Variants

- Si el catalogo no tiene simbolos bajo el path, la operacion degrada a evidencia textual y emite warning accionable.
- Si `include_archetype=false`, placeholders conocidos no se incluyen en `entities` ni `event_consumers`, pero siguen apareciendo en `archetype_matches`.
- El comando no devuelve porcentaje de completitud, ni `low|medium|high`, ni conclusion final de auditoria.

## 8. Data Model Impact

- `QueryEnvelope`
- `ServiceSurfaceSummary`

## 9. Expanded Acceptance Criteria (Gherkin)

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
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-009`
- Positivo: `TP-QRY / TC-QRY-010`
- Negativo: `TP-QRY / TC-QRY-011`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no declarar completitud solo por ausencia de clases CQRS
  - no convertir el summary en score fuerte
- Decisiones cerradas:
  - el output es evidence-first
  - placeholders van a `archetype_matches`
- TODO explicit = 0
- Fuera de alcance:
  - score de completitud
  - generacion automatica de documentacion de arquitectura

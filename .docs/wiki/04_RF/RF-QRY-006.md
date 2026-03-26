# RF-QRY-006 - Devolver vecindario semantico de un simbolo

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-006 |
| Titulo | Devolver vecindario semantico de un simbolo (definicion, callers, implementors, tests) |
| Actores | Skill, Agente, CLI |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace indexado | funcional | obligatorio |
| Simbolo existe en catalogo | funcional | obligatorio |
| Backend semantico disponible | tecnica | preferido; graceful degradation |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `symbol` | string | si | CLI | nombre valido de simbolo | RF-QRY-006 |
| `--depth` | entero | no | CLI | 0-3; default 1 | RF-QRY-006 |
| `--language` | enum | no | CLI | `python`, `typescript`, `csharp`; infer si no se informa | RF-QRY-006 |

## 4. Process Steps (Happy Path)

1. La CLI recibe un nombre de simbolo.
2. El core busca en el catalogo sintactico.
3. Resuelve la definicion del simbolo.
4. Encuentra callers (referencias), implementors (subclases/interfaces), tests (referencias en test files).
5. Aplica `--depth` para limitar la expansion transitive.
6. Devuelve `SymbolNeighborhood` con todos los vecinos.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `neighborhood` | objeto | usuario/skill | SymbolNeighborhood con definition, callers, implementors, tests |
| `depth_applied` | entero | usuario/skill | profundidad efectiva usada |
| `backend` | string | usuario/skill | sintactico o semantico |
| `warnings` | lista | usuario/skill | degradaciones si backend semantico no disponible |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_SYMBOL_NOT_FOUND` | simbolo no en catalogo | nombre inexistente | devolver error con sugerencia de busqueda |
| `QRY_SEMANTIC_UNAVAILABLE` | backend semantico no disponible | backend proceso no responde | degrade a sintactico con warning |
| `QRY_INVALID_DEPTH` | profundidad invalida | depth < 0 o > 3 | rechazar con valor default 1 |

## 7. Special Cases and Variants

- Si el backend semantico falla, continua con sintactico.
- Profundidad 0 = solo definicion.
- Profundidad 1 = definicion + directos callers/implementors.
- Profundidad 2+ = expansion transitive limitada.

## 8. Data Model Impact

- `SymbolNeighborhood` (definition, callers, implementors, tests)
- `SymbolRecord`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Obtener vecindario semantico con profundidad 1
  Given un simbolo valido en el catalogo
  When ejecuto "mi-lsp nav related --symbol MyClass --depth 1"
  Then la respuesta incluye definicion, callers directos, implementors directos, tests
  And no expansion transitive
  And "backend" indica semantico o sintactico

Scenario: Degrade graceful si semantic backend falla
  Given backend semantico no disponible
  When ejecuto la consulta
  Then la respuesta incluye definicion sintactica
  And warning explico degradacion
  And callers/implementors basados en texto

Scenario: Rechazar simbolo no encontrado
  Given nombre de simbolo inexistente
  When ejecuto la consulta
  Then fallo con "QRY_SYMBOL_NOT_FOUND"
  And sugerir busqueda textual alternativa
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-020`
- Positivo: `TP-QRY / TC-QRY-021`
- Negativo: `TP-QRY / TC-QRY-022`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no garantizar transitividad completa
  - no asumir que backend semantico esta disponible
- Decisiones cerradas:
  - profundidad limitada para evitar explosion
  - graceful degrade a sintactico
- TODO explicit = 0
- Fuera de alcance:
  - analisis de flujo de datos
  - rastreo transitive completo
- Dependencias externas explicitas:
  - catalogo sintactico local
  - backend semantico opcional (Roslyn, tsserver, pyright)

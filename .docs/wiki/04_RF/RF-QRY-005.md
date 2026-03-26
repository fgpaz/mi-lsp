# RF-QRY-005 - Ejecutar N operaciones heterogeneas en batch con dispatch paralelo

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-005 |
| Titulo | Ejecutar N operaciones heterogeneas en batch con dispatch paralelo |
| Actores | Skill, Agente, CLI |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Backend semantico disponible | tecnica | obligatorio |
| Operaciones soportadas en RF | funcional | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `operations` | array JSON | si | stdin | array de objetos {op, params} | RF-QRY-005 |
| `parallelism` | entero | no | CLI/config | enteros positivos; default CPU count | RF-QRY-005 |
| `stdin_max_size` | entero | no | config | limit en bytes; default 10MB | RF-QRY-005 |

## 4. Process Steps (Happy Path)

1. La CLI recibe JSON array con N operaciones.
2. Valida que cada operacion sea un tipo soportado.
3. Dispatch paralelo hasta `parallelism` workers.
4. Cada operacion ejecuta independientemente.
5. Colecta resultados y devuelve BatchResult array.
6. Si algun contexto se cancela, interrumpe gracefully.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado general (false si algunas operaciones fallan) |
| `batch_results` | array | usuario/skill | array de resultados para cada operacion |
| `stats` | objeto | usuario/skill | timing, parallelism_actual, operaciones completadas |
| `warnings` | lista | usuario/skill | errores parciales o degradaciones |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_INVALID_OPERATION` | tipo de operacion no soportado | op type desconocido | rechazar con error tipado |
| `QRY_STDIN_TOO_LARGE` | payload excede limite | stdin > 10MB | abortar con error |
| `QRY_CONTEXT_CANCELLED` | contexto cancelado durante ejecucion | timeout/señal | interrumpir y devolver resultados parciales |

## 7. Special Cases and Variants

- Si una operacion falla, devuelve su error pero continua con las demas.
- `ok=true` si al menos una operacion completa exitosamente.
- Timeout por operacion (default 30s) para evitar bloqueos.

## 8. Data Model Impact

- `QueryEnvelope`
- `BatchOperation` (op, params)
- `BatchResult` (operation_index, ok, result, error)

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Ejecutar batch con multiples operaciones
  Given un JSON array con 3 operaciones validas diferentes
  When envio por stdin "mi-lsp nav batch < operations.json"
  Then la respuesta incluye 3 resultados paralelos en "batch_results"
  And "stats" reporta tiempo total < suma de tiempos secuenciales

Scenario: Continuar si una operacion falla
  Given un batch con 1 operacion valida y 1 invalida
  When ejecuto el batch
  Then "ok" es "true" (al menos una completo)
  And devuelvo error tipado para la operacion invalida
  And la operacion valida completa normalmente

Scenario: Rechazar payload demasiado grande
  Given JSON stdin > 10MB
  When ejecuto batch
  Then fallo con "QRY_STDIN_TOO_LARGE"
  And no proceso ninguna operacion
```

## 10. Test Traceability

- Positivo: `TP-QRY / TC-QRY-017`
- Positivo: `TP-QRY / TC-QRY-018`
- Negativo: `TP-QRY / TC-QRY-019`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir orden de ejecucion
  - no garantizar atomicidad transacional
- Decisiones cerradas:
  - dispatch paralelo fino pero sin transacciones globales
  - resultados ordenados por indice original
- TODO explicit = 0
- Fuera de alcance:
  - garantias ACID
  - rollback distribuido
- Dependencias externas explicitas:
  - pool de workers local
